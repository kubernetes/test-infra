/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package tide contains a controller for managing a tide pool of PRs. The
// controller will automatically retest PRs in the pool and merge them if they
// pass tests.
package tide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/prometheus/client_golang/prometheus"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/tide/blockers"
	"k8s.io/test-infra/prow/tide/history"
	_ "k8s.io/test-infra/prow/version"
)

// For mocking out sleep during unit tests.
var sleep = time.Sleep

type githubClient interface {
	CreateStatus(string, string, string, github.Status) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	ListCheckRuns(org, repo, ref string) (*github.CheckRunList, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetRef(string, string, string) (string, error)
	GetRepo(owner, name string) (github.FullRepo, error)
	Merge(string, string, int, github.MergeDetails) error
	QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error
}

type contextChecker interface {
	// IsOptional tells whether a context is optional.
	IsOptional(string) bool
	// MissingRequiredContexts tells if required contexts are missing from the list of contexts provided.
	MissingRequiredContexts([]string) []string
}

// Controller knows how to sync PRs and PJs.
type syncController struct {
	ctx           context.Context
	logger        *logrus.Entry
	config        config.Getter
	prowJobClient ctrlruntimeclient.Client
	provider      provider
	pickNewBatch  func(sp subpool, candidates []CodeReviewCommon, maxBatchSize int) ([]CodeReviewCommon, error)

	m     sync.Mutex
	pools []Pool

	// changedFiles caches the names of files changed by PRs.
	// Cache entries expire if they are not used during a sync loop.
	changedFiles *changedFilesAgent

	History *history.History

	// Shared fields with status controller
	statusUpdate *statusUpdate
}

// Action represents what actions the controller can take. It will take
// exactly one action each sync.
type Action string

// Constants for various actions the controller might take
const (
	Wait         Action = "WAIT"
	Trigger      Action = "TRIGGER"
	TriggerBatch Action = "TRIGGER_BATCH"
	Merge        Action = "MERGE"
	MergeBatch   Action = "MERGE_BATCH"
	PoolBlocked  Action = "BLOCKED"
)

// recordableActions is the subset of actions that we keep historical record of.
// Ignore idle actions to avoid flooding the records with useless data.
var recordableActions = map[Action]bool{
	Trigger:      true,
	TriggerBatch: true,
	Merge:        true,
	MergeBatch:   true,
}

// Pool represents information about a tide pool. There is one for every
// org/repo/branch combination that has PRs in the pool.
type Pool struct {
	Org    string
	Repo   string
	Branch string

	// PRs with passing tests, pending tests, and missing or failed tests.
	// Note that these results are rolled up. If all tests for a PR are passing
	// except for one pending, it will be in PendingPRs.
	SuccessPRs []CodeReviewCommon
	PendingPRs []CodeReviewCommon
	MissingPRs []CodeReviewCommon

	// Empty if there is no pending batch.
	BatchPending []CodeReviewCommon

	// Which action did we last take, and to what target(s), if any.
	Action   Action
	Target   []CodeReviewCommon
	Blockers []blockers.Blocker
	Error    string

	// All of the TenantIDs associated with PRs in the pool.
	TenantIDs []string
}

// PoolForDeck contains the same data as Pool, the only exception is that it has
// a minified version of CodeReviewCommon which is good for deck, as
// MinCodeReview is a very small superset of CodeReviewCommon.
type PoolForDeck struct {
	Org    string
	Repo   string
	Branch string

	// PRs with passing tests, pending tests, and missing or failed tests.
	// Note that these results are rolled up. If all tests for a PR are passing
	// except for one pending, it will be in PendingPRs.
	SuccessPRs []MinCodeReviewCommon
	PendingPRs []MinCodeReviewCommon
	MissingPRs []MinCodeReviewCommon

	// Empty if there is no pending batch.
	BatchPending []MinCodeReviewCommon

	// Which action did we last take, and to what target(s), if any.
	Action   Action
	Target   []MinCodeReviewCommon
	Blockers []blockers.Blocker
	Error    string

	// All of the TenantIDs associated with PRs in the pool.
	TenantIDs []string
}

func PoolToPoolForDeck(p *Pool) *PoolForDeck {
	crcToMin := func(crcs []CodeReviewCommon) []MinCodeReviewCommon {
		var res []MinCodeReviewCommon
		for _, crc := range crcs {
			res = append(res, MinCodeReviewCommon(crc))
		}
		return res
	}
	pfd := &PoolForDeck{
		Org:          p.Org,
		Repo:         p.Repo,
		Branch:       p.Branch,
		SuccessPRs:   crcToMin(p.SuccessPRs),
		PendingPRs:   crcToMin(p.PendingPRs),
		MissingPRs:   crcToMin(p.MissingPRs),
		BatchPending: crcToMin(p.BatchPending),
		Action:       p.Action,
		Target:       crcToMin(p.Target),
		Blockers:     p.Blockers,
		Error:        p.Error,
		TenantIDs:    p.TenantIDs,
	}
	return pfd
}

// Prometheus Metrics
var (
	tideMetrics = struct {
		// Per pool
		pooledPRs    *prometheus.GaugeVec
		updateTime   *prometheus.GaugeVec
		merges       *prometheus.HistogramVec
		poolErrors   *prometheus.CounterVec
		queryResults *prometheus.CounterVec

		// Singleton
		syncDuration         prometheus.Gauge
		statusUpdateDuration prometheus.Gauge

		// Per controller
		syncHeartbeat *prometheus.CounterVec
	}{
		pooledPRs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pooledprs",
			Help: "Number of PRs in each Tide pool.",
		}, []string{
			"org",
			"repo",
			"branch",
		}),
		updateTime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "updatetime",
			Help: "The last time each subpool was synced. (Used to determine 'pooledprs' freshness.)",
		}, []string{
			"org",
			"repo",
			"branch",
		}),

		merges: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "merges",
			Help:    "Histogram of merges where values are the number of PRs merged together.",
			Buckets: []float64{1, 2, 3, 4, 5, 7, 10, 15, 25},
		}, []string{
			"org",
			"repo",
			"branch",
		}),

		poolErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tidepoolerrors",
			Help: "Count of Tide pool sync errors.",
		}, []string{
			"org",
			"repo",
			"branch",
		}),

		queryResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tidequeryresults",
			Help: "Count of Tide queries by query index, org shard, and result (success/error).",
		}, []string{
			"query_index",
			"org_shard",
			"result",
		}),

		// Use the sync heartbeat counter to monitor for liveness. Use the duration
		// gauges for precise sync duration graphs since the prometheus scrape
		// period is likely much larger than the loop periods.
		syncDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "syncdur",
			Help: "The duration of the last loop of the sync controller.",
		}),
		statusUpdateDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "statusupdatedur",
			Help: "The duration of the last loop of the status update controller.",
		}),

		syncHeartbeat: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tidesyncheartbeat",
			Help: "Count of Tide syncs per controller.",
		}, []string{
			"controller",
		}),
	}
)

func init() {
	prometheus.MustRegister(tideMetrics.pooledPRs)
	prometheus.MustRegister(tideMetrics.updateTime)
	prometheus.MustRegister(tideMetrics.merges)
	prometheus.MustRegister(tideMetrics.syncDuration)
	prometheus.MustRegister(tideMetrics.statusUpdateDuration)
	prometheus.MustRegister(tideMetrics.syncHeartbeat)
	prometheus.MustRegister(tideMetrics.poolErrors)
	prometheus.MustRegister(tideMetrics.queryResults)
}

type manager interface {
	GetClient() ctrlruntimeclient.Client
	GetFieldIndexer() ctrlruntimeclient.FieldIndexer
}

type Controller struct {
	syncCtrl   *syncController
	statusCtrl *statusController
}

// Shutdown signals the statusController to stop working and waits for it to
// finish its last update loop before terminating.
// Controller.Sync() should not be used after this function is called.
func (c *Controller) Shutdown() {
	c.syncCtrl.History.Flush()
	c.statusCtrl.shutdown()
}

func (c *Controller) Sync() error {
	return c.syncCtrl.Sync()
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.syncCtrl.ServeHTTP(w, r)
}

func (c *Controller) History() *history.History {
	return c.syncCtrl.History
}

// NewController makes a Controller out of the given clients.
func NewController(
	ghcSync,
	ghcStatus github.Client,
	mgr manager,
	cfg config.Getter,
	gc git.ClientFactory,
	maxRecordsPerPool int,
	opener io.Opener,
	historyURI,
	statusURI string,
	logger *logrus.Entry,
	usesGitHubAppsAuth bool,
) (*Controller, error) {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	hist, err := history.New(maxRecordsPerPool, opener, historyURI)
	if err != nil {
		return nil, fmt.Errorf("error initializing history client from %q: %w", historyURI, err)
	}
	mergeChecker := newMergeChecker(cfg, ghcSync)

	ctx := context.Background()
	// Shared fields

	statusUpdate := &statusUpdate{
		dontUpdateStatus: &threadSafePRSet{},
		newPoolPending:   make(chan bool),
	}

	sc, err := newStatusController(ctx, logger, ghcStatus, mgr, gc, cfg, opener, statusURI, mergeChecker, usesGitHubAppsAuth, statusUpdate)
	if err != nil {
		return nil, err
	}
	go sc.run()

	provider := newGitHubProvider(logger, ghcSync, gc, cfg, mergeChecker, usesGitHubAppsAuth)
	syncCtrl, err := newSyncController(ctx, logger, mgr, provider, cfg, gc, hist, usesGitHubAppsAuth, statusUpdate)
	if err != nil {
		return nil, err
	}
	return &Controller{syncCtrl: syncCtrl, statusCtrl: sc}, nil
}

func newStatusController(
	ctx context.Context,
	logger *logrus.Entry,
	ghc githubClient,
	mgr manager,
	gc git.ClientFactory,
	cfg config.Getter,
	opener io.Opener,
	statusURI string,
	mergeChecker *mergeChecker,
	usesGitHubAppsAuth bool,
	statusUpdate *statusUpdate,
) (*statusController, error) {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &prowapi.ProwJob{}, indexNamePassingJobs, indexFuncPassingJobs); err != nil {
		return nil, fmt.Errorf("failed to add index for passing jobs to cache: %w", err)
	}
	return &statusController{
		pjClient:           mgr.GetClient(),
		logger:             logger.WithField("controller", "status-update"),
		ghProvider:         newGitHubProvider(logger, ghc, gc, cfg, mergeChecker, usesGitHubAppsAuth),
		ghc:                ghc,
		gc:                 gc,
		usesGitHubAppsAuth: usesGitHubAppsAuth,
		config:             cfg,
		shutDown:           make(chan bool),
		opener:             opener,
		path:               statusURI,
		statusUpdate:       statusUpdate,
	}, nil
}

func newSyncController(
	ctx context.Context,
	logger *logrus.Entry,
	mgr manager,
	provider provider,
	cfg config.Getter,
	gc git.ClientFactory,
	hist *history.History,
	usesGitHubAppsAuth bool,
	statusUpdate *statusUpdate,
) (*syncController, error) {
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&prowapi.ProwJob{},
		cacheIndexName,
		cacheIndexFunc,
	); err != nil {
		return nil, fmt.Errorf("failed to add baseSHA index to cache: %w", err)
	}
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&prowapi.ProwJob{},
		nonFailedBatchByNameBaseAndPullsIndexName,
		nonFailedBatchByNameBaseAndPullsIndexFunc,
	); err != nil {
		return nil, fmt.Errorf("failed to add index for non failed batches: %w", err)
	}

	return &syncController{
		ctx:           ctx,
		logger:        logger.WithField("controller", "sync"),
		prowJobClient: mgr.GetClient(),
		config:        cfg,
		provider:      provider,
		pickNewBatch:  pickNewBatch(gc, cfg, provider),
		changedFiles: &changedFilesAgent{
			provider:        provider,
			nextChangeCache: make(map[changeCacheKey][]string),
		},
		History:      hist,
		statusUpdate: statusUpdate,
	}, nil
}

func prKey(pr *CodeReviewCommon) string {
	return fmt.Sprintf("%s#%d", string(pr.NameWithOwner), pr.Number)
}

// newExpectedContext creates a Context with Expected state.
func newExpectedContext(c string) Context {
	return Context{
		Context:     githubql.String(c),
		State:       githubql.StatusStateExpected,
		Description: githubql.String(""),
	}
}

// contextsToStrings converts a list Context to a list of string
func contextsToStrings(contexts []Context) []string {
	var names []string
	for _, c := range contexts {
		names = append(names, string(c.Context))
	}
	// Sorting names improves readability of logs and simplifies unit tests.
	sort.Strings(names)
	return names
}

// Sync runs one sync iteration.
func (c *syncController) Sync() error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.WithField("duration", duration.String()).Info("Synced")
		tideMetrics.syncDuration.Set(duration.Seconds())
		tideMetrics.syncHeartbeat.WithLabelValues("sync").Inc()
	}()
	defer c.changedFiles.prune()
	c.config().BranchProtectionWarnings(c.logger, c.config().PresubmitsStatic)

	c.logger.Debug("Building tide pool.")
	var queryErrors []error
	prs, err := c.provider.Query()
	if err != nil {
		c.logger.WithError(err).Debug("failed to query GitHub for some prs")
		queryErrors = append(queryErrors, err)
	}
	c.logger.WithFields(logrus.Fields{
		"duration":       time.Since(start).String(),
		"found_pr_count": len(prs),
	}).Debug("Found (unfiltered) pool PRs.")

	var blocks blockers.Blockers
	if len(prs) > 0 {
		blocks, err = c.provider.blockers()
		if err != nil {
			return fmt.Errorf("failed getting blockers: %v", err)
		}
	}
	// Partition PRs into subpools and filter out non-pool PRs.
	rawPools, err := c.dividePool(prs)
	if err != nil {
		return err
	}
	filteredPools := c.filterSubpools(c.provider.isAllowedToMerge, rawPools)

	// Notify statusController about the new pool.
	c.statusUpdate.Lock()
	c.statusUpdate.blocks = blocks
	c.statusUpdate.poolPRs = poolPRMap(filteredPools)
	c.statusUpdate.baseSHAs = baseSHAMap(filteredPools)
	c.statusUpdate.requiredContexts = requiredContextsMap(filteredPools)
	select {
	case c.statusUpdate.newPoolPending <- true:
		c.statusUpdate.dontUpdateStatus.reset()
	default:
	}
	c.statusUpdate.Unlock()

	// Sync subpools in parallel.
	poolChan := make(chan Pool, len(filteredPools))
	subpoolsInParallel(
		c.config().Tide.MaxGoroutines,
		filteredPools,
		func(sp *subpool) {
			// blocks.GetApplicable will be noop if blocks is not initialized at
			// all. This applies to both cases where there is no blocking label
			// configured, or other source control systems that don't support
			// blockers yet.
			pool, err := c.syncSubpool(*sp, blocks.GetApplicable(sp.org, sp.repo, sp.branch))
			if err != nil {
				tideMetrics.poolErrors.WithLabelValues(sp.org, sp.repo, sp.branch).Inc()
				sp.log.WithError(err).Errorf("Error syncing subpool.")
			}
			poolChan <- pool
		},
	)

	close(poolChan)
	pools := make([]Pool, 0, len(poolChan))
	for pool := range poolChan {
		pools = append(pools, pool)
	}
	sortPools(pools)
	c.m.Lock()
	c.pools = pools
	c.m.Unlock()

	c.History.Flush()
	return utilerrors.NewAggregate(queryErrors)
}

func (c *syncController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.m.Lock()
	defer c.m.Unlock()
	b, err := json.Marshal(c.pools)
	if err != nil {
		c.logger.WithError(err).Error("Encoding JSON.")
		b = []byte("[]")
	}
	if _, err = w.Write(b); err != nil {
		c.logger.WithError(err).Error("Writing JSON response.")
	}
}

func subpoolsInParallel(goroutines int, sps map[string]*subpool, process func(*subpool)) {
	// Load the subpools into a channel for use as a work queue.
	queue := make(chan *subpool, len(sps))
	for _, sp := range sps {
		queue <- sp
	}
	close(queue)

	if goroutines > len(queue) {
		goroutines = len(queue)
	}
	wg := &sync.WaitGroup{}
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for sp := range queue {
				process(sp)
			}
		}()
	}
	wg.Wait()
}

// filterSubpools filters non-pool PRs out of the initially identified subpools,
// deleting any pools that become empty.
// See filterSubpool for filtering details.
func (c *syncController) filterSubpools(mergeAllowed func(*CodeReviewCommon) (string, error), raw map[string]*subpool) map[string]*subpool {
	filtered := make(map[string]*subpool)
	var lock sync.Mutex

	subpoolsInParallel(
		c.config().Tide.MaxGoroutines,
		raw,
		func(sp *subpool) {
			if err := c.initSubpoolData(sp); err != nil {
				sp.log.WithError(err).Error("Error initializing subpool.")
				return
			}
			key := poolKey(sp.org, sp.repo, sp.branch)
			if spFiltered := filterSubpool(c.provider, mergeAllowed, sp); spFiltered != nil {
				sp.log.WithField("key", key).WithField("pool", spFiltered).Debug("filtered sub-pool")

				lock.Lock()
				filtered[key] = spFiltered
				lock.Unlock()
			} else {
				sp.log.WithField("key", key).WithField("pool", spFiltered).Debug("filtering sub-pool removed all PRs")
			}
		},
	)
	return filtered
}

// initSubpoolData fetches presubmit jobs and context checkers for the subpool.
func (c *syncController) initSubpoolData(sp *subpool) error {
	var err error
	sp.presubmits, err = c.presubmitsByPull(sp)
	if err != nil {
		return fmt.Errorf("error determining required presubmit prowjobs: %w", err)
	}
	// CloneURI is used by Gerrit to retrieve inrepoconfig; this is not used by
	// GitHub at all.
	// It's known that cloneURI is the only reliable way for Gerrit to correctly
	// clone, so it should be safe to assume that cloneURI is identical among jobs.
	var cloneURI string
	for _, presubmits := range sp.presubmits {
		for _, p := range presubmits {
			if p.CloneURI != "" {
				cloneURI = p.CloneURI
				break
			}
		}
	}
	sp.cloneURI = cloneURI

	sp.cc = make(map[int]contextChecker, len(sp.prs))
	for _, pr := range sp.prs {
		sp.cc[pr.Number], err = c.provider.GetTideContextPolicy(sp.org, sp.repo, sp.branch, refGetterFactory(string(sp.sha)), &pr)
		if err != nil {
			return fmt.Errorf("error setting up context checker for pr %d: %w", pr.Number, err)
		}
	}
	return nil
}

// filterSubpool filters PRs from an initially identified subpool, returning the
// filtered subpool.
// If the subpool becomes empty 'nil' is returned to indicate that the subpool
// should be deleted.
//
// This function works for any source code provider.
func filterSubpool(provider provider, mergeAllowed func(*CodeReviewCommon) (string, error), sp *subpool) *subpool {
	var toKeep []CodeReviewCommon
	for _, pr := range sp.prs {
		if !filterPR(provider, mergeAllowed, sp, &pr) {
			toKeep = append(toKeep, pr)
		}
	}
	if len(toKeep) == 0 {
		return nil
	}
	sp.prs = toKeep
	return sp
}

// filterPR indicates if a PR should be filtered out of the subpool.
// Specifically we filter out PRs that:
//   - Have known merge conflicts or invalid merge method.
//   - Have failing or missing status contexts.
//   - Have pending required status contexts that are not associated with a
//     ProwJob. (This ensures that the 'tide' context indicates that the pending
//     status is preventing merge. Required ProwJob statuses are allowed to be
//     'pending' because this prevents kicking PRs from the pool when Tide is
//     retesting them.)
//
// This function works for any source code provider.
func filterPR(provider provider, mergeAllowed func(*CodeReviewCommon) (string, error), sp *subpool, pr *CodeReviewCommon) bool {
	log := sp.log.WithFields(pr.logFields())
	// Skip PRs that are known to be unmergeable.
	if reason, err := mergeAllowed(pr); err != nil {
		log.WithError(err).Error("Error checking PR mergeability.")
		return true
	} else if reason != "" {
		log.WithField("reason", reason).Debug("filtering out PR as it is not mergeable")
		return true
	}

	// Filter out PRs with unsuccessful contexts unless the only unsuccessful
	// contexts are pending required prowjobs.
	contexts, err := provider.headContexts(pr)
	if err != nil {
		log.WithError(err).Error("Getting head contexts.")
		return true
	}
	presubmitsHaveContext := func(context string) bool {
		for _, job := range sp.presubmits[pr.Number] {
			if job.Context == context {
				return true
			}
		}
		return false
	}
	for _, ctx := range unsuccessfulContexts(contexts, sp.cc[pr.Number], log) {
		if ctx.State != githubql.StatusStatePending {
			log.WithField("context", ctx.Context).Debug("filtering out PR as unsuccessful context is not pending")
			return true
		}
		if !presubmitsHaveContext(string(ctx.Context)) {
			log.WithField("context", ctx.Context).Debug("filtering out PR as unsuccessful context is not Prow-controlled")
			return true
		}
	}

	return false
}

func baseSHAMap(subpoolMap map[string]*subpool) map[string]string {
	baseSHAs := make(map[string]string, len(subpoolMap))
	for key, sp := range subpoolMap {
		baseSHAs[key] = sp.sha
	}
	return baseSHAs
}

// poolPRMap collects all subpool PRs into a map containing all pooled PRs.
func poolPRMap(subpoolMap map[string]*subpool) map[string]CodeReviewCommon {
	prs := make(map[string]CodeReviewCommon)
	for _, sp := range subpoolMap {
		for _, pr := range sp.prs {
			prs[prKey(&pr)] = pr
		}
	}
	return prs
}

func requiredContextsMap(subpoolMap map[string]*subpool) map[string][]string {
	requiredContextsMap := map[string][]string{}
	for _, sp := range subpoolMap {
		for _, pr := range sp.prs {
			requiredContextsSet := sets.Set[string]{}
			for _, requiredJob := range sp.presubmits[pr.Number] {
				requiredContextsSet.Insert(requiredJob.Context)
			}
			requiredContextsMap[prKey(&pr)] = sets.List(requiredContextsSet)
		}
	}
	return requiredContextsMap
}

type simpleState string

const (
	failureState simpleState = "failure"
	pendingState simpleState = "pending"
	successState simpleState = "success"
)

func toSimpleState(s prowapi.ProwJobState) simpleState {
	if s == prowapi.TriggeredState || s == prowapi.PendingState {
		return pendingState
	} else if s == prowapi.SuccessState {
		return successState
	}
	return failureState
}

// isPassingTests returns whether or not all contexts set on the PR except for
// the tide pool context are passing.
func (c *syncController) isPassingTests(log *logrus.Entry, pr *CodeReviewCommon, cc contextChecker) bool {
	log = log.WithFields(pr.logFields())

	contexts, err := c.provider.headContexts(pr)
	if err != nil {
		log.WithError(err).Error("Getting head commit status contexts.")
		// If we can't get the status of the commit, assume that it is failing.
		return false
	}
	unsuccessful := unsuccessfulContexts(contexts, cc, log)
	return len(unsuccessful) == 0
}

// unsuccessfulContexts determines which contexts from the list that we care about are
// failed. For instance, we do not care about our own context.
// If the branchProtection is set to only check for required checks, we will skip
// all non-required tests. If required tests are missing from the list, they will be
// added to the list of failed contexts.
func unsuccessfulContexts(contexts []Context, cc contextChecker, log *logrus.Entry) []Context {
	var failed []Context
	for _, ctx := range contexts {
		if string(ctx.Context) == statusContext {
			continue
		}
		if cc.IsOptional(string(ctx.Context)) {
			continue
		}
		if ctx.State != githubql.StatusStateSuccess {
			failed = append(failed, ctx)
		}
	}
	for _, c := range cc.MissingRequiredContexts(contextsToStrings(contexts)) {
		failed = append(failed, newExpectedContext(c))
	}

	log.WithFields(logrus.Fields{
		"total_context_count":  len(contexts),
		"context_names":        contextsToStrings(contexts),
		"failed_context_count": len(failed),
		"failed_context_names": contextsToStrings(failed),
	}).Debug("Filtered out failed contexts")
	return failed
}

// hasAllLabels is used by pickHighestPriorityPR. Returns true when wantLabels
// is empty, otherwise ensures that PR labels contain all wantLabels.
func hasAllLabels(pr CodeReviewCommon, wantLabels []string) bool {
	if len(wantLabels) == 0 {
		return true
	}
	prLabels := sets.New[string]()
	if labels := pr.GitHubLabels(); labels != nil {
		for _, l2 := range labels.Nodes {
			prLabels.Insert(string(l2.Name))
		}
	}
	for _, label := range wantLabels {
		altLabels := strings.Split(label, ",")
		if !prLabels.HasAny(altLabels...) {
			return false
		}
	}
	return true
}

func pickHighestPriorityPR(log *logrus.Entry, prs []CodeReviewCommon, cc map[int]contextChecker, isPassingTestsFunc func(*logrus.Entry, *CodeReviewCommon, contextChecker) bool, priorities []config.TidePriority) (bool, CodeReviewCommon) {
	smallestNumber := -1
	var smallestPR CodeReviewCommon
	for _, p := range append(priorities, config.TidePriority{}) {
		for _, pr := range prs {
			// This should only apply to GitHub PRs, for Gerrit this is always true.
			if !hasAllLabels(pr, p.Labels) {
				continue
			}
			if smallestNumber != -1 && pr.Number >= smallestNumber {
				continue
			}
			if !isPassingTestsFunc(log, &pr, cc[pr.Number]) {
				continue
			}
			smallestNumber = pr.Number
			smallestPR = pr
		}
		if smallestNumber > -1 {
			return true, smallestPR
		}
	}
	return false, smallestPR
}

// accumulateBatch looks at existing batch ProwJobs and, if applicable, returns:
// * A list of PRs that are part of a batch test that finished successfully
// * A list of PRs that are part of a batch test that hasn't finished yet but
// didn't have any failures so far
//
// jobs that are configured as `run_before_merge` are required to be returned as
// successBatch, it's possible that these jobs haven't run yet, and in the case
// we should consider this batch as failed so that takeAction can trigger a new
// batch.
func (c *syncController) accumulateBatch(sp subpool) (successBatch []CodeReviewCommon, pendingBatch []CodeReviewCommon) {
	sp.log.Debug("accumulating PRs for batch testing")
	prNums := make(map[int]CodeReviewCommon)
	for _, pr := range sp.prs {
		prNums[pr.Number] = pr
	}
	type accState struct {
		prs       []CodeReviewCommon
		jobStates map[string]simpleState
		// Are the pull requests in the ref still acceptable? That is, do they
		// still point to the heads of the PRs?
		validPulls bool
	}
	states := make(map[string]*accState)
	for _, pj := range sp.pjs {
		if pj.Spec.Type != prowapi.BatchJob {
			continue
		}
		// First validate the batch job's refs.
		ref := pj.Spec.Refs.String()
		if _, ok := states[ref]; !ok {
			state := &accState{
				jobStates:  make(map[string]simpleState),
				validPulls: true,
			}
			for _, pull := range pj.Spec.Refs.Pulls {
				if pr, ok := prNums[pull.Number]; ok && pr.HeadRefOID == pull.SHA {
					state.prs = append(state.prs, pr)
				} else if !ok {
					state.validPulls = false
					sp.log.WithField("batch", ref).WithFields(pr.logFields()).Debug("batch job invalid, PR left pool")
					break
				} else {
					state.validPulls = false
					sp.log.WithField("batch", ref).WithFields(pr.logFields()).Debug("batch job invalid, PR HEAD changed")
					break
				}
			}
			states[ref] = state
		}
		if !states[ref].validPulls {
			// The batch contains a PR ref that has changed. Skip it.
			continue
		}

		// Batch job refs are valid. Now accumulate job states by batch ref.
		context := pj.Spec.Context
		jobState := toSimpleState(pj.Status.State)
		// Store the best result for this ref+context.
		states[ref].jobStates[context] = getBetterSimpleState(states[ref].jobStates[context], jobState)
	}
	for ref, state := range states {
		if !state.validPulls {
			continue
		}

		// presubmitsForBatch includes jobs that are `run_before_merge`, the
		// jobs are not triggered before entering tide pool, and will need to be
		// handled below.
		requiredPresubmits, err := c.presubmitsForBatch(state.prs, sp.org, sp.repo, sp.sha, sp.branch)
		if err != nil {
			sp.log.WithError(err).Error("Error getting presubmits for batch")
			continue
		}

		overallState := successState
		for _, p := range requiredPresubmits {
			if s, ok := state.jobStates[p.Context]; !ok {
				// This could happen to jobs configured as `run_before_merge` as
				// these jobs are triggered only by tide. There is no need to
				// handle it differently as a new batch is expected in both cases.
				overallState = failureState
				sp.log.WithField("batch", ref).Debugf("batch invalid, required presubmit %s is missing", p.Context)
				break
			} else if s == failureState {
				overallState = failureState
				sp.log.WithField("batch", ref).Debugf("batch invalid, required presubmit %s is not passing", p.Context)
				break
			} else if s == pendingState && overallState == successState {
				overallState = pendingState
			}
		}
		switch overallState {
		// Currently we only consider 1 pending batch and 1 success batch at a time.
		// If more are somehow present they will be ignored.
		case pendingState:
			pendingBatch = state.prs
		case successState:
			successBatch = state.prs
		}
	}
	return successBatch, pendingBatch
}

// prowJobsFromContexts constructs ProwJob objects from all successful presubmit contexts that include a baseSHA.
// This is needed because otherwise we would always need retesting for results that are older than sinkers
// max_prowjob_age.
func (c *syncController) prowJobsFromContexts(pr *CodeReviewCommon, baseSHA string) ([]prowapi.ProwJob, error) {
	headContexts, err := c.provider.headContexts(pr)
	if err != nil {
		return nil, fmt.Errorf("failed to get head contexts: %w", err)
	}
	var passingCurrentContexts []string
	for _, headContext := range headContexts {
		if headContext.State != githubql.StatusStateSuccess {
			continue
		}
		if baseSHAForContext := config.BaseSHAFromContextDescription(string(headContext.Description)); baseSHAForContext != "" && baseSHAForContext == baseSHA {
			passingCurrentContexts = append(passingCurrentContexts, string((headContext.Context)))
		}
	}

	var prowjobsFromContexts []prowapi.ProwJob
	for _, passingCurrentContext := range passingCurrentContexts {
		prowjobsFromContexts = append(prowjobsFromContexts, prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Context: passingCurrentContext,
				Refs:    &prowapi.Refs{Pulls: []prowapi.Pull{{Number: pr.Number, SHA: pr.HeadRefOID}}},
				Type:    prowapi.PresubmitJob,
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.SuccessState,
			},
		})
	}

	return prowjobsFromContexts, nil
}

// accumulate returns the supplied PRs sorted into three buckets based on their
// accumulated state across the presubmits.
func (c *syncController) accumulate(presubmits map[int][]config.Presubmit, prs []CodeReviewCommon, pjs []prowapi.ProwJob, baseSHA string) (successes, pendings, missings []CodeReviewCommon, missingTests map[int][]config.Presubmit) {
	log := c.logger
	missingTests = map[int][]config.Presubmit{}
	for _, pr := range prs {

		if prowjobsFromContexts, err := c.prowJobsFromContexts(&pr, baseSHA); err != nil {
			log.WithError(err).Error("failed to get prowjobs from contexts")
		} else {
			pjs = append(pjs, prowjobsFromContexts...)
		}

		// Accumulate the best result for each job (Passing > Pending > Failing/Unknown)
		// We can ignore the baseSHA here because the subPool only contains ProwJobs with the correct baseSHA
		psStates := make(map[string]simpleState)
		for _, pj := range pjs {
			if pj.Spec.Type != prowapi.PresubmitJob {
				continue
			}
			if pj.Spec.Refs.Pulls[0].Number != pr.Number {
				continue
			}
			if pj.Spec.Refs.Pulls[0].SHA != pr.HeadRefOID {
				continue
			}

			name := pj.Spec.Context
			psStates[name] = getBetterSimpleState(psStates[name], toSimpleState(pj.Status.State))
		}
		// The overall result for the PR is the worst of the best of all its
		// required Presubmits
		overallState := successState
		for _, ps := range presubmits[pr.Number] {
			if s, ok := psStates[ps.Context]; !ok {
				// No PJ with correct baseSHA+headSHA exists
				missingTests[pr.Number] = append(missingTests[pr.Number], ps)
				log.WithFields(pr.logFields()).Debugf("missing presubmit %s", ps.Context)
			} else if s == failureState {
				// PJ with correct baseSHA+headSHA exists but failed
				missingTests[pr.Number] = append(missingTests[pr.Number], ps)
				log.WithFields(pr.logFields()).Debugf("presubmit %s not passing", ps.Context)
			} else if s == pendingState {
				log.WithFields(pr.logFields()).Debugf("presubmit %s pending", ps.Context)
				overallState = pendingState
			}
		}
		if len(missingTests[pr.Number]) > 0 {
			overallState = failureState
		}

		if overallState == successState {
			successes = append(successes, pr)
		} else if overallState == pendingState {
			pendings = append(pendings, pr)
		} else {
			missings = append(missings, pr)
		}
	}
	return
}

func prNumbers(prs []CodeReviewCommon) []int {
	var nums []int
	for _, pr := range prs {
		nums = append(nums, pr.Number)
	}
	return nums
}

// pickNewBatch picks PRs to form a new batch, it's only used by pickBatch.
//
// This function works for any source code provider.
func pickNewBatch(gc git.ClientFactory, cfg config.Getter, provider provider) func(sp subpool, candidates []CodeReviewCommon, maxBatchSize int) ([]CodeReviewCommon, error) {
	return func(sp subpool, candidates []CodeReviewCommon, maxBatchSize int) ([]CodeReviewCommon, error) {
		var res []CodeReviewCommon
		// TODO(chaodaiG): make sure cloning works for gerrit.
		r, err := gc.ClientFor(sp.org, sp.repo)
		if err != nil {
			return nil, err
		}
		defer r.Clean()
		if err := r.Config("user.name", "prow"); err != nil {
			return nil, err
		}
		if err := r.Config("user.email", "prow@localhost"); err != nil {
			return nil, err
		}
		if err := r.Config("commit.gpgsign", "false"); err != nil {
			sp.log.Warningf("Cannot set gpgsign=false in gitconfig: %v", err)
		}
		if err := r.Checkout(sp.sha); err != nil {
			return nil, err
		}

		for _, pr := range candidates {
			mergeMethod := provider.prMergeMethod(&pr)
			if mergeMethod == nil {
				sp.log.WithFields(pr.logFields()).Warnln("Failed to get merge method for PR, will skip.")
				continue
			}
			if ok, err := r.MergeWithStrategy(pr.HeadRefOID, string(*mergeMethod)); err != nil {
				// we failed to abort the merge and our git client is
				// in a bad state; it must be cleaned before we try again
				return nil, err
			} else if ok {
				res = append(res, pr)
				// TODO: Make this configurable per subpool.
				if maxBatchSize > 0 && len(res) >= maxBatchSize {
					break
				}
			}
		}

		return res, nil
	}
}

type newBatchFunc func(sp subpool, candidates []CodeReviewCommon, maxBatchSize int) ([]CodeReviewCommon, error)

// pickBatch picks PRs to form a batch.
//
// This function works for any source code provider.
func (c *syncController) pickBatch(sp subpool, cc map[int]contextChecker, newBatchFunc newBatchFunc) ([]CodeReviewCommon, []config.Presubmit, error) {
	// BatchSizeLimit is a global option, it will work for any source code provider.
	batchLimit := c.config().Tide.BatchSizeLimit(config.OrgRepo{Org: sp.org, Repo: sp.repo})
	if batchLimit < 0 {
		sp.log.Debug("Batch merges disabled by configuration in this repo.")
		return nil, nil, nil
	}

	// we must choose the oldest PRs for the batch
	sort.Slice(sp.prs, func(i, j int) bool { return sp.prs[i].Number < sp.prs[j].Number })

	var candidates []CodeReviewCommon
	for _, pr := range sp.prs {
		// c.isRetestEligible appends `Commits` into the passed in PullRequest
		// struct, which is used later to avoid repeatedly looking up on GitHub.
		if c.isRetestEligible(sp.log, &pr, cc[pr.Number]) {
			candidates = append(candidates, pr)
		}
	}

	log := sp.log.WithField("subpool_pr_count", len(sp.prs))
	if len(candidates) == 0 {
		log.Debug("None of the prs in the subpool was passing tests, no batch will be created")
		return nil, nil, nil
	}
	log.WithField("candidate_count", len(candidates)).Debug("Found PRs with passing tests when picking batch")

	var res []CodeReviewCommon
	// PrioritizeExistingBatches is a global option, it will work for any source
	// code provider.
	if c.config().Tide.PrioritizeExistingBatches(config.OrgRepo{Repo: sp.repo, Org: sp.org}) {
		res = pickBatchWithPreexistingTests(sp, candidates, batchLimit)
	}
	// No batch with pre-existing tests found or prioritize_existing_batches disabled
	if len(res) == 0 {
		var err error
		res, err = newBatchFunc(sp, candidates, batchLimit)
		if err != nil {
			return nil, nil, err
		}
	}

	// presubmitsForBatch returns jobs that should run via trigger, as well as
	// jobs that are `run_before_merge`.
	presubmits, err := c.presubmitsForBatch(res, sp.org, sp.repo, sp.sha, sp.branch)
	if err != nil {
		return nil, nil, err
	}

	return res, presubmits, nil
}

// isRetestEligible determines retesting eligibility. It allows PRs where all mandatory contexts
// are either passing or pending. Pending ones are only allowed if we find a ProwJob that corresponds to them
// and was created by Tide, as that allows us to infer that this job passed in the past.
// We look at the actively running ProwJob rather than a previous successful one, because the latter might
// already be garbage collected.
func (c *syncController) isRetestEligible(log *logrus.Entry, candidate *CodeReviewCommon, cc contextChecker) bool {
	candidateHeadContexts, err := c.provider.headContexts(candidate)
	if err != nil {
		log.WithError(err).WithFields(candidate.logFields()).Debug("failed to get headContexts for batch candidate, ignoring.")
		return false
	}
	var contextNames []string
	for _, headContext := range candidateHeadContexts {
		contextNames = append(contextNames, string(headContext.Context))
	}

	if missedContexts := cc.MissingRequiredContexts(contextNames); len(missedContexts) > 0 {
		return false
	}

	for _, headContext := range candidateHeadContexts {
		if headContext.Context == statusContext || cc.IsOptional(string(headContext.Context)) || headContext.State == githubql.StatusStateSuccess {
			continue
		}
		if headContext.State != githubql.StatusStatePending {
			return false
		}

		// In the case where a status is pending,
		// If the prowjob was triggered by tide, then tide had considered it a
		// good candidate. We should still consider it as a candidate.
		pjLabels := make(map[string]string)
		pjLabels[kube.CreatedByTideLabel] = "true"
		pjLabels[kube.ProwJobTypeLabel] = string(prowapi.PresubmitJob)
		pjLabels[kube.OrgLabel] = string(candidate.Org)
		pjLabels[kube.RepoLabel] = string(candidate.Repo)
		pjLabels[kube.BaseRefLabel] = string(candidate.BaseRefName)
		pjLabels[kube.PullLabel] = string(strconv.Itoa(int(candidate.Number)))
		pjLabels[kube.ContextAnnotation] = string(headContext.Context)

		var pjs prowapi.ProwJobList
		if err := c.prowJobClient.List(c.ctx,
			&pjs,
			ctrlruntimeclient.InNamespace(c.config().ProwJobNamespace),
			ctrlruntimeclient.MatchingLabels(pjLabels),
		); err != nil {
			log.WithError(err).Debug("failed to list prowjobs for PR, ignoring")
			return false
		}

		if prowJobListHasProwJobWithMatchingHeadSHA(&pjs, string(candidate.HeadRefOID)) {
			continue
		}

		return false
	}

	return true
}

func prowJobListHasProwJobWithMatchingHeadSHA(pjs *prowapi.ProwJobList, headSHA string) bool {
	for _, pj := range pjs.Items {
		if pj.Spec.Refs != nil && len(pj.Spec.Refs.Pulls) == 1 && pj.Spec.Refs.Pulls[0].SHA == headSHA {
			return true
		}
	}
	return false
}

// setTideStatusSuccess ensures the tide context is set to success
//
// Used only by mergePRs, referenced by GitHubProvider only.
func setTideStatusSuccess(pr CodeReviewCommon, ghc githubClient, cfg *config.Config, log *logrus.Entry) error {
	// Do not waste api tokens and risk hitting the 2.5k context limit by setting it to success if it is
	// already set to success.
	if prHasSuccessfullTideStatusContext(pr) {
		return nil
	}
	return ghc.CreateStatus(
		pr.Org,
		pr.Repo,
		pr.HeadRefOID,
		github.Status{
			Context:   statusContext,
			State:     "success",
			TargetURL: targetURL(cfg, &pr, log),
		})
}

// prHasSuccessfullTideStatusContext is used only by setTideStatusSuccess.
//
// Used only by setTideStatusSuccess, referenced only by GitHubProvider.
func prHasSuccessfullTideStatusContext(pr CodeReviewCommon) bool {
	commits := pr.GitHubCommits()
	if commits == nil {
		return false
	}
	for _, commit := range commits.Nodes {
		if string(commit.Commit.OID) != pr.HeadRefOID {
			continue
		}
		for _, context := range commit.Commit.Status.Contexts {
			if strings.EqualFold(string(context.Context), statusContext) {
				return strings.EqualFold(string(context.State), string(githubql.StatusStateSuccess))
			}
		}
	}

	return false
}

// tryMerge attempts 1 merge and returns a bool indicating if we should try
// to merge the remaining PRs and possibly an error.
//
// tryMerge is used by mergePRs only, referenced by GitHubProvider only.
func tryMerge(mergeFunc func() error) (bool, error) {
	var err error
	const maxRetries = 3
	backoff := time.Second * 4
	for retry := 0; retry < maxRetries; retry++ {
		if err = mergeFunc(); err == nil {
			// Successful merge!
			return true, nil
		}
		// TODO: Add a config option to abort batches if a PR in the batch
		// cannot be merged for any reason. This would skip merging
		// not just the changed PR, but also the other PRs in the batch.
		// This shouldn't be the default behavior as merging batches is high
		// priority and this is unlikely to be problematic.
		// Note: We would also need to be able to roll back any merges for the
		// batch that were already successfully completed before the failure.
		// Ref: https://github.com/kubernetes/test-infra/issues/10621
		if _, ok := err.(github.ModifiedHeadError); ok {
			// This is a possible source of incorrect behavior. If someone
			// modifies their PR as we try to merge it in a batch then we
			// end up in an untested state. This is unlikely to cause any
			// real problems.
			return true, fmt.Errorf("PR was modified: %w", err)
		} else if _, ok = err.(github.UnmergablePRBaseChangedError); ok {
			//  complained that the base branch was modified. This is a
			// strange error because the API doesn't even allow the request to
			// specify the base branch sha, only the head sha.
			// We suspect that github is complaining because we are making the
			// merge requests too rapidly and it cannot recompute mergability
			// in time. https://github.com/kubernetes/test-infra/issues/5171
			// We handle this by sleeping for a few seconds before trying to
			// merge again.
			err = fmt.Errorf("base branch was modified: %w", err)
			if retry+1 < maxRetries {
				sleep(backoff)
				backoff *= 2
			}
		} else if _, ok = err.(github.UnauthorizedToPushError); ok {
			// GitHub let us know that the token used cannot push to the branch.
			// Even if the robot is set up to have write access to the repo, an
			// overzealous branch protection setting will not allow the robot to
			// push to a specific branch.
			// We won't be able to merge the other PRs.
			return false, fmt.Errorf("branch needs to be configured to allow this robot to push: %w", err)
		} else if _, ok = err.(github.MergeCommitsForbiddenError); ok {
			// GitHub let us know that the merge method configured for this repo
			// is not allowed by other repo settings, so we should let the admins
			// know that the configuration needs to be updated.
			// We won't be able to merge the other PRs.
			return false, fmt.Errorf("Tide needs to be configured to use the 'rebase' merge method for this repo or the repo needs to allow merge commits: %w", err)
		} else if _, ok = err.(github.UnmergablePRError); ok {
			return true, fmt.Errorf("PR is unmergable. Do the Tide merge requirements match the GitHub settings for the repo? %w", err)
		} else {
			return true, err
		}
	}
	// We ran out of retries. Return the last transient error.
	return true, err
}

func (c *syncController) trigger(sp subpool, presubmits []config.Presubmit, prs []CodeReviewCommon) error {
	refs, err := c.provider.refsForJob(sp, prs)
	if err != nil {
		return fmt.Errorf("failed creating refs: %v", err)
	}

	// If PRs require the same job, we only want to trigger it once.
	// If multiple required jobs have the same context, we assume the
	// same shard will be run to provide those contexts
	triggeredContexts := sets.New[string]()
	enableScheduling := c.config().Scheduler.Enabled
	for _, ps := range presubmits {
		if triggeredContexts.Has(string(ps.Context)) {
			continue
		}
		triggeredContexts.Insert(string(ps.Context))
		var spec prowapi.ProwJobSpec
		if len(prs) == 1 {
			spec = pjutil.PresubmitSpec(ps, refs)
		} else {
			if c.nonFailedBatchForJobAndRefsExists(ps.Name, &refs) {
				continue
			}
			spec = pjutil.BatchSpec(ps, refs)
		}
		labels, annotations := c.provider.labelsAndAnnotations(sp.org, ps.Labels, ps.Annotations, prs...)
		pj := pjutil.NewProwJob(spec, labels, annotations, pjutil.RequireScheduling(enableScheduling))
		pj.Namespace = c.config().ProwJobNamespace
		log := c.logger.WithFields(pjutil.ProwJobFields(&pj))
		start := time.Now()
		if pj.Labels == nil {
			pj.Labels = map[string]string{}
		}
		pj.Labels[kube.CreatedByTideLabel] = "true"
		if err := c.prowJobClient.Create(c.ctx, &pj); err != nil {
			log.WithField("duration", time.Since(start).String()).Debug("Failed to create ProwJob on the cluster.")
			return fmt.Errorf("failed to create a ProwJob for job: %q, PRs: %v: %w", spec.Job, prNumbers(prs), err)
		}
		log.WithField("duration", time.Since(start).String()).Debug("Created ProwJob on the cluster.")
	}
	return nil
}

// nonFailedBatchForJobAndRefsExists ensures that the batch job exists
func (c *syncController) nonFailedBatchForJobAndRefsExists(jobName string, refs *prowapi.Refs) bool {
	pjs := &prowapi.ProwJobList{}
	if err := c.prowJobClient.List(c.ctx,
		pjs,
		ctrlruntimeclient.MatchingFields{nonFailedBatchByNameBaseAndPullsIndexName: nonFailedBatchByNameBaseAndPullsIndexKey(jobName, refs)},
		ctrlruntimeclient.InNamespace(c.config().ProwJobNamespace),
	); err != nil {
		c.logger.WithError(err).Error("Failed to list non-failed batches")
		return false
	}

	return len(pjs.Items) > 0
}

func (c *syncController) takeAction(sp subpool, batchPending, successes, pendings, missings, batchMerges []CodeReviewCommon, missingSerialTests map[int][]config.Presubmit) (Action, []CodeReviewCommon, error) {
	var merged []CodeReviewCommon
	var err error
	defer func() {
		if len(merged) > 0 {
			tideMetrics.merges.WithLabelValues(sp.org, sp.repo, sp.branch).Observe(float64(len(merged)))
		}
	}()

	// Merge the batch!
	if len(batchMerges) > 0 {
		merged, err = c.provider.mergePRs(sp, batchMerges, c.statusUpdate.dontUpdateStatus)
		return MergeBatch, batchMerges, err
	}
	// Do not merge PRs while waiting for a batch to complete. We don't want to
	// invalidate the old batch result.
	if len(successes) > 0 && len(batchPending) == 0 {
		if ok, pr := pickHighestPriorityPR(sp.log, successes, sp.cc, c.isPassingTests, c.config().Tide.Priority); ok {
			merged, err = c.provider.mergePRs(sp, []CodeReviewCommon{pr}, c.statusUpdate.dontUpdateStatus)
			return Merge, []CodeReviewCommon{pr}, err
		}
	}
	// If no presubmits are configured, just wait.
	if len(sp.presubmits) == 0 {
		return Wait, nil, nil
	}
	// If we have no batch, trigger one.
	if len(sp.prs) > 1 && len(batchPending) == 0 {
		batch, presubmits, err := c.pickBatch(sp, sp.cc, c.pickNewBatch)
		if err != nil {
			return Wait, nil, err
		}
		if len(batch) > 1 {
			return TriggerBatch, batch, c.trigger(sp, presubmits, batch)
		}
	}
	// If we have no serial jobs pending or successful, trigger one.
	if len(missings) > 0 && len(pendings) == 0 && len(successes) == 0 {
		if ok, pr := pickHighestPriorityPR(sp.log, missings, sp.cc, c.isRetestEligible, c.config().Tide.Priority); ok {
			return Trigger, []CodeReviewCommon{pr}, c.trigger(sp, missingSerialTests[pr.Number], []CodeReviewCommon{pr})
		}
	}
	return Wait, nil, nil
}

// changedFilesAgent queries and caches the names of files changed by PRs.
// Cache entries expire if they are not used during a sync loop.
type changedFilesAgent struct {
	provider    provider
	changeCache map[changeCacheKey][]string
	// nextChangeCache caches file change info that is relevant this sync for use next sync.
	// This becomes the new changeCache when prune() is called at the end of each sync.
	nextChangeCache map[changeCacheKey][]string
	sync.RWMutex
}

type changeCacheKey struct {
	org, repo string
	number    int
	sha       string
}

// prChanges gets the files changed by the PR, either from the cache or by
// querying GitHub.
func (c *changedFilesAgent) prChanges(pr *CodeReviewCommon) config.ChangedFilesProvider {
	return func() ([]string, error) {
		cacheKey := changeCacheKey{
			org:    pr.Org,
			repo:   pr.Repo,
			number: pr.Number,
			sha:    pr.HeadRefOID,
		}

		c.RLock()
		changedFiles, ok := c.changeCache[cacheKey]
		if ok {
			c.RUnlock()
			c.Lock()
			c.nextChangeCache[cacheKey] = changedFiles
			c.Unlock()
			return changedFiles, nil
		}
		if changedFiles, ok = c.nextChangeCache[cacheKey]; ok {
			c.RUnlock()
			return changedFiles, nil
		}
		c.RUnlock()

		// We need to query the changes from GitHub.
		changes, err := c.provider.GetChangedFiles(
			pr.Org,
			pr.Repo,
			pr.Number,
		)
		if err != nil {
			return nil, fmt.Errorf("error getting PR changes for #%d: %w", pr.Number, err)
		}

		changedFiles = make([]string, 0, len(changes))
		changedFiles = append(changedFiles, changes...)
		c.Lock()
		c.nextChangeCache[cacheKey] = changedFiles
		c.Unlock()
		return changedFiles, nil
	}
}

func (c *changedFilesAgent) batchChanges(prs []CodeReviewCommon) config.ChangedFilesProvider {
	return func() ([]string, error) {
		result := sets.Set[string]{}
		for _, pr := range prs {
			changes, err := c.prChanges(&pr)()
			if err != nil {
				return nil, err
			}

			result.Insert(changes...)
		}

		return sets.List(result), nil
	}
}

// prune removes any cached file changes that were not used since the last prune.
func (c *changedFilesAgent) prune() {
	c.Lock()
	defer c.Unlock()
	c.changeCache = c.nextChangeCache
	c.nextChangeCache = make(map[changeCacheKey][]string)
}

func refGetterFactory(ref string) config.RefGetter {
	return func() (string, error) {
		return ref, nil
	}
}

// presubmitsByPull creates a map pr -> requiredPresubmits and will filter out all PRs
// where we failed to find out the required presubmits (can happen if inrepoconfig is enabled).
func (c *syncController) presubmitsByPull(sp *subpool) (map[int][]config.Presubmit, error) {
	presubmits := make(map[int][]config.Presubmit, len(sp.prs))

	// filtered PRs contains all PRs for which we were able to get the presubmits
	var filteredPRs []CodeReviewCommon

	for _, pr := range sp.prs {
		log := c.logger.WithField("base-sha", sp.sha).WithFields(pr.logFields())
		requireManuallyTriggeredJobs := requireManuallyTriggeredJobs(c.config(), sp.org, sp.repo, pr.BaseRefName)
		presubmitsForPull, err := c.provider.GetPresubmits(sp.org+"/"+sp.repo, pr.BaseRefName, refGetterFactory(sp.sha), refGetterFactory(pr.HeadRefOID))
		if err != nil {
			log.WithError(err).Debug("Failed to get presubmits for PR, excluding from subpool")
			continue
		}
		filteredPRs = append(filteredPRs, pr)
		log.WithField("num_possible_presubmit", len(presubmitsForPull)).Debug("Found possible presubmits")

		for _, ps := range presubmitsForPull {
			if !c.provider.jobIsRequiredByTide(&ps, &pr) {
				continue
			}

			// Only keep the jobs that are required for this PR. Order of
			// filters:
			// - Brancher
			// - RunBeforeMerge
			// - Files changed
			forceRun := (requireManuallyTriggeredJobs && ps.ContextRequired() && ps.NeedsExplicitTrigger()) || ps.RunBeforeMerge
			shouldRun, err := ps.ShouldRun(sp.branch, c.changedFiles.prChanges(&pr), forceRun, false)
			if err != nil {
				return nil, err
			}
			if !shouldRun {
				log.WithField("context", ps.Context).Debug("Presubmit excluded by ps.ShouldRun")
				continue
			}

			presubmits[pr.Number] = append(presubmits[pr.Number], ps)
		}
		log.WithField("required-presubmit-count", len(presubmits[pr.Number])).Debug("Determined required presubmits for PR.")
	}

	sp.prs = filteredPRs
	return presubmits, nil
}

// presubmitsForBatch filters presubmit jobs from a repo based on the PRs in the
// pool.
//
// Aside from jobs that should run based on triggers, jobs that are configured
// as `run_before_merge` are also returned.
func (c *syncController) presubmitsForBatch(prs []CodeReviewCommon, org, repo, baseSHA, baseBranch string) ([]config.Presubmit, error) {
	log := c.logger.WithFields(logrus.Fields{"repo": repo, "org": org, "base-sha": baseSHA, "base-branch": baseBranch})

	if len(prs) == 0 {
		log.Debug("No PRs, skip looking for presubmits for batch.")
		return nil, errors.New("no PRs are provided")
	}

	var headRefGetters []config.RefGetter
	for _, pr := range prs {
		headRefGetters = append(headRefGetters, refGetterFactory(pr.HeadRefOID))
	}

	presubmits, err := c.provider.GetPresubmits(org+"/"+repo, baseBranch, refGetterFactory(baseSHA), headRefGetters...)
	if err != nil {
		return nil, fmt.Errorf("failed to get presubmits for batch: %w", err)
	}
	log.Debugf("Found %d possible presubmits for batch", len(presubmits))

	requireManuallyTriggeredJobs := requireManuallyTriggeredJobs(c.config(), org, repo, baseBranch)

	var result []config.Presubmit
	for _, ps := range presubmits {
		// PR is required only by Gerrit, the required "label" will be extracted
		// from a PR. Assuming the submission requirement for a given label is
		// consistent across all PRs from the same repo at a given time point,
		// which should be a safe assumption.
		if !c.provider.jobIsRequiredByTide(&ps, &prs[0]) {
			continue
		}

		forceRun := (requireManuallyTriggeredJobs && ps.ContextRequired() && ps.NeedsExplicitTrigger()) || ps.RunBeforeMerge
		shouldRun, err := ps.ShouldRun(baseBranch, c.changedFiles.batchChanges(prs), forceRun, false)
		if err != nil {
			return nil, err
		}
		if !shouldRun {
			log.WithField("context", ps.Context).Debug("Presubmit excluded by ps.ShouldRun")
			continue
		}

		result = append(result, ps)
	}

	log.Debugf("After filtering, %d presubmits remained for batch", len(result))
	return result, nil
}

func (c *syncController) syncSubpool(sp subpool, blocks []blockers.Blocker) (Pool, error) {
	sp.log.WithField("num_prs", len(sp.prs)).WithField("num_prowjobs", len(sp.pjs)).Info("Syncing subpool")
	successes, pendings, missings, missingSerialTests := c.accumulate(sp.presubmits, sp.prs, sp.pjs, sp.sha)
	batchMerge, batchPending := c.accumulateBatch(sp)
	sp.log.WithFields(logrus.Fields{
		"prs-passing":   prNumbers(successes),
		"prs-pending":   prNumbers(pendings),
		"prs-missing":   prNumbers(missings),
		"batch-passing": prNumbers(batchMerge),
		"batch-pending": prNumbers(batchPending),
	}).Info("Subpool accumulated.")

	tenantIDs := sp.TenantIDs()
	var act Action
	var targets []CodeReviewCommon
	var err error
	var errorString string
	if len(blocks) > 0 {
		act = PoolBlocked
	} else {
		act, targets, err = c.takeAction(sp, batchPending, successes, pendings, missings, batchMerge, missingSerialTests)
		if err != nil {
			errorString = err.Error()
		}
		if recordableActions[act] {
			c.History.Record(
				poolKey(sp.org, sp.repo, sp.branch),
				string(act),
				sp.sha,
				errorString,
				prMeta(targets...),
				tenantIDs,
			)
		}
	}

	sp.log.WithFields(logrus.Fields{
		"action":  string(act),
		"targets": prNumbers(targets),
	}).Info("Subpool synced.")
	tideMetrics.pooledPRs.WithLabelValues(sp.org, sp.repo, sp.branch).Set(float64(len(sp.prs)))
	tideMetrics.updateTime.WithLabelValues(sp.org, sp.repo, sp.branch).Set(float64(time.Now().Unix()))
	return Pool{
			Org:    sp.org,
			Repo:   sp.repo,
			Branch: sp.branch,

			SuccessPRs: successes,
			PendingPRs: pendings,
			MissingPRs: missings,

			BatchPending: batchPending,

			Action:   act,
			Target:   targets,
			Blockers: blocks,
			Error:    errorString,

			TenantIDs: tenantIDs,
		},
		err
}

func prMeta(prs ...CodeReviewCommon) []prowapi.Pull {
	var res []prowapi.Pull
	for _, pr := range prs {
		res = append(res, prowapi.Pull{
			Number:  pr.Number,
			Author:  pr.AuthorLogin,
			Title:   pr.Title,
			SHA:     pr.HeadRefOID,
			HeadRef: pr.HeadRefName,
		})
	}
	return res
}

func sortPools(pools []Pool) {
	sort.Slice(pools, func(i, j int) bool {
		if string(pools[i].Org) != string(pools[j].Org) {
			return string(pools[i].Org) < string(pools[j].Org)
		}
		if string(pools[i].Repo) != string(pools[j].Repo) {
			return string(pools[i].Repo) < string(pools[j].Repo)
		}
		return string(pools[i].Branch) < string(pools[j].Branch)
	})

	sortPRs := func(prs []CodeReviewCommon) {
		sort.Slice(prs, func(i, j int) bool { return int(prs[i].Number) < int(prs[j].Number) })
	}
	for i := range pools {
		sortPRs(pools[i].SuccessPRs)
		sortPRs(pools[i].PendingPRs)
		sortPRs(pools[i].MissingPRs)
		sortPRs(pools[i].BatchPending)
	}
}

type subpool struct {
	log      *logrus.Entry
	org      string
	repo     string
	cloneURI string
	branch   string
	// sha is the baseSHA for this subpool
	sha string

	// pjs contains all ProwJobs of type Presubmit or Batch
	// that have the same baseSHA as the subpool
	pjs []prowapi.ProwJob
	prs []CodeReviewCommon

	cc map[int]contextChecker
	// presubmit contains all required presubmits for each PR
	// in this subpool
	presubmits map[int][]config.Presubmit
}

func (sp subpool) TenantIDs() []string {
	ids := sets.Set[string]{}
	for _, pj := range sp.pjs {
		if pj.Spec.ProwJobDefault == nil || pj.Spec.ProwJobDefault.TenantID == "" {
			ids.Insert("")
		} else {
			ids.Insert(pj.Spec.ProwJobDefault.TenantID)
		}
	}
	return sets.List(ids)
}

func poolKey(org, repo, branch string) string {
	return fmt.Sprintf("%s/%s:%s", org, repo, branch)
}

// dividePool splits up the list of pull requests and prow jobs into a group
// per repo and branch. It only keeps ProwJobs that match the latest branch.
func (c *syncController) dividePool(pool map[string]CodeReviewCommon) (map[string]*subpool, error) {
	sps := make(map[string]*subpool)
	for _, pr := range pool {
		org := pr.Org
		repo := pr.Repo
		branch := pr.BaseRefName
		branchRef := pr.BaseRefPrefix + pr.BaseRefName
		fn := poolKey(org, repo, branch)
		if sps[fn] == nil {
			sha, err := c.provider.GetRef(org, repo, strings.TrimPrefix(branchRef, "refs/"))
			if err != nil {
				return nil, err
			}
			sps[fn] = &subpool{
				log: c.logger.WithFields(logrus.Fields{
					"org":      org,
					"repo":     repo,
					"branch":   branch,
					"base-sha": sha,
				}),
				org:    org,
				repo:   repo,
				branch: branch,
				sha:    sha,
			}
		}
		sps[fn].prs = append(sps[fn].prs, pr)
	}

	for subpoolkey, sp := range sps {
		pjs := &prowapi.ProwJobList{}
		err := c.prowJobClient.List(
			c.ctx,
			pjs,
			ctrlruntimeclient.MatchingFields{cacheIndexName: cacheIndexKey(sp.org, sp.repo, sp.branch, sp.sha)},
			ctrlruntimeclient.InNamespace(c.config().ProwJobNamespace))
		if err != nil {
			return nil, fmt.Errorf("failed to list jobs for subpool %s: %w", subpoolkey, err)
		}
		sp.log.WithField("subpool", subpoolkey).WithField("pj_count", len(pjs.Items)).Debug("Found prowjobs")
		sps[subpoolkey].pjs = pjs.Items
	}
	return sps, nil
}

// PullRequest holds graphql data about a PR, including its commits and their
// contexts.
// This struct is GitHub specific
type PullRequest struct {
	Number githubql.Int
	Author struct {
		Login githubql.String
	}
	BaseRef struct {
		Name   githubql.String
		Prefix githubql.String
	}
	HeadRefName  githubql.String `graphql:"headRefName"`
	HeadRefOID   githubql.String `graphql:"headRefOid"`
	Mergeable    githubql.MergeableState
	CanBeRebased githubql.Boolean `graphql:"canBeRebased"`
	Repository   struct {
		Name          githubql.String
		NameWithOwner githubql.String
		Owner         struct {
			Login githubql.String
		}
	}
	ReviewDecision githubql.PullRequestReviewDecision `graphql:"reviewDecision"`
	// Request the 'last' 4 commits hoping that one of them is the logically 'last'
	// commit with OID matching HeadRefOID. If we don't find it we have to use an
	// additional API token. (see the 'headContexts' func for details)
	// We can't raise this too much or we could hit the limit of 50,000 nodes
	// per query: https://developer.github.com/v4/guides/resource-limitations/#node-limit
	Commits   Commits `graphql:"commits(last: 4)"`
	Labels    Labels  `graphql:"labels(first: 100)"`
	Milestone *Milestone
	Body      githubql.String
	Title     githubql.String
	UpdatedAt githubql.DateTime
}

func (pr *PullRequest) logFields() logrus.Fields {
	return logrus.Fields{
		"org":    pr.Repository.Owner.Login,
		"repo":   pr.Repository.Name,
		"pr":     pr.Number,
		"branch": pr.BaseRef.Name,
		"sha":    pr.HeadRefOID,
	}
}

type Labels struct {
	Nodes []struct {
		Name githubql.String
	}
}

type Milestone struct {
	Title githubql.String
}

type Commits struct {
	Nodes []struct {
		Commit Commit
	}
}

type CommitNode struct {
	Commit Commit
}

// Commit holds graphql data about commits and which contexts they have
type Commit struct {
	Status            CommitStatus
	OID               githubql.String `graphql:"oid"`
	StatusCheckRollup StatusCheckRollup
}

type CommitStatus struct {
	Contexts []Context
}

type StatusCheckRollup struct {
	Contexts StatusCheckRollupContext `graphql:"contexts(last: 100)"`
}

type StatusCheckRollupContext struct {
	Nodes []CheckRunNode
}

type CheckRunNode struct {
	CheckRun CheckRun `graphql:"... on CheckRun"`
}

type CheckRun struct {
	Name       githubql.String
	Conclusion githubql.String
	Status     githubql.String
}

// Context holds graphql response data for github contexts.
type Context struct {
	// Context is the name of the context, it's identical to the full name of a
	// prowjob if the context is for a prowjob.
	Context githubql.String
	// Description is the description for a context, it's formed by
	// config.ContextDescriptionWithBaseSha for a prowjob.
	Description githubql.String
	// State is the state for a prowjob: EXPECTED, ERROR, FAILURE, PENDING, SUCCESS.
	State githubql.StatusState
}

type PRNode struct {
	PullRequest PullRequest `graphql:"... on PullRequest"`
}

type searchQuery struct {
	RateLimit struct {
		Cost      githubql.Int
		Remaining githubql.Int
	}
	Search struct {
		PageInfo struct {
			HasNextPage githubql.Boolean
			EndCursor   githubql.String
		}
		Nodes []PRNode
	} `graphql:"search(type: ISSUE, first: 37, after: $searchCursor, query: $query)"`
}

// orgRepoQueryStrings returns the GitHub query strings for given orgs and
// repos. Make sure that this is only used by GitHub interactor.
func orgRepoQueryStrings(orgs, repos []string, orgExceptions map[string]sets.Set[string]) map[string]string {
	queriesByOrg := map[string]string{}

	for _, org := range orgs {
		queriesByOrg[org] = fmt.Sprintf(`org:"%s"`, org)

		for _, exception := range sets.List(orgExceptions[org]) {
			queriesByOrg[org] += fmt.Sprintf(` -repo:"%s"`, exception)
		}
	}

	for _, repo := range repos {
		if org, _, ok := splitOrgRepoString(repo); ok {
			queriesByOrg[org] += fmt.Sprintf(` repo:"%s"`, repo)
		}
	}

	return queriesByOrg
}

// splitOrgRepoString is used only by orgRepoQueryStrings, which is only used by
// GitHub related functions.
func splitOrgRepoString(orgRepo string) (string, string, bool) {
	split := strings.Split(orgRepo, "/")
	if len(split) != 2 {
		// Just do it like the github search itself and ignore invalid orgRepo identifiers
		return "", "", false
	}
	return split[0], split[1], true
}

// cacheIndexName is the name of the index that indexes presubmit+batch ProwJobs by
// org+repo+branch+baseSHA. Use the cacheIndexKey func to get the correct key.
const cacheIndexName = "tide-global-index"

// cacheIndexKey returns the index key for the tideCacheIndex
func cacheIndexKey(org, repo, branch, baseSHA string) string {
	return fmt.Sprintf("%s/%s:%s@%s", org, repo, branch, baseSHA)
}

// cacheIndexFunc ensures that the passed in Prowjob is only batch job.
//
// Used only by manager.FieldIndexer, so that only batch job is indexed.
func cacheIndexFunc(obj ctrlruntimeclient.Object) []string {
	pj := obj.(*prowapi.ProwJob)
	// We do not care about jobs other than presubmit and batch
	if pj.Spec.Type != prowapi.PresubmitJob && pj.Spec.Type != prowapi.BatchJob {
		return nil
	}
	if pj.Spec.Refs == nil {
		return nil
	}
	return []string{cacheIndexKey(pj.Spec.Refs.Org, pj.Spec.Refs.Repo, pj.Spec.Refs.BaseRef, pj.Spec.Refs.BaseSHA)}
}

// nonFailedBatchByNameBaseAndPullsIndexName is used as the key of a label, for
// non failed batching job. Use the nonFailedBatchByNameBaseAndPullsIndexKey
// function to get the correct value.
const nonFailedBatchByNameBaseAndPullsIndexName = "tide-non-failed-jobs-by-name-base-and-pulls"

// nonFailedBatchByNameBaseAndPullsIndexKey collects the PR numbers and SHAs from
// the batch job, and returns a string contain all of them. This is used only by
// nonFailedBatchByNameBaseAndPullsIndexFunc.
func nonFailedBatchByNameBaseAndPullsIndexKey(jobName string, refs *prowapi.Refs) string {
	// sort the pulls to make sure this is deterministic
	sort.Slice(refs.Pulls, func(i, j int) bool {
		return refs.Pulls[i].Number < refs.Pulls[j].Number
	})

	keys := []string{jobName, refs.Org, refs.Repo, refs.BaseRef, refs.BaseSHA}
	for _, pull := range refs.Pulls {
		keys = append(keys, strconv.Itoa(pull.Number), pull.SHA)
	}

	return strings.Join(keys, "|")
}

// nonFailedBatchByNameBaseAndPullsIndexFunc ensures that the passed in ProwJob
// object is a succeeded batch job, and returns the key from the job.
//
// Used only by manager.FieldIndexer, so that only non failed batch job is indexed.
func nonFailedBatchByNameBaseAndPullsIndexFunc(obj ctrlruntimeclient.Object) []string {
	pj := obj.(*prowapi.ProwJob)
	if pj.Spec.Type != prowapi.BatchJob || pj.Spec.Refs == nil {
		return nil
	}

	if pj.Complete() && pj.Status.State != prowapi.SuccessState {
		return nil
	}

	return []string{nonFailedBatchByNameBaseAndPullsIndexKey(pj.Spec.Job, pj.Spec.Refs)}
}

func checkRunNodesToContexts(log *logrus.Entry, nodes []CheckRunNode) []Context {
	var result []Context
	for _, node := range nodes {
		// GitHub gives us an empty checkrun per status context. In theory they could
		// at some point decide to create a virtual check run per status context.
		// If that were to happen, we would retrieve redundant data as we get the
		// status context both directly as a status context and as a checkrun, however
		// the actual data in there should be identical, hence this isn't a problem.
		if string(node.CheckRun.Name) == "" {
			continue
		}
		result = append(result, checkRunToContext(node.CheckRun))
	}
	result = deduplicateContexts(result)
	if len(result) > 0 {
		log.WithField("checkruns", len(result)).Debug("Transformed checkruns to contexts")
	}
	return result
}

type descriptionAndState struct {
	description githubql.String
	state       githubql.StatusState
}

// deduplicateContexts deduplicates contexts, returning the best result for
// contexts that have multiple entries.
//
// deduplicateContexts is used only by checkRunNodesToContexts.
func deduplicateContexts(contexts []Context) []Context {
	result := map[githubql.String]descriptionAndState{}
	for _, context := range contexts {
		previousResult, found := result[context.Context]
		if !found {
			result[context.Context] = descriptionAndState{description: context.Description, state: context.State}
			continue
		}
		if isStateBetter(previousResult.state, context.State) {
			result[context.Context] = descriptionAndState{description: context.Description, state: context.State}
		}
	}

	var resultSlice []Context
	for name, descriptionAndState := range result {
		resultSlice = append(resultSlice, Context{Context: name, Description: descriptionAndState.description, State: descriptionAndState.state})
	}

	return resultSlice
}

// isStateBetter is used only by deduplicateContexts.
func isStateBetter(previous, current githubql.StatusState) bool {
	if current == githubql.StatusStateSuccess {
		return true
	}
	if current == githubql.StatusStatePending && (previous == githubql.StatusStateError || previous == githubql.StatusStateFailure || previous == githubql.StatusStateExpected) {
		return true
	}
	if previous == githubql.StatusStateExpected && (current == githubql.StatusStateError || current == githubql.StatusStateFailure) {
		return true
	}

	return false
}

const (
	checkRunStatusCompleted   = githubql.String("COMPLETED")
	checkRunConclusionNeutral = githubql.String("NEUTRAL")
)

// checkRunToContext translates a checkRun to a classic context
// ref: https://developer.github.com/v3/checks/runs/#parameters
func checkRunToContext(checkRun CheckRun) Context {
	context := Context{
		Context: checkRun.Name,
	}
	if checkRun.Status != checkRunStatusCompleted {
		context.State = githubql.StatusStatePending
		return context
	}

	if checkRun.Conclusion == checkRunConclusionNeutral || checkRun.Conclusion == githubql.String(githubql.StatusStateSuccess) {
		context.State = githubql.StatusStateSuccess
		return context
	}

	context.State = githubql.StatusStateFailure
	return context
}

func pickBatchWithPreexistingTests(sp subpool, candidates []CodeReviewCommon, maxSize int) []CodeReviewCommon {
	batchCandidatesBySuccessfulJobCount := map[string]int{}
	batchCandidatesByPendingJobCount := map[string]int{}

	prNumbersToMapKey := func(prs []prowapi.Pull) string {
		var numbers []string
		for _, pr := range prs {
			numbers = append(numbers, strconv.Itoa(pr.Number))
		}
		return strings.Join(numbers, "|")
	}
	prNumbersFromMapKey := func(s string) []int {
		var result []int
		for _, element := range strings.Split(s, "|") {
			intVal, err := strconv.Atoi(element)
			if err != nil {
				logrus.WithField("element", element).Error("BUG: Found element in pr numbers map that was not parseable as int")
				return nil
			}
			result = append(result, intVal)
		}
		return result
	}
	for _, pj := range sp.pjs {
		if pj.Spec.Type != prowapi.BatchJob || (maxSize != 0 && len(pj.Spec.Refs.Pulls) > maxSize) || (pj.Status.State != prowapi.SuccessState && pj.Status.State != prowapi.PendingState) {
			continue
		}
		var hasInvalidPR bool
		for _, pull := range pj.Spec.Refs.Pulls {
			if !isPullInPRList(pull, candidates) {
				hasInvalidPR = true
				break
			}
		}
		if hasInvalidPR {
			continue
		}
		if pj.Status.State == prowapi.SuccessState {
			batchCandidatesBySuccessfulJobCount[prNumbersToMapKey(pj.Spec.Refs.Pulls)]++
		} else {
			batchCandidatesByPendingJobCount[prNumbersToMapKey(pj.Spec.Refs.Pulls)]++
		}
	}

	var resultPullNumbers []int
	if len(batchCandidatesBySuccessfulJobCount) > 0 {
		resultPullNumbers = prNumbersFromMapKey(mapKeyWithHighestvalue(batchCandidatesBySuccessfulJobCount))
	} else if len(batchCandidatesByPendingJobCount) > 0 {
		resultPullNumbers = prNumbersFromMapKey(mapKeyWithHighestvalue(batchCandidatesByPendingJobCount))
	}

	var result []CodeReviewCommon
	for _, resultPRNumber := range resultPullNumbers {
		for _, pr := range sp.prs {
			if pr.Number == resultPRNumber {
				result = append(result, pr)
				break
			}
		}
	}

	return result
}

func isPullInPRList(pull prowapi.Pull, allPRs []CodeReviewCommon) bool {
	for _, pullRequest := range allPRs {
		if pull.Number != int(pullRequest.Number) {
			continue
		}
		return pull.SHA == string(pullRequest.HeadRefOID)
	}

	return false
}

func mapKeyWithHighestvalue(m map[string]int) string {
	var result string
	var resultVal int
	for k, v := range m {
		if v > resultVal {
			result = k
			resultVal = v
		}
	}

	return result
}

// getBetterSimpleState returns the better simple state. It supports
// no state, failure, pending and success.
func getBetterSimpleState(a, b simpleState) simpleState {
	if a == "" || a == failureState || b == successState {
		// b can't be worse than no state or failure and a can't be beter than success
		return b
	}

	// a must be pending and b can not be success, so b can't be better than a
	return a
}

func requireManuallyTriggeredJobs(c *config.Config, org, repo, branch string) bool {
	options := config.ParseTideContextPolicyOptions(org, repo, branch, c.Tide.ContextOptions)
	if options.FromBranchProtection != nil && *options.FromBranchProtection {
		if b, err := c.BranchProtection.GetOrg(org).GetRepo(repo).GetBranch(branch); err == nil {
			if policy, err := c.GetPolicy(org, repo, branch, *b, []config.Presubmit{}, nil); err == nil && policy != nil {
				return policy.RequireManuallyTriggeredJobs != nil && *policy.RequireManuallyTriggeredJobs
			}
		}
	}
	return false
}

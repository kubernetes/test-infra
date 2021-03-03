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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/tide/blockers"
	"k8s.io/test-infra/prow/tide/history"
	"k8s.io/test-infra/prow/version"
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
type Controller struct {
	ctx                context.Context
	logger             *logrus.Entry
	config             config.Getter
	ghc                githubClient
	prowJobClient      ctrlruntimeclient.Client
	gc                 git.ClientFactory
	usesGitHubAppsAuth bool

	sc *statusController

	m     sync.Mutex
	pools []Pool

	// changedFiles caches the names of files changed by PRs.
	// Cache entries expire if they are not used during a sync loop.
	changedFiles *changedFilesAgent

	mergeChecker *mergeChecker

	History *history.History
}

// Action represents what actions the controller can take. It will take
// exactly one action each sync.
type Action string

// Constants for various actions the controller might take
const (
	Wait         Action = "WAIT"
	Trigger             = "TRIGGER"
	TriggerBatch        = "TRIGGER_BATCH"
	Merge               = "MERGE"
	MergeBatch          = "MERGE_BATCH"
	PoolBlocked         = "BLOCKED"
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
	SuccessPRs []PullRequest
	PendingPRs []PullRequest
	MissingPRs []PullRequest

	// Empty if there is no pending batch.
	BatchPending []PullRequest

	// Which action did we last take, and to what target(s), if any.
	Action   Action
	Target   []PullRequest
	Blockers []blockers.Blocker
	Error    string
}

// Prometheus Metrics
var (
	tideMetrics = struct {
		// Per pool
		pooledPRs  *prometheus.GaugeVec
		updateTime *prometheus.GaugeVec
		merges     *prometheus.HistogramVec
		poolErrors *prometheus.CounterVec

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
}

type manager interface {
	GetClient() ctrlruntimeclient.Client
	GetFieldIndexer() ctrlruntimeclient.FieldIndexer
}

// NewController makes a Controller out of the given clients.
func NewController(ghcSync, ghcStatus github.Client, mgr manager, cfg config.Getter, gc git.ClientFactory, maxRecordsPerPool int, opener io.Opener, historyURI, statusURI string, logger *logrus.Entry, usesGitHubAppsAuth bool) (*Controller, error) {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	hist, err := history.New(maxRecordsPerPool, opener, historyURI)
	if err != nil {
		return nil, fmt.Errorf("error initializing history client from %q: %v", historyURI, err)
	}
	mergeChecker := newMergeChecker(cfg, ghcSync)

	ctx := context.Background()
	sc, err := newStatusController(ctx, logger, ghcStatus, mgr, gc, cfg, opener, statusURI, mergeChecker)
	if err != nil {
		return nil, err
	}
	go sc.run()

	return newSyncController(ctx, logger, ghcSync, mgr, cfg, gc, sc, hist, mergeChecker, usesGitHubAppsAuth)
}

func newStatusController(ctx context.Context, logger *logrus.Entry, ghc githubClient, mgr manager, gc git.ClientFactory, cfg config.Getter, opener io.Opener, statusURI string, mergeChecker *mergeChecker) (*statusController, error) {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &prowapi.ProwJob{}, indexNamePassingJobs, indexFuncPassingJobs); err != nil {
		return nil, fmt.Errorf("failed to add index for passing jobs to cache: %v", err)
	}
	return &statusController{
		pjClient:       mgr.GetClient(),
		logger:         logger.WithField("controller", "status-update"),
		ghc:            ghc,
		gc:             gc,
		config:         cfg,
		mergeChecker:   mergeChecker,
		newPoolPending: make(chan bool, 1),
		shutDown:       make(chan bool),
		opener:         opener,
		path:           statusURI,
	}, nil
}

func newSyncController(
	ctx context.Context,
	logger *logrus.Entry,
	ghcSync githubClient,
	mgr manager,
	cfg config.Getter,
	gc git.ClientFactory,
	sc *statusController,
	hist *history.History,
	mergeChecker *mergeChecker,
	usesGitHubAppsAuth bool,
) (*Controller, error) {
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&prowapi.ProwJob{},
		cacheIndexName,
		cacheIndexFunc,
	); err != nil {
		return nil, fmt.Errorf("failed to add baseSHA index to cache: %v", err)
	}
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&prowapi.ProwJob{},
		nonFailedBatchByNameBaseAndPullsIndexName,
		nonFailedBatchByNameBaseAndPullsIndexFunc,
	); err != nil {
		return nil, fmt.Errorf("failed to add index for non failed batches: %w", err)
	}
	return &Controller{
		ctx:                ctx,
		logger:             logger.WithField("controller", "sync"),
		ghc:                ghcSync,
		prowJobClient:      mgr.GetClient(),
		config:             cfg,
		gc:                 gc,
		usesGitHubAppsAuth: usesGitHubAppsAuth,
		sc:                 sc,
		changedFiles: &changedFilesAgent{
			ghc:             ghcSync,
			nextChangeCache: make(map[changeCacheKey][]string),
		},
		mergeChecker: mergeChecker,
		History:      hist,
	}, nil
}

// Shutdown signals the statusController to stop working and waits for it to
// finish its last update loop before terminating.
// Controller.Sync() should not be used after this function is called.
func (c *Controller) Shutdown() {
	c.History.Flush()
	c.sc.shutdown()
}

func prKey(pr *PullRequest) string {
	return fmt.Sprintf("%s#%d", string(pr.Repository.NameWithOwner), int(pr.Number))
}

// org/repo#number -> pr
func byRepoAndNumber(prs []PullRequest) map[string]PullRequest {
	m := make(map[string]PullRequest)
	for _, pr := range prs {
		key := prKey(&pr)
		m[key] = pr
	}
	return m
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
	return names
}

// Sync runs one sync iteration.
func (c *Controller) Sync() error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.WithField("duration", duration.String()).Info("Synced")
		tideMetrics.syncDuration.Set(duration.Seconds())
		tideMetrics.syncHeartbeat.WithLabelValues("sync").Inc()
		version.GatherProwVersion(c.logger)
	}()
	defer c.changedFiles.prune()
	c.config().BranchProtectionWarnings(c.logger, c.config().PresubmitsStatic)

	c.logger.Debug("Building tide pool.")
	prs, err := c.query()
	if err != nil {
		return fmt.Errorf("failed to query GitHub for prs: %w", err)
	}
	c.logger.WithFields(logrus.Fields{
		"duration":       time.Since(start).String(),
		"found_pr_count": len(prs),
	}).Debug("Found (unfiltered) pool PRs.")

	var blocks blockers.Blockers
	if len(prs) > 0 {
		if label := c.config().Tide.BlockerLabel; label != "" {
			c.logger.Debugf("Searching for blocking issues (label %q).", label)
			orgExcepts, repos := c.config().Tide.Queries.OrgExceptionsAndRepos()
			orgs := make([]string, 0, len(orgExcepts))
			for org := range orgExcepts {
				orgs = append(orgs, org)
			}
			orgRepoQuery := orgRepoQueryString(orgs, repos.UnsortedList(), orgExcepts)
			blocks, err = blockers.FindAll(c.ghc, c.logger, label, orgRepoQuery)
			if err != nil {
				return err
			}
		}
	}
	// Partition PRs into subpools and filter out non-pool PRs.
	rawPools, err := c.dividePool(prs)
	if err != nil {
		return err
	}
	filteredPools := c.filterSubpools(c.mergeChecker.isAllowed, rawPools)

	// Notify statusController about the new pool.
	c.sc.Lock()
	c.sc.blocks = blocks
	c.sc.poolPRs = poolPRMap(filteredPools)
	c.sc.baseSHAs = baseSHAMap(filteredPools)
	c.sc.requiredContexts = requiredContextsMap(filteredPools)
	select {
	case c.sc.newPoolPending <- true:
	default:
	}
	c.sc.Unlock()

	// Sync subpools in parallel.
	poolChan := make(chan Pool, len(filteredPools))
	subpoolsInParallel(
		c.config().Tide.MaxGoroutines,
		filteredPools,
		func(sp *subpool) {
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
	return nil
}

func (c *Controller) query() (map[string]PullRequest, error) {
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	prs := make(map[string]PullRequest)
	var errs []error
	for _, query := range c.config().Tide.Queries {

		// Use org-sharded queries only when GitHub apps auth is in use
		var queries map[string]string
		if c.usesGitHubAppsAuth {
			queries = query.OrgQueries()
		} else {
			queries = map[string]string{"": query.Query()}
		}

		for org, q := range queries {
			org, q := org, q
			wg.Add(1)
			go func() {
				defer wg.Done()
				results, err := search(c.ghc.QueryWithGitHubAppsSupport, c.logger, q, time.Time{}, time.Now(), org)
				lock.Lock()
				defer lock.Unlock()

				if err != nil && len(results) == 0 {
					errs = append(errs, fmt.Errorf("query %q, err: %v", q, err))
					return
				}
				if err != nil {
					c.logger.WithError(err).WithField("query", q).Warning("found partial results")
				}

				for _, pr := range results {
					prs[prKey(&pr)] = pr
				}
			}()
		}
	}
	wg.Wait()

	return prs, utilerrors.NewAggregate(errs)
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
func (c *Controller) filterSubpools(mergeAllowed func(*PullRequest) (string, error), raw map[string]*subpool) map[string]*subpool {
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
			if spFiltered := filterSubpool(c.ghc, mergeAllowed, sp); spFiltered != nil {
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

func (c *Controller) initSubpoolData(sp *subpool) error {
	var err error
	sp.presubmits, err = c.presubmitsByPull(sp)
	if err != nil {
		return fmt.Errorf("error determining required presubmit prowjobs: %v", err)
	}
	sp.cc = make(map[int]contextChecker, len(sp.prs))
	for _, pr := range sp.prs {
		sp.cc[int(pr.Number)], err = c.config().GetTideContextPolicy(c.gc, sp.org, sp.repo, sp.branch, refGetterFactory(string(sp.sha)), string(pr.HeadRefOID))
		if err != nil {
			return fmt.Errorf("error setting up context checker for pr %d: %v", int(pr.Number), err)
		}
	}
	return nil
}

// filterSubpool filters PRs from an initially identified subpool, returning the
// filtered subpool.
// If the subpool becomes empty 'nil' is returned to indicate that the subpool
// should be deleted.
func filterSubpool(ghc githubClient, mergeAllowed func(*PullRequest) (string, error), sp *subpool) *subpool {
	var toKeep []PullRequest
	for _, pr := range sp.prs {
		if !filterPR(ghc, mergeAllowed, sp, &pr) {
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
// - Have known merge conflicts or invalid merge method.
// - Have failing or missing status contexts.
// - Have pending required status contexts that are not associated with a
//   ProwJob. (This ensures that the 'tide' context indicates that the pending
//   status is preventing merge. Required ProwJob statuses are allowed to be
//   'pending' because this prevents kicking PRs from the pool when Tide is
//   retesting them.)
func filterPR(ghc githubClient, mergeAllowed func(*PullRequest) (string, error), sp *subpool, pr *PullRequest) bool {
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
	contexts, err := headContexts(log, ghc, pr)
	if err != nil {
		log.WithError(err).Error("Getting head contexts.")
		return true
	}
	presubmitsHaveContext := func(context string) bool {
		for _, job := range sp.presubmits[int(pr.Number)] {
			if job.Context == context {
				return true
			}
		}
		return false
	}
	for _, ctx := range unsuccessfulContexts(contexts, sp.cc[int(pr.Number)], log) {
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

// mergeChecker provides a function to check if a PR can be merged with
// the requested method and does not have a merge conflict.
// It caches results and should be cleared periodically with clearCache()
type mergeChecker struct {
	config config.Getter
	ghc    githubClient

	sync.Mutex
	cache map[config.OrgRepo]map[github.PullRequestMergeType]bool
}

func newMergeChecker(cfg config.Getter, ghc githubClient) *mergeChecker {
	m := &mergeChecker{
		config: cfg,
		ghc:    ghc,
		cache:  map[config.OrgRepo]map[github.PullRequestMergeType]bool{},
	}

	go m.clearCache()
	return m
}

func (m *mergeChecker) clearCache() {
	// Only do this once per token reset since it could be a bit expensive for
	// Tide instances that handle hundreds of repos.
	ticker := time.NewTicker(time.Hour)
	for {
		<-ticker.C
		m.Lock()
		m.cache = make(map[config.OrgRepo]map[github.PullRequestMergeType]bool)
		m.Unlock()
	}
}

func (m *mergeChecker) repoMethods(orgRepo config.OrgRepo) (map[github.PullRequestMergeType]bool, error) {
	m.Lock()
	defer m.Unlock()

	repoMethods, ok := m.cache[orgRepo]
	if !ok {
		fullRepo, err := m.ghc.GetRepo(orgRepo.Org, orgRepo.Repo)
		if err != nil {
			return nil, err
		}
		repoMethods = map[github.PullRequestMergeType]bool{
			github.MergeMerge:  fullRepo.AllowMergeCommit,
			github.MergeSquash: fullRepo.AllowSquashMerge,
			github.MergeRebase: fullRepo.AllowRebaseMerge,
		}
		m.cache[orgRepo] = repoMethods
	}
	return repoMethods, nil
}

// isAllowed checks if a PR does not have merge conflicts and requests an
// allowed merge method. If there is no error it returns a string explanation if
// not allowed or "" if allowed.
func (m *mergeChecker) isAllowed(pr *PullRequest) (string, error) {
	if pr.Mergeable == githubql.MergeableStateConflicting {
		return "PR has a merge conflict.", nil
	}
	mergeMethod, err := prMergeMethod(m.config().Tide, pr)
	if err != nil {
		// This should be impossible.
		return "", fmt.Errorf("Programmer error! Failed to determine a merge method: %v", err)
	}
	orgRepo := config.OrgRepo{Org: string(pr.Repository.Owner.Login), Repo: string(pr.Repository.Name)}
	repoMethods, err := m.repoMethods(orgRepo)
	if err != nil {
		return "", fmt.Errorf("error getting repo data: %v", err)
	}
	if allowed, exists := repoMethods[mergeMethod]; !exists {
		// Should be impossible as well.
		return "", fmt.Errorf("Programmer error! PR requested the unrecognized merge type %q", mergeMethod)
	} else if !allowed {
		return fmt.Sprintf("Merge type %q disallowed by repo settings", mergeMethod), nil
	}
	return "", nil
}

func baseSHAMap(subpoolMap map[string]*subpool) map[string]string {
	baseSHAs := make(map[string]string, len(subpoolMap))
	for key, sp := range subpoolMap {
		baseSHAs[key] = sp.sha
	}
	return baseSHAs
}

// poolPRMap collects all subpool PRs into a map containing all pooled PRs.
func poolPRMap(subpoolMap map[string]*subpool) map[string]PullRequest {
	prs := make(map[string]PullRequest)
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
			requiredContextsSet := sets.String{}
			for _, requiredJob := range sp.presubmits[int(pr.Number)] {
				requiredContextsSet.Insert(requiredJob.Context)
			}
			requiredContextsMap[prKey(&pr)] = requiredContextsSet.List()
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
func isPassingTests(log *logrus.Entry, ghc githubClient, pr PullRequest, cc contextChecker) bool {
	log = log.WithFields(pr.logFields())
	contexts, err := headContexts(log, ghc, &pr)
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

	log.Debugf("from %d total contexts (%v) found %d failing contexts: %v", len(contexts), contextsToStrings(contexts), len(failed), contextsToStrings(failed))
	return failed
}

func hasAllLabels(pr PullRequest, labels []string) bool {
	if len(labels) == 0 {
		return true
	}
	prLabels := sets.NewString()
	for _, l := range pr.Labels.Nodes {
		prLabels.Insert(string(l.Name))
	}
	requiredLabels := sets.NewString(labels...)
	return prLabels.Intersection(requiredLabels).Equal(requiredLabels)
}

func pickHighestPriorityPR(log *logrus.Entry, ghc githubClient, prs []PullRequest, cc map[int]contextChecker, isPassingTestsFunc func(*logrus.Entry, githubClient, PullRequest, contextChecker) bool, priorities []config.TidePriority) (bool, PullRequest) {
	smallestNumber := -1
	var smallestPR PullRequest
	for _, p := range append(priorities, config.TidePriority{}) {
		for _, pr := range prs {
			if !hasAllLabels(pr, p.Labels) {
				continue
			}
			if smallestNumber != -1 && int(pr.Number) >= smallestNumber {
				continue
			}
			if len(pr.Commits.Nodes) < 1 {
				continue
			}
			if !isPassingTestsFunc(log, ghc, pr, cc[int(pr.Number)]) {
				continue
			}
			smallestNumber = int(pr.Number)
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
// * A list of PRs that are part of a batch test that hasn't finished yet but didn't have any failures so far
func (c *Controller) accumulateBatch(sp subpool) (successBatch []PullRequest, pendingBatch []PullRequest) {
	sp.log.Debug("accumulating PRs for batch testing")
	prNums := make(map[int]PullRequest)
	for _, pr := range sp.prs {
		prNums[int(pr.Number)] = pr
	}
	type accState struct {
		prs       []PullRequest
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
				if pr, ok := prNums[pull.Number]; ok && string(pr.HeadRefOID) == pull.SHA {
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
		if s, ok := states[ref].jobStates[context]; !ok || s == failureState || jobState == successState {
			states[ref].jobStates[context] = jobState
		}
	}
	for ref, state := range states {
		if !state.validPulls {
			continue
		}

		requiredPresubmits, err := c.presubmitsForBatch(state.prs, sp.org, sp.repo, sp.sha, sp.branch)
		if err != nil {
			sp.log.WithError(err).Error("Error getting presubmits for batch")
			continue
		}

		overallState := successState
		for _, p := range requiredPresubmits {
			if s, ok := state.jobStates[p.Context]; !ok || s == failureState {
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

// accumulate returns the supplied PRs sorted into three buckets based on their
// accumulated state across the presubmits.
func accumulate(presubmits map[int][]config.Presubmit, prs []PullRequest, pjs []prowapi.ProwJob, log *logrus.Entry) (successes, pendings, missings []PullRequest, missingTests map[int][]config.Presubmit) {

	missingTests = map[int][]config.Presubmit{}
	for _, pr := range prs {
		// Accumulate the best result for each job (Passing > Pending > Failing/Unknown)
		// We can ignore the baseSHA here because the subPool only contains ProwJobs with the correct baseSHA
		psStates := make(map[string]simpleState)
		for _, pj := range pjs {
			if pj.Spec.Type != prowapi.PresubmitJob {
				continue
			}
			if pj.Spec.Refs.Pulls[0].Number != int(pr.Number) {
				continue
			}
			if pj.Spec.Refs.Pulls[0].SHA != string(pr.HeadRefOID) {
				continue
			}

			name := pj.Spec.Context
			oldState := psStates[name]
			newState := toSimpleState(pj.Status.State)
			if oldState == failureState || oldState == "" {
				psStates[name] = newState
			} else if oldState == pendingState && newState == successState {
				psStates[name] = successState
			}
		}
		// The overall result for the PR is the worst of the best of all its
		// required Presubmits
		overallState := successState
		for _, ps := range presubmits[int(pr.Number)] {
			if s, ok := psStates[ps.Context]; !ok {
				// No PJ with correct baseSHA+headSHA exists
				missingTests[int(pr.Number)] = append(missingTests[int(pr.Number)], ps)
				log.WithFields(pr.logFields()).Debugf("missing presubmit %s", ps.Context)
			} else if s == failureState {
				// PJ with correct baseSHA+headSHA exists but failed
				missingTests[int(pr.Number)] = append(missingTests[int(pr.Number)], ps)
				log.WithFields(pr.logFields()).Debugf("presubmit %s not passing", ps.Context)
			} else if s == pendingState {
				log.WithFields(pr.logFields()).Debugf("presubmit %s pending", ps.Context)
				overallState = pendingState
			}
		}
		if len(missingTests[int(pr.Number)]) > 0 {
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

func prNumbers(prs []PullRequest) []int {
	var nums []int
	for _, pr := range prs {
		nums = append(nums, int(pr.Number))
	}
	return nums
}

func (c *Controller) pickBatch(sp subpool, cc map[int]contextChecker) ([]PullRequest, []config.Presubmit, error) {
	batchLimit := c.config().Tide.BatchSizeLimit(config.OrgRepo{Org: sp.org, Repo: sp.repo})
	if batchLimit < 0 {
		sp.log.Debug("Batch merges disabled by configuration in this repo.")
		return nil, nil, nil
	}

	// we must choose the oldest PRs for the batch
	sort.Slice(sp.prs, func(i, j int) bool { return sp.prs[i].Number < sp.prs[j].Number })

	var candidates []PullRequest
	for _, pr := range sp.prs {
		if isPassingTests(sp.log, c.ghc, pr, cc[int(pr.Number)]) {
			candidates = append(candidates, pr)
		}
	}

	if len(candidates) == 0 {
		sp.log.Debugf("of %d possible PRs, none were passing tests, no batch will be created", len(sp.prs))
		return nil, nil, nil
	}
	sp.log.Debugf("of %d possible PRs, %d are passing tests", len(sp.prs), len(candidates))

	r, err := c.gc.ClientFor(sp.org, sp.repo)
	if err != nil {
		return nil, nil, err
	}
	defer r.Clean()
	if err := r.Config("user.name", "prow"); err != nil {
		return nil, nil, err
	}
	if err := r.Config("user.email", "prow@localhost"); err != nil {
		return nil, nil, err
	}
	if err := r.Config("commit.gpgsign", "false"); err != nil {
		sp.log.Warningf("Cannot set gpgsign=false in gitconfig: %v", err)
	}
	if err := r.Checkout(sp.sha); err != nil {
		return nil, nil, err
	}

	var res []PullRequest
	for _, pr := range candidates {
		if ok, err := r.Merge(string(pr.HeadRefOID)); err != nil {
			// we failed to abort the merge and our git client is
			// in a bad state; it must be cleaned before we try again
			return nil, nil, err
		} else if ok {
			res = append(res, pr)
			// TODO: Make this configurable per subpool.
			if batchLimit > 0 && len(res) >= batchLimit {
				break
			}
		}
	}

	presubmits, err := c.presubmitsForBatch(res, sp.org, sp.repo, sp.sha, sp.branch)
	if err != nil {
		return nil, nil, err
	}

	return res, presubmits, nil
}

func (c *Controller) prepareMergeDetails(commitTemplates config.TideMergeCommitTemplate, pr PullRequest, mergeMethod github.PullRequestMergeType) github.MergeDetails {
	ghMergeDetails := github.MergeDetails{
		SHA:         string(pr.HeadRefOID),
		MergeMethod: string(mergeMethod),
	}

	if commitTemplates.Title != nil {
		var b bytes.Buffer

		if err := commitTemplates.Title.Execute(&b, pr); err != nil {
			c.logger.Errorf("error executing commit title template: %v", err)
		} else {
			ghMergeDetails.CommitTitle = b.String()
		}
	}

	if commitTemplates.Body != nil {
		var b bytes.Buffer

		if err := commitTemplates.Body.Execute(&b, pr); err != nil {
			c.logger.Errorf("error executing commit body template: %v", err)
		} else {
			ghMergeDetails.CommitMessage = b.String()
		}
	}

	return ghMergeDetails
}

func prMergeMethod(c config.Tide, pr *PullRequest) (github.PullRequestMergeType, error) {
	repo := config.OrgRepo{Org: string(pr.Repository.Owner.Login), Repo: string(pr.Repository.Name)}
	method := c.MergeMethod(repo)
	squashLabel := c.SquashLabel
	rebaseLabel := c.RebaseLabel
	mergeLabel := c.MergeLabel
	if squashLabel != "" || rebaseLabel != "" || mergeLabel != "" {
		labelCount := 0
		for _, prlabel := range pr.Labels.Nodes {
			switch string(prlabel.Name) {
			case "":
				continue
			case squashLabel:
				method = github.MergeSquash
				labelCount++
			case rebaseLabel:
				method = github.MergeRebase
				labelCount++
			case mergeLabel:
				method = github.MergeMerge
				labelCount++
			}
			if labelCount > 1 {
				return "", fmt.Errorf("conflicting merge method override labels")
			}
		}
	}
	return method, nil
}

func (c *Controller) mergePRs(sp subpool, prs []PullRequest) error {
	var merged, failed []int
	defer func() {
		if len(merged) == 0 {
			return
		}
		tideMetrics.merges.WithLabelValues(sp.org, sp.repo, sp.branch).Observe(float64(len(merged)))
	}()

	var errs []error
	log := sp.log.WithField("merge-targets", prNumbers(prs))
	tideConfig := c.config().Tide
	for i, pr := range prs {
		log := log.WithFields(pr.logFields())
		mergeMethod, err := prMergeMethod(tideConfig, &pr)
		if err != nil {
			log.WithError(err).Error("Failed to determine merge method.")
			errs = append(errs, err)
			failed = append(failed, int(pr.Number))
			continue
		}

		commitTemplates := tideConfig.MergeCommitTemplate(config.OrgRepo{Org: sp.org, Repo: sp.repo})
		keepTrying, err := tryMerge(func() error {
			ghMergeDetails := c.prepareMergeDetails(commitTemplates, pr, mergeMethod)
			return c.ghc.Merge(sp.org, sp.repo, int(pr.Number), ghMergeDetails)
		})
		if err != nil {
			// These are user errors, shouldn't be printed as tide errors
			log.WithError(err).Debug("Merge failed.")
		} else {
			log.Info("Merged.")
			merged = append(merged, int(pr.Number))
		}
		if !keepTrying {
			break
		}
		// If we successfully merged this PR and have more to merge, sleep to give
		// GitHub time to recalculate mergeability.
		if err == nil && i+1 < len(prs) {
			sleep(time.Second * 5)
		}
	}

	if len(errs) == 0 {
		return nil
	}

	// Construct a more informative error.
	var batch string
	if len(prs) > 1 {
		batch = fmt.Sprintf(" from batch %v", prNumbers(prs))
		if len(merged) > 0 {
			batch = fmt.Sprintf("%s, partial merge %v", batch, merged)
		}
	}
	return fmt.Errorf("failed merging %v%s: %v", failed, batch, utilerrors.NewAggregate(errs))
}

// tryMerge attempts 1 merge and returns a bool indicating if we should try
// to merge the remaining PRs and possibly an error.
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
			return true, fmt.Errorf("PR was modified: %v", err)
		} else if _, ok = err.(github.UnmergablePRBaseChangedError); ok {
			//  complained that the base branch was modified. This is a
			// strange error because the API doesn't even allow the request to
			// specify the base branch sha, only the head sha.
			// We suspect that github is complaining because we are making the
			// merge requests too rapidly and it cannot recompute mergability
			// in time. https://github.com/kubernetes/test-infra/issues/5171
			// We handle this by sleeping for a few seconds before trying to
			// merge again.
			err = fmt.Errorf("base branch was modified: %v", err)
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
			return false, fmt.Errorf("branch needs to be configured to allow this robot to push: %v", err)
		} else if _, ok = err.(github.MergeCommitsForbiddenError); ok {
			// GitHub let us know that the merge method configured for this repo
			// is not allowed by other repo settings, so we should let the admins
			// know that the configuration needs to be updated.
			// We won't be able to merge the other PRs.
			return false, fmt.Errorf("Tide needs to be configured to use the 'rebase' merge method for this repo or the repo needs to allow merge commits: %v", err)
		} else if _, ok = err.(github.UnmergablePRError); ok {
			return true, fmt.Errorf("PR is unmergable. Do the Tide merge requirements match the GitHub settings for the repo? %v", err)
		} else {
			return true, err
		}
	}
	// We ran out of retries. Return the last transient error.
	return true, err
}

func (c *Controller) trigger(sp subpool, presubmits []config.Presubmit, prs []PullRequest) error {
	refs := prowapi.Refs{
		Org:     sp.org,
		Repo:    sp.repo,
		BaseRef: sp.branch,
		BaseSHA: sp.sha,
	}
	for _, pr := range prs {
		refs.Pulls = append(
			refs.Pulls,
			prowapi.Pull{
				Number: int(pr.Number),
				Author: string(pr.Author.Login),
				SHA:    string(pr.HeadRefOID),
			},
		)
	}

	// If PRs require the same job, we only want to trigger it once.
	// If multiple required jobs have the same context, we assume the
	// same shard will be run to provide those contexts
	triggeredContexts := sets.NewString()
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
		pj := pjutil.NewProwJob(spec, ps.Labels, ps.Annotations)
		pj.Namespace = c.config().ProwJobNamespace
		log := c.logger.WithFields(pjutil.ProwJobFields(&pj))
		start := time.Now()
		if err := c.prowJobClient.Create(c.ctx, &pj); err != nil {
			log.WithField("duration", time.Since(start).String()).Debug("Failed to create ProwJob on the cluster.")
			return fmt.Errorf("failed to create a ProwJob for job: %q, PRs: %v: %v", spec.Job, prNumbers(prs), err)
		}
		log.WithField("duration", time.Since(start).String()).Debug("Created ProwJob on the cluster.")
	}
	return nil
}

func (c *Controller) nonFailedBatchForJobAndRefsExists(jobName string, refs *prowapi.Refs) bool {
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

func (c *Controller) takeAction(sp subpool, batchPending, successes, pendings, missings, batchMerges []PullRequest, missingSerialTests map[int][]config.Presubmit) (Action, []PullRequest, error) {
	// Merge the batch!
	if len(batchMerges) > 0 {
		return MergeBatch, batchMerges, c.mergePRs(sp, batchMerges)
	}
	// Do not merge PRs while waiting for a batch to complete. We don't want to
	// invalidate the old batch result.
	if len(successes) > 0 && len(batchPending) == 0 {
		if ok, pr := pickHighestPriorityPR(sp.log, c.ghc, successes, sp.cc, isPassingTests, c.config().Tide.Priority); ok {
			return Merge, []PullRequest{pr}, c.mergePRs(sp, []PullRequest{pr})
		}
	}
	// If no presubmits are configured, just wait.
	if len(sp.presubmits) == 0 {
		return Wait, nil, nil
	}
	// If we have no batch, trigger one.
	if len(sp.prs) > 1 && len(batchPending) == 0 {
		batch, presubmits, err := c.pickBatch(sp, sp.cc)
		if err != nil {
			return Wait, nil, err
		}
		if len(batch) > 1 {
			return TriggerBatch, batch, c.trigger(sp, presubmits, batch)
		}
	}
	// If we have no serial jobs pending or successful, trigger one.
	if len(missings) > 0 && len(pendings) == 0 && len(successes) == 0 {
		if ok, pr := pickHighestPriorityPR(sp.log, c.ghc, missings, sp.cc, isPassingTests, c.config().Tide.Priority); ok {
			return Trigger, []PullRequest{pr}, c.trigger(sp, missingSerialTests[int(pr.Number)], []PullRequest{pr})
		}
	}
	return Wait, nil, nil
}

// changedFilesAgent queries and caches the names of files changed by PRs.
// Cache entries expire if they are not used during a sync loop.
type changedFilesAgent struct {
	ghc         githubClient
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
func (c *changedFilesAgent) prChanges(pr *PullRequest) config.ChangedFilesProvider {
	return func() ([]string, error) {
		cacheKey := changeCacheKey{
			org:    string(pr.Repository.Owner.Login),
			repo:   string(pr.Repository.Name),
			number: int(pr.Number),
			sha:    string(pr.HeadRefOID),
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
		changes, err := c.ghc.GetPullRequestChanges(
			string(pr.Repository.Owner.Login),
			string(pr.Repository.Name),
			int(pr.Number),
		)
		if err != nil {
			return nil, fmt.Errorf("error getting PR changes for #%d: %v", int(pr.Number), err)
		}
		changedFiles = make([]string, 0, len(changes))
		for _, change := range changes {
			changedFiles = append(changedFiles, change.Filename)
		}

		c.Lock()
		c.nextChangeCache[cacheKey] = changedFiles
		c.Unlock()
		return changedFiles, nil
	}
}

func (c *changedFilesAgent) batchChanges(prs []PullRequest) config.ChangedFilesProvider {
	return func() ([]string, error) {
		result := sets.String{}
		for _, pr := range prs {
			changes, err := c.prChanges(&pr)()
			if err != nil {
				return nil, err
			}

			result.Insert(changes...)
		}

		return result.List(), nil
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

func (c *Controller) presubmitsByPull(sp *subpool) (map[int][]config.Presubmit, error) {
	presubmits := make(map[int][]config.Presubmit, len(sp.prs))
	record := func(num int, job config.Presubmit) {
		if jobs, ok := presubmits[num]; ok {
			presubmits[num] = append(jobs, job)
		} else {
			presubmits[num] = []config.Presubmit{job}
		}
	}

	// filtered PRs contains all PRs for which we were able to get the presubmits
	var filteredPRs []PullRequest

	for _, pr := range sp.prs {
		log := c.logger.WithField("base-sha", sp.sha).WithFields(pr.logFields())
		presubmitsForPull, err := c.config().GetPresubmits(c.gc, sp.org+"/"+sp.repo, refGetterFactory(sp.sha), refGetterFactory(string(pr.HeadRefOID)))
		if err != nil {
			c.logger.WithError(err).Debug("Failed to get presubmits for PR, excluding from subpool")
			continue
		}
		filteredPRs = append(filteredPRs, pr)
		log.Debugf("Found %d possible presubmits", len(presubmitsForPull))

		for _, ps := range presubmitsForPull {
			if !ps.ContextRequired() {
				continue
			}

			shouldRun, err := ps.ShouldRun(sp.branch, c.changedFiles.prChanges(&pr), false, false)
			if err != nil {
				return nil, err
			}
			if !shouldRun {
				log.WithField("context", ps.Context).Debug("Presubmit excluded by ps.ShouldRun")
				continue
			}

			record(int(pr.Number), ps)
		}
	}

	sp.prs = filteredPRs
	return presubmits, nil
}

func (c *Controller) presubmitsForBatch(prs []PullRequest, org, repo, baseSHA, baseBranch string) ([]config.Presubmit, error) {
	log := c.logger.WithFields(logrus.Fields{"repo": repo, "org": org, "base-sha": baseSHA, "base-branch": baseBranch})

	var headRefGetters []config.RefGetter
	for _, pr := range prs {
		headRefGetters = append(headRefGetters, refGetterFactory(string(pr.HeadRefOID)))
	}

	presubmits, err := c.config().GetPresubmits(c.gc, org+"/"+repo, refGetterFactory(baseSHA), headRefGetters...)
	if err != nil {
		return nil, fmt.Errorf("failed to get presubmits for batch: %v", err)
	}
	log.Debugf("Found %d possible presubmits for batch", len(presubmits))

	var result []config.Presubmit
	for _, ps := range presubmits {
		if !ps.ContextRequired() {
			continue
		}

		shouldRun, err := ps.ShouldRun(baseBranch, c.changedFiles.batchChanges(prs), false, false)
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

func (c *Controller) syncSubpool(sp subpool, blocks []blockers.Blocker) (Pool, error) {
	sp.log.Infof("Syncing subpool: %d PRs, %d PJs.", len(sp.prs), len(sp.pjs))
	successes, pendings, missings, missingSerialTests := accumulate(sp.presubmits, sp.prs, sp.pjs, sp.log)
	batchMerge, batchPending := c.accumulateBatch(sp)
	sp.log.WithFields(logrus.Fields{
		"prs-passing":   prNumbers(successes),
		"prs-pending":   prNumbers(pendings),
		"prs-missing":   prNumbers(missings),
		"batch-passing": prNumbers(batchMerge),
		"batch-pending": prNumbers(batchPending),
	}).Info("Subpool accumulated.")

	var act Action
	var targets []PullRequest
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
		},
		err
}

func prMeta(prs ...PullRequest) []prowapi.Pull {
	var res []prowapi.Pull
	for _, pr := range prs {
		res = append(res, prowapi.Pull{
			Number: int(pr.Number),
			Author: string(pr.Author.Login),
			Title:  string(pr.Title),
			SHA:    string(pr.HeadRefOID),
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

	sortPRs := func(prs []PullRequest) {
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
	log    *logrus.Entry
	org    string
	repo   string
	branch string
	// sha is the baseSHA for this subpool
	sha string

	// pjs contains all ProwJobs of type Presubmit or Batch
	// that have the same baseSHA as the subpool
	pjs []prowapi.ProwJob
	prs []PullRequest

	cc map[int]contextChecker
	// presubmit contains all required presubmits for each PR
	// in this subpool
	presubmits map[int][]config.Presubmit
}

func poolKey(org, repo, branch string) string {
	return fmt.Sprintf("%s/%s:%s", org, repo, branch)
}

// dividePool splits up the list of pull requests and prow jobs into a group
// per repo and branch. It only keeps ProwJobs that match the latest branch.
func (c *Controller) dividePool(pool map[string]PullRequest) (map[string]*subpool, error) {
	sps := make(map[string]*subpool)
	for _, pr := range pool {
		org := string(pr.Repository.Owner.Login)
		repo := string(pr.Repository.Name)
		branch := string(pr.BaseRef.Name)
		branchRef := string(pr.BaseRef.Prefix) + string(pr.BaseRef.Name)
		fn := poolKey(org, repo, branch)
		if sps[fn] == nil {
			sha, err := c.ghc.GetRef(org, repo, strings.TrimPrefix(branchRef, "refs/"))
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
			return nil, fmt.Errorf("failed to list jobs for subpool %s: %v", subpoolkey, err)
		}
		c.logger.WithField("subpool", subpoolkey).Debugf("Found %d prowjobs.", len(pjs.Items))
		sps[subpoolkey].pjs = pjs.Items
	}
	return sps, nil
}

// PullRequest holds graphql data about a PR, including its commits and their contexts.
type PullRequest struct {
	Number githubql.Int
	Author struct {
		Login githubql.String
	}
	BaseRef struct {
		Name   githubql.String
		Prefix githubql.String
	}
	HeadRefName githubql.String `graphql:"headRefName"`
	HeadRefOID  githubql.String `graphql:"headRefOid"`
	Mergeable   githubql.MergeableState
	Repository  struct {
		Name          githubql.String
		NameWithOwner githubql.String
		Owner         struct {
			Login githubql.String
		}
	}
	Commits struct {
		Nodes []struct {
			Commit Commit
		}
		// Request the 'last' 4 commits hoping that one of them is the logically 'last'
		// commit with OID matching HeadRefOID. If we don't find it we have to use an
		// additional API token. (see the 'headContexts' func for details)
		// We can't raise this too much or we could hit the limit of 50,000 nodes
		// per query: https://developer.github.com/v4/guides/resource-limitations/#node-limit
	} `graphql:"commits(last: 4)"`
	Labels struct {
		Nodes []struct {
			Name githubql.String
		}
	} `graphql:"labels(first: 100)"`
	Milestone *struct {
		Title githubql.String
	}
	Body      githubql.String
	Title     githubql.String
	UpdatedAt githubql.DateTime
}

// Commit holds graphql data about commits and which contexts they have
type Commit struct {
	Status struct {
		Contexts []Context
	}
	OID               githubql.String `graphql:"oid"`
	Message           githubql.String
	MessageBody       githubql.String
	StatusCheckRollup StatusCheckRollup
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
	Context     githubql.String
	Description githubql.String
	State       githubql.StatusState
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

func (pr *PullRequest) logFields() logrus.Fields {
	return logrus.Fields{
		"org":    string(pr.Repository.Owner.Login),
		"repo":   string(pr.Repository.Name),
		"pr":     int(pr.Number),
		"branch": string(pr.BaseRef.Name),
		"sha":    string(pr.HeadRefOID),
	}
}

// headContexts gets the status contexts for the commit with OID == pr.HeadRefOID
//
// First, we try to get this value from the commits we got with the PR query.
// Unfortunately the 'last' commit ordering is determined by author date
// not commit date so if commits are reordered non-chronologically on the PR
// branch the 'last' commit isn't necessarily the logically last commit.
// We list multiple commits with the query to increase our chance of success,
// but if we don't find the head commit we have to ask GitHub for it
// specifically (this costs an API token).
func headContexts(log *logrus.Entry, ghc githubClient, pr *PullRequest) ([]Context, error) {
	for _, node := range pr.Commits.Nodes {
		if node.Commit.OID == pr.HeadRefOID {
			return append(node.Commit.Status.Contexts, checkRunNodesToContexts(log, node.Commit.StatusCheckRollup.Contexts.Nodes)...), nil
		}
	}
	// We didn't get the head commit from the query (the commits must not be
	// logically ordered) so we need to specifically ask GitHub for the status
	// and coerce it to a graphql type.
	org := string(pr.Repository.Owner.Login)
	repo := string(pr.Repository.Name)
	// Log this event so we can tune the number of commits we list to minimize this.
	// TODO alvaroaleman: Add checkrun support here. Doesn't seem to happen often though,
	// openshift doesn't have a single occurrence of this in the past seven days.
	log.Warnf("'last' %d commits didn't contain logical last commit. Querying GitHub...", len(pr.Commits.Nodes))
	combined, err := ghc.GetCombinedStatus(org, repo, string(pr.HeadRefOID))
	if err != nil {
		return nil, fmt.Errorf("failed to get the combined status: %v", err)
	}
	checkRunList, err := ghc.ListCheckRuns(org, repo, string(pr.HeadRefOID))
	if err != nil {
		return nil, fmt.Errorf("Failed to list checkruns: %w", err)
	}
	checkRunNodes := make([]CheckRunNode, 0, len(checkRunList.CheckRuns))
	for _, checkRun := range checkRunList.CheckRuns {
		checkRunNodes = append(checkRunNodes, CheckRunNode{CheckRun: CheckRun{
			Name: githubql.String(checkRun.Name),
			// They are uppercase in the V4 api and lowercase in the V3 api
			Conclusion: githubql.String(strings.ToUpper(checkRun.Conclusion)),
			Status:     githubql.String(strings.ToUpper(checkRun.Status)),
		}})
	}

	contexts := make([]Context, 0, len(combined.Statuses)+len(checkRunNodes))
	for _, status := range combined.Statuses {
		contexts = append(contexts, Context{
			Context:     githubql.String(status.Context),
			Description: githubql.String(status.Description),
			State:       githubql.StatusState(strings.ToUpper(status.State)),
		})
	}
	contexts = append(contexts, checkRunNodesToContexts(log, checkRunNodes)...)

	// Add a commit with these contexts to pr for future look ups.
	pr.Commits.Nodes = append(pr.Commits.Nodes,
		struct{ Commit Commit }{
			Commit: Commit{
				OID:    pr.HeadRefOID,
				Status: struct{ Contexts []Context }{Contexts: contexts},
			},
		},
	)
	return contexts, nil
}

func orgRepoQueryString(orgs, repos []string, orgExceptions map[string]sets.String) string {
	toks := make([]string, 0, len(orgs))
	for _, o := range orgs {
		toks = append(toks, fmt.Sprintf("org:\"%s\"", o))

		for _, e := range orgExceptions[o].List() {
			toks = append(toks, fmt.Sprintf("-repo:\"%s\"", e))
		}
	}
	for _, r := range repos {
		toks = append(toks, fmt.Sprintf("repo:\"%s\"", r))
	}
	return strings.Join(toks, " ")
}

// cacheIndexName is the name of the index that indexes presubmit+batch ProwJobs by
// org+repo+branch+baseSHA. Use the cacheIndexKey func to get the correct key.
const cacheIndexName = "tide-global-index"

// cacheIndexKey returns the index key for the tideCacheIndex
func cacheIndexKey(org, repo, branch, baseSHA string) string {
	return fmt.Sprintf("%s/%s:%s@%s", org, repo, branch, baseSHA)
}

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

const nonFailedBatchByNameBaseAndPullsIndexName = "tide-non-failed-jobs-by-name-base-and-pulls"

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
// contexts that have multiple entries
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

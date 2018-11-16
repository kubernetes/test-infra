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
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/tide/blockers"
)

type kubeClient interface {
	ListProwJobs(string) ([]kube.ProwJob, error)
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

type githubClient interface {
	CreateStatus(string, string, string, github.Status) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetRef(string, string, string) (string, error)
	Merge(string, string, int, github.MergeDetails) error
	Query(context.Context, interface{}, map[string]interface{}) error
}

type contextChecker interface {
	// IsOptional tells whether a context is optional.
	IsOptional(string) bool
	// MissingRequiredContexts tells if required contexts are missing from the list of contexts provided.
	MissingRequiredContexts([]string) []string
}

// Controller knows how to sync PRs and PJs.
type Controller struct {
	logger *logrus.Entry
	ca     *config.Agent
	ghc    githubClient
	kc     kubeClient
	gc     *git.Client

	sc *statusController

	m     sync.Mutex
	pools []Pool

	// changedFiles caches the names of files changed by PRs.
	// Cache entries expire if they are not used during a sync loop.
	changedFiles *changedFilesAgent
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
}

// Prometheus Metrics
var (
	tideMetrics = struct {
		// Per pool
		pooledPRs  *prometheus.GaugeVec
		updateTime *prometheus.GaugeVec
		merges     *prometheus.HistogramVec

		// Singleton
		syncDuration         prometheus.Gauge
		statusUpdateDuration prometheus.Gauge
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

		syncDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "syncdur",
			Help: "The duration of the last loop of the sync controller.",
		}),

		statusUpdateDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "statusupdatedur",
			Help: "The duration of the last loop of the status update controller.",
		}),
	}
)

func init() {
	prometheus.MustRegister(tideMetrics.pooledPRs)
	prometheus.MustRegister(tideMetrics.updateTime)
	prometheus.MustRegister(tideMetrics.merges)
	prometheus.MustRegister(tideMetrics.syncDuration)
	prometheus.MustRegister(tideMetrics.statusUpdateDuration)
}

// NewController makes a Controller out of the given clients.
func NewController(ghcSync, ghcStatus *github.Client, kc *kube.Client, ca *config.Agent, gc *git.Client, logger *logrus.Entry) *Controller {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	sc := &statusController{
		logger:         logger.WithField("controller", "status-update"),
		ghc:            ghcStatus,
		ca:             ca,
		newPoolPending: make(chan bool, 1),
		shutDown:       make(chan bool),

		trackedOrgs:  sets.NewString(),
		trackedRepos: sets.NewString(),
	}
	go sc.run()
	return &Controller{
		logger: logger.WithField("controller", "sync"),
		ghc:    ghcSync,
		kc:     kc,
		ca:     ca,
		gc:     gc,
		sc:     sc,
		changedFiles: &changedFilesAgent{
			ghc:             ghcSync,
			nextChangeCache: make(map[changeCacheKey][]string),
		},
	}
}

// Shutdown signals the statusController to stop working and waits for it to
// finish its last update loop before terminating.
// Controller.Sync() should not be used after this function is called.
func (c *Controller) Shutdown() {
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
	}()
	defer c.changedFiles.prune()

	ctx := context.Background()
	c.logger.Debug("Building tide pool.")
	prs := make(map[string]PullRequest)
	for _, q := range c.ca.Config().Tide.Queries {
		results, err := newSearchExecutor(ctx, c.ghc, c.logger, q.Query()).search()
		if err != nil {
			return err
		}
		for _, pr := range results {
			prs[prKey(&pr)] = pr
		}
	}

	var pjs []kube.ProwJob
	var blocks blockers.Blockers
	var err error
	if len(prs) > 0 {
		pjs, err = c.kc.ListProwJobs(kube.EmptySelector)
		if err != nil {
			return err
		}

		if label := c.ca.Config().Tide.BlockerLabel; label != "" {
			c.logger.Debugf("Searching for blocking issues (label %q).", label)
			orgExcepts, repos := c.ca.Config().Tide.Queries.OrgExceptionsAndRepos()
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
	rawPools, err := c.dividePool(prs, pjs)
	if err != nil {
		return err
	}
	filteredPools := c.filterSubpools(c.ca.Config().Tide.MaxGoroutines, rawPools)

	// Notify statusController about the new pool.
	c.sc.Lock()
	c.sc.poolPRs = poolPRMap(filteredPools)
	select {
	case c.sc.newPoolPending <- true:
	default:
	}
	c.sc.Unlock()

	// Sync subpools in parallel.
	poolChan := make(chan Pool, len(filteredPools))
	subpoolsInParallel(
		c.ca.Config().Tide.MaxGoroutines,
		filteredPools,
		func(sp *subpool) {
			spBlocks := blocks.GetApplicable(sp.org, sp.repo, sp.branch)
			if pool, err := c.syncSubpool(*sp, spBlocks); err != nil {
				sp.log.WithError(err).Errorf("Error syncing subpool.")
			} else {
				poolChan <- pool
			}
		},
	)

	close(poolChan)
	pools := make([]Pool, 0, len(poolChan))
	for pool := range poolChan {
		pools = append(pools, pool)
	}
	sortPools(pools)
	c.m.Lock()
	defer c.m.Unlock()
	c.pools = pools
	return nil
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
func (c *Controller) filterSubpools(goroutines int, raw map[string]*subpool) map[string]*subpool {
	filtered := make(map[string]*subpool)
	var lock sync.Mutex

	subpoolsInParallel(
		goroutines,
		raw,
		func(sp *subpool) {
			if err := c.initSubpoolData(sp); err != nil {
				sp.log.WithError(err).Error("Error initializing subpool.")
				return
			}
			key := fmt.Sprintf("%s/%s %s", sp.org, sp.repo, sp.branch)
			if spFiltered := filterSubpool(c.ghc, sp); spFiltered != nil {
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
	sp.presubmitContexts, err = c.presubmitsByPull(sp)
	if err != nil {
		return fmt.Errorf("error determining required presubmit prowjobs: %v", err)
	}
	sp.cc, err = c.ca.Config().GetTideContextPolicy(sp.org, sp.repo, sp.branch)
	if err != nil {
		return fmt.Errorf("error setting up context checker: %v", err)
	}
	return nil
}

// filterSubpool filters PRs from an initially identified subpool, returning the
// filtered subpool.
// If the subpool becomes empty 'nil' is returned to indicate that the subpool
// should be deleted.
func filterSubpool(ghc githubClient, sp *subpool) *subpool {
	var toKeep []PullRequest
	for _, pr := range sp.prs {
		if !filterPR(ghc, sp, &pr) {
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
// - Have known merge conflicts.
// - Have failing or missing status contexts.
// - Have pending required status contexts that are not associated with a
//   ProwJob. (This ensures that the 'tide' context indicates that the pending
//   status is preventing merge. Required ProwJob statuses are allowed to be
//   'pending' because this prevents kicking PRs from the pool when Tide is
//   retesting them.)
func filterPR(ghc githubClient, sp *subpool, pr *PullRequest) bool {
	log := sp.log.WithFields(pr.logFields())
	// Skip PRs that are known to be unmergeable.
	if pr.Mergeable == githubql.MergeableStateConflicting {
		log.Debug("filtering out PR as it is unmergeable")
		return true
	}
	// Filter out PRs with unsuccessful contexts unless the only unsuccessful
	// contexts are pending required prowjobs.
	contexts, err := headContexts(log, ghc, pr)
	if err != nil {
		log.WithError(err).Error("Getting head contexts.")
		return true
	}
	pjContexts := sp.presubmitContexts[int(pr.Number)]
	for _, ctx := range unsuccessfulContexts(contexts, sp.cc, log) {
		if ctx.State != githubql.StatusStatePending || !pjContexts.Has(string(ctx.Context)) {
			log.WithField("context", ctx.Context).Debug("filtering out PR as unsuccessful context is not a pending Prow-controlled context")
			return true
		}
	}

	return false
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

type simpleState string

const (
	noneState    simpleState = "none"
	pendingState simpleState = "pending"
	successState simpleState = "success"
)

func toSimpleState(s kube.ProwJobState) simpleState {
	if s == kube.TriggeredState || s == kube.PendingState {
		return pendingState
	} else if s == kube.SuccessState {
		return successState
	}
	return noneState
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

func pickSmallestPassingNumber(log *logrus.Entry, ghc githubClient, prs []PullRequest, cc contextChecker) (bool, PullRequest) {
	smallestNumber := -1
	var smallestPR PullRequest
	for _, pr := range prs {
		if smallestNumber != -1 && int(pr.Number) >= smallestNumber {
			continue
		}
		if len(pr.Commits.Nodes) < 1 {
			continue
		}
		if !isPassingTests(log, ghc, pr, cc) {
			continue
		}
		smallestNumber = int(pr.Number)
		smallestPR = pr
	}
	return smallestNumber > -1, smallestPR
}

// accumulateBatch returns a list of PRs that can be merged after passing batch
// testing, if any exist. It also returns a list of PRs currently being batch
// tested.
func accumulateBatch(presubmits map[int]sets.String, prs []PullRequest, pjs []kube.ProwJob, log *logrus.Entry) ([]PullRequest, []PullRequest) {
	log.Debug("accumulating PRs for batch testing")
	if len(presubmits) == 0 {
		log.Debug("no presubmits configured, no batch can be triggered")
		return nil, nil
	}
	prNums := make(map[int]PullRequest)
	for _, pr := range prs {
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
	for _, pj := range pjs {
		if pj.Spec.Type != kube.BatchJob {
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
					log.WithField("batch", ref).WithFields(pr.logFields()).Debug("batch job invalid, PR left pool")
					break
				} else {
					state.validPulls = false
					log.WithField("batch", ref).WithFields(pr.logFields()).Debug("batch job invalid, PR HEAD changed")
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
		if s, ok := states[ref].jobStates[context]; !ok || s == noneState || jobState == successState {
			states[ref].jobStates[context] = jobState
		}
	}
	var pendingBatch, successBatch []PullRequest
	for ref, state := range states {
		if !state.validPulls {
			continue
		}
		requiredPresubmits := sets.NewString()
		for _, pr := range state.prs {
			requiredPresubmits = requiredPresubmits.Union(presubmits[int(pr.Number)])
		}
		overallState := successState
		for _, p := range requiredPresubmits.List() {
			if s, ok := state.jobStates[p]; !ok || s == noneState {
				overallState = noneState
				log.WithField("batch", ref).Debugf("batch invalid, required presubmit %s is not passing", p)
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
func accumulate(presubmits map[int]sets.String, prs []PullRequest, pjs []kube.ProwJob, log *logrus.Entry) (successes, pendings, nones []PullRequest) {
	for _, pr := range prs {
		// Accumulate the best result for each job.
		psStates := make(map[string]simpleState)
		for _, pj := range pjs {
			if pj.Spec.Type != kube.PresubmitJob {
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
			if oldState == noneState || oldState == "" {
				psStates[name] = newState
			} else if oldState == pendingState && newState == successState {
				psStates[name] = successState
			}
		}
		// The overall result is the worst of the best.
		overallState := successState
		for _, ps := range presubmits[int(pr.Number)].List() {
			if s, ok := psStates[ps]; !ok {
				overallState = noneState
				log.WithFields(pr.logFields()).Debugf("missing presubmit %s", ps)
				break
			} else if s == noneState {
				overallState = noneState
				log.WithFields(pr.logFields()).Debugf("presubmit %s not passing", ps)
				break
			} else if s == pendingState {
				log.WithFields(pr.logFields()).Debugf("presubmit %s pending", ps)
				overallState = pendingState
			}
		}
		if overallState == successState {
			successes = append(successes, pr)
		} else if overallState == pendingState {
			pendings = append(pendings, pr)
		} else {
			nones = append(nones, pr)
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

func (c *Controller) pickBatch(sp subpool, cc contextChecker) ([]PullRequest, error) {
	// we must choose the oldest PRs for the batch
	sort.Slice(sp.prs, func(i, j int) bool { return sp.prs[i].Number < sp.prs[j].Number })

	var candidates []PullRequest
	for _, pr := range sp.prs {
		if isPassingTests(sp.log, c.ghc, pr, cc) {
			candidates = append(candidates, pr)
		}
	}

	if len(candidates) == 0 {
		sp.log.Debugf("of %d possible PRs, none were passing tests, no batch will be created", len(sp.prs))
		return nil, nil
	}
	sp.log.Debugf("of %d possible PRs, %d are passing tests", len(sp.prs), len(candidates))

	r, err := c.gc.Clone(sp.org + "/" + sp.repo)
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

	var res []PullRequest
	for _, pr := range candidates {
		if ok, err := r.Merge(string(pr.HeadRefOID)); err != nil {
			// we failed to abort the merge and our git client is
			// in a bad state; it must be cleaned before we try again
			return nil, err
		} else if ok {
			res = append(res, pr)
			// TODO: Make this configurable per subpool.
			if len(res) == 5 {
				break
			}
		}
	}
	return res, nil
}

func (c *Controller) mergePRs(sp subpool, prs []PullRequest) error {
	successCount := 0
	defer func() {
		if successCount == 0 {
			return
		}
		tideMetrics.merges.WithLabelValues(sp.org, sp.repo, sp.branch).Observe(float64(successCount))
	}()

	maxRetries := 3
	for i, pr := range prs {
		backoff := time.Second * 4
		log := sp.log.WithFields(pr.logFields())
		mergeMethod := c.ca.Config().Tide.MergeMethod(sp.org, sp.repo)
		if squashLabel := c.ca.Config().Tide.SquashLabel; squashLabel != "" {
			for _, prlabel := range pr.Labels.Nodes {
				if string(prlabel.Name) == squashLabel {
					mergeMethod = github.MergeSquash
					break
				}
			}
		}
		for retry := 0; retry < maxRetries; retry++ {
			if err := c.ghc.Merge(sp.org, sp.repo, int(pr.Number), github.MergeDetails{
				SHA:         string(pr.HeadRefOID),
				MergeMethod: string(mergeMethod),
			}); err != nil {
				if _, ok := err.(github.ModifiedHeadError); ok {
					// This is a possible source of incorrect behavior. If someone
					// modifies their PR as we try to merge it in a batch then we
					// end up in an untested state. This is unlikely to cause any
					// real problems.
					log.WithError(err).Warning("Merge failed: PR was modified.")
					break
				} else if _, ok = err.(github.UnmergablePRBaseChangedError); ok {
					// Github complained that the base branch was modified. This is a
					// strange error because the API doesn't even allow the request to
					// specify the base branch sha, only the head sha.
					// We suspect that github is complaining because we are making the
					// merge requests too rapidly and it cannot recompute mergability
					// in time. https://github.com/kubernetes/test-infra/issues/5171
					// We handle this by sleeping for a few seconds before trying to
					// merge again.
					log.WithError(err).Warning("Merge failed: Base branch was modified.")
					if retry+1 < maxRetries {
						time.Sleep(backoff)
						backoff *= 2
					}
				} else if _, ok = err.(github.UnauthorizedToPushError); ok {
					// Github let us know that the token used cannot push to the branch.
					// Even if the robot is set up to have write access to the repo, an
					// overzealous branch protection setting will not allow the robot to
					// push to a specific branch.
					log.WithError(err).Error("Merge failed: Branch needs to be configured to allow this robot to push.")
					break
				} else if _, ok = err.(github.UnmergablePRError); ok {
					log.WithError(err).Error("Merge failed: PR is unmergable. How did it pass tests?!")
					break
				} else {
					log.WithError(err).Error("Merge failed.")
					return err
				}
			} else {
				log.Info("Merged.")
				successCount++
				// If we have more PRs to merge, sleep to give Github time to recalculate
				// mergeability.
				if i+1 < len(prs) {
					time.Sleep(time.Second * 3)
				}
				break
			}
		}
	}
	return nil
}

func (c *Controller) trigger(sp subpool, presubmitContexts map[int]sets.String, prs []PullRequest) error {
	requiredContexts := sets.NewString()
	for _, pr := range prs {
		requiredContexts = requiredContexts.Union(presubmitContexts[int(pr.Number)])
	}

	// TODO(cjwagner): DRY this out when generalizing triggering code (and code to determine required and to-run jobs).
	for _, ps := range c.ca.Config().Presubmits[sp.org+"/"+sp.repo] {
		if ps.SkipReport || !ps.RunsAgainstBranch(sp.branch) || !requiredContexts.Has(ps.Context) {
			continue
		}

		refs := kube.Refs{
			Org:     sp.org,
			Repo:    sp.repo,
			BaseRef: sp.branch,
			BaseSHA: sp.sha,
		}
		for _, pr := range prs {
			refs.Pulls = append(
				refs.Pulls,
				kube.Pull{
					Number: int(pr.Number),
					Author: string(pr.Author.Login),
					SHA:    string(pr.HeadRefOID),
				},
			)
		}
		var spec kube.ProwJobSpec
		if len(prs) == 1 {
			spec = pjutil.PresubmitSpec(ps, refs)
		} else {
			spec = pjutil.BatchSpec(ps, refs)
		}
		pj := pjutil.NewProwJob(spec, ps.Labels)
		if _, err := c.kc.CreateProwJob(pj); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) takeAction(sp subpool, batchPending, successes, pendings, nones, batchMerges []PullRequest) (Action, []PullRequest, error) {
	// Merge the batch!
	if len(batchMerges) > 0 {
		return MergeBatch, batchMerges, c.mergePRs(sp, batchMerges)
	}
	// Do not merge PRs while waiting for a batch to complete. We don't want to
	// invalidate the old batch result.
	if len(successes) > 0 && len(batchPending) == 0 {
		if ok, pr := pickSmallestPassingNumber(sp.log, c.ghc, successes, sp.cc); ok {
			return Merge, []PullRequest{pr}, c.mergePRs(sp, []PullRequest{pr})
		}
	}
	// If no presubmits are configured, just wait.
	if len(sp.presubmitContexts) == 0 {
		return Wait, nil, nil
	}
	// If we have no serial jobs pending or successful, trigger one.
	if len(nones) > 0 && len(pendings) == 0 && len(successes) == 0 {
		if ok, pr := pickSmallestPassingNumber(sp.log, c.ghc, nones, sp.cc); ok {
			return Trigger, []PullRequest{pr}, c.trigger(sp, sp.presubmitContexts, []PullRequest{pr})
		}
	}
	// If we have no batch, trigger one.
	if len(sp.prs) > 1 && len(batchPending) == 0 {
		batch, err := c.pickBatch(sp, sp.cc)
		if err != nil {
			return Wait, nil, err
		}
		if len(batch) > 1 {
			return TriggerBatch, batch, c.trigger(sp, sp.presubmitContexts, batch)
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
func (c *changedFilesAgent) prChanges(pr *PullRequest) ([]string, error) {
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

// prune removes any cached file changes that were not used since the last prune.
func (c *changedFilesAgent) prune() {
	c.Lock()
	defer c.Unlock()
	c.changeCache = c.nextChangeCache
	c.nextChangeCache = make(map[changeCacheKey][]string)
}

func (c *Controller) presubmitsByPull(sp *subpool) (map[int]sets.String, error) {
	presubmits := make(map[int]sets.String, len(sp.prs))
	record := func(num int, context string) {
		if jobs, ok := presubmits[num]; ok {
			jobs.Insert(context)
		} else {
			presubmits[num] = sets.NewString(context)
		}
	}

	for _, ps := range c.ca.Config().Presubmits[sp.org+"/"+sp.repo] {
		if !ps.ContextRequired() || !ps.RunsAgainstBranch(sp.branch) {
			continue
		}

		if ps.AlwaysRun {
			// Every PR requires this job.
			for _, pr := range sp.prs {
				record(int(pr.Number), ps.Context)
			}
		} else if ps.RunIfChanged != "" {
			// This is a run if changed job so we need to check if each PR requires it.
			for _, pr := range sp.prs {
				changedFiles, err := c.changedFiles.prChanges(&pr)
				if err != nil {
					return nil, err
				}
				if ps.RunsAgainstChanges(changedFiles) {
					record(int(pr.Number), ps.Context)
				}
			}
		}
	}
	return presubmits, nil
}

func (c *Controller) syncSubpool(sp subpool, blocks []blockers.Blocker) (Pool, error) {
	sp.log.Infof("Syncing subpool: %d PRs, %d PJs.", len(sp.prs), len(sp.pjs))
	successes, pendings, nones := accumulate(sp.presubmitContexts, sp.prs, sp.pjs, sp.log)
	batchMerge, batchPending := accumulateBatch(sp.presubmitContexts, sp.prs, sp.pjs, sp.log)
	sp.log.WithFields(logrus.Fields{
		"prs-passing":   prNumbers(successes),
		"prs-pending":   prNumbers(pendings),
		"prs-missing":   prNumbers(nones),
		"batch-passing": prNumbers(batchMerge),
		"batch-pending": prNumbers(batchPending),
	}).Info("Subpool accumulated.")

	var act Action
	var targets []PullRequest
	var err error
	if len(blocks) > 0 {
		act = PoolBlocked
	} else {
		act, targets, err = c.takeAction(sp, batchPending, successes, pendings, nones, batchMerge)
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
			MissingPRs: nones,

			BatchPending: batchPending,

			Action:   act,
			Target:   targets,
			Blockers: blocks,
		},
		err
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
	sha    string

	pjs []kube.ProwJob
	prs []PullRequest

	cc                contextChecker
	presubmitContexts map[int]sets.String
}

// dividePool splits up the list of pull requests and prow jobs into a group
// per repo and branch. It only keeps ProwJobs that match the latest branch.
func (c *Controller) dividePool(pool map[string]PullRequest, pjs []kube.ProwJob) (map[string]*subpool, error) {
	sps := make(map[string]*subpool)
	for _, pr := range pool {
		org := string(pr.Repository.Owner.Login)
		repo := string(pr.Repository.Name)
		branch := string(pr.BaseRef.Name)
		branchRef := string(pr.BaseRef.Prefix) + string(pr.BaseRef.Name)
		fn := fmt.Sprintf("%s/%s %s", org, repo, branch)
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
	for _, pj := range pjs {
		if pj.Spec.Type != kube.PresubmitJob && pj.Spec.Type != kube.BatchJob {
			continue
		}
		fn := fmt.Sprintf("%s/%s %s", pj.Spec.Refs.Org, pj.Spec.Refs.Repo, pj.Spec.Refs.BaseRef)
		if sps[fn] == nil || pj.Spec.Refs.BaseSHA != sps[fn].sha {
			continue
		}
		sps[fn].pjs = append(sps[fn].pjs, pj)
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
	Title githubql.String
}

// Commit holds graphql data about commits and which contexts they have
type Commit struct {
	Status struct {
		Contexts []Context
	}
	OID githubql.String `graphql:"oid"`
}

// Context holds graphql response data for github contexts.
type Context struct {
	Context     githubql.String
	Description githubql.String
	State       githubql.StatusState
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
		IssueCount githubql.Int
		Nodes      []struct {
			PullRequest PullRequest `graphql:"... on PullRequest"`
		}
	} `graphql:"search(type: ISSUE, first: 100, after: $searchCursor, query: $query)"`
}

func (pr *PullRequest) logFields() logrus.Fields {
	return logrus.Fields{
		"org":  string(pr.Repository.Owner.Login),
		"repo": string(pr.Repository.Name),
		"pr":   int(pr.Number),
		"sha":  string(pr.HeadRefOID),
	}
}

// headContexts gets the status contexts for the commit with OID == pr.HeadRefOID
//
// First, we try to get this value from the commits we got with the PR query.
// Unfortunately the 'last' commit ordering is determined by author date
// not commit date so if commits are reordered non-chronologically on the PR
// branch the 'last' commit isn't necessarily the logically last commit.
// We list multiple commits with the query to increase our chance of success,
// but if we don't find the head commit we have to ask Github for it
// specifically (this costs an API token).
func headContexts(log *logrus.Entry, ghc githubClient, pr *PullRequest) ([]Context, error) {
	for _, node := range pr.Commits.Nodes {
		if node.Commit.OID == pr.HeadRefOID {
			return node.Commit.Status.Contexts, nil
		}
	}
	// We didn't get the head commit from the query (the commits must not be
	// logically ordered) so we need to specifically ask Github for the status
	// and coerce it to a graphql type.
	org := string(pr.Repository.Owner.Login)
	repo := string(pr.Repository.Name)
	// Log this event so we can tune the number of commits we list to minimize this.
	log.Warnf("'last' %d commits didn't contain logical last commit. Querying Github...", len(pr.Commits.Nodes))
	combined, err := ghc.GetCombinedStatus(org, repo, string(pr.HeadRefOID))
	if err != nil {
		return nil, fmt.Errorf("failed to get the combined status: %v", err)
	}
	contexts := make([]Context, 0, len(combined.Statuses))
	for _, status := range combined.Statuses {
		contexts = append(
			contexts,
			Context{
				Context:     githubql.String(status.Context),
				Description: githubql.String(status.Description),
				State:       githubql.StatusState(strings.ToUpper(status.State)),
			},
		)
	}
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

		for _, e := range orgExceptions[o].UnsortedList() {
			toks = append(toks, fmt.Sprintf("-repo:\"%s\"", e))
		}
	}
	for _, r := range repos {
		toks = append(toks, fmt.Sprintf("repo:\"%s\"", r))
	}
	return strings.Join(toks, " ")
}

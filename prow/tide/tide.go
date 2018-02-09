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
	"strings"
	"sync"
	"time"

	"github.com/shurcooL/githubql"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	statusContext   string = "tide"
	statusInPool           = "In tide pool."
	statusNotInPool        = "Not in tide pool."
)

type kubeClient interface {
	ListProwJobs(string) ([]kube.ProwJob, error)
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

type githubClient interface {
	CreateStatus(string, string, string, github.Status) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetRef(string, string, string) (string, error)
	Merge(string, string, int, github.MergeDetails) error
	Query(context.Context, interface{}, map[string]interface{}) error
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
}

type statusController struct {
	logger *logrus.Entry
	ca     *config.Agent
	ghc    githubClient

	// newPoolPending is a size 1 chan that signals that the main Tide loop has
	// updated the 'poolPRs' field with a freshly updated pool.
	newPoolPending chan bool

	// lastSyncStartTime is used to only list PRs that have changed since we last
	// checked in order to make status context updates cheaper.
	lastSyncStartTime time.Time

	sync.Mutex
	poolPRs []PullRequest
}

// Action represents what actions the controller can take. It will take
// exactly one action each sync.
type Action string

const (
	Wait         Action = "WAIT"
	Trigger             = "TRIGGER"
	TriggerBatch        = "TRIGGER_BATCH"
	Merge               = "MERGE"
	MergeBatch          = "MERGE_BATCH"
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
	Action Action
	Target []PullRequest
}

// NewController makes a Controller out of the given clients.
func NewController(ghcSync, ghcStatus *github.Client, kc *kube.Client, ca *config.Agent, gc *git.Client, logger *logrus.Entry) *Controller {
	sc := &statusController{
		logger:         logger.WithField("controller", "status-update"),
		ghc:            ghcStatus,
		ca:             ca,
		newPoolPending: make(chan bool, 1),
	}
	go sc.run()
	return &Controller{
		logger: logger.WithField("controller", "sync"),
		ghc:    ghcSync,
		kc:     kc,
		ca:     ca,
		gc:     gc,
		sc:     sc,
	}
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

// Returns expected status state and description.
// TODO(spxtr): Useful information such as "missing label: foo."
func expectedStatus(pr *PullRequest, pool map[string]PullRequest) (string, string) {
	key := prKey(pr)
	if _, ok := pool[key]; !ok {
		return github.StatusPending, statusNotInPool
	}
	return github.StatusSuccess, statusInPool
}

func (sc *statusController) setStatuses(all, pool []PullRequest) {
	poolM := byRepoAndNumber(pool)
	processed := sets.NewString()

	process := func(pr *PullRequest) {
		processed.Insert(prKey(pr))

		log := sc.logger.WithFields(pr.logFields())
		contexts, err := headContexts(log, sc.ghc, pr)
		if err != nil {
			log.WithError(err).Error("Getting head commit status contexts, skipping...")
			return
		}

		wantState, wantDesc := expectedStatus(pr, poolM)
		var actualState githubql.StatusState
		var actualDesc string
		for _, ctx := range contexts {
			if string(ctx.Context) == statusContext {
				actualState = ctx.State
				actualDesc = string(ctx.Description)
			}
		}
		if wantState != strings.ToLower(string(actualState)) || wantDesc != actualDesc {
			if err := sc.ghc.CreateStatus(
				string(pr.Repository.Owner.Login),
				string(pr.Repository.Name),
				string(pr.HeadRefOID),
				github.Status{
					Context:     statusContext,
					State:       wantState,
					Description: wantDesc,
					TargetURL:   sc.ca.Config().Tide.TargetURL,
				}); err != nil {
				log.WithError(err).Errorf(
					"Failed to set status context from %q to %q.",
					string(actualState),
					wantState,
				)
			}
		}
	}

	for _, pr := range all {
		process(&pr)
	}
	// The list of all open PRs may not contain a PR if it was merged before we
	// listed all open PRs. To prevent a new PR that starts in the pool and
	// immediately merges from missing a tide status context we need to ensure that
	// every PR in the pool is processed even if it doesn't appear in all.
	//
	// Note: We could still fail to update a status context if the statusController
	// falls behind the main Tide sync loop by multiple loops (if we are lapped).
	// This would be unlikely to occur, could only occur if the status update sync
	// period is smaller than the main sync period, and would only result in a
	// missing tide status context on a successfully merged PR.
	for key, poolPR := range poolM {
		if !processed.Has(key) {
			process(&poolPR)
		}
	}
}

func (sc *statusController) run() {
	lastStart := time.Time{} // Zero value so that we don't wait the first loop.
	// wait for a new pool
	for range sc.newPoolPending {
		// wait for the min sync period time to elapse if needed.
		time.Sleep(time.Until(lastStart.Add(sc.ca.Config().Tide.StatusUpdatePeriod)))
		lastStart = time.Now()

		sc.Lock()
		pool := sc.poolPRs
		sc.Unlock()
		sc.sync(pool)
	}
}

func (sc *statusController) sync(pool []PullRequest) {
	ctx := context.Background()
	syncStartTime := time.Now()
	var all []PullRequest
	for _, q := range sc.ca.Config().Tide.Queries {
		allPRs, err := search(sc.ghc, sc.logger, ctx, q.AllPRsSince(sc.lastSyncStartTime))
		if err != nil {
			sc.logger.WithError(err).Errorf("Searching for open PRs.")
			return
		}
		all = append(all, allPRs...)
	}
	// We were able to find all open PRs so update the last sync time.
	sc.lastSyncStartTime = syncStartTime
	sc.setStatuses(all, pool)
}

// Sync runs one sync iteration.
func (c *Controller) Sync() error {
	ctx := context.Background()
	c.logger.Info("Building tide pool.")
	var pool []PullRequest
	for _, q := range c.ca.Config().Tide.Queries {
		poolPRs, err := search(c.ghc, c.logger, ctx, q.Query())
		if err != nil {
			return err
		}
		for _, pr := range poolPRs {
			// Only keep PRs that are mergeable or haven't had mergeability computed.
			if pr.Mergeable != githubql.MergeableStateConflicting {
				pool = append(pool, pr)
			}
		}
	}
	// Notify statusController about the new pool.
	c.sc.Lock()
	c.sc.poolPRs = pool
	select {
	case c.sc.newPoolPending <- true:
	default:
	}
	c.sc.Unlock()

	var pjs []kube.ProwJob
	var err error
	if len(pool) > 0 {
		pjs, err = c.kc.ListProwJobs(kube.EmptySelector)
		if err != nil {
			return err
		}
	}
	sps, err := c.dividePool(pool, pjs)
	if err != nil {
		return err
	}

	goroutines := c.ca.Config().Tide.MaxGoroutines
	if goroutines > len(sps) {
		goroutines = len(sps)
	}
	wg := &sync.WaitGroup{}
	wg.Add(goroutines)
	c.logger.Debugf("Firing up %d goroutines", goroutines)
	poolChan := make(chan Pool, len(sps))
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for sp := range sps {
				if pool, err := c.syncSubpool(sp); err != nil {
					sp.log.WithError(err).Errorf("Syncing subpool.")
				} else {
					poolChan <- pool
				}
			}
		}()
	}
	wg.Wait()
	close(poolChan)

	pools := make([]Pool, 0, len(sps))
	for pool := range poolChan {
		pools = append(pools, pool)
	}
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
		c.logger.WithError(err).Error("Decoding JSON.")
		b = []byte("[]")
	}
	fmt.Fprintf(w, string(b))
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
func isPassingTests(log *logrus.Entry, ghc githubClient, pr PullRequest) bool {
	log = log.WithFields(pr.logFields())
	contexts, err := headContexts(log, ghc, &pr)
	if err != nil {
		log.WithError(err).Error("Getting head commit status contexts.")
		// If we can't get the status of the commit, assume that it is failing.
		return false
	}
	for _, ctx := range contexts {
		if string(ctx.Context) == statusContext {
			continue
		}
		if ctx.State != githubql.StatusStateSuccess {
			return false
		}
	}
	return true
}

func pickSmallestPassingNumber(log *logrus.Entry, ghc githubClient, prs []PullRequest) (bool, PullRequest) {
	smallestNumber := -1
	var smallestPR PullRequest
	for _, pr := range prs {
		if smallestNumber != -1 && int(pr.Number) >= smallestNumber {
			continue
		}
		if len(pr.Commits.Nodes) < 1 {
			continue
		}
		if !isPassingTests(log, ghc, pr) {
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
func accumulateBatch(presubmits []string, prs []PullRequest, pjs []kube.ProwJob) ([]PullRequest, []PullRequest) {
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
		// If any batch job is pending, return now.
		if toSimpleState(pj.Status.State) == pendingState {
			var pending []PullRequest
			for _, pull := range pj.Spec.Refs.Pulls {
				pending = append(pending, prNums[pull.Number])
			}
			return nil, pending
		}
		// Otherwise, accumulate results.
		ref := pj.Spec.Refs.String()
		if _, ok := states[ref]; !ok {
			states[ref] = &accState{
				jobStates:  make(map[string]simpleState),
				validPulls: true,
			}
			for _, pull := range pj.Spec.Refs.Pulls {
				if pr, ok := prNums[pull.Number]; ok && string(pr.HeadRefOID) == pull.SHA {
					states[ref].prs = append(states[ref].prs, pr)
				} else {
					states[ref].validPulls = false
					break
				}
			}
		}
		if !states[ref].validPulls {
			// The batch contains a PR ref that has changed. Skip it.
			continue
		}
		job := pj.Spec.Job
		if s, ok := states[ref].jobStates[job]; !ok || s == noneState {
			states[ref].jobStates[job] = toSimpleState(pj.Status.State)
		}
	}
	for _, state := range states {
		if !state.validPulls {
			continue
		}
		passesAll := true
		for _, p := range presubmits {
			if s, ok := state.jobStates[p]; !ok || s != successState {
				passesAll = false
				continue
			}
		}
		if !passesAll {
			continue
		}
		return state.prs, nil
	}
	return nil, nil
}

// accumulate returns the supplied PRs sorted into three buckets based on their
// accumulated state across the presubmits.
func accumulate(presubmits []string, prs []PullRequest, pjs []kube.ProwJob) (successes, pendings, nones []PullRequest) {
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
			name := pj.Spec.Job
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
		for _, ps := range presubmits {
			if s, ok := psStates[ps]; s == noneState || !ok {
				overallState = noneState
				break
			} else if s == pendingState {
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

func (c *Controller) pickBatch(sp subpool) ([]PullRequest, error) {
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
	for i, pr := range sp.prs {
		// TODO: Make this configurable per subpool.
		if i == 5 {
			break
		}
		if !isPassingTests(sp.log, c.ghc, pr) {
			continue
		}
		if ok, err := r.Merge(string(pr.HeadRefOID)); err != nil {
			return nil, err
		} else if ok {
			res = append(res, pr)
		}
	}
	return res, nil
}

func (c *Controller) mergePRs(sp subpool, prs []PullRequest) error {
	maxRetries := 3
	for i, pr := range prs {
		backoff := time.Second * 4
		log := sp.log.WithFields(pr.logFields())
		for retry := 0; retry < maxRetries; retry++ {
			if err := c.ghc.Merge(sp.org, sp.repo, int(pr.Number), github.MergeDetails{
				SHA:         string(pr.HeadRefOID),
				MergeMethod: string(c.ca.Config().Tide.MergeMethod(sp.org, sp.repo)),
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
				} else if _, ok = err.(github.UnmergablePRError); ok {
					log.WithError(err).Error("Merge failed: PR is unmergable. How did it pass tests?!")
					break
				} else {
					return err
				}
			} else {
				log.Info("Merged.")
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

func (c *Controller) trigger(sp subpool, prs []PullRequest) error {
	for _, ps := range c.ca.Config().Presubmits[sp.org+"/"+sp.repo] {
		if ps.SkipReport || !ps.AlwaysRun || !ps.RunsAgainstBranch(sp.branch) {
			continue
		}

		var spec kube.ProwJobSpec
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
		if ok, pr := pickSmallestPassingNumber(sp.log, c.ghc, successes); ok {
			return Merge, []PullRequest{pr}, c.mergePRs(sp, []PullRequest{pr})
		}
	}
	// If we have no serial jobs pending or successful, trigger one.
	if len(nones) > 0 && len(pendings) == 0 && len(successes) == 0 {
		if ok, pr := pickSmallestPassingNumber(sp.log, c.ghc, nones); ok {
			return Trigger, []PullRequest{pr}, c.trigger(sp, []PullRequest{pr})
		}
	}
	// If we have no batch, trigger one.
	if len(sp.prs) > 1 && len(batchPending) == 0 {
		batch, err := c.pickBatch(sp)
		if err != nil {
			return Wait, nil, err
		}
		if len(batch) > 1 {
			return TriggerBatch, batch, c.trigger(sp, batch)
		}
	}
	return Wait, nil, nil
}

func (c *Controller) syncSubpool(sp subpool) (Pool, error) {
	sp.log.Infof("Syncing subpool: %d PRs, %d PJs.", len(sp.prs), len(sp.pjs))
	var presubmits []string
	for _, ps := range c.ca.Config().Presubmits[sp.org+"/"+sp.repo] {
		if ps.SkipReport || !ps.AlwaysRun || !ps.RunsAgainstBranch(sp.branch) {
			continue
		}
		presubmits = append(presubmits, ps.Name)
	}
	successes, pendings, nones := accumulate(presubmits, sp.prs, sp.pjs)
	batchMerge, batchPending := accumulateBatch(presubmits, sp.prs, sp.pjs)
	sp.log.WithFields(logrus.Fields{
		"prs-passing":   prNumbers(successes),
		"prs-pending":   prNumbers(pendings),
		"prs-missing":   prNumbers(nones),
		"batch-passing": prNumbers(batchMerge),
		"batch-pending": prNumbers(batchPending),
	}).Info("Subpool accumulated.")
	act, targets, err := c.takeAction(sp, batchPending, successes, pendings, nones, batchMerge)
	sp.log.WithFields(logrus.Fields{
		"action":  string(act),
		"targets": prNumbers(targets),
	}).Info("Subpool synced.")
	return Pool{
			Org:    sp.org,
			Repo:   sp.repo,
			Branch: sp.branch,

			SuccessPRs: successes,
			PendingPRs: pendings,
			MissingPRs: nones,

			BatchPending: batchPending,

			Action: act,
			Target: targets,
		},
		err
}

type subpool struct {
	log    *logrus.Entry
	org    string
	repo   string
	branch string
	sha    string
	pjs    []kube.ProwJob
	prs    []PullRequest
}

// dividePool splits up the list of pull requests and prow jobs into a group
// per repo and branch. It only keeps ProwJobs that match the latest branch.
func (c *Controller) dividePool(pool []PullRequest, pjs []kube.ProwJob) (chan subpool, error) {
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
	ret := make(chan subpool, len(sps))
	for _, sp := range sps {
		ret <- *sp
	}
	close(ret)
	return ret, nil
}

func search(ghc githubClient, log *logrus.Entry, ctx context.Context, q string) ([]PullRequest, error) {
	var ret []PullRequest
	vars := map[string]interface{}{
		"query":        githubql.String(q),
		"searchCursor": (*githubql.String)(nil),
	}
	var totalCost int
	var remaining int
	for {
		sq := searchQuery{}
		if err := ghc.Query(ctx, &sq, vars); err != nil {
			return nil, err
		}
		totalCost += int(sq.RateLimit.Cost)
		remaining = int(sq.RateLimit.Remaining)
		for _, n := range sq.Search.Nodes {
			ret = append(ret, n.PullRequest)
		}
		if !sq.Search.PageInfo.HasNextPage {
			break
		}
		vars["searchCursor"] = githubql.NewString(sq.Search.PageInfo.EndCursor)
	}
	log.Infof("Search for query \"%s\" cost %d point(s). %d remaining.", q, totalCost, remaining)
	return ret, nil
}

type PullRequest struct {
	Number githubql.Int
	Author struct {
		Login githubql.String
	}
	BaseRef struct {
		Name   githubql.String
		Prefix githubql.String
	}
	HeadRefOID githubql.String `graphql:"headRefOid"`
	Mergeable  githubql.MergeableState
	Repository struct {
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
		// an additional API token. (see the 'headContexts' func for details)
		// We can't raise this too much or we could hit the limit of 50,000 nodes
		// per query: https://developer.github.com/v4/guides/resource-limitations/#node-limit
	} `graphql:"commits(last: 4)"`
}

type Commit struct {
	Status struct {
		Contexts []Context
	}
	OID githubql.String `graphql:"oid"`
}

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
		Nodes []struct {
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
	return contexts, nil
}

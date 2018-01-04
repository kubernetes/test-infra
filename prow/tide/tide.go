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

	"github.com/shurcooL/githubql"
	"github.com/sirupsen/logrus"

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
	GetRef(string, string, string) (string, error)
	CreateStatus(string, string, string, github.Status) error
	Query(context.Context, interface{}, map[string]interface{}) error
	Merge(string, string, int, github.MergeDetails) error
}

// Controller knows how to sync PRs and PJs.
type Controller struct {
	logger *logrus.Entry
	ca     *config.Agent
	ghc    githubClient
	kc     kubeClient
	gc     *git.Client

	m     sync.Mutex
	pools []Pool
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
func NewController(ghc *github.Client, kc *kube.Client, ca *config.Agent, gc *git.Client, logger *logrus.Entry) *Controller {
	return &Controller{
		logger: logger,
		ghc:    ghc,
		kc:     kc,
		ca:     ca,
		gc:     gc,
	}
}

// org/repo -> number -> pr
func byRepoAndNumber(prs []PullRequest) map[string]map[int]PullRequest {
	m := make(map[string]map[int]PullRequest)
	for _, pr := range prs {
		r := string(pr.Repository.NameWithOwner)
		if _, ok := m[r]; !ok {
			m[r] = make(map[int]PullRequest)
		}
		m[r][int(pr.Number)] = pr
	}
	return m
}

// Returns expected status state and description.
// TODO(spxtr): Useful information such as "missing label: foo."
func expectedStatus(pr PullRequest, pool map[string]map[int]PullRequest) (string, string) {
	if _, ok := pool[string(pr.Repository.NameWithOwner)][int(pr.Number)]; !ok {
		return github.StatusPending, statusNotInPool
	}
	return github.StatusSuccess, statusInPool
}

func (c *Controller) setStatuses(all, pool []PullRequest) {
	poolM := byRepoAndNumber(pool)
	for _, pr := range all {
		wantState, wantDesc := expectedStatus(pr, poolM)
		var actualState githubql.StatusState
		var actualDesc string
		for _, ctx := range pr.Commits.Nodes[0].Commit.Status.Contexts {
			if string(ctx.Context) == statusContext {
				actualState = ctx.State
				actualDesc = string(ctx.Description)
			}
		}
		if wantState != strings.ToLower(string(actualState)) || wantDesc != actualDesc {
			if err := c.ghc.CreateStatus(
				string(pr.Repository.Owner.Login),
				string(pr.Repository.Name),
				string(pr.HeadRef.Target.OID),
				github.Status{
					Context:     statusContext,
					State:       wantState,
					Description: wantDesc,
					TargetURL:   c.ca.Config().Tide.TargetURL,
				}); err != nil {
				c.logger.WithError(err).Errorf(
					"Failed to set status context for %s/%s#%d sha: %s.",
					string(pr.Repository.Owner.Login),
					string(pr.Repository.Name),
					int(pr.Number),
					string(pr.HeadRef.Target.OID),
				)
			}
		}
	}
}

// Sync runs one sync iteration.
func (c *Controller) Sync() error {
	ctx := context.Background()
	c.logger.Info("Building tide pool.")
	var pool []PullRequest
	var all []PullRequest
	for _, q := range c.ca.Config().Tide.Queries {
		poolPRs, err := c.search(ctx, q.Query())
		if err != nil {
			return err
		}
		pool = append(pool, poolPRs...)
		allPRs, err := c.search(ctx, q.AllPRs())
		if err != nil {
			return err
		}
		all = append(all, allPRs...)
	}
	c.setStatuses(all, pool)

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

	pools := make([]Pool, 0, len(sps))
	for _, sp := range sps {
		if pool, err := c.syncSubpool(sp); err != nil {
			c.logger.WithError(err).Errorf("Syncing subpool %s/%s:%s.", sp.org, sp.repo, sp.branch)
		} else {
			pools = append(pools, pool)
		}
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
func isPassingTests(pr PullRequest) bool {
	for _, ctx := range pr.Commits.Nodes[0].Commit.Status.Contexts {
		if string(ctx.Context) == statusContext {
			continue
		}
		if ctx.State != githubql.StatusStateSuccess {
			return false
		}
	}
	return true
}

func pickSmallestPassingNumber(prs []PullRequest) (bool, PullRequest) {
	smallestNumber := -1
	var smallestPR PullRequest
	for _, pr := range prs {
		if smallestNumber != -1 && int(pr.Number) >= smallestNumber {
			continue
		}
		if len(pr.Commits.Nodes) < 1 {
			continue
		}
		if !isPassingTests(pr) {
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
				if pr, ok := prNums[pull.Number]; ok && string(pr.HeadRef.Target.OID) == pull.SHA {
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
	if err := r.Checkout(sp.sha); err != nil {
		return nil, err
	}
	var res []PullRequest
	for i, pr := range sp.prs {
		// TODO: Make this configurable per subpool.
		if i == 5 {
			break
		}
		if !isPassingTests(pr) {
			continue
		}
		if ok, err := r.Merge(string(pr.HeadRef.Target.OID)); err != nil {
			return nil, err
		} else if ok {
			res = append(res, pr)
		}
	}
	return res, nil
}

func (c *Controller) mergePRs(sp subpool, prs []PullRequest) error {
	for _, pr := range prs {
		if err := c.ghc.Merge(sp.org, sp.repo, int(pr.Number), github.MergeDetails{
			SHA:         string(pr.HeadRef.Target.OID),
			MergeMethod: string(c.ca.Config().Tide.MergeMethod(sp.org, sp.repo)),
		}); err != nil {
			if _, ok := err.(github.ModifiedHeadError); ok {
				// This is a possible source of incorrect behavior. If someone
				// modifies their PR as we try to merge it in a batch then we
				// end up in an untested state. This is unlikely to cause any
				// real problems.
				c.logger.WithError(err).Info("Merge failed: PR was modified.")
			} else if _, ok = err.(github.UnmergablePRError); ok {
				c.logger.WithError(err).Warning("Merge failed: PR is unmergable. How did it pass tests?!")
			} else {
				return err
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
					SHA:    string(pr.HeadRef.Target.OID),
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
		if ok, pr := pickSmallestPassingNumber(successes); ok {
			return Merge, []PullRequest{pr}, c.mergePRs(sp, []PullRequest{pr})
		}
	}
	// If we have no serial jobs pending or successful, trigger one.
	if len(nones) > 0 && len(pendings) == 0 && len(successes) == 0 {
		if ok, pr := pickSmallestPassingNumber(nones); ok {
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
	c.logger.Infof("%s/%s %s: %d PRs, %d PJs.", sp.org, sp.repo, sp.branch, len(sp.prs), len(sp.pjs))
	var presubmits []string
	for _, ps := range c.ca.Config().Presubmits[sp.org+"/"+sp.repo] {
		if ps.SkipReport || !ps.AlwaysRun || !ps.RunsAgainstBranch(sp.branch) {
			continue
		}
		presubmits = append(presubmits, ps.Name)
	}
	successes, pendings, nones := accumulate(presubmits, sp.prs, sp.pjs)
	batchMerge, batchPending := accumulateBatch(presubmits, sp.prs, sp.pjs)
	c.logger.Infof("Passing PRs: %v", prNumbers(successes))
	c.logger.Infof("Pending PRs: %v", prNumbers(pendings))
	c.logger.Infof("Missing PRs: %v", prNumbers(nones))
	c.logger.Infof("Passing batch: %v", prNumbers(batchMerge))
	c.logger.Infof("Pending batch: %v", prNumbers(batchPending))
	act, targets, err := c.takeAction(sp, batchPending, successes, pendings, nones, batchMerge)
	c.logger.Infof("Action: %v, Targets: %v", act, targets)
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
	org    string
	repo   string
	branch string
	sha    string
	pjs    []kube.ProwJob
	prs    []PullRequest
}

// dividePool splits up the list of pull requests and prow jobs into a group
// per repo and branch. It only keeps ProwJobs that match the latest branch.
func (c *Controller) dividePool(pool []PullRequest, pjs []kube.ProwJob) ([]subpool, error) {
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
	var ret []subpool
	for _, sp := range sps {
		ret = append(ret, *sp)
	}
	return ret, nil
}

func (c *Controller) search(ctx context.Context, q string) ([]PullRequest, error) {
	var ret []PullRequest
	vars := map[string]interface{}{
		"query":        githubql.String(q),
		"searchCursor": (*githubql.String)(nil),
	}
	var totalCost int
	var remaining int
	for {
		sq := searchQuery{}
		if err := c.ghc.Query(ctx, &sq, vars); err != nil {
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
	c.logger.Infof("Search for query \"%s\" cost %d point(s). %d remaining.", q, totalCost, remaining)
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
	Repository struct {
		Name          githubql.String
		NameWithOwner githubql.String
		Owner         struct {
			Login githubql.String
		}
	}
	HeadRef struct {
		Target struct {
			OID githubql.String `graphql:"oid"`
		}
	}
	Commits struct {
		Nodes []struct {
			Commit Commit
		}
	} `graphql:"commits(last: 1)"`
}

type Commit struct {
	Status struct {
		Contexts []Context
	}
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

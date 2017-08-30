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

// Package tide contains a controller for managing a tide pool of PRs.
package tide

import (
	"context"
	"fmt"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/shurcooL/githubql"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
)

type githubClient interface {
	GetRef(string, string, string) (string, error)
	Query(context.Context, interface{}, map[string]interface{}) error
}

// Controller knows how to sync PRs and PJs.
type Controller struct {
	log *logrus.Entry
	ca  *config.Agent
	ghc githubClient
	kc  *kube.Client
}

// NewController makes a Controller out of the given clients.
func NewController(l *logrus.Entry, ghc *github.Client, kc *kube.Client, ca *config.Agent) *Controller {
	return &Controller{
		log: l,
		ghc: ghc,
		kc:  kc,
		ca:  ca,
	}
}

// Sync runs one sync iteration.
func (c *Controller) Sync() error {
	ctx := context.Background()
	c.log.Info("Building tide pool.")
	var pool []pullRequest
	for _, q := range c.ca.Config().Tide.Queries {
		prs, err := c.search(ctx, q)
		if err != nil {
			return err
		}
		pool = append(pool, prs...)
	}
	var pjs []kube.ProwJob
	var err error
	if len(pool) > 0 {
		c.log.Info("Listing ProwJobs.")
		pjs, err = c.kc.ListProwJobs(nil)
		if err != nil {
			return err
		}
	}
	sps, err := c.dividePool(pool, pjs)
	if err != nil {
		return err
	}
	for _, sp := range sps {
		if err := c.syncSubpool(ctx, sp); err != nil {
			return err
		}
	}
	return nil
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

func pickSmallestPassingNumber(prs []pullRequest) (bool, pullRequest) {
	smallestNumber := -1
	var smallestPR pullRequest
	for _, pr := range prs {
		if smallestNumber != -1 && int(pr.Number) >= smallestNumber {
			continue
		}
		if len(pr.Commits.Nodes) < 1 {
			continue
		}
		if string(pr.Commits.Nodes[0].Commit.Status.State) != "SUCCESS" {
			continue
		}
		smallestNumber = int(pr.Number)
		smallestPR = pr
	}
	return smallestNumber > -1, smallestPR
}

// accumulate returns the supplied PRs sorted into three buckets based on their
// accumulated state across the presubmits.
func accumulate(presubmits []string, prs []pullRequest, pjs []kube.ProwJob) (successes, pendings, nones []pullRequest) {
	for _, pr := range prs {
		// Accumulate the best result for each job.
		psStates := make(map[string]simpleState)
		for _, pj := range pjs {
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

// TODO(spxtr): Batch merges.
func (c *Controller) syncSubpool(ctx context.Context, sp subpool) error {
	c.log.Infof("%s/%s %s: %d PRs, %d PJs.", sp.org, sp.repo, sp.branch, len(sp.prs), len(sp.pjs))
	var presubmits []string
	for _, ps := range c.ca.Config().Presubmits[sp.org+"/"+sp.repo] {
		if ps.SkipReport || !ps.AlwaysRun || !ps.RunsAgainstBranch(sp.branch) {
			continue
		}
		presubmits = append(presubmits, ps.Name)
	}
	successes, pendings, nones := accumulate(presubmits, sp.prs, sp.pjs)
	if len(successes) > 0 {
		if ok, pr := pickSmallestPassingNumber(successes); ok {
			c.log.Infof("Merge PR #%d.", int(pr.Number))
			return nil
		}
	}
	if len(pendings) > 0 {
		c.log.Info("Do nothing. Waiting for pending PRs.")
		return nil
	}
	if len(nones) > 0 {
		if ok, pr := pickSmallestPassingNumber(nones); ok {
			c.log.Infof("Trigger tests for PR #%d.", int(pr.Number))
			return nil
		}
	}
	return nil
}

type subpool struct {
	org    string
	repo   string
	branch string
	sha    string
	pjs    []kube.ProwJob
	prs    []pullRequest
}

// dividePool splits up the list of pull requests and prow jobs into a group
// per repo and branch. It only keeps ProwJobs that match the latest branch.
func (c *Controller) dividePool(pool []pullRequest, pjs []kube.ProwJob) ([]subpool, error) {
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

func (c *Controller) search(ctx context.Context, q string) ([]pullRequest, error) {
	var ret []pullRequest
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
	c.log.Infof("Search for query \"%s\" cost %d point(s). %d remaining.", q, totalCost, remaining)
	return ret, nil
}

type pullRequest struct {
	Number  githubql.Int
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
			Commit struct {
				Status struct {
					State githubql.String
				}
			}
		}
	} `graphql:"commits(last: 1)"`
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
			PullRequest pullRequest `graphql:"... on PullRequest"`
		}
	} `graphql:"search(type: ISSUE, first: 100, after: $searchCursor, query: $query)"`
}

/*
Copyright 2022 The Kubernetes Authors.

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

package tide

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/tide/blockers"
)

// CodeReviewForDeck contains superset of data from CodeReviewCommon, it's meant
// to be consumed by deck only.
//
// Tide serves Pool data to deck via http request inside cluster, which could
// contain many PullRequests, sending over full PullRequest struct could be very
// expensive in some cases.
type CodeReviewForDeck struct {
	Title      string
	Number     int
	HeadRefOID string
	Mergeable  string
}

func FromCodeReviewCommonToCodeReviewForDeck(crc *CodeReviewCommon) *CodeReviewForDeck {
	if crc == nil {
		return nil
	}
	return &CodeReviewForDeck{
		Title:      crc.Title,
		Number:     crc.Number,
		HeadRefOID: crc.HeadRefOID,
		Mergeable:  crc.Mergeable,
	}
}

// MinCodeReviewCommon can be casted into full CodeReviewCommon, which will
// result in json marshal/unmarshal overrides.
//
// This should be used only right before serialization, and for now it's
// consumed only by Deck.
type MinCodeReviewCommon CodeReviewCommon

// MarshalJSON marshals MinCodeReviewCommon into CodeReviewForDeck
func (m *MinCodeReviewCommon) MarshalJSON() ([]byte, error) {
	min := &CodeReviewForDeck{
		Title:      m.Title,
		Number:     m.Number,
		HeadRefOID: m.HeadRefOID,
		Mergeable:  m.Mergeable,
	}
	return json.Marshal(min)
}

// UnmarshalJSON overrides unmarshal function, the marshalled bytes should only
// be used by Typescript for now
func (m *MinCodeReviewCommon) UnmarshalJSON(b []byte) error {
	return errors.New("this is not implemented")
}

type CodeReviewCommon struct {
	// NameWithOwner is from graphql.NameWithOwner, <org>/<repo>
	NameWithOwner string
	// The number of PR
	Number int
	Org    string
	Repo   string
	// BaseRefPrefix gets prefix of ref, such as /refs/head, /refs/tags
	BaseRefPrefix string
	BaseRefName   string
	HeadRefName   string
	HeadRefOID    string

	Title string
	Body  string
	// AuthorLogin is the author login from the fork on GitHub, this will be the
	// author login from Gerrit.
	AuthorLogin   string
	UpdatedAtTime time.Time

	Mergeable    string
	CanBeRebased bool

	GitHub *PullRequest
}

func (crc *CodeReviewCommon) logFields() logrus.Fields {
	return logrus.Fields{
		"org":    crc.Org,
		"repo":   crc.Repo,
		"pr":     crc.Number,
		"branch": crc.BaseRefName,
		"sha":    crc.HeadRefOID,
	}
}

// GitHubLabels returns labels struct for GitHub, using this function is almost
// equivalent to `if isGitHub() {// then do that}`.
//
// This is useful for determining the merging strategy.
func (crc *CodeReviewCommon) GitHubLabels() *Labels {
	if crc.GitHub == nil {
		return nil
	}
	return &crc.GitHub.Labels
}

// GitHubCommits returns Commits struct from GitHub.
//
// This is used by checking status context to determine whether the PR is ready
// for merge or not.
func (crc *CodeReviewCommon) GitHubCommits() *Commits {
	if crc.GitHub == nil {
		return nil
	}
	return &crc.GitHub.Commits
}

// CodeReviewCommonFromPullRequest derives CodeReviewCommon struct from GitHub
// PullRequest struct, by extracting shared fields among different code review
// providers.
func CodeReviewCommonFromPullRequest(pr *PullRequest) *CodeReviewCommon {
	if pr == nil {
		return nil
	}
	// Make a copy
	prCopy := *pr
	crc := &CodeReviewCommon{
		NameWithOwner: string(pr.Repository.NameWithOwner),
		Number:        int(pr.Number),
		Org:           string(pr.Repository.Owner.Login),
		Repo:          string(pr.Repository.Name),
		BaseRefPrefix: string(pr.BaseRef.Prefix),
		BaseRefName:   string(pr.BaseRef.Name),
		HeadRefName:   string(pr.HeadRefName),
		HeadRefOID:    string(pr.HeadRefOID),
		Title:         string(pr.Title),
		Body:          string(pr.Body),
		AuthorLogin:   string(pr.Author.Login),
		Mergeable:     string(pr.Mergeable),
		CanBeRebased:  bool(pr.CanBeRebased),
		UpdatedAtTime: pr.UpdatedAt.Time,

		GitHub: &prCopy,
	}

	return crc
}

type CodeReviewEntity interface {
	PullRequest
}

// provider is the interface implemented by each source code
// providers, such as GitHub and Gerrit.
type provider interface {
	Query() (map[string]CodeReviewCommon, error)
	blockers() (blockers.Blockers, error)
	isAllowedToMerge(crc *CodeReviewCommon) (string, error)
	GetRef(org, repo, ref string) (string, error)
	// headContexts returns Contexts from all presubmit requirements.
	// Tide needs to know whether a PR passed all tests or not, this includes
	// prow jobs, but also any external tests that are required by GitHub branch
	// protection, for example GH actions. For GitHub these are all reflected on
	// status contexts, and more importantly each prowjob is a context. For
	// Gerrit we can transform every prow jobs into a context, and mark it
	// optional if the prowjob doesn't vote on label that's required for
	// merging. And also transform any other label that is not voted by prow
	// into a context.
	headContexts(pr *CodeReviewCommon) ([]Context, error)
	search(query querier, log *logrus.Entry, q string, start, end time.Time, org string) ([]PullRequest, error)
	mergePRs(sp subpool, prs []CodeReviewCommon, dontUpdateStatus *threadSafePRSet) error
	GetTideContextPolicy(gitClient git.ClientFactory, org, repo, branch string, baseSHAGetter config.RefGetter, headSHA string) (contextChecker, error)
	refsForJob(sp subpool, prs []CodeReviewCommon) prowapi.Refs
	prMergeMethod(crc *CodeReviewCommon) (types.PullRequestMergeType, error)
}

// GitHubProvider implements provider, used by tide Controller for
// interacting directly with GitHub.
//
// Tide Controller should only use GitHubProvider for communicating with GitHub.
type GitHubProvider struct {
	cfg                config.Getter
	ghc                githubClient
	usesGitHubAppsAuth bool

	*mergeChecker
	logger *logrus.Entry
}

func (gi *GitHubProvider) blockers() (blockers.Blockers, error) {
	label := gi.cfg().Tide.BlockerLabel
	if label == "" {
		return blockers.Blockers{}, nil
	}

	gi.logger.WithField("blocker_label", label).Debug("Searching for blocker issues")
	orgExcepts, repos := gi.cfg().Tide.Queries.OrgExceptionsAndRepos()
	orgs := make([]string, 0, len(orgExcepts))
	for org := range orgExcepts {
		orgs = append(orgs, org)
	}
	orgRepoQuery := orgRepoQueryStrings(orgs, repos.UnsortedList(), orgExcepts)
	return blockers.FindAll(gi.ghc, gi.logger, label, orgRepoQuery, gi.usesGitHubAppsAuth)
}

// Query gets all open PRs based on tide configuration.
func (gi *GitHubProvider) Query() (map[string]CodeReviewCommon, error) {
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	prs := make(map[string]CodeReviewCommon)
	var errs []error
	for i, query := range gi.cfg().Tide.Queries {

		// Use org-sharded queries only when GitHub apps auth is in use
		var queries map[string]string
		if gi.usesGitHubAppsAuth {
			queries = query.OrgQueries()
		} else {
			queries = map[string]string{"": query.Query()}
		}

		for org, q := range queries {
			org, q, i := org, q, i
			wg.Add(1)
			go func() {
				defer wg.Done()
				results, err := gi.search(gi.ghc.QueryWithGitHubAppsSupport, gi.logger, q, time.Time{}, time.Now(), org)

				resultString := "success"
				if err != nil {
					resultString = "error"
				}
				tideMetrics.queryResults.WithLabelValues(strconv.Itoa(i), org, resultString).Inc()

				lock.Lock()
				defer lock.Unlock()
				if err != nil && len(results) == 0 {
					gi.logger.WithField("query", q).WithError(err).Warn("Failed to execute query.")
					errs = append(errs, fmt.Errorf("query %d, err: %w", i, err))
					return
				}
				if err != nil {
					gi.logger.WithError(err).WithField("query", q).Warning("found partial results")
				}

				for _, pr := range results {
					crc := CodeReviewCommonFromPullRequest(&pr)
					prs[prKey(crc)] = *crc
				}
			}()
		}
	}
	wg.Wait()

	return prs, utilerrors.NewAggregate(errs)
}

func (gi *GitHubProvider) GetRef(org, repo, ref string) (string, error) {
	return gi.ghc.GetRef(org, repo, ref)
}

func (gi *GitHubProvider) GetTideContextPolicy(gitClient git.ClientFactory, org, repo, branch string, baseSHAGetter config.RefGetter, headSHA string) (contextChecker, error) {
	return gi.cfg().GetTideContextPolicy(gitClient, org, repo, branch, baseSHAGetter, headSHA)
}

func (gi *GitHubProvider) refsForJob(sp subpool, prs []CodeReviewCommon) prowapi.Refs {
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
				Number: pr.Number,
				Title:  pr.Title,
				Author: string(pr.AuthorLogin),
				SHA:    pr.HeadRefOID,
			},
		)
	}
	return refs
}

func (gi *GitHubProvider) prMergeMethod(crc *CodeReviewCommon) (types.PullRequestMergeType, error) {
	return gi.mergeChecker.prMergeMethod(gi.cfg().Tide, crc)
}

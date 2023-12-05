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
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/tide/blockers"

	githubql "github.com/shurcooL/githubv4"
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

	Mergeable string

	GitHub *PullRequest
	Gerrit *gerrit.ChangeInfo
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
		UpdatedAtTime: pr.UpdatedAt.Time,

		GitHub: &prCopy,
	}

	return crc
}

// CodeReviewCommonFromGerrit derives CodeReviewCommon struct from Gerrit
// ChangeInfo struct, by extracting shared fields among different code review
// providers.
//
// Gerrit ChangeInfo doesn't know which host it's from, which makes sense, as
// host for Gerrit is like `github.com` for GitHub, so it's required to be
// passed in by caller.
func CodeReviewCommonFromGerrit(gci *gerrit.ChangeInfo, instance string) *CodeReviewCommon {
	if gci == nil {
		return nil
	}
	// Make a copy
	gciCopy := *gci

	// MergeableState is an enum with three different values:
	// MergeableStateUnknown, MergeableStateMergeable, and
	// MergeableStateConflicting.
	// Ref: https://pkg.go.dev/github.com/shurcooL/githubv4#MergeableState
	mergeable := string(githubql.MergeableStateUnknown)
	if gci.Mergeable {
		mergeable = string(githubql.MergeableStateMergeable)
	} else if gci.ContainsGitConflicts {
		mergeable = string(githubql.MergeableStateConflicting)
	}
	crc := &CodeReviewCommon{
		NameWithOwner: instance + "/" + gci.Project, // org + "/" + repo
		Number:        gci.Number,
		Org:           instance,
		Repo:          gci.Project,
		BaseRefPrefix: "refs/",          // This will be stripped
		BaseRefName:   gci.Branch,       // Target branch without `/refs/for` prefix.
		HeadRefName:   "not_applicable", // Used by GitHub status controller, not useful for Gerrit at all.
		HeadRefOID:    gci.CurrentRevision,
		Title:         gci.Subject,
		Body:          "",
		AuthorLogin:   gci.Owner.Username,
		Mergeable:     mergeable,
		UpdatedAtTime: gci.Updated.Time,

		Gerrit: &gciCopy,
	}

	return crc
}

// provider is the interface implemented by each source code
// providers, such as GitHub and Gerrit.
type provider interface {
	Query() (map[string]CodeReviewCommon, error)
	blockers() (blockers.Blockers, error)
	isAllowedToMerge(crc *CodeReviewCommon) (string, error)
	// GetRef returns the SHA of the given ref, such as the latest SHA for
	// "heads/master", which tide will use for making decision on whether a
	// prowjob was tested against latest HEAD, it has to be from remote server.
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
	// mergePRs attempts to merge the specified PRs and returns the prs that were successfully merged.
	mergePRs(sp subpool, prs []CodeReviewCommon, dontUpdateStatus *threadSafePRSet) ([]CodeReviewCommon, error)
	GetTideContextPolicy(org, repo, branch string, baseSHAGetter config.RefGetter, pr *CodeReviewCommon) (contextChecker, error)
	prMergeMethod(crc *CodeReviewCommon) *types.PullRequestMergeType

	// GetPresubmits will return all presubmits for the given identifier. This includes
	// Presubmits that are versioned inside the tested repo, if the inrepoconfig feature
	// is enabled.
	// Consumers that pass in a RefGetter implementation that does a call to GitHub and who
	// also need the result of that GitHub call just keep a pointer to its result, but must
	// nilcheck that pointer before accessing it.
	GetPresubmits(identifier, baseBranch string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Presubmit, error)
	GetChangedFiles(org, repo string, number int) ([]string, error)

	refsForJob(sp subpool, prs []CodeReviewCommon) (prowapi.Refs, error)
	labelsAndAnnotations(instance string, jobLabels, jobAnnotations map[string]string, changes ...CodeReviewCommon) (labels, annotations map[string]string)

	// jobIsRequiredByTide is defined by each provider for figuring out whether
	// a job is required by Tide.
	jobIsRequiredByTide(ps *config.Presubmit, pr *CodeReviewCommon) bool
}

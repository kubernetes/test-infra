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

// Package mergecommitblocker adds a do-not-merge label to pull requests which contain merge commits
// Merge commits are defined as commits that contain more than one parent commit SHA

package mergecommitblocker

import (
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "mergecommitblocker"

// init registers out plugin as a pull request handler
func init() {

}

// helpProvider provides information on the plugin
func helpProvider() {

}

func handlePullRequest(pc plugins.Agent, pe github.PullRequestEvent) error {
	return handlePR(pc.GitHubClient, pc.Logger, pe)
}

// Strict subset of *github.Client methods.
type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	ListPRCommits(org, repo string, number int) ([]github.RepositoryCommit, error)
}

// handlePR takes a github client, a pull request event and applies, or removes applicable labels
func handlePR(gc githubClient, le *logrus.Entry, pe github.PullRequestEvent) error {

	if !isPRChanged(pe) {
		return nil
	}
	// Store all info about the owner, repo, num, and base sha of pull request
	var (
		owner = pe.PullRequest.Base.Repo.Owner.Login
		repo  = pe.PullRequest.Base.Repo.Name
		num   = pe.PullRequest.Number
	)

	// Use github client to get the commits in the pull request
	commits, err := gc.ListPRCommits(owner, repo, num)
	if err != nil {

	}
	// Iterate through them and check for parent commits
	var needsLabel bool = false

	for _, commit := range commits {
		if len(commit.Parents) > 1 {
			needsLabel = true
			continue
		}
	}

	// Once finished iterating, Label if merge commits were identified
	issueLabels, err := gc.GetIssueLabels(owner, repo, num)
	if err != nil {
		le.Warnf("while retrieving labels, error: %v", err)
	}

	f := func(label string, labels []github.Label) bool {
		return github.HasLabel(label, labels)
	}
	hasLabel := f(labels.MergeCommits, issueLabels)

	if hasLabel && !needsLabel {
		le.Infof("Removing %q Label for %s/%s#%d", labels.MergeCommits, owner, repo, num)
		return gc.RemoveLabel(owner, repo, num, labels.MergeCommits)
	} else if !hasLabel && needsLabel {
		le.Infof("Adding %q Label for %s/%s#%d", labels.MergeCommits, owner, repo, num)
		return gc.AddLabel(owner, repo, num, labels.MergeCommits)
	}
	return nil
}

// isPRChanged takes a github Pull request event and returns a boolean value, which indicates if code diffs have changed
func isPRChanged(pe github.PullRequestEvent) bool {
	switch pe.Action {
	case github.PullRequestActionOpened:
		return true
	case github.PullRequestActionReopened:
		return true
	case github.PullRequestActionSynchronize:
		return true
	case github.PullRequestActionEdited:
		return true
	default:
		return false
	}
}

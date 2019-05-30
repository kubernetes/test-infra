/*
Copyright 2019 The Kubernetes Authors.

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

// Package invalidcommitmsg adds the "do-not-merge/invalid-commit-message"
// label on PRs containing commit messages with @mentions or
// keywords that can automatically close issues.
package invalidcommitmsg

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/dco"
)

const (
	pluginName            = "invalidcommitmsg"
	invalidCommitMsgLabel = "do-not-merge/invalid-commit-message"
	commentBody           = `[Keywords](https://help.github.com/articles/closing-issues-using-keywords) which can automatically close issues and at(@) mentions are not allowed in commit messages.

**The list of commits with invalid commit messages**:

%s

<details>

%s
</details>
`
	commentPruneBody = "**The list of commits with invalid commit messages**:"
)

var (
	closeIssueRegex = regexp.MustCompile(`((?i)(clos(?:e[sd]?))|(fix(?:(es|ed)?))|(resolv(?:e[sd]?)))[\s:]+(\w+/\w+)?#(\d+)`)
	atMentionRegex  = regexp.MustCompile(`\B([@][\w_-]+)`)
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// Only the Description field is specified because this plugin is not triggered with commands and is not configurable.
	return &pluginhelp.PluginHelp{
			Description: "The invalidcommitmsg plugin applies the '" + invalidCommitMsgLabel + "' label to pull requests whose commit messages contain @ mentions or keywords which can automatically close issues.",
		},
		nil
}

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	CreateComment(owner, repo string, number int, comment string) error
	ListPRCommits(org, repo string, number int) ([]github.RepositoryCommit, error)
}

type commentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

func handlePullRequest(pc plugins.Agent, pr github.PullRequestEvent) error {
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.GitHubClient, pc.Logger, pr, cp)
}

func handle(gc githubClient, log *logrus.Entry, pr github.PullRequestEvent, cp commentPruner) error {
	// Only consider actions indicating that the code diffs may have changed.
	if !hasPRChanged(pr) {
		return nil
	}

	var (
		org    = pr.Repo.Owner.Login
		repo   = pr.Repo.Name
		number = pr.Number
	)

	labels, err := gc.GetIssueLabels(org, repo, number)
	if err != nil {
		return err
	}
	hasInvalidCommitMsgLabel := github.HasLabel(invalidCommitMsgLabel, labels)

	allCommits, err := gc.ListPRCommits(org, repo, number)
	if err != nil {
		return fmt.Errorf("error listing commits for pull request: %v", err)
	}
	log.Debugf("Found %d commits in PR", len(allCommits))

	var invalidCommits []github.GitCommit
	for _, commit := range allCommits {
		if closeIssueRegex.MatchString(commit.Commit.Message) || atMentionRegex.MatchString(commit.Commit.Message) {
			c := commit.Commit
			c.SHA = commit.SHA
			invalidCommits = append(invalidCommits, c)
		}
	}

	// if we have the label but all commits are valid,
	// remove the label and prune comments
	if hasInvalidCommitMsgLabel && len(invalidCommits) == 0 {
		if err := gc.RemoveLabel(org, repo, number, invalidCommitMsgLabel); err != nil {
			log.WithError(err).Errorf("GitHub failed to remove the following label: %s", invalidCommitMsgLabel)
		}
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, commentPruneBody)
		})
	}

	// if we don't have the label and there are invalid commits,
	// add the label
	if !hasInvalidCommitMsgLabel && len(invalidCommits) != 0 {
		if err := gc.AddLabel(org, repo, number, invalidCommitMsgLabel); err != nil {
			log.WithError(err).Errorf("GitHub failed to add the following label: %s", invalidCommitMsgLabel)
		}
	}

	// if there are invalid commits, add a comment
	if len(invalidCommits) != 0 {
		// prune old comments before adding a new one
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, commentPruneBody)
		})

		log.Debugf("Commenting on PR to advise users of invalid commit messages")
		if err := gc.CreateComment(org, repo, number, fmt.Sprintf(commentBody, dco.MarkdownSHAList(org, repo, invalidCommits), plugins.AboutThisBot)); err != nil {
			log.WithError(err).Errorf("Could not create comment for invalid commit messages")
		}
	}

	return nil
}

// hasPRChanged indicates that the code diff may have changed.
func hasPRChanged(pr github.PullRequestEvent) bool {
	switch pr.Action {
	case github.PullRequestActionOpened:
		return true
	case github.PullRequestActionReopened:
		return true
	case github.PullRequestActionSynchronize:
		return true
	default:
		return false
	}
}

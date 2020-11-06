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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/dco"
)

const (
	pluginName                  = "invalidcommitmsg"
	invalidCommitMsgLabel       = "do-not-merge/invalid-commit-message"
	invalidCommitMsgCommentBody = `[Keywords](https://help.github.com/articles/closing-issues-using-keywords) which can automatically close issues and at(@) or hashtag(#) mentions are not allowed in commit messages.

**The list of commits with invalid commit messages**:

%s

<details>

%s
</details>
`
	invalidCommitMsgCommentPruneBody = "**The list of commits with invalid commit messages**:"
	invalidTitleCommentBody          = `[Keywords](https://help.github.com/articles/closing-issues-using-keywords) which can automatically close issues and at(@) mentions are not allowed in the title of a Pull Request.

You can edit the title by writing **/retitle <new-title>** in a comment.

<details>
When GitHub merges a Pull Request, the title is included in the merge commit. To avoid invalid keywords in the merge commit, please edit the title of the PR.

%s
</details>
`
	invalidTitleCommentPruneBody = "not allowed in the title of a Pull Request"
)

var (
	CloseIssueRegex = regexp.MustCompile(`((?i)(clos(?:e[sd]?))|(fix(?:(es|ed)?))|(resolv(?:e[sd]?)))[\s:]+(\w+/\w+)?#(\d+)`)
	AtMentionRegex  = regexp.MustCompile(`\B([@][\w_-]+)`)
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	// Only the Description field is specified because this plugin is not triggered with commands and is not configurable.
	return &pluginhelp.PluginHelp{
			Description: "The invalidcommitmsg plugin applies the '" + invalidCommitMsgLabel + "' label to pull requests whose commit messages and titles contain @ mentions or keywords which can automatically close issues.",
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
		title  = pr.PullRequest.Title
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

	var invalidCommits []github.RepositoryCommit
	for _, commit := range allCommits {
		if CloseIssueRegex.MatchString(commit.Commit.Message) || AtMentionRegex.MatchString(commit.Commit.Message) {
			invalidCommits = append(invalidCommits, commit)
		}
	}

	invalidPRTitle := CloseIssueRegex.MatchString(title) || AtMentionRegex.MatchString(title)

	// if we have the label but all commits and the PR title is valid,
	// remove the label and prune comments
	if hasInvalidCommitMsgLabel && len(invalidCommits) == 0 && !invalidPRTitle {
		if err := gc.RemoveLabel(org, repo, number, invalidCommitMsgLabel); err != nil {
			log.WithError(err).Errorf("GitHub failed to remove the following label: %s", invalidCommitMsgLabel)
		}
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, invalidCommitMsgCommentPruneBody)
		})
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, invalidTitleCommentPruneBody)
		})
	}

	// if we don't have the label and
	// if the PR title is invalid OR there are invalid commits
	// add the label
	if !hasInvalidCommitMsgLabel && (len(invalidCommits) != 0 || invalidPRTitle) {
		if err := gc.AddLabel(org, repo, number, invalidCommitMsgLabel); err != nil {
			log.WithError(err).Errorf("GitHub failed to add the following label: %s", invalidCommitMsgLabel)
		}
	}

	// if there are invalid commits, add a comment
	if len(invalidCommits) != 0 {
		// prune old comments before adding a new one
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, invalidCommitMsgCommentPruneBody)
		})

		log.Debug("Commenting on PR to advise users of invalid commit messages")
		if err := gc.CreateComment(org, repo, number, fmt.Sprintf(invalidCommitMsgCommentBody, dco.MarkdownSHAList(org, repo, invalidCommits), plugins.AboutThisBot)); err != nil {
			log.WithError(err).Error("Could not create comment for invalid commit messages")
		}
	}

	// if the PR title is invalid, add a comment
	if invalidPRTitle {
		// prune old comments before adding a new one
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, invalidTitleCommentPruneBody)
		})

		log.Debug("Commenting on PR to advise users of an invalid PR title")
		if err := gc.CreateComment(org, repo, number, fmt.Sprintf(invalidTitleCommentBody, plugins.AboutThisBot)); err != nil {
			log.WithError(err).Error("Could not create comment for invalid PR title")
		}
	}

	return nil
}

// hasPRChanged indicates that the code diff or PR title may have changed.
func hasPRChanged(pr github.PullRequestEvent) bool {
	switch pr.Action {
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

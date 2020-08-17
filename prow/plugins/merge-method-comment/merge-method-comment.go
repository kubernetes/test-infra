/*
Copyright 2020 The Kubernetes Authors.

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

// Package mergemethodcomment contains a Prow plugin which comments on PRs with
// 2 or more commits, informing the user:
// - How to request commits to be squashed if default merge method is merge,
// - How to request commits to be merged if the repo squashes commits by default,
// - That the commits will be merged/squashed if it is not possible to override
// the default merge method.
package mergemethodcomment

import (
	"fmt"
	"strings"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "merge-method-comment"

// Strict subset of github.Client methods.
type githubClient interface {
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateComment(org, repo string, number int, comment string) error
	BotName() (string, error)
}

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	return &pluginhelp.PluginHelp{
			Description: "The merge-method-comment plugin adds a comment on how to request a different-from-default merge method to PRs with more than 1 commit",
		},
		nil
}

func handlePullRequest(pc plugins.Agent, pe github.PullRequestEvent) error {
	return handlePR(pc.GitHubClient, pc.Config.ProwConfig.Tide, pe)
}

func handlePR(gc githubClient, c config.Tide, pe github.PullRequestEvent) error {
	if !isPRChanged(pe) {
		return nil
	}

	commentNeeded, comment := needsComment(c, pe)
	if !commentNeeded {
		return nil
	}

	owner := pe.PullRequest.Base.Repo.Owner.Login
	repo := pe.PullRequest.Base.Repo.Name
	num := pe.PullRequest.Number

	hasComment, err := issueHasComment(gc, owner, repo, num, comment)
	if err != nil {
		return err
	}
	if hasComment {
		return nil
	}

	return gc.CreateComment(owner, repo, num, plugins.FormatSimpleResponse(pe.PullRequest.User.Login, comment))
}

func needsComment(c config.Tide, pe github.PullRequestEvent) (bool, string) {
	if pe.PullRequest.Commits <= 1 {
		return false, ""
	}

	orgRepo := config.OrgRepo{
		Org:  pe.PullRequest.Base.Repo.Owner.Login,
		Repo: pe.PullRequest.Base.Repo.Name,
	}
	method := c.MergeMethod(orgRepo)
	comment := fmt.Sprintf("This PR has multiple commits, and the default merge method is: %s.\n", method)

	switch {
	case method == github.MergeSquash && c.MergeLabel != "":
		comment = fmt.Sprintf("%sYou can request commits to be merged using the label: %s", comment, c.MergeLabel)
	case method == github.MergeSquash && c.MergeLabel == "":
		comment = comment + "Commits will be squashed, as no merge labels are defined"
	case method == github.MergeMerge && c.SquashLabel != "":
		comment = fmt.Sprintf("%sYou can request commits to be squashed using the label: %s", comment, c.SquashLabel)
	case method == github.MergeMerge && c.SquashLabel == "":
		comment = comment + "Commits will be merged, as no squash labels are defined"
	}

	return true, comment
}

func issueHasComment(gc githubClient, org, repo string, number int, comment string) (bool, error) {
	botName, err := gc.BotName()
	if err != nil {
		return false, err
	}

	comments, err := gc.ListIssueComments(org, repo, number)
	if err != nil {
		return false, fmt.Errorf("error listing issue comments: %v", err)
	}

	for _, c := range comments {
		if c.User.Login == botName && strings.Contains(c.Body, comment) {
			return true, nil
		}
	}
	return false, nil
}

// These are the only actions indicating the code diffs may have changed.
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

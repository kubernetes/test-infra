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

package mergecommitblocker

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName = "mergecommitblocker"
)

var (
	commentBody = fmt.Sprintf("Adding label `%s` because PR contains merge commits, which are not allowed in this repository.\nUse `git rebase` to reapply your commits on top of the target branch. Detailed instructions for doing so can be found [here](https://git.k8s.io/community/contributors/guide/github-workflow.md#4-keep-your-branch-in-sync).", labels.MergeCommits)
)

// init registers out plugin as a pull request handler
func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

// helpProvider provides information on the plugin
func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// Only the Description field is specified because this plugin is not triggered with commands and is not configurable.
	return &pluginhelp.PluginHelp{
		Description: fmt.Sprintf("The merge commit blocker plugin adds the %s label to pull requests that contain merge commits", labels.MergeCommits),
	}, nil
}

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	CreateComment(org, repo string, number int, comment string) error
}

type pruneClient interface {
	PruneComments(func(ic github.IssueComment) bool)
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened &&
		pre.Action != github.PullRequestActionReopened &&
		pre.Action != github.PullRequestActionSynchronize {
		return nil
	}
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.GitHubClient, pc.GitClient, cp, pc.Logger, &pre)
}

func handle(ghc githubClient, gc *git.Client, cp pruneClient, log *logrus.Entry, pre *github.PullRequestEvent) error {
	var (
		org  = pre.PullRequest.Base.Repo.Owner.Login
		repo = pre.PullRequest.Base.Repo.Name
		num  = pre.PullRequest.Number
	)

	// Clone the repo, checkout the PR.
	r, err := gc.Clone(fmt.Sprintf("%s/%s", org, repo))
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			log.WithError(err).Error("Error cleaning up repo.")
		}
	}()
	if err := r.CheckoutPullRequest(num); err != nil {
		return err
	}
	// We are guaranteed to have both Base.SHA and Head.SHA
	target, head := pre.PullRequest.Base.SHA, pre.PullRequest.Head.SHA
	existMergeCommits, err := r.MergeCommitsExistBetween(target, head)
	if err != nil {
		return err
	}
	issueLabels, err := ghc.GetIssueLabels(org, repo, num)
	if err != nil {
		return err
	}
	hasLabel := github.HasLabel(labels.MergeCommits, issueLabels)
	if hasLabel && !existMergeCommits {
		log.Infof("Removing %q Label for %s/%s#%d", labels.MergeCommits, org, repo, num)
		if err := ghc.RemoveLabel(org, repo, num, labels.MergeCommits); err != nil {
			return err
		}
		cp.PruneComments(func(ic github.IssueComment) bool {
			return strings.Contains(ic.Body, commentBody)
		})
	} else if !hasLabel && existMergeCommits {
		log.Infof("Adding %q Label for %s/%s#%d", labels.MergeCommits, org, repo, num)
		if err := ghc.AddLabel(org, repo, num, labels.MergeCommits); err != nil {
			return err
		}
		msg := plugins.FormatSimpleResponse(pre.PullRequest.User.Login, commentBody)
		return ghc.CreateComment(org, repo, num, msg)
	}
	return nil
}

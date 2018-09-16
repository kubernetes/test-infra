/*
Copyright 2018 The Kubernetes Authors.

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

package tbr

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const PluginName = "tbr"

var (
	// TODO(bentheelder,cjwagner): move label definitions to a common package
	LGTMLabel     = "lgtm"
	approvedLabel = "approved"
	TBRLabel      = "to-be-reviewed"
	tbrRE         = regexp.MustCompile(`(?mi)^/tbr\s*$`)
)

func init() {
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericCommentEvent, helpProvider)
	//plugins.RegisterPullRequestHandler()
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The lgtm plugin manages the application and removal of the 'lgtm' (Looks Good To Me) label which is typically used to gate merging.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/tbr",
		Description: "Adds the 'lgtm' label which is typically used to gate merging and the 'to-be-reviewed' label.",
		Featured:    false,
		WhoCanUse:   "Pull Request Authors. See also the 'lgtm' plugin.",
		Examples:    []string{"/tbr"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	CreateComment(owner, repo string, number int, comment string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func handleGenericCommentEvent(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	return handleGenericComment(pc.GitHubClient, pc.PluginConfig, pc.Logger, e)
}

func handleGenericComment(gc githubClient, config *plugins.Configuration, log *logrus.Entry, e github.GenericCommentEvent) error {
	// Only consider open PRs and new comments.
	if !e.IsPR || e.IssueState != "open" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	// ignore if the comment doesn't match the tbr command
	if !tbrRE.MatchString(e.Body) {
		return nil
	}

	// extract some information about the pull request
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	body := e.Body
	htmlURL := e.HTMLURL
	author := e.User.Login
	prAuthor := e.IssueAuthor.Login

	// only the PR author can TBR
	isAuthor := author == prAuthor
	if !isAuthor {
		resp := "/tbr is restricted to PR authors."
		log.Infof("Reply to /tbr request with comment: \"%s\"", resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(body, htmlURL, author, resp))
	}

	// you can only TBR if the PR is already approved
	approved, err := isApproved(gc, org, repo, number)
	if err != nil {
		return err
	}
	if !approved {
		resp := "/tbr can only be used on approved PRs"
		log.Infof("Reply to /tbr request with comment: \"%s\"", resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(body, htmlURL, author, resp))
	}

	// at this point we are actually going to TBR this PR
	// add comment
	resp := fmt.Sprintf(
		"Adding %s and %s labels. This PR should be reviewed later.",
		TBRLabel, LGTMLabel,
	)
	formattedResp := plugins.FormatResponseRaw(body, htmlURL, author, resp)
	if err := gc.CreateComment(org, repo, number, formattedResp); err != nil {
		return err
	}
	// add lgtm + tbr labels
	if err := gc.AddLabel(org, repo, number, TBRLabel); err != nil {
		return err
	}
	if err := gc.AddLabel(org, repo, number, LGTMLabel); err != nil {
		return err
	}
	return nil
}

// isApproved checks if a PR has the approved label
func isApproved(gc githubClient, org, repo string, number int) (bool, error) {
	labels, err := gc.GetIssueLabels(org, repo, number)
	if err != nil {
		return false, err
	}
	for _, label := range labels {
		if label.Name == approvedLabel {
			return true, nil
		}
	}
	return false, nil
}

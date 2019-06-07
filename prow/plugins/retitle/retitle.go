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

// Package retitle implements the retitle plugin
package retitle

import (
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/trigger"
)

const (
	// pluginName defines this plugin's registered name.
	pluginName = "retitle"
)

var (
	retitleRe = regexp.MustCompile(`(?mi)^/retitle\s*(.*)$`)
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericCommentEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The retitle plugin allows users to re-title pull requests and issues where GitHub permissions don't allow them to.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/retitle <title>",
		Description: "Edits the pull request or issue title.",
		Featured:    true,
		WhoCanUse:   "Collaborators on the repository.",
		Examples:    []string{"/retitle New Title"},
	})
	return pluginHelp, nil
}

func handleGenericCommentEvent(pc plugins.Agent, e github.GenericCommentEvent) error {
	var (
		org  = e.Repo.Owner.Login
		repo = e.Repo.Name
	)
	return handleGenericComment(pc.GitHubClient, func(user string) (bool, error) {
		return trigger.TrustedUser(pc.GitHubClient, pc.PluginConfig.TriggerFor(org, repo), user, org, repo)
	}, pc.Logger, e)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	EditPullRequest(org, repo string, number int, pr *github.PullRequest) (*github.PullRequest, error)
	GetIssue(org, repo string, number int) (*github.Issue, error)
	EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error)
}

func handleGenericComment(gc githubClient, isTrusted func(string) (bool, error), log *logrus.Entry, gce github.GenericCommentEvent) error {
	// Only consider open PRs and issues, and new comments.
	if gce.IssueState != "open" || gce.Action != github.GenericCommentActionCreated {
		return nil
	}

	// Make sure they are requesting a re-title
	if !retitleRe.MatchString(gce.Body) {
		return nil
	}

	var (
		org    = gce.Repo.Owner.Login
		repo   = gce.Repo.Name
		number = gce.Number
		user   = gce.User.Login
	)

	trusted, err := isTrusted(user)
	if err != nil {
		log.WithError(err).Error("Could not check if user was trusted.")
		return err
	}
	if !trusted {
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(gce.Body, gce.HTMLURL, user, `Re-titling can only be requested by trusted users, like repository collaborators.`))
	}

	matches := retitleRe.FindStringSubmatch(gce.Body)
	if matches == nil {
		// this shouldn't happen since we checked above
		return nil
	}
	newTitle := matches[1]
	if newTitle == "" {
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(gce.Body, gce.HTMLURL, user, `Titles may not be empty.`))
	}

	if gce.IsPR {
		pr, err := gc.GetPullRequest(org, repo, number)
		if err != nil {
			return err
		}
		pr.Title = newTitle
		_, err = gc.EditPullRequest(org, repo, number, pr)
		return err
	} else {
		issue, err := gc.GetIssue(org, repo, number)
		if err != nil {
			return err
		}
		issue.Title = newTitle
		_, err = gc.EditIssue(org, repo, number, issue)
		return err
	}
}

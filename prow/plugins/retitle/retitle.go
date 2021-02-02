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
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/invalidcommitmsg"
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

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	var configMsg string
	if config.Retitle.AllowClosedIssues {
		configMsg = "The retitle plugin also allows retitling closed/merged issues and PRs."
	} else {
		configMsg = "The retitle plugin does not allow retitling closed/merged issues and PRs."
	}
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Retitle: plugins.Retitle{
			AllowClosedIssues: true,
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The retitle plugin allows users to re-title pull requests and issues where GitHub permissions don't allow them to.",
		Config: map[string]string{
			"": configMsg,
		},
		Snippet: yamlSnippet,
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
		t := pc.PluginConfig.TriggerFor(org, repo)
		trustedResponse, err := trigger.TrustedUser(pc.GitHubClient, t.OnlyOrgMembers, t.TrustedOrg, user, org, repo)
		return trustedResponse.IsTrusted, err
	}, pc.PluginConfig.Retitle.AllowClosedIssues, pc.Logger, e)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	EditPullRequest(org, repo string, number int, pr *github.PullRequest) (*github.PullRequest, error)
	GetIssue(org, repo string, number int) (*github.Issue, error)
	EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error)
}

func handleGenericComment(gc githubClient, isTrusted func(string) (bool, error), allowClosedIssues bool, log *logrus.Entry, gce github.GenericCommentEvent) error {
	// If closed/merged issues and PRs shouldn't be considered,
	// return early if issue state is not open.
	if !allowClosedIssues && gce.IssueState != "open" {
		return nil
	}

	// Only consider new comments.
	if gce.Action != github.GenericCommentActionCreated {
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
	newTitle := strings.TrimSpace(matches[1])
	if newTitle == "" {
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(gce.Body, gce.HTMLURL, user, `Titles may not be empty.`))
	}

	if invalidcommitmsg.AtMentionRegex.MatchString(newTitle) || invalidcommitmsg.CloseIssueRegex.MatchString(newTitle) {
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(gce.Body, gce.HTMLURL, user, `Titles may not contain [keywords](https://help.github.com/articles/closing-issues-using-keywords) which can automatically close issues and at(@) mentions.`))
	}

	if gce.IsPR {
		pr, err := gc.GetPullRequest(org, repo, number)
		if err != nil {
			return err
		}
		pr.Title = newTitle
		_, err = gc.EditPullRequest(org, repo, number, pr)
		return err
	}
	issue, err := gc.GetIssue(org, repo, number)
	if err != nil {
		return err
	}
	issue.Title = newTitle
	_, err = gc.EditIssue(org, repo, number, issue)
	return err
}

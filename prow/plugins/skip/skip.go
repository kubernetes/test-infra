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

// Package skip implements the `/skip` command which allows users
// to clean up commit statuses of non-blocking presubmits on PRs.
package skip

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "skip"

var (
	skipRe = regexp.MustCompile(`(?mi)^/skip\s*$`)
)

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	CreateStatus(org, repo, ref string, s github.Status) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	ListStatuses(org, repo, ref string) ([]github.Status, error)
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The skip plugin allows users to clean up Github stale commit statuses for non-blocking jobs on a PR.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/skip",
		Description: "Cleans up Github stale commit statuses for non-blocking jobs on a PR.",
		Featured:    false,
		WhoCanUse:   "Anyone can trigger this command on a PR.",
		Examples:    []string{"/skip"},
	})
	return pluginHelp, nil
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e, pc.Config.Presubmits[e.Repo.FullName])
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, presubmits []config.Presubmit) error {
	if !e.IsPR || e.IssueState != "open" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	if !skipRe.MatchString(e.Body) {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	pr, err := gc.GetPullRequest(org, repo, number)
	if err != nil {
		resp := fmt.Sprintf("Cannot get PR #%d in %s/%s: %v", number, org, repo, err)
		log.Warn(resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
	}

	changesFull, err := gc.GetPullRequestChanges(org, repo, number)
	if err != nil {
		resp := fmt.Sprintf("Cannot get changes for PR #%d in %s/%s: %v", number, org, repo, err)
		log.Warn(resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
	}
	var changes []string
	for _, change := range changesFull {
		changes = append(changes, change.Filename)
	}

	statuses, err := gc.ListStatuses(org, repo, pr.Head.SHA)
	if err != nil {
		resp := fmt.Sprintf("Cannot get commit statuses for PR #%d in %s/%s: %v", number, org, repo, err)
		log.Warn(resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
	}

	for _, job := range presubmits {
		// Ignore blocking jobs.
		if !job.SkipReport && job.AlwaysRun {
			continue
		}
		// Ignore jobs that need to run against the PR changes.
		if !job.SkipReport && job.RunIfChanged != "" && job.RunsAgainstChanges(changes) {
			continue
		}
		// Ignore jobs that don't have a status yet.
		if !statusExists(job, statuses) {
			continue
		}
		// Ignore jobs that have a green status.
		if !isNotSuccess(job, statuses) {
			continue
		}
		context := job.Context
		status := github.Status{
			State:       github.StatusSuccess,
			Description: "Skipped",
			Context:     context,
		}
		if err := gc.CreateStatus(org, repo, pr.Head.SHA, status); err != nil {
			resp := fmt.Sprintf("Cannot update PR status for context %s: %v", context, err)
			log.Warn(resp)
			return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
		}
	}
	return nil
}

func statusExists(job config.Presubmit, statuses []github.Status) bool {
	for _, status := range statuses {
		if status.Context == job.Context {
			return true
		}
	}
	return false
}

func isNotSuccess(job config.Presubmit, statuses []github.Status) bool {
	for _, status := range statuses {
		if status.Context == job.Context && status.State != github.StatusSuccess {
			return true
		}
	}
	return false
}

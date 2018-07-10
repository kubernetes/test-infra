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

// Package override supports the /override context command.
package override

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "override"

var (
	overrideRe = regexp.MustCompile(`(?mi)^/override (.+)\s*$`)
)

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	CreateStatus(org, repo, ref string, s github.Status) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	ListStatuses(org, repo, ref string) ([]github.Status, error)
	HasPermission(org, repo, user string, role ...string) (bool, error)
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The override plugin allows repo admins to force a github status context to pass",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/override [context]",
		Description: "Forces a github status context to green (one per line).",
		Featured:    false,
		WhoCanUse:   "Repo administrators",
		Examples:    []string{"/override pull-repo-whatever", "/override continuous-integration/travis\n/override deleted-job"},
	})
	return pluginHelp, nil
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e)
}

func authorized(gc githubClient, log *logrus.Entry, org, repo, user string) bool {
	ok, err := gc.HasPermission(org, repo, user, github.RoleAdmin)
	if err != nil {
		log.Warnf("cannot determine whether %s is an admin of  %s/%s", user, org, repo)
		return false
	}
	return ok
}

func description(user string) string {
	return fmt.Sprintf("Overridden by %s", user)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	if !e.IsPR || e.IssueState != "open" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	mat := overrideRe.FindAllStringSubmatch(e.Body, -1)
	if len(mat) == 0 {
		return nil
	}

	overrides := sets.String{}

	for _, m := range mat {
		overrides.Insert(m[1])
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	user := e.User.Login

	if !authorized(gc, log, org, repo, user) {
		resp := fmt.Sprintf("%s unauthorized: /override is restricted to repo administrators", user)
		log.Warn(resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	pr, err := gc.GetPullRequest(org, repo, number)
	if err != nil {
		resp := fmt.Sprintf("Cannot get PR #%d in %s/%s: %v", number, org, repo, err)
		log.Warn(resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	sha := pr.Head.SHA
	statuses, err := gc.ListStatuses(org, repo, sha)
	if err != nil {
		resp := fmt.Sprintf("Cannot get commit statuses for PR #%d in %s/%s: %v", number, org, repo, err)
		log.Warn(resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	done := sets.String{}

	defer func() {
		if len(done) == 0 {
			return
		}
		msg := fmt.Sprintf("Overrode contexts on behalf of %s: %s", user, strings.Join(done.List(), ", "))
		log.Info(msg)
		gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, msg))
	}()

	for _, status := range statuses {
		if status.State == github.StatusSuccess || !overrides.Has(status.Context) {
			continue
		}
		status.State = github.StatusSuccess
		status.Description = description(user)
		if err := gc.CreateStatus(org, repo, sha, status); err != nil {
			resp := fmt.Sprintf("Cannot update PR status for context %s: %v", status.Context, err)
			log.Warn(resp)
			return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
		}
		done.Insert(status.Context)
	}
	return nil
}

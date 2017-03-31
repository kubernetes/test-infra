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

package assign

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "assign"

var assignRe = regexp.MustCompile(`(?mi)^/(un)?assign(( @[-\w]+?)*)\s*\r?$`)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
	plugins.RegisterIssueHandler(pluginName, handleIssue)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest)
}

type githubClient interface {
	AssignIssue(owner, repo string, number int, logins []string) error
	CreateComment(owner, repo string, number int, comment string) error
	UnassignIssue(owner, repo string, number int, logins []string) error
}

type assignEvent struct {
	action string
	body   string
	login  string
	org    string
	repo   string
	url    string
	number int
}

func handlePullRequest(pc plugins.PluginClient, pr github.PullRequestEvent) error {
	ae := assignEvent{
		action: pr.Action,
		body:   pr.PullRequest.Body,
		login:  pr.PullRequest.User.Login,
		org:    pr.PullRequest.Base.Repo.Owner.Login,
		repo:   pr.PullRequest.Base.Repo.Name,
		url:    pr.PullRequest.HTMLURL,
		number: pr.Number,
	}
	return handle(pc.GitHubClient, pc.Logger, ae)
}

func handleIssue(pc plugins.PluginClient, i github.IssueEvent) error {
	ae := assignEvent{
		action: i.Action,
		body:   i.Issue.Body,
		login:  i.Issue.User.Login,
		org:    i.Repo.Owner.Login,
		repo:   i.Repo.Name,
		url:    i.Issue.HTMLURL,
		number: i.Issue.Number,
	}
	return handle(pc.GitHubClient, pc.Logger, ae)
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	ae := assignEvent{
		action: ic.Action,
		body:   ic.Comment.Body,
		login:  ic.Comment.User.Login,
		org:    ic.Repo.Owner.Login,
		repo:   ic.Repo.Name,
		url:    ic.Comment.HTMLURL,
		number: ic.Issue.Number,
	}
	return handle(pc.GitHubClient, pc.Logger, ae)
}

func parseLogins(text string) []string {
	var parts []string
	for _, p := range strings.Split(text, " ") {
		t := strings.Trim(p, "@ ")
		if t == "" {
			continue
		}
		parts = append(parts, t)
	}
	return parts
}

func handle(gc githubClient, log *logrus.Entry, ae assignEvent) error {
	if ae.action != "created" && ae.action != "opened" { // Only consider new comments/issues
		return nil
	}

	matches := assignRe.FindAllStringSubmatch(ae.body, -1)
	if matches == nil {
		return nil
	}

	assignments := make(map[string]bool)
	for _, re := range matches {

		add := re[1] != "un" // unassign == !add
		if re[2] == "" {
			assignments[ae.login] = add
			continue
		}
		for _, login := range parseLogins(re[2]) {
			assignments[login] = add
		}
	}

	var assign, unassign []string

	for login, add := range assignments {
		if add {
			assign = append(assign, login)
		} else {
			unassign = append(unassign, login)
		}
	}

	if len(unassign) > 0 {
		log.Printf("Removing assignees from %s/%s#%d: %v", ae.org, ae.repo, ae.number, unassign)
		if err := gc.UnassignIssue(ae.org, ae.repo, ae.number, unassign); err != nil {
			return err
		}
	}
	if len(assign) > 0 {
		log.Printf("Adding assignees to %s/%s#%d: %v", ae.org, ae.repo, ae.number, assign)
		if err := gc.AssignIssue(ae.org, ae.repo, ae.number, assign); err != nil {
			if mu, ok := err.(github.MissingUsers); ok {
				msg := fmt.Sprintf("GitHub didn't allow me to assign the following users: %s.\n\nNote that only [%s members](https://github.com/orgs/%s/people) can be assigned.",
					strings.Join(mu, ", "), ae.org, ae.org)
				if e2 := gc.CreateComment(ae.org, ae.repo, ae.number, plugins.FormatResponseRaw(ae.body, ae.url, ae.login, msg)); e2 != nil {
					return fmt.Errorf("comment err: %v", e2)
				}
				return nil
			}
			return err
		}
	}
	return nil
}

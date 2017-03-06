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
}

type githubClient interface {
	AssignIssue(owner, repo string, number int, logins []string) error
	UnassignIssue(owner, repo string, number int, logins []string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, ic)
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

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
	if ic.Action != "created" { // Only consider new comments.
		return nil
	}

	matches := assignRe.FindAllStringSubmatch(ic.Comment.Body, -1)
	if matches == nil {
		return nil
	}

	assignments := make(map[string]bool)
	for _, re := range matches {

		add := re[1] != "un" // unassign == !add
		if re[2] == "" {
			assignments[ic.Comment.User.Login] = add
			continue
		}
		for _, login := range parseLogins(re[2]) {
			assignments[login] = add
		}
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	var assign, unassign []string

	for login, add := range assignments {
		if add {
			assign = append(assign, login)
		} else {
			unassign = append(unassign, login)
		}
	}

	if len(unassign) > 0 {
		log.Printf("Removing assignees from %s/%s#%d: %v", org, repo, number, unassign)
		if err := gc.UnassignIssue(org, repo, number, unassign); err != nil {
			return err
		}
	}
	if len(assign) > 0 {
		log.Printf("Adding assignees to %s/%s#%d: %v", org, repo, number, assign)
		if err := gc.AssignIssue(org, repo, number, assign); err != nil {
			return err
		}
	}
	return nil
}

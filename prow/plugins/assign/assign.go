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

var assignRe = regexp.MustCompile(`(?mi)^/(un)?assign(( @[-\w]+?)*)\r?$`)

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

	re := assignRe.FindStringSubmatch(ic.Comment.Body)
	if re == nil {
		return nil
	}

	var logins []string
	if re[2] == "" {
		logins = append(logins, ic.Comment.User.Login)
	} else {
		logins = parseLogins(re[2])
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	if re[1] == "un" {
		log.Printf("Removing assignees from %s/%s#%d: %v", org, repo, number, logins)
		return gc.UnassignIssue(org, repo, number, logins)
	} else {
		log.Printf("Adding assignees to %s/%s#%d: %v", org, repo, number, logins)
		return gc.AssignIssue(org, repo, number, logins)
	}
}

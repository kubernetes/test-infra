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

package reopen

import (
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "reopen"

var reopenRe = regexp.MustCompile(`(?mi)^/reopen\s*$`)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	ReopenIssue(owner, repo string, number int) error
	ReopenPR(owner, repo string, number int) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, ic)
}

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
	// Only consider closed issues and new comments.
	if ic.Issue.State != "closed" || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	if !reopenRe.MatchString(ic.Comment.Body) {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	commentAuthor := ic.Comment.User.Login

	// Allow assignees and authors to re-open issues.
	if !ic.Issue.IsAuthor(commentAuthor) && !ic.Issue.IsAssignee(commentAuthor) {
		resp := "you can't re-open an issue/PR unless you authored it or you are assigned to it."
		log.Infof("Commenting \"%s\".", resp)
		return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
	}

	if ic.Issue.IsPullRequest() {
		log.Info("Re-opening PR.")
		return gc.ReopenPR(org, repo, number)
	}

	log.Info("Re-opening issue.")
	return gc.ReopenIssue(org, repo, number)
}

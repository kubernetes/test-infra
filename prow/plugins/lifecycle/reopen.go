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

package lifecycle

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

var reopenRe = regexp.MustCompile(`(?mi)^/reopen\s*$`)

type githubClient interface {
	IsCollaborator(owner, repo, login string) (bool, error)
	CreateComment(owner, repo string, number int, comment string) error
	ReopenIssue(owner, repo string, number int) error
	ReopenPR(owner, repo string, number int) error
}

func handleReopen(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only consider closed issues and new comments.
	if e.IssueState != "closed" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	if !reopenRe.MatchString(e.Body) {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	commentAuthor := e.User.Login

	isAuthor := e.IssueAuthor.Login == commentAuthor
	isCollaborator, err := gc.IsCollaborator(org, repo, commentAuthor)
	if err != nil {
		log.WithError(err).Errorf("Failed IsCollaborator(%s, %s, %s)", org, repo, commentAuthor)
	}

	// Only authors and collaborators are allowed to reopen issues or PRs.
	if !isAuthor && !isCollaborator {
		response := "You can't reopen an issue/PR unless you authored it or you are a collaborator."
		log.Infof("Commenting \"%s\".", response)
		return gc.CreateComment(
			org,
			repo,
			number,
			plugins.FormatResponseRaw(e.Body, e.HTMLURL, commentAuthor, response),
		)
	}

	if e.IsPR {
		log.Info("/reopen PR")
		if err := gc.ReopenPR(org, repo, number); err != nil {
			if scbc, ok := err.(github.StateCannotBeChanged); ok {
				resp := fmt.Sprintf("Failed to re-open PR: %v", scbc)
				return gc.CreateComment(
					org,
					repo,
					number,
					plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp),
				)
			}
			return err
		}
		// Add a comment after reopening the PR to leave an audit trail of who
		// asked to reopen it.
		return gc.CreateComment(
			org,
			repo,
			number,
			plugins.FormatResponseRaw(e.Body, e.HTMLURL, commentAuthor, "Reopened this PR."),
		)
	}

	log.Info("/reopen issue")
	if err := gc.ReopenIssue(org, repo, number); err != nil {
		if scbc, ok := err.(github.StateCannotBeChanged); ok {
			resp := fmt.Sprintf("Failed to re-open Issue: %v", scbc)
			return gc.CreateComment(
				org,
				repo,
				number,
				plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp),
			)
		}
		return err
	}
	// Add a comment after reopening the issue to leave an audit trail of who
	// asked to reopen it.
	return gc.CreateComment(
		org,
		repo,
		number,
		plugins.FormatResponseRaw(e.Body, e.HTMLURL, commentAuthor, "Reopened this issue."),
	)
}

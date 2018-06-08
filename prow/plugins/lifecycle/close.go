/*
Copyright 2016 The Kubernetes Authors.

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

var closeRe = regexp.MustCompile(`(?mi)^/close\s*$`)

type closeClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	CloseIssue(owner, repo string, number int) error
	ClosePR(owner, repo string, number int) error
	IsMember(owner, login string) (bool, error)
	AssignIssue(owner, repo string, number int, assignees []string) error
	GetIssueLabels(owner, repo string, number int) ([]github.Label, error)
}

func isActive(gc closeClient, org, repo string, number int) (bool, error) {
	labels, err := gc.GetIssueLabels(org, repo, number)
	if err != nil {
		return true, fmt.Errorf("list issue labels error: %v", err)
	}
	for _, label := range []string{"lifecycle/stale", "lifecycle/rotten"} {
		if github.HasLabel(label, labels) {
			return false, nil
		}
	}
	return true, nil
}

func handleClose(gc closeClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only consider open issues and new comments.
	if e.IssueState != "open" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	if !closeRe.MatchString(e.Body) {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	commentAuthor := e.User.Login

	// Allow assignees and authors to close issues.
	isAssignee := false
	for _, assignee := range e.Assignees {
		if commentAuthor == assignee.Login {
			isAssignee = true
			break
		}
	}
	isAuthor := e.IssueAuthor.Login == commentAuthor

	if !isAssignee && !isAuthor {
		active, err := isActive(gc, org, repo, number)
		if err != nil {
			log.Infof("Cannot determine if issue is active: %v", err)
			active = true // Fail active
		}

		if active {
			// Try to assign the issue to the comment author
			log.Infof("Assign to %s", commentAuthor)
			if err := gc.AssignIssue(org, repo, number, []string{commentAuthor}); err != nil {
				msg := "Assigning you to the issue failed."
				if ok, merr := gc.IsMember(org, commentAuthor); merr == nil && !ok {
					msg = "Can only assign issues to org members and/or repo collaborators."
				} else if merr != nil {
					log.WithError(merr).Errorf("Failed IsMember(%s, %s)", org, commentAuthor)
				} else {
					log.WithError(err).Errorf("Failed AssignIssue(%s, %s, %d, %s)", org, repo, number, commentAuthor)
				}
				resp := fmt.Sprintf("you can't close an active issue unless you authored it or you are assigned to it, %s.", msg)
				log.Infof("Commenting \"%s\".", resp)
				return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, commentAuthor, resp))
			}
		}
	}

	if e.IsPR {
		log.Info("Closing PR.")
		return gc.ClosePR(org, repo, number)
	}

	log.Info("Closing issue.")
	return gc.CloseIssue(org, repo, number)
}

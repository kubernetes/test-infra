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

package lgtm

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "lgtm"

var (
	lgtmLabel    = "lgtm"
	lgtmRe       = regexp.MustCompile(`(?mi)^/lgtm(?: no-issue)?\s*$`)
	lgtmCancelRe = regexp.MustCompile(`(?mi)^/lgtm cancel\s*$`)
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, nil)
}

type githubClient interface {
	IsMember(owner, login string) (bool, error)
	AddLabel(owner, repo string, number int, label string) error
	AssignIssue(owner, repo string, number int, assignees []string) error
	CreateComment(owner, repo string, number int, comment string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only consider open PRs and new comments.
	if !e.IsPR || e.IssueState != "open" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	// If we create an "/lgtm" comment, add lgtm if necessary.
	// If we create a "/lgtm cancel" comment, remove lgtm if necessary.
	wantLGTM := false
	if lgtmRe.MatchString(e.Body) {
		wantLGTM = true
	} else if lgtmCancelRe.MatchString(e.Body) {
		wantLGTM = false
	} else {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	commentAuthor := e.User.Login

	// Allow authors to cancel LGTM. Do not allow authors to LGTM, and do not
	// accept commands from any other user.
	isAssignee := false
	for _, assignee := range e.Assignees {
		if assignee.Login == e.User.Login {
			isAssignee = true
			break
		}
	}
	isAuthor := e.User.Login == e.IssueAuthor.Login
	if isAuthor && wantLGTM {
		resp := "you cannot LGTM your own PR."
		log.Infof("Commenting with \"%s\".", resp)
		return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, commentAuthor, resp))
	} else if !isAuthor && !isAssignee {
		log.Infof("Assigning %s/%s#%d to %s", org, repo, e.Number, commentAuthor)
		if err := gc.AssignIssue(org, repo, e.Number, []string{commentAuthor}); err != nil {
			msg := "assigning you to the PR failed"
			if ok, merr := gc.IsMember(org, commentAuthor); merr == nil && !ok {
				msg = fmt.Sprintf("only %s org members may be assigned issues", org)
			} else if merr != nil {
				log.WithError(merr).Errorf("Failed IsMember(%s, %s)", org, commentAuthor)
			} else {
				log.WithError(err).Errorf("Failed AssignIssue(%s, %s, %d, %s)", org, repo, e.Number, commentAuthor)
			}
			resp := "changing LGTM is restricted to assignees, and " + msg + "."
			log.Infof("Reply to assign via /lgtm request with comment: \"%s\"", resp)
			return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, commentAuthor, resp))
		}
	}

	// Only add the label if it doesn't have it, and vice versa.
	hasLGTM := false
	labels, err := gc.GetIssueLabels(org, repo, e.Number)
	if err != nil {
		log.WithError(err).Errorf("Failed to get the labels on %s/%s#%d.", org, repo, e.Number)
	}
	for _, candidate := range labels {
		if candidate.Name == lgtmLabel {
			hasLGTM = true
			break
		}
	}
	if hasLGTM && !wantLGTM {
		log.Info("Removing LGTM label.")
		return gc.RemoveLabel(org, repo, e.Number, lgtmLabel)
	} else if !hasLGTM && wantLGTM {
		log.Info("Adding LGTM label.")
		return gc.AddLabel(org, repo, e.Number, lgtmLabel)
	}
	return nil
}

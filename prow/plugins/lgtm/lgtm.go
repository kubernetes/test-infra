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
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "lgtm"

var (
	lgtmLabel    = "lgtm"
	lgtmRe       = regexp.MustCompile(`(?mi)^\/lgtm\r?$`)
	lgtmCancelRe = regexp.MustCompile(`(?mi)^\/lgtm cancel\r?$`)
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
}

type githubClient interface {
	IsMember(owner, login string) (bool, error)
	AddLabel(owner, repo string, number int, label string) error
	AssignIssue(owner, repo string, number int, assignees []string) error
	CreateComment(owner, repo string, number int, comment string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, ic)
}

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
	// Only consider open PRs.
	if !ic.Issue.IsPullRequest() || ic.Issue.State != "open" || ic.Action != "created" {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	// If we create an "/lgtm" comment, add lgtm if necessary.
	// If we create a "/lgtm cancel" comment, remove lgtm if necessary.
	wantLGTM := false
	if lgtmRe.MatchString(ic.Comment.Body) {
		wantLGTM = true
	} else if lgtmCancelRe.MatchString(ic.Comment.Body) {
		wantLGTM = false
	} else {
		return nil
	}

	// Allow authors to cancel LGTM. Do not allow authors to LGTM, and do not
	// accept commands from any other user.
	commentAuthor := ic.Comment.User.Login
	isAssignee := ic.Issue.IsAssignee(commentAuthor)
	isAuthor := ic.Issue.IsAuthor(commentAuthor)
	if isAuthor && wantLGTM {
		resp := "you cannot LGTM your own PR"
		log.Infof("Commenting with \"%s\".", resp)
		return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
	} else if !isAuthor && !isAssignee {
		log.Infof("Assigning %s/%s#%d to %s", org, repo, number, commentAuthor)
		if err := gc.AssignIssue(org, repo, number, []string{commentAuthor}); err != nil {
			msg := "assigning you to the PR failed"
			if ok, merr := gc.IsMember(org, commentAuthor); merr == nil && !ok {
				msg = "only kubernetes org members may be assigned issues"
			} else if merr != nil {
				log.WithError(merr).Errorf("Failed IsMember(%s, %s)", org, commentAuthor)
			} else {
				log.WithError(err).Errorf("Failed AssignIssue(%s, %s, %d, %s)", org, repo, number, commentAuthor)
			}
			resp := "changing LGTM is restricted to assignees, and " + msg
			log.Infof("Reply to assign via /lgtm request with comment: \"%s\"", resp)
			return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
		}
	}

	// Only add the label if it doesn't have it, and vice versa.
	hasLGTM := ic.Issue.HasLabel(lgtmLabel)
	if hasLGTM && !wantLGTM {
		log.Info("Removing LGTM label.")
		return gc.RemoveLabel(org, repo, number, lgtmLabel)
	} else if !hasLGTM && wantLGTM {
		log.Info("Adding LGTM label.")
		return gc.AddLabel(org, repo, number, lgtmLabel)
	}
	return nil
}

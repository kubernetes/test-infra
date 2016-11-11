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
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
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
		resp := "you can't LGTM your own PR"
		log.Infof("Commenting with \"%s\".", resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponse(ic.Comment, resp))
	} else if !isAuthor {
		if !isAssignee && wantLGTM {
			resp := "you can't LGTM a PR unless you are assigned as a reviewer"
			log.Infof("Commenting with \"%s\".", resp)
			return gc.CreateComment(org, repo, number, plugins.FormatResponse(ic.Comment, resp))
		} else if !isAssignee && !wantLGTM {
			resp := "you can't remove LGTM from a PR unless you are assigned as a reviewer"
			log.Infof("Commenting with \"%s\".", resp)
			return gc.CreateComment(org, repo, number, plugins.FormatResponse(ic.Comment, resp))
		}
	}

	// Only add the label if it doesn't have it, and vice versa.
	hasLGTM := issueHasLabel(ic.Issue, lgtmLabel)
	if hasLGTM && !wantLGTM {
		log.Info("Removing LGTM label.")
		return gc.RemoveLabel(org, repo, number, lgtmLabel)
	} else if !hasLGTM && wantLGTM {
		log.Info("Adding LGTM label.")
		return gc.AddLabel(org, repo, number, lgtmLabel)
	}
	return nil
}

func issueHasLabel(i github.Issue, label string) bool {
	for _, label := range i.Labels {
		if label.Name == lgtmLabel {
			return true
		}
	}
	return false
}

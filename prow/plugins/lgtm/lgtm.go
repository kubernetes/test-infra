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

	"k8s.io/test-infra/prow/github"
)

const PluginName = "lgtm"

var (
	lgtmLabel    = "lgtm"
	lgtmRe       = regexp.MustCompile(`(?mi)^\/lgtm\r?$`)
	lgtmCancelRe = regexp.MustCompile(`(?mi)^\/lgtm cancel\r?$`)
)

type GitHubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

// HandleIssueComment adds or remove LGTM when reviewers comment "/lgtm" or
// "/lgtm cancel".
func HandleIssueComment(gc GitHubClient, ic github.IssueCommentEvent) error {
	// Only consider open PRs.
	if ic.Issue.PullRequest == nil || ic.Issue.State != "open" || ic.Action != "created" {
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
	isAssignee := isIssueAssignee(ic.Issue, ic.Comment.User.Login)
	isAuthor := ic.Comment.User.Login == ic.Issue.User.Login
	if isAuthor && wantLGTM {
		return gc.CreateComment(org, repo, number, fmt.Sprintf("@%s: you can't LGTM your own PR.", ic.Comment.User.Login))
	} else if !isAuthor {
		if !isAssignee && wantLGTM {
			return gc.CreateComment(org, repo, number, fmt.Sprintf("@%s: you can't LGTM a PR unless you are assigned as a reviewer.", ic.Comment.User.Login))
		} else if !isAssignee && !wantLGTM {
			return gc.CreateComment(org, repo, number, fmt.Sprintf("@%s: you can't remove LGTM from a PR unless you are assigned as a reviewer.", ic.Comment.User.Login))
		}
	}

	// Only add the label if it doesn't have it, and vice versa.
	hasLGTM := issueHasLabel(ic.Issue, lgtmLabel)
	if hasLGTM && !wantLGTM {
		return gc.RemoveLabel(org, repo, number, lgtmLabel)
	} else if !hasLGTM && wantLGTM {
		return gc.AddLabel(org, repo, number, lgtmLabel)
	}
	return nil
}

func isIssueAssignee(i github.Issue, user string) bool {
	for _, assignee := range i.Assignees {
		if user == assignee.Login {
			return true
		}
	}
	return false
}

func issueHasLabel(i github.Issue, label string) bool {
	for _, label := range i.Labels {
		if label.Name == lgtmLabel {
			return true
		}
	}
	return false
}

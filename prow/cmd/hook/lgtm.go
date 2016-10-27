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

package main

import (
	"fmt"
	"regexp"

	"k8s.io/test-infra/prow/github"
)

var (
	lgtmRe       = regexp.MustCompile(`(?mi)^\/lgtm\r?$`)
	lgtmCancelRe = regexp.MustCompile(`(?mi)^\/lgtm cancel\r?$`)
)

// Add or remove LGTM when reviewers comment "/lgtm" or "/lgtm cancel".
func (ga *GitHubAgent) lgtmComment(ic github.IssueCommentEvent) error {
	// Only consider open PRs.
	if ic.Issue.PullRequest == nil || ic.Issue.State != "open" {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	// If we create or edit a "/lgtm" comment, add lgtm if necessary.
	// If we delete a "/lgtm" comment, remove lgtm if necessary.
	// If we create or edit a "/lgtm cancel" comment, remove lgtm if necessary.
	act := false
	wantLGTM := false
	if lgtmRe.MatchString(ic.Comment.Body) {
		act = true
		wantLGTM = ic.Action == "created" || ic.Action == "edited"
	} else if lgtmCancelRe.MatchString(ic.Comment.Body) {
		act = ic.Action == "created" || ic.Action == "edited"
		wantLGTM = false
	}
	if !act {
		return nil
	}

	isAssignee := false
	for _, assignee := range ic.Issue.Assignees {
		if ic.Comment.User.Login == assignee.Login {
			isAssignee = true
			break
		}
	}
	isAuthor := ic.Comment.User.Login == ic.Issue.User.Login
	// Allow authors to cancel LGTM. Do not allow authors to LGTM, and do not
	// accept commands from any other user.
	if isAuthor && wantLGTM {
		return ga.GitHubClient.CreateComment(org, repo, number, fmt.Sprintf("@%s: you can't LGTM your own PR.", ic.Comment.User.Login))
	} else if !isAuthor {
		if !isAssignee && wantLGTM {
			return ga.GitHubClient.CreateComment(org, repo, number, fmt.Sprintf("@%s: you can't LGTM a PR unless you are assigned as a reviewer.", ic.Comment.User.Login))
		} else if !isAssignee && !wantLGTM {
			return ga.GitHubClient.CreateComment(org, repo, number, fmt.Sprintf("@%s: you can't remove LGTM from a PR unless you are assigned as a reviewer.", ic.Comment.User.Login))
		}
	}

	hasLGTM := false
	for _, label := range ic.Issue.Labels {
		if label.Name == lgtmLabel {
			hasLGTM = true
			break
		}
	}

	if hasLGTM && !wantLGTM {
		return ga.GitHubClient.RemoveLabel(org, repo, number, lgtmLabel)
	} else if !hasLGTM && wantLGTM {
		return ga.GitHubClient.AddLabel(org, repo, number, lgtmLabel)
	}
	return nil
}

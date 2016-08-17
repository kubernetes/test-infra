/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package mungers

import (
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers/mungerutil"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// LGTMHandler will
// - apply LGTM label if reviewer has commented "/lgtm", or
// - remove LGTM label if reviewer has commented "/lgtm cancel"
type LGTMHandler struct{}

func init() {
	l := LGTMHandler{}
	RegisterMungerOrDie(l)
}

// Name is the name usable in --pr-mungers
func (LGTMHandler) Name() string { return "lgtm-handler" }

// RequiredFeatures is a slice of 'features' that must be provided
func (LGTMHandler) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (LGTMHandler) Initialize(config *github.Config, features *features.Features) error { return nil }

// EachLoop is called at the start of every munge loop
func (LGTMHandler) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (LGTMHandler) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (h LGTMHandler) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	reviewers := getReviewers(obj)
	if len(reviewers) == 0 {
		return
	}

	comments, err := getComments(obj)
	if err != nil {
		glog.Errorf("unexpected error getting comments: %v", err)
		return
	}

	if !obj.HasLabel(lgtmLabel) {
		h.addLGTMIfCommented(obj, comments, reviewers)
		return
	}
	h.removeLGTMIfCancelled(obj, comments, reviewers)
}

func (h *LGTMHandler) addLGTMIfCommented(obj *github.MungeObject, comments []*githubapi.IssueComment, reviewers mungerutil.UserSet) {
	// Assumption: The comments should be sorted (by default from github api) from oldest to latest
	for i := len(comments) - 1; i >= 0; i-- {
		comment := comments[i]
		if !mungerutil.IsValidUser(comment.User) {
			continue
		}

		// TODO: An approver should be acceptable.
		// See https://github.com/kubernetes/contrib/pull/1428#discussion_r72563935
		if !mungerutil.IsMungeBot(comment.User) && !isReviewer(comment.User, reviewers) {
			continue
		}

		fields := getFields(*comment.Body)
		if isCancelComment(fields) {
			// "/lgtm cancel" if commented more recently than "/lgtm"
			return
		}

		if !isLGTMComment(fields) {
			continue
		}

		// TODO: support more complex policies for multiple reviewers.
		// See https://github.com/kubernetes/contrib/issues/1389#issuecomment-235161164
		glog.Infof("Adding lgtm label. Reviewer (%s) LGTM", *comment.User.Login)
		obj.AddLabel(lgtmLabel)
		return
	}
}

func (h *LGTMHandler) removeLGTMIfCancelled(obj *github.MungeObject, comments []*githubapi.IssueComment, reviewers mungerutil.UserSet) {
	for i := len(comments) - 1; i >= 0; i-- {
		comment := comments[i]
		if !mungerutil.IsValidUser(comment.User) {
			continue
		}

		if !mungerutil.IsMungeBot(comment.User) && !isReviewer(comment.User, reviewers) {
			continue
		}

		fields := getFields(*comment.Body)
		if isLGTMComment(fields) {
			// "/lgtm" if commented more recently than "/lgtm cancel"
			return
		}

		if !isCancelComment(fields) {
			continue
		}

		glog.Infof("Removing lgtm label. Reviewer (%s) cancelled", *comment.User.Login)
		obj.RemoveLabel(lgtmLabel)
		return
	}
}

func isLGTMComment(fields []string) bool {
	// Note: later we'd probably move all the bot-command parsing code to its own package.
	return len(fields) == 1 && strings.ToLower(fields[0]) == "/lgtm"
}

func isCancelComment(fields []string) bool {
	return len(fields) == 2 && strings.ToLower(fields[0]) == "/lgtm" && strings.ToLower(fields[1]) == "cancel"
}

func getReviewers(obj *github.MungeObject) mungerutil.UserSet {
	// Note: assuming assignees are reviewers
	allAssignees := append(obj.Issue.Assignees, obj.Issue.Assignee)
	return mungerutil.GetUsers(allAssignees...)
}

func getComments(obj *github.MungeObject) ([]*githubapi.IssueComment, error) {
	afterLastModified := func(opt *githubapi.IssueListCommentsOptions) *githubapi.IssueListCommentsOptions {
		// Only comments updated at or after this time are returned.
		// One possible case is that reviewer might "/lgtm" first, contributor updated PR, and reviewer updated "/lgtm".
		// This is still valid. We don't recommend user to update it.
		lastModified := *obj.LastModifiedTime()
		opt.Since = lastModified
		return opt
	}
	return obj.ListComments(afterLastModified)
}

func isReviewer(user *githubapi.User, reviewers mungerutil.UserSet) bool {
	return reviewers.Has(user)
}

// getFields will move to a different package where we do command
// parsing in the near future.
func getFields(commentBody string) []string {
	// remove the comment portion if present and read the command.
	cmd := strings.Split(commentBody, "//")[0]
	strings.TrimSpace(cmd)
	return strings.Fields(cmd)
}

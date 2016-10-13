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

package fsm

import (
	"time"

	"k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers/matchers/comment"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

// NeedsReview is the state when the ball is in the reviewer's court.
type NeedsReview struct{}

var _ State = &NeedsReview{}

const (
	lgtmLabel = "lgtm"
)

// Process does the necessary processing to compute whether to stay in
// this state, or proceed to the next.
func (nr *NeedsReview) Process(obj *github.MungeObject) (State, error) {
	if nr.checkLGTM(obj) {
		if obj.HasLabel(labelNeedsReview) {
			obj.RemoveLabel(labelNeedsReview)
		}
		if obj.HasLabel(labelChangesNeeded) {
			obj.RemoveLabel(labelChangesNeeded)
		}
		return &End{}, nil
	}

	reviewerActionNeeded, err := isReviewerActionNeeded(obj)
	if err != nil {
		return &End{}, err
	}

	if !reviewerActionNeeded {
		if obj.HasLabel(labelNeedsReview) {
			obj.RemoveLabel(labelNeedsReview)
		}
		return &ChangesNeeded{}, nil
	}

	if !obj.HasLabel(labelNeedsReview) {
		glog.Infof("PR #%v needs reviewer action", *obj.Issue.Number)
		obj.AddLabel(labelNeedsReview)
	}
	return &End{}, nil
}

func (nr *NeedsReview) checkLGTM(obj *github.MungeObject) bool {
	return obj.HasLabel(lgtmLabel)
}

// assigneeActionNeeded returns true if we are waiting on an action from the reviewer.
func isReviewerActionNeeded(obj *github.MungeObject) (bool, error) {
	comments, err := obj.ListComments()
	if err != nil {
		return false, err
	}

	lastAuthorCommentTime := comment.LastComment(comments, comment.Author(*obj.Issue.User), nil)
	lastReviewerCommentTime := getLastReviewerComment(obj, comments)

	if lastReviewerCommentTime == nil {
		// this implies that no reviewer has commented on the PR yet.
		return true, nil
	}

	if obj.LastModifiedTime().After(*lastReviewerCommentTime) {
		return true, nil
	}

	if lastAuthorCommentTime == nil {
		return false, nil
	}

	return lastReviewerCommentTime.Before(*lastAuthorCommentTime), nil
}

func getLastReviewerComment(obj *github.MungeObject, comments []*githubapi.IssueComment) *time.Time {
	var lastCommentTime *time.Time
	for _, reviewer := range obj.Issue.Assignees {
		lastReviewerCommentTime := comment.LastComment(comments, comment.Author(*reviewer), nil)
		if lastReviewerCommentTime == nil {
			continue
		}
		if lastCommentTime != nil && lastReviewerCommentTime.Before(*lastCommentTime) {
			continue
		}
		lastCommentTime = lastReviewerCommentTime
	}
	return lastCommentTime
}

// Name is the name of the state machine's state.
func (nr *NeedsReview) Name() string {
	return "NeedsReview"
}

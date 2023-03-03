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
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type fakeClientClose struct {
	commented      bool
	closed         bool
	stateReason    string
	AssigneesAdded []string
	labels         []string
}

func (c *fakeClientClose) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = true
	return nil
}

func (c *fakeClientClose) CloseIssue(owner, repo string, number int) error {
	c.closed = true
	c.stateReason = "completed"
	return nil
}

func (c *fakeClientClose) CloseIssueAsNotPlanned(org, repo string, number int) error {
	c.closed = true
	c.stateReason = "not_planned"
	return nil
}

func (c *fakeClientClose) ClosePR(owner, repo string, number int) error {
	c.closed = true
	return nil
}

func (c *fakeClientClose) IsCollaborator(owner, repo, login string) (bool, error) {
	if login == "collaborator" {
		return true, nil
	}
	return false, nil
}

func (c *fakeClientClose) GetIssueLabels(owner, repo string, number int) ([]github.Label, error) {
	var labels []github.Label
	for _, l := range c.labels {
		if l == "error" {
			return nil, errors.New("issue label 500")
		}
		labels = append(labels, github.Label{Name: l})
	}
	return labels, nil
}

func TestCloseComment(t *testing.T) {
	var testcases = []struct {
		name          string
		action        github.GenericCommentEventAction
		state         string
		stateReason   string
		body          string
		commenter     string
		labels        []string
		shouldClose   bool
		shouldComment bool
		isPr          bool
	}{
		{
			name:          "non-close comment",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "uh oh",
			commenter:     "random-person",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by author",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "completed",
			body:          "/close",
			commenter:     "author",
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close by author, trailing space.",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "completed",
			body:          "/close \r",
			commenter:     "author",
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close by collaborator",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "completed",
			body:          "/close",
			commenter:     "collaborator",
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close edited by author",
			action:        github.GenericCommentActionEdited,
			state:         "open",
			body:          "/close",
			commenter:     "author",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by author on closed issue",
			action:        github.GenericCommentActionCreated,
			state:         "closed",
			body:          "/close",
			commenter:     "author",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by non-collaborator on active issue, cannot close",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-collaborator",
			shouldClose:   false,
			shouldComment: true,
		},
		{
			name:          "close by non-collaborator on stale issue",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "completed",
			body:          "/close",
			commenter:     "non-collaborator",
			labels:        []string{"lifecycle/stale"},
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close by non-collaborator on rotten issue",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "completed",
			body:          "/close",
			commenter:     "non-collaborator",
			labels:        []string{"lifecycle/rotten"},
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "cannot close stale issue by non-collaborator when list issue fails",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-collaborator",
			labels:        []string{"error"},
			shouldClose:   false,
			shouldComment: true,
		},
		{
			name:          "close by author as not planned",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "not_planned",
			body:          "/close not-planned",
			commenter:     "author",
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close by author as not planned, trailing space.",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "not_planned",
			body:          "/close not-planned \r",
			commenter:     "author",
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close by author, multiple spaces in between.",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "not_planned",
			body:          "/close not-planned",
			commenter:     "author",
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close as not planned by non-collaborator on active issue, cannot close",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close not-planned",
			commenter:     "non-collaborator",
			shouldClose:   false,
			shouldComment: true,
		},
		{
			name:          "close as not planned by non-collaborator on stale issue",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "not_planned",
			body:          "/close not-planned",
			commenter:     "non-collaborator",
			labels:        []string{"lifecycle/stale"},
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "close as not planned by non-collaborator on rotten issue",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			stateReason:   "not_planned",
			body:          "/close not-planned",
			commenter:     "non-collaborator",
			labels:        []string{"lifecycle/rotten"},
			shouldClose:   true,
			shouldComment: true,
		},
		{
			name:          "cannot close stale issue as not planned by non-collaborator when list issue fails",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close not-planned",
			commenter:     "non-collaborator",
			labels:        []string{"error"},
			shouldClose:   false,
			shouldComment: true,
		},
		{
			name:          "cannot close PR as not planned",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close not-planned",
			commenter:     "author",
			labels:        []string{"error"},
			shouldClose:   false,
			shouldComment: true,
			isPr:          true,
		},
	}
	for _, tc := range testcases {
		fc := &fakeClientClose{labels: tc.labels}
		e := &github.GenericCommentEvent{
			Action:      tc.action,
			IssueState:  tc.state,
			Body:        tc.body,
			User:        github.User{Login: tc.commenter},
			Number:      5,
			IssueAuthor: github.User{Login: "author"},
			IsPR:        tc.isPr,
		}
		if err := handleClose(fc, logrus.WithField("plugin", "fake-close"), e); err != nil {
			t.Errorf("For case %s, didn't expect error from handle: %v", tc.name, err)
			continue
		}
		if tc.shouldClose && !fc.closed {
			t.Errorf("For case %s, should have closed but didn't.", tc.name)
		} else if !tc.shouldClose && fc.closed {
			t.Errorf("For case %s, should not have closed but did.", tc.name)
		}
		if tc.shouldComment && !fc.commented {
			t.Errorf("For case %s, should have commented but didn't.", tc.name)
		} else if !tc.shouldComment && fc.commented {
			t.Errorf("For case %s, should not have commented but did.", tc.name)
		}
		if !tc.isPr && fc.stateReason != tc.stateReason {
			t.Errorf("For case %s, unexpected state_reason value, expected %s, but got %s", tc.name, tc.stateReason, fc.stateReason)
		}
	}
}

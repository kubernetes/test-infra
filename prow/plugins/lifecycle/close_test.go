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
	AssigneesAdded []string
	labels         []string
}

func (c *fakeClientClose) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = true
	return nil
}

func (c *fakeClientClose) CloseIssue(owner, repo string, number int) error {
	c.closed = true
	return nil
}

func (c *fakeClientClose) ClosePR(owner, repo string, number int) error {
	c.closed = true
	return nil
}

func (c *fakeClientClose) IsMember(owner, login string) (bool, error) {
	if login == "non-member" {
		return false, nil
	}
	return true, nil
}

func (c *fakeClientClose) AssignIssue(owner, repo string, number int, assignees []string) error {
	if assignees[0] == "non-member" || assignees[0] == "non-owner-assign-error" {
		return errors.New("Failed to assign")
	}
	c.AssigneesAdded = append(c.AssigneesAdded, assignees...)
	return nil
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
	// "a" is the author, "r1", and "r2" are reviewers.
	var testcases = []struct {
		name          string
		action        github.GenericCommentEventAction
		state         string
		body          string
		commenter     string
		labels        []string
		shouldClose   bool
		shouldComment bool
		shouldAssign  bool
	}{
		{
			name:          "non-close comment",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "uh oh",
			commenter:     "o",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by author",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "a",
			shouldClose:   true,
			shouldComment: false,
		},
		{
			name:          "close by author, trailing space.",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close \r",
			commenter:     "a",
			shouldClose:   true,
			shouldComment: false,
		},
		{
			name:          "close by reviewer",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "r1",
			shouldClose:   true,
			shouldComment: false,
		},
		{
			name:          "close edited by author",
			action:        github.GenericCommentActionEdited,
			state:         "open",
			body:          "/close",
			commenter:     "a",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by author on closed issue",
			action:        github.GenericCommentActionCreated,
			state:         "closed",
			body:          "/close",
			commenter:     "a",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by other person, non-member cannot close",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-member",
			shouldClose:   false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:          "close by other person, failed to assign",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-owner-assign-error",
			shouldClose:   false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:          "close by other person, assign and close",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-owner",
			shouldClose:   true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "close by other person, stale issue",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-member",
			labels:        []string{"lifecycle/stale"},
			shouldClose:   true,
			shouldComment: false,
			shouldAssign:  false,
		},
		{
			name:          "close by other person, rotten issue",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-member",
			labels:        []string{"lifecycle/rotten"},
			shouldClose:   true,
			shouldComment: false,
			shouldAssign:  false,
		},
		{
			name:          "cannot close stale issue by other person when list issue fails",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/close",
			commenter:     "non-member",
			labels:        []string{"error"},
			shouldClose:   false,
			shouldComment: true,
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
			Assignees:   []github.User{{Login: "a"}, {Login: "r1"}, {Login: "r2"}},
			IssueAuthor: github.User{Login: "a"},
		}
		if err := handleClose(fc, logrus.WithField("plugin", "fake-close"), e, false); err != nil {
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
		if tc.shouldAssign && len(fc.AssigneesAdded) != 1 {
			t.Errorf("For case %s, should have assigned but didn't.", tc.name)
		} else if !tc.shouldAssign && len(fc.AssigneesAdded) == 1 {
			t.Errorf("For case %s, should not have assigned but did.", tc.name)
		}
	}
}

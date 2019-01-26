/*
Copyright 2017 The Kubernetes Authors.

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
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type fakeClientReopen struct {
	commented bool
	open      bool
}

func (c *fakeClientReopen) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = true
	return nil
}

func (c *fakeClientReopen) ReopenIssue(owner, repo string, number int) error {
	c.open = true
	return nil
}

func (c *fakeClientReopen) ReopenPR(owner, repo string, number int) error {
	c.open = true
	return nil
}

func (c *fakeClientReopen) IsCollaborator(owner, repo, login string) (bool, error) {
	if login == "collaborator" {
		return true, nil
	}
	return false, nil
}

func TestReopenComment(t *testing.T) {
	var testcases = []struct {
		name          string
		action        github.GenericCommentEventAction
		state         string
		body          string
		commenter     string
		shouldReopen  bool
		shouldComment bool
	}{
		{
			name:          "non-open comment",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "does not matter",
			commenter:     "random-person",
			shouldReopen:  false,
			shouldComment: false,
		},
		{
			name:          "re-open by author",
			action:        github.GenericCommentActionCreated,
			state:         "closed",
			body:          "/reopen",
			commenter:     "author",
			shouldReopen:  true,
			shouldComment: true,
		},
		{
			name:          "re-open by collaborator",
			action:        github.GenericCommentActionCreated,
			state:         "closed",
			body:          "/reopen",
			commenter:     "collaborator",
			shouldReopen:  true,
			shouldComment: true,
		},
		{
			name:          "re-open by collaborator, trailing space.",
			action:        github.GenericCommentActionCreated,
			state:         "closed",
			body:          "/reopen \r",
			commenter:     "collaborator",
			shouldReopen:  true,
			shouldComment: true,
		},
		{
			name:          "re-open edited by author",
			action:        github.GenericCommentActionEdited,
			state:         "closed",
			body:          "/reopen",
			commenter:     "author",
			shouldReopen:  false,
			shouldComment: false,
		},
		{
			name:          "open by author on already open issue",
			action:        github.GenericCommentActionCreated,
			state:         "open",
			body:          "/reopen",
			commenter:     "author",
			shouldReopen:  false,
			shouldComment: false,
		},
		{
			name:          "re-open by non-collaborator, cannot reopen",
			action:        github.GenericCommentActionCreated,
			state:         "closed",
			body:          "/reopen",
			commenter:     "non-collaborator",
			shouldReopen:  false,
			shouldComment: true,
		},
	}
	for _, tc := range testcases {
		fc := &fakeClientReopen{}
		e := &github.GenericCommentEvent{
			Action:      tc.action,
			IssueState:  tc.state,
			Body:        tc.body,
			User:        github.User{Login: tc.commenter},
			Number:      5,
			IssueAuthor: github.User{Login: "author"},
		}
		if err := handleReopen(fc, logrus.WithField("plugin", "fake-reopen"), e); err != nil {
			t.Errorf("For case %s, didn't expect error from handle: %v", tc.name, err)
			continue
		}
		if tc.shouldReopen && !fc.open {
			t.Errorf("For case %s, should have reopened but didn't.", tc.name)
		} else if !tc.shouldReopen && fc.open {
			t.Errorf("For case %s, should not have reopened but did.", tc.name)
		}
		if tc.shouldComment && !fc.commented {
			t.Errorf("For case %s, should have commented but didn't.", tc.name)
		} else if !tc.shouldComment && fc.commented {
			t.Errorf("For case %s, should not have commented but did.", tc.name)
		}
	}
}

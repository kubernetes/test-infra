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

package reopen

import (
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type fakeClient struct {
	commented bool
	open      bool
}

func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = true
	return nil
}

func (c *fakeClient) ReopenIssue(owner, repo string, number int) error {
	c.open = true
	return nil
}

func (c *fakeClient) ReopenPR(owner, repo string, number int) error {
	c.open = true
	return nil
}

func TestOpenComment(t *testing.T) {
	// "a" is the author, "r1", and "r2" are reviewers.
	var testcases = []struct {
		name          string
		action        github.IssueCommentEventAction
		state         string
		body          string
		commenter     string
		shouldReopen  bool
		shouldComment bool
	}{
		{
			name:          "non-open comment",
			action:        github.IssueCommentActionCreated,
			state:         "open",
			body:          "does not matter",
			commenter:     "o",
			shouldReopen:  false,
			shouldComment: false,
		},
		{
			name:          "re-open by author",
			action:        github.IssueCommentActionCreated,
			state:         "closed",
			body:          "/reopen",
			commenter:     "a",
			shouldReopen:  true,
			shouldComment: false,
		},
		{
			name:          "re-open by reviewer",
			action:        github.IssueCommentActionCreated,
			state:         "closed",
			body:          "/reopen",
			commenter:     "r1",
			shouldReopen:  true,
			shouldComment: false,
		},
		{
			name:          "re-open by reviewer, trailing space.",
			action:        github.IssueCommentActionCreated,
			state:         "closed",
			body:          "/reopen \r",
			commenter:     "r1",
			shouldReopen:  true,
			shouldComment: false,
		},
		{
			name:          "re-open edited by author",
			action:        github.IssueCommentActionEdited,
			state:         "closed",
			body:          "/reopen",
			commenter:     "a",
			shouldReopen:  false,
			shouldComment: false,
		},
		{
			name:          "open by author on already open issue",
			action:        github.IssueCommentActionCreated,
			state:         "open",
			body:          "/reopen",
			commenter:     "a",
			shouldReopen:  false,
			shouldComment: false,
		},
		{
			name:          "re-open by other person",
			action:        github.IssueCommentActionCreated,
			state:         "closed",
			body:          "/reopen",
			commenter:     "o",
			shouldReopen:  false,
			shouldComment: true,
		},
	}
	for _, tc := range testcases {
		fc := &fakeClient{}
		ice := github.IssueCommentEvent{
			Action: tc.action,
			Comment: github.IssueComment{
				Body: tc.body,
				User: github.User{Login: tc.commenter},
			},
			Issue: github.Issue{
				User:      github.User{Login: "a"},
				Number:    5,
				State:     tc.state,
				Assignees: []github.User{{Login: "a"}, {Login: "r1"}, {Login: "r2"}},
			},
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), ice); err != nil {
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

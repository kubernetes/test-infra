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

package close

import (
	"testing"

	"k8s.io/test-infra/prow/github"
)

type fakeClient struct {
	commented bool
	closed    bool
}

func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = true
	return nil
}

func (c *fakeClient) CloseIssue(owner, repo string, number int) error {
	c.closed = true
	return nil
}

func TestCloseComment(t *testing.T) {
	// "a" is the author, "r1", and "r2" are reviewers.
	var testcases = []struct {
		name          string
		action        string
		state         string
		body          string
		commenter     string
		shouldClose   bool
		shouldComment bool
	}{
		{
			name:          "non-close comment",
			action:        "created",
			state:         "open",
			body:          "uh oh",
			commenter:     "o",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by author",
			action:        "created",
			state:         "open",
			body:          "/close",
			commenter:     "a",
			shouldClose:   true,
			shouldComment: false,
		},
		{
			name:          "close by reviewer",
			action:        "created",
			state:         "open",
			body:          "/close",
			commenter:     "r1",
			shouldClose:   true,
			shouldComment: false,
		},
		{
			name:          "close edited by author",
			action:        "edited",
			state:         "open",
			body:          "/close",
			commenter:     "a",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by author on closed issue",
			action:        "created",
			state:         "closed",
			body:          "/close",
			commenter:     "a",
			shouldClose:   false,
			shouldComment: false,
		},
		{
			name:          "close by other person",
			action:        "created",
			state:         "open",
			body:          "/close",
			commenter:     "o",
			shouldClose:   false,
			shouldComment: true,
		},
	}
	for _, tc := range testcases {
		fc := &fakeClient{}
		ice := github.IssueCommentEvent{
			Action: tc.action,
			Comment: github.IssueComment{
				Body: tc.body,
				User: github.User{tc.commenter},
			},
			Issue: github.Issue{
				User:      github.User{"a"},
				Number:    5,
				State:     tc.state,
				Assignees: []github.User{{"a"}, {"r1"}, {"r2"}},
			},
		}
		if err := handle(fc, ice); err != nil {
			t.Errorf("For case %s, didn't expect error from handle: %v", tc.name, err)
			continue
		}
		if tc.shouldClose && !fc.closed {
			t.Errorf("For case %s, should have closed but didn't.")
		} else if !tc.shouldClose && fc.closed {
			t.Errorf("For case %s, should not have closed but did.")
		}
		if tc.shouldComment && !fc.commented {
			t.Errorf("For case %s, should have commented but didn't.")
		} else if !tc.shouldComment && fc.commented {
			t.Errorf("For case %s, should not have commented but did.")
		}
	}
}

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
	"errors"
	"testing"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type fakeClient struct {
	commented      bool
	closed         bool
	AssigneesAdded []string
}

func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = true
	return nil
}

func (c *fakeClient) CloseIssue(owner, repo string, number int) error {
	c.closed = true
	return nil
}

func (c *fakeClient) ClosePR(owner, repo string, number int) error {
	c.closed = true
	return nil
}

func (c *fakeClient) IsMember(owner, login string) (bool, error) {
	if login == "non-member" {
		return false, nil
	}
	return true, nil
}

func (c *fakeClient) AssignIssue(owner, repo string, number int, assignees []string) error {
	if assignees[0] == "non-member" || assignees[0] == "non-owner-assign-error" {
		return errors.New("Failed to assign")
	}
	c.AssigneesAdded = append(c.AssigneesAdded, assignees...)
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
		shouldAssign  bool
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
			name:          "close by author, trailing space.",
			action:        "created",
			state:         "open",
			body:          "/close \r",
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
			name:          "close by other person, non-member cannot close",
			action:        "created",
			state:         "open",
			body:          "/close",
			commenter:     "non-member",
			shouldClose:   false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:          "close by other person, failed to assign",
			action:        "created",
			state:         "open",
			body:          "/close",
			commenter:     "non-owner-assign-error",
			shouldClose:   false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:          "close by other person, assign and close",
			action:        "created",
			state:         "open",
			body:          "/close",
			commenter:     "non-owner",
			shouldClose:   true,
			shouldComment: false,
			shouldAssign:  true,
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

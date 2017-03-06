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

package assign

import (
	"testing"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type fakeClient struct {
	assigned   []string
	unassigned []string
}

func (c *fakeClient) UnassignIssue(owner, repo string, number int, assignees []string) error {
	c.unassigned = assignees
	return nil
}

func (c *fakeClient) AssignIssue(owner, repo string, number int, assignees []string) error {
	c.assigned = assignees
	return nil
}

func TestParseLogins(t *testing.T) {
	var testcases = []struct {
		name   string
		text   string
		logins []string
	}{
		{
			name: "empty",
			text: "",
		},
		{
			name:   "one",
			text:   " @jungle",
			logins: []string{"jungle"},
		},
		{
			name:   "two",
			text:   " @erick @fejta",
			logins: []string{"erick", "fejta"},
		},
	}
	for _, tc := range testcases {
		l := parseLogins(tc.text)
		if len(l) != len(tc.logins) {
			t.Errorf("For case %s, expected %s and got %s", tc.name, tc.logins, l)
		}
		for n, who := range l {
			if tc.logins[n] != who {
				t.Errorf("For case %s, expected %s and got %s", tc.name, tc.logins, l)
			}
		}
	}
}

func TestAssignComment(t *testing.T) {
	var testcases = []struct {
		name      string
		action    string
		body      string
		commenter string
		added     []string
		removed   []string
	}{
		{
			name:      "unrelated comment",
			action:    "created",
			body:      "uh oh",
			commenter: "o",
		},
		{
			name:      "not created",
			action:    "something",
			body:      "uh oh",
			commenter: "o",
		},
		{
			name:      "assign me",
			action:    "created",
			body:      "/assign",
			commenter: "rando",
			added:     []string{"rando"},
		},
		{
			name:      "unassign myself",
			action:    "created",
			body:      "/unassign",
			commenter: "rando",
			removed:   []string{"rando"},
		},
		{
			name:      "tab completion",
			action:    "created",
			body:      "/assign @fejta ",
			commenter: "rando",
			added:     []string{"fejta"},
		},
		{
			name:      "multi commands",
			action:    "created",
			body:      "/assign @fejta\n/unassign @spxtr",
			commenter: "rando",
			added:     []string{"fejta"},
			removed:   []string{"spxtr"},
		},
		{
			name:      "interesting names",
			action:    "created",
			body:      "/assign @hello-world @allow_underscore",
			commenter: "rando",
			added:     []string{"hello-world", "allow_underscore"},
		},
		{
			name:      "bad login",
			action:    "created",
			commenter: "rando",
			body:      "/assign @Invalid$User",
		},
		{
			name:      "require @",
			action:    "created",
			commenter: "rando",
			body:      "/assign no at symbol",
		},
		{
			name:      "assign friends",
			action:    "created",
			body:      "/assign @bert @ernie",
			commenter: "rando",
			added:     []string{"bert", "ernie"},
		},
		{
			name:      "unassign buddies",
			action:    "created",
			body:      "/unassign @ashitaka @eboshi",
			commenter: "san",
			removed:   []string{"ashitaka", "eboshi"},
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
				Assignees: []github.User{{Login: "a"}, {Login: "r1"}, {Login: "r2"}},
			},
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), ice); err != nil {
			t.Errorf("For case %s, didn't expect error from handle: %v", tc.name, err)
			continue
		}
		if len(fc.assigned) != len(tc.added) {
			t.Errorf("For case %s, assigned actual %s != expected %s", tc.name, fc.assigned, tc.added)
		} else {
			for n, who := range tc.added {
				if who != fc.assigned[n] {
					t.Errorf("For case %s, assigned actual %s != expected %s", tc.name, fc.assigned, tc.added)
					break
				}
			}
		}
		if len(fc.unassigned) != len(tc.removed) {
			t.Errorf("For case %s, unassigned %s != %s", tc.name, fc.unassigned, tc.removed)
		} else {
			for n, who := range tc.removed {
				if who != fc.unassigned[n] {
					t.Errorf("For case %s, unassigned %s != %s", tc.name, fc.unassigned, tc.removed)
					break
				}
			}
		}
	}
}

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

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type fakeClient struct {
	assigned   map[string]int
	unassigned map[string]int

	requested    map[string]int
	unrequested  map[string]int
	contributors map[string]bool

	commented bool
}

func (c *fakeClient) UnassignIssue(owner, repo string, number int, assignees []string) error {
	for _, who := range assignees {
		c.unassigned[who]++
	}

	return nil
}

func (c *fakeClient) AssignIssue(owner, repo string, number int, assignees []string) error {
	var missing github.MissingUsers
	for _, who := range assignees {
		if who != "evil" {
			c.assigned[who]++
		} else {
			missing.Users = append(missing.Users, who)
		}
	}

	if len(missing.Users) == 0 {
		return nil
	}
	return missing
}

func (c *fakeClient) RequestReview(org, repo string, number int, logins []string) error {
	var missing github.MissingUsers
	for _, user := range logins {
		if c.contributors[user] {
			c.requested[user]++
		} else {
			missing.Users = append(missing.Users, user)
		}
	}
	if len(missing.Users) > 0 {
		return missing
	}
	return nil
}

func (c *fakeClient) UnrequestReview(org, repo string, number int, logins []string) error {
	for _, user := range logins {
		c.unrequested[user]++
	}
	return nil
}

func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = comment != ""
	return nil
}

func newFakeClient(contribs []string) *fakeClient {
	c := &fakeClient{
		contributors: make(map[string]bool),
		requested:    make(map[string]int),
		unrequested:  make(map[string]int),
		assigned:     make(map[string]int),
		unassigned:   make(map[string]int),
	}
	for _, user := range contribs {
		c.contributors[user] = true
	}
	return c
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

// TestAssignAndReview tests that the handle function uses the github client
// to correctly create and/or delete assignments and PR review requests.
func TestAssignAndReview(t *testing.T) {
	var testcases = []struct {
		name        string
		body        string
		commenter   string
		assigned    []string
		unassigned  []string
		requested   []string
		unrequested []string
		commented   bool
	}{
		{
			name:      "unrelated comment",
			body:      "uh oh",
			commenter: "o",
		},
		{
			name:      "assign on open",
			body:      "/assign",
			commenter: "rando",
			assigned:  []string{"rando"},
		},
		{
			name:      "assign me",
			body:      "/assign",
			commenter: "rando",
			assigned:  []string{"rando"},
		},
		{
			name:       "unassign myself",
			body:       "/unassign",
			commenter:  "rando",
			unassigned: []string{"rando"},
		},
		{
			name:      "tab completion",
			body:      "/assign @fejta ",
			commenter: "rando",
			assigned:  []string{"fejta"},
		},
		{
			name:      "no @ works too",
			body:      "/assign fejta",
			commenter: "rando",
			assigned:  []string{"fejta"},
		},
		{
			name:       "multi commands",
			body:       "/assign @fejta\n/unassign @spxtr",
			commenter:  "rando",
			assigned:   []string{"fejta"},
			unassigned: []string{"spxtr"},
		},
		{
			name:      "interesting names",
			body:      "/assign @hello-world @allow_underscore",
			commenter: "rando",
			assigned:  []string{"hello-world", "allow_underscore"},
		},
		{
			name:      "bad login",
			commenter: "rando",
			body:      "/assign @Invalid$User",
		},
		{
			name:      "bad login, no @",
			commenter: "rando",
			body:      "/assign Invalid$User",
		},
		{
			name:      "assign friends",
			body:      "/assign @bert @ernie",
			commenter: "rando",
			assigned:  []string{"bert", "ernie"},
		},
		{
			name:       "unassign buddies",
			body:       "/unassign @ashitaka @eboshi",
			commenter:  "san",
			unassigned: []string{"ashitaka", "eboshi"},
		},
		{
			name:       "unassign buddies, trailing space.",
			body:       "/unassign @ashitaka @eboshi \r",
			commenter:  "san",
			unassigned: []string{"ashitaka", "eboshi"},
		},
		{
			name:      "evil commenter",
			body:      "/assign @merlin",
			commenter: "evil",
			assigned:  []string{"merlin"},
		},
		{
			name:      "evil commenter self assign",
			body:      "/assign",
			commenter: "evil",
			commented: true,
		},
		{
			name:      "evil assignee",
			body:      "/assign @evil @merlin",
			commenter: "innocent",
			assigned:  []string{"merlin"},
			commented: true,
		},
		{
			name:       "evil unassignee",
			body:       "/unassign @evil @merlin",
			commenter:  "innocent",
			unassigned: []string{"evil", "merlin"},
		},
		{
			name:      "review on open",
			body:      "/cc @merlin",
			commenter: "rando",
			requested: []string{"merlin"},
		},
		{
			name:      "tab completion",
			body:      "/cc @cjwagner ",
			commenter: "rando",
			requested: []string{"cjwagner"},
		},
		{
			name:      "no @ works too",
			body:      "/cc cjwagner ",
			commenter: "rando",
			requested: []string{"cjwagner"},
		},
		{
			name:        "multi commands",
			body:        "/cc @cjwagner\n/uncc @spxtr",
			commenter:   "rando",
			requested:   []string{"cjwagner"},
			unrequested: []string{"spxtr"},
		},
		{
			name:      "interesting names",
			body:      "/cc @hello-world @allow_underscore",
			commenter: "rando",
			requested: []string{"hello-world", "allow_underscore"},
		},
		{
			name:      "bad login",
			commenter: "rando",
			body:      "/cc @Invalid$User",
		},
		{
			name:      "bad login",
			commenter: "rando",
			body:      "/cc Invalid$User",
		},
		{
			name:      "request multiple",
			body:      "/cc @cjwagner @merlin",
			commenter: "rando",
			requested: []string{"cjwagner", "merlin"},
		},
		{
			name:        "unrequest buddies",
			body:        "/uncc @ashitaka @eboshi",
			commenter:   "san",
			unrequested: []string{"ashitaka", "eboshi"},
		},
		{
			name:      "evil commenter",
			body:      "/cc @merlin",
			commenter: "evil",
			requested: []string{"merlin"},
		},
		{
			name:      "evil reviewer requested",
			body:      "/cc @evil @merlin",
			commenter: "innocent",
			requested: []string{"merlin"},
			commented: true,
		},
		{
			name:        "evil reviewer unrequested",
			body:        "/uncc @evil @merlin",
			commenter:   "innocent",
			unrequested: []string{"evil", "merlin"},
		},
		{
			name:        "multi command types",
			body:        "/assign @fejta\n/unassign @spxtr @cjwagner\n/uncc @merlin \n/cc @cjwagner",
			commenter:   "rando",
			assigned:    []string{"fejta"},
			unassigned:  []string{"spxtr", "cjwagner"},
			requested:   []string{"cjwagner"},
			unrequested: []string{"merlin"},
		},
		{
			name:      "request review self",
			body:      "/cc",
			commenter: "cjwagner",
			requested: []string{"cjwagner"},
		},
		{
			name:        "unrequest review self",
			body:        "/uncc",
			commenter:   "cjwagner",
			unrequested: []string{"cjwagner"},
		},
		{
			name:        "request review self, with unrequest friend, with trailing space.",
			body:        "/cc \n/uncc @spxtr ",
			commenter:   "cjwagner",
			requested:   []string{"cjwagner"},
			unrequested: []string{"spxtr"},
		},
	}
	for _, tc := range testcases {
		fc := newFakeClient([]string{"hello-world", "allow_underscore", "cjwagner", "merlin"})
		e := github.GenericCommentEvent{
			Body:   tc.body,
			User:   github.User{Login: tc.commenter},
			Repo:   github.Repo{Name: "repo", Owner: github.User{Login: "org"}},
			Number: 5,
		}
		if err := handle(newAssignHandler(e, fc, logrus.WithField("plugin", pluginName))); err != nil {
			t.Errorf("For case %s, didn't expect error from handle: %v", tc.name, err)
			continue
		}
		if err := handle(newReviewHandler(e, fc, logrus.WithField("plugin", pluginName))); err != nil {
			t.Errorf("For case %s, didn't expect error from handle: %v", tc.name, err)
			continue
		}

		if tc.commented != fc.commented {
			t.Errorf("For case %s, expect commented: %v, got commented %v", tc.name, tc.commented, fc.commented)
		}

		if len(fc.assigned) != len(tc.assigned) {
			t.Errorf("For case %s, assigned actual %v != expected %s", tc.name, fc.assigned, tc.assigned)
		} else {
			for _, who := range tc.assigned {
				if n, ok := fc.assigned[who]; !ok || n < 1 {
					t.Errorf("For case %s, assigned actual %v != expected %s", tc.name, fc.assigned, tc.assigned)
					break
				}
			}
		}
		if len(fc.unassigned) != len(tc.unassigned) {
			t.Errorf("For case %s, unassigned %v != %s", tc.name, fc.unassigned, tc.unassigned)
		} else {
			for _, who := range tc.unassigned {
				if n, ok := fc.unassigned[who]; !ok || n < 1 {
					t.Errorf("For case %s, unassigned %v != %s", tc.name, fc.unassigned, tc.unassigned)
					break
				}
			}
		}

		if len(fc.requested) != len(tc.requested) {
			t.Errorf("For case %s, requested actual %v != expected %s", tc.name, fc.requested, tc.requested)
		} else {
			for _, who := range tc.requested {
				if n, ok := fc.requested[who]; !ok || n < 1 {
					t.Errorf("For case %s, requested actual %v != expected %s", tc.name, fc.requested, tc.requested)
					break
				}
			}
		}
		if len(fc.unrequested) != len(tc.unrequested) {
			t.Errorf("For case %s, unrequested %v != %s", tc.name, fc.unrequested, tc.unrequested)
		} else {
			for _, who := range tc.unrequested {
				if n, ok := fc.unrequested[who]; !ok || n < 1 {
					t.Errorf("For case %s, unrequested %v != %s", tc.name, fc.unrequested, tc.unrequested)
					break
				}
			}
		}
	}
}

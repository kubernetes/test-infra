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

package trigger

import (
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
)

func TestTrusted(t *testing.T) {
	const rando = "random-person"
	const member = "org-member"
	const sister = "trusted-org-member"
	const friend = "repo-collaborator"

	const accept = "/ok-to-test"
	const chatter = "ignore random stuff"

	var testcases = []struct {
		name      string
		author    string
		comment   string
		commenter string
		onlyOrg   bool
		expected  bool
	}{
		{
			name:     "trust org member",
			author:   member,
			expected: true,
		},
		{
			name:     "trust member of other trusted org",
			author:   sister,
			expected: true,
		},
		{
			name: "reject random author",
		},
		{
			name:      "reject random author on random org member commentary",
			comment:   chatter,
			commenter: member,
		},
		{
			name:      "accept random PR after org member ok",
			comment:   accept,
			commenter: member,
			expected:  true,
		},
		{
			name:      "accept random PR after ok from trusted org member",
			comment:   accept,
			commenter: sister,
			expected:  true,
		},
		{
			name:      "ok may end with a \\r",
			comment:   accept + "\r",
			commenter: member,
			expected:  true,
		},
		{
			name:      "ok start on a middle line",
			comment:   "hello\n" + accept + "\r\nplease",
			commenter: member,
			expected:  true,
		},
		{
			name:      "require ok on start of line",
			comment:   "please, " + accept,
			commenter: member,
		},
		{
			name:      "reject acceptance from random person",
			comment:   accept,
			commenter: rando + " III",
		},
		{
			name:      "reject acceptance from this bot",
			comment:   accept,
			commenter: fakegithub.Bot,
		},
		{
			name:      "reject acceptance from random author",
			comment:   accept,
			commenter: rando,
		},
		{
			name:      "reject acceptance from repo collaborator in org-only mode",
			comment:   accept,
			commenter: friend,
			onlyOrg:   true,
		},
		{
			name:      "accept ok from repo collaborator",
			comment:   accept,
			commenter: friend,
			expected:  true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.author == "" {
				tc.author = rando
			}
			g := &fakegithub.FakeClient{
				OrgMembers:    map[string][]string{"kubernetes": {sister}, "kubernetes-incubator": {member, fakegithub.Bot}},
				Collaborators: []string{friend},
				IssueComments: map[int][]github.IssueComment{},
			}
			trigger := plugins.Trigger{
				TrustedOrg:     "kubernetes",
				OnlyOrgMembers: tc.onlyOrg,
			}
			var comments []github.IssueComment
			if tc.comment != "" {
				comments = append(comments, github.IssueComment{
					Body: tc.comment,
					User: github.User{Login: tc.commenter},
				})
			}
			actual, err := trustedPullRequest(g, &trigger, tc.author, "kubernetes-incubator", "random-repo", comments)
			if err != nil {
				t.Fatalf("Didn't expect error: %s", err)
			}
			if actual != tc.expected {
				t.Errorf("actual result %t != expected %t", actual, tc.expected)
			}
		})
	}
}

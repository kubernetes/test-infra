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
)

func TestTrusted(t *testing.T) {
	var testcases = []struct {
		PR       github.PullRequest
		Comments []github.IssueComment
		Trusted  bool
	}{
		// Org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "t1"},
			},
			Trusted: true,
		},
		// Repo's org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "t2"},
			},
			Trusted: true,
		},
		// Non org member, no comments.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Trusted: false,
		},
		// Non org member, random comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "this is evil!",
					User: github.User{Login: "t1"},
				},
			},
			Trusted: false,
		},
		// Non org member, "not ok to test" comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "not ok to test",
					User: github.User{Login: "t1"},
				},
			},
			Trusted: false,
		},
		// Non org member, ok to test comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "/ok-to-test",
					User: github.User{Login: "t1"},
				},
			},
			Trusted: true,
		},
		// Non org member, ok to test comment by repo's org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},

			Comments: []github.
				IssueComment{
				{
					Body: "/ok-to-test",
					User: github.User{Login: "t2"},
				},
			},
			Trusted: true,
		},
		// Non org member, ok to test comment with carriage return by org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "/ok-to-test\r",
					User: github.User{Login: "t1"},
				},
			},
			Trusted: true,
		},
		// Non org member, multiline ok to test comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "hello\n/ok-to-test\r\nplease",
					User: github.User{Login: "t1"},
				},
			},
			Trusted: true,
		},
		// Non org member, ok to test with additional comment before by org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "please, /ok-to-test",
					User: github.User{Login: "t1"},
				},
			},
			Trusted: false,
		},
		// Non org member, ok to test comment by non-org member.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "/ok-to-test",
					User: github.User{Login: "u2"},
				},
			},
			Trusted: false,
		},
		// Non org member, ok to test comment by bot.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "/ok-to-test",
					User: github.User{Login: "k8s-bot"},
				},
			},
			Trusted: false,
		},
		// Non org member, ok to test comment by author.
		{
			PR: github.PullRequest{
				User: github.User{Login: "u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "/ok-to-test",
					User: github.User{Login: "u"},
				},
			},
			Trusted: false,
		},
	}
	for _, tc := range testcases {
		g := &fakegithub.FakeClient{
			OrgMembers: map[string][]string{"kubernetes": {"t1"}, "kubernetes-incubator": {"t2"}},
			IssueComments: map[int][]github.IssueComment{
				0: tc.Comments,
			},
		}
		tc.PR.Base.Repo.Owner.Login = "kubernetes-incubator"
		trusted, err := trustedPullRequest(g, tc.PR, "kubernetes", tc.Comments)
		if err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		if trusted != tc.Trusted {
			t.Errorf("Wrong trusted: %+v", tc)
		}
	}
}

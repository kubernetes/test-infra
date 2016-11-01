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

package main

import (
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/jobs"
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
				User: github.User{"t1"},
			},
			Trusted: true,
		},
		// Non org member, no comments.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Trusted: false,
		},
		// Non org member, random comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "this is evil!",
					User: github.User{"t1"},
				},
			},
			Trusted: false,
		},
		// Non org member, "not ok to test" comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "not ok to test",
					User: github.User{"t1"},
				},
			},
			Trusted: false,
		},
		// Non org member, ok to test comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "@k8s-bot ok to test",
					User: github.User{"t1"},
				},
			},
			Trusted: true,
		},
		// Non org member, multiline ok to test comment by org member.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "ok to test\r\nthanks",
					User: github.User{"t1"},
				},
			},
			Trusted: true,
		},
		// Non org member, ok to test comment by non-org member.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "ok to test",
					User: github.User{"u2"},
				},
			},
			Trusted: false,
		},
		// Non org member, ok to test comment by bot.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "ok to test",
					User: github.User{"k8s-bot"},
				},
			},
			Trusted: false,
		},
		// Non org member, ok to test comment by author.
		{
			PR: github.PullRequest{
				User: github.User{"u"},
			},
			Comments: []github.IssueComment{
				{
					Body: "ok to test",
					User: github.User{"u"},
				},
			},
			Trusted: false,
		},
	}
	for _, tc := range testcases {
		g := &fakegithub.FakeClient{
			OrgMembers: []string{"t1"},
			IssueComments: map[int][]github.IssueComment{
				0: tc.Comments,
			},
		}
		s := &GitHubAgent{
			GitHubClient: g,
		}
		trusted, err := s.trustedPullRequest(tc.PR)
		if err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		if trusted != tc.Trusted {
			t.Errorf("Wrong trusted: %+v", tc)
		}
	}
}

func TestHandleIssueComment(t *testing.T) {
	var testcases = []struct {
		Author      string
		Body        string
		State       string
		IsPR        bool
		ShouldBuild bool
	}{
		// Not a PR.
		{
			Author:      "t",
			Body:        "ok to test",
			State:       "open",
			IsPR:        false,
			ShouldBuild: false,
		},
		// Closed PR.
		{
			Author:      "t",
			Body:        "ok to test",
			State:       "closed",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Comment by a bot.
		{
			Author:      "k8s-bot",
			Body:        "ok to test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Non-trusted member.
		{
			Author:      "u",
			Body:        "ok to test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Trusted member's ok to test.
		{
			Author:      "t",
			Body:        "looks great, thanks!\nok to test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		// Trusted member's not ok to test.
		{
			Author:      "t",
			Body:        "not ok to test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Trusted member's test this.
		{
			Author:      "t",
			Body:        "@k8s-bot test this",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
	}
	for _, tc := range testcases {
		brc := make(chan KubeRequest, 1)
		g := &fakegithub.FakeClient{
			OrgMembers: []string{"t"},
			PullRequests: map[int]*github.PullRequest{
				0: &github.PullRequest{
					Number: 0,
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Name: "repo",
						},
					},
				},
			},
		}
		s := &GitHubAgent{
			GitHubClient:  g,
			JenkinsJobs:   &jobs.JobAgent{},
			BuildRequests: brc,
		}
		s.JenkinsJobs.SetJobs(map[string][]jobs.JenkinsJob{
			"org/repo": {
				{
					Name:      "job",
					AlwaysRun: true,
					Context:   "job job",
					Trigger:   "@k8s-bot test this",
				},
			},
		})

		var pr *struct{}
		if tc.IsPR {
			pr = &struct{}{}
		}
		event := github.IssueCommentEvent{
			Action: "created",
			Repo: github.Repo{
				Name:     "repo",
				FullName: "org/repo",
			},
			Comment: github.IssueComment{
				Body: tc.Body,
				User: github.User{tc.Author},
			},
			Issue: github.Issue{
				PullRequest: pr,
				State:       tc.State,
			},
		}

		if err := s.commentTrigger(event); err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		var built bool
		select {
		case <-brc:
			built = true
		default:
			built = false
		}
		if built != tc.ShouldBuild {
			if built {
				t.Errorf("Built but should not have: %+v", tc)
			} else {
				t.Errorf("Not built but should have: %+v", tc)
			}
		}
	}
}

// Make sure we delete all jobs when a PR is closed.
func TestClosePR(t *testing.T) {
	drc := make(chan KubeRequest, 2)
	s := &GitHubAgent{
		JenkinsJobs:    &jobs.JobAgent{},
		DeleteRequests: drc,
	}
	s.JenkinsJobs.SetJobs(map[string][]jobs.JenkinsJob{
		"org/repo": {
			{
				Name:      "job1",
				AlwaysRun: true,
			},
			{
				Name:      "job2",
				AlwaysRun: false,
			},
		},
	})

	err := s.prTrigger(github.PullRequestEvent{
		Action: "closed",
		PullRequest: github.PullRequest{
			Number: 3,
			Base: github.PullRequestBranch{
				Repo: github.Repo{
					FullName: "org/repo",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Didn't expect error: %s", err)
	}
	select {
	case kr := <-drc:
		t.Logf("Deleting job: %s", kr.JobName)
	default:
		t.Fatal("Didn't delete any jobs.")
	}
	select {
	case kr := <-drc:
		t.Logf("Deleting job: %s", kr.JobName)
	default:
		t.Fatal("Only deleted one job.")
	}
}

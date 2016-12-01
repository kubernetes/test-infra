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

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/jobs"
	"k8s.io/test-infra/prow/kube"
)

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
		g := &fakegithub.FakeClient{
			IssueComments: map[int][]github.IssueComment{},
			OrgMembers:    []string{"t"},
			PullRequests: map[int]*github.PullRequest{
				0: {
					Number: 0,
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Name: "repo",
						},
					},
				},
			},
		}
		c := client{
			GitHubClient: g,
			JobAgent:     &jobs.JobAgent{},
			Logger:       logrus.WithField("plugin", pluginName),
		}
		c.JobAgent.SetJobs(map[string][]jobs.JenkinsJob{
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
				User: github.User{Login: tc.Author},
			},
			Issue: github.Issue{
				PullRequest: pr,
				State:       tc.State,
			},
		}

		oldLineStartPRJob := lineStartPRJob
		oldLineDeletePRJob := lineDeletePRJob
		defer func() {
			lineStartPRJob = oldLineStartPRJob
			lineDeletePRJob = oldLineDeletePRJob
		}()
		var startedJobs []string
		lineStartPRJob = func(k *kube.Client, jobName, context string, pr github.PullRequest, ref string) error {
			startedJobs = append(startedJobs, jobName)
			return nil
		}
		lineDeletePRJob = func(k *kube.Client, jobName string, pr github.PullRequest) error {
			return nil
		}
		if err := handleIC(c, event); err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		if len(startedJobs) > 0 && !tc.ShouldBuild {
			t.Errorf("Built but should not have: %+v", tc)
		} else if len(startedJobs) == 0 && tc.ShouldBuild {
			t.Errorf("Not built but should have: %+v", tc)
		}
	}
}

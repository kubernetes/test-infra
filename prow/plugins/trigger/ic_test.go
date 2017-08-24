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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"
)

type fkc struct {
	started []string
}

func (c *fkc) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	c.started = append(c.started, pj.Spec.Context)
	return pj, nil
}

func TestHandleIssueComment(t *testing.T) {
	var testcases = []struct {
		Author        string
		Body          string
		State         string
		IsPR          bool
		Branch        string
		ShouldBuild   bool
		HasOkToTest   bool
		StartsExactly string
	}{
		// Not a PR.
		{
			Author:      "t",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        false,
			ShouldBuild: false,
		},
		// Closed PR.
		{
			Author:      "t",
			Body:        "/ok-to-test",
			State:       "closed",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Comment by a bot.
		{
			Author:      "k8s-bot",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Non-trusted member's ok to test.
		{
			Author:      "u",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Non-trusted member after "ok to test".
		{
			Author:      "u",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			HasOkToTest: true,
			ShouldBuild: true,
		},
		// Trusted member's ok to test
		{
			Author:      "t",
			Body:        "looks great, thanks!\n/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		// Trusted member's ok to test, trailing space.
		{
			Author:      "t",
			Body:        "looks great, thanks!\n/ok-to-test \r",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		// Trusted member's not ok to test.
		{
			Author:      "t",
			Body:        "not /ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		// Trusted member's test this.
		{
			Author:      "t",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		// Wrong branch.
		{
			Author:      "t",
			Body:        "/test",
			State:       "open",
			IsPR:        true,
			Branch:      "other",
			ShouldBuild: false,
		},
		// Retest with one running and one failed
		{
			Author:        "t",
			Body:          "/retest",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		// Retest with one running and one failed, trailing space.
		{
			Author:        "t",
			Body:          "/retest \r",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
	}
	for _, tc := range testcases {
		if tc.Branch == "" {
			tc.Branch = "master"
		}
		g := &fakegithub.FakeClient{
			IssueComments: map[int][]github.IssueComment{},
			OrgMembers:    []string{"t"},
			PullRequests: map[int]*github.PullRequest{
				0: {
					Number: 0,
					Head: github.PullRequestBranch{
						SHA: "cafe",
					},
					Base: github.PullRequestBranch{
						Ref: tc.Branch,
						Repo: github.Repo{
							Name: "repo",
						},
					},
				},
			},
			CombinedStatuses: map[string]*github.CombinedStatus{
				"cafe": {
					Statuses: []github.Status{
						{State: "pending", Context: "pull-job"},
						{State: "failure", Context: "pull-jib"},
					},
				},
			},
		}
		kc := &fkc{}
		c := client{
			GitHubClient: g,
			KubeClient:   kc,
			Config:       &config.Config{},
			Logger:       logrus.WithField("plugin", pluginName),
		}
		c.Config.SetPresubmits(map[string][]config.Presubmit{
			"org/repo": {
				{
					Name:      "job",
					AlwaysRun: true,
					Context:   "pull-job",
					Trigger:   `/test all`,
					Brancher:  config.Brancher{Branches: []string{"master"}},
				},
				{
					Name:      "jib",
					AlwaysRun: false,
					Context:   "pull-jib",
					Trigger:   `/test jib`,
				},
			},
		})

		var pr *struct{}
		if tc.IsPR {
			pr = &struct{}{}
		}
		if tc.HasOkToTest {
			g.IssueComments[0] = []github.IssueComment{{
				Body: "/ok-to-test",
				User: github.User{Login: "t"},
			}}
		}
		event := github.IssueCommentEvent{
			Action: github.IssueCommentActionCreated,
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

		if err := handleIC(c, event); err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		if len(kc.started) > 0 && !tc.ShouldBuild {
			t.Errorf("Built but should not have: %+v", tc)
		} else if len(kc.started) == 0 && tc.ShouldBuild {
			t.Errorf("Not built but should have: %+v", tc)
		}
		if tc.StartsExactly != "" && (len(kc.started) != 1 || kc.started[0] != tc.StartsExactly) {
			t.Errorf("Didn't build expected context %v, instead built %v", tc.StartsExactly, kc.started)
		}
	}
}

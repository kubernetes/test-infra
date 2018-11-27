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
	"fmt"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

type fkc struct {
	started []string
}

func (c *fkc) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	if !sets.NewString(c.started...).Has(pj.Spec.Context) {
		c.started = append(c.started, pj.Spec.Context)
	}
	return pj, nil
}

func issueLabels(labels ...string) []string {
	var ls []string
	for _, label := range labels {
		ls = append(ls, fmt.Sprintf("org/repo#0:%s", label))
	}
	return ls
}

type testcase struct {
	name string

	Author         string
	PRAuthor       string
	Body           string
	State          string
	IsPR           bool
	Branch         string
	ShouldBuild    bool
	ShouldReport   bool
	AddedLabels    []string
	RemovedLabels  []string
	StartsExactly  string
	Presubmits     map[string][]config.Presubmit
	IssueLabels    []string
	IgnoreOkToTest bool
}

func TestHandleGenericComment(t *testing.T) {
	var testcases = []testcase{
		{
			name: "Not a PR.",

			Author:      "t",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        false,
			ShouldBuild: false,
		},
		{
			name: "Closed PR.",

			Author:      "t",
			Body:        "/ok-to-test",
			State:       "closed",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name: "Comment by a bot.",

			Author:      "k8s-bot",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name: "Non-trusted member's ok to test.",

			Author:      "u",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name:        "accept /test from non-trusted member if PR author is trusted",
			Author:      "u",
			PRAuthor:    "t",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		{
			name:        "reject /test from non-trusted member when PR author is untrusted",
			Author:      "u",
			PRAuthor:    "u",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name: `Non-trusted member after "/ok-to-test".`,

			Author:      "u",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
			IssueLabels: issueLabels(labels.OkToTest),
		},
		{
			name: `Non-trusted member after "/ok-to-test", needs-ok-to-test label wasn't deleted.`,

			Author:        "u",
			Body:          "/test all",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			IssueLabels:   issueLabels(labels.NeedsOkToTest, labels.OkToTest),
			RemovedLabels: issueLabels(labels.NeedsOkToTest),
		},
		{
			name: "Trusted member's ok to test, IgnoreOkToTest",

			Author:         "t",
			Body:           "/ok-to-test",
			State:          "open",
			IsPR:           true,
			ShouldBuild:    false,
			IgnoreOkToTest: true,
		},
		{
			name: "Trusted member's ok to test",

			Author:      "t",
			Body:        "looks great, thanks!\n/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
			AddedLabels: issueLabels(labels.OkToTest),
		},
		{
			name: "Trusted member's ok to test, trailing space.",

			Author:      "t",
			Body:        "looks great, thanks!\n/ok-to-test \r",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
			AddedLabels: issueLabels(labels.OkToTest),
		},
		{
			name: "Trusted member's not ok to test.",

			Author:      "t",
			Body:        "not /ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name: "Trusted member's test this.",

			Author:      "t",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		{
			name: "Wrong branch.",

			Author:       "t",
			Body:         "/test all",
			State:        "open",
			IsPR:         true,
			Branch:       "other",
			ShouldBuild:  false,
			ShouldReport: true,
		},
		{
			name: "Retest with one running and one failed",

			Author:        "t",
			Body:          "/retest",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		{
			name: "Retest with one running and one failed, trailing space.",

			Author:        "t",
			Body:          "/retest \r",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		{
			name: "needs-ok-to-test label is removed when no presubmit runs by default",

			Author:      "t",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "job",
						},
						AlwaysRun:    false,
						Context:      "pull-job",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						AlwaysRun:    false,
						Context:      "pull-jib",
						Trigger:      `/test jib`,
						RerunCommand: `/test jib`,
					},
				},
			},
			IssueLabels:   issueLabels(labels.NeedsOkToTest),
			AddedLabels:   issueLabels(labels.OkToTest),
			RemovedLabels: issueLabels(labels.NeedsOkToTest),
		},
		{
			name:   "Wrong branch w/ SkipReport",
			Author: "t",
			Body:   "/test all",
			Branch: "other",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "job",
						},
						AlwaysRun:    true,
						SkipReport:   true,
						Context:      "pull-job",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
						Brancher:     config.Brancher{Branches: []string{"master"}},
					},
				},
			},
		},
		{
			name:   "Retest of run_if_changed job that hasn't run. Changes require job",
			Author: "t",
			Body:   "/retest",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED",
						},
						SkipReport:   true,
						Context:      "pull-jab",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jab",
		},
		{
			name:   "Retest of run_if_changed job that failed. Changes require job",
			Author: "t",
			Body:   "/retest",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED",
						},
						Context:      "pull-jib",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		{
			name:   "/test of run_if_changed job that has passed",
			Author: "t",
			Body:   "/test jub",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jub",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED",
						},
						Context:      "pull-jub",
						Trigger:      `/test jub`,
						RerunCommand: `/test jub`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jub",
		},
		{
			name:   "Retest of run_if_changed job that failed. Changes do not require the job",
			Author: "t",
			Body:   "/retest",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED2",
						},
						Context:      "pull-jib",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
					},
				},
			},
			ShouldBuild: true,
		},
		{
			name:   "Run if changed job triggered by /ok-to-test",
			Author: "t",
			Body:   "/ok-to-test",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED",
						},
						Context:      "pull-jab",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jab",
			IssueLabels:   issueLabels(labels.NeedsOkToTest),
			AddedLabels:   issueLabels(labels.OkToTest),
			RemovedLabels: issueLabels(labels.NeedsOkToTest),
		},
		{
			name:   "/test of branch-sharded job",
			Author: "t",
			Body:   "/test jab",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher:     config.Brancher{Branches: []string{"master"}},
						Context:      "pull-jab",
						Trigger:      `/test jab`,
						RerunCommand: `/test jab`,
					},
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher:     config.Brancher{Branches: []string{"release"}},
						Context:      "pull-jab",
						Trigger:      `/test jab`,
						RerunCommand: `/test jab`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jab",
		},
		{
			name:   "branch-sharded job. no shard matches base branch",
			Author: "t",
			Branch: "branch",
			Body:   "/test jab",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher:     config.Brancher{Branches: []string{"master"}},
						Context:      "pull-jab",
						Trigger:      `/test jab`,
						RerunCommand: `/test jab`,
					},
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher:     config.Brancher{Branches: []string{"release"}},
						Context:      "pull-jab",
						Trigger:      `/test jab`,
						RerunCommand: `/test jab`,
					},
				},
			},
			ShouldReport: true,
		},
		{
			name: "/retest of RunIfChanged job that doesn't need to run and hasn't run",

			Author: "t",
			Body:   "/retest",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jeb",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED2",
						},
						Context:      "pull-jeb",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
					},
				},
			},
			ShouldReport: true,
		},
		{
			name: "explicit /test for RunIfChanged job that doesn't need to run",

			Author: "t",
			Body:   "/test pull-jeb",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jeb",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED2",
						},
						Context:      "pull-jib",
						Trigger:      `/test (all|pull-jeb)`,
						RerunCommand: `/test pull-jeb`,
					},
				},
			},
			ShouldBuild: true,
		},
		{
			name:   "/test all of run_if_changed job that has passed and needs to run",
			Author: "t",
			Body:   "/test all",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jub",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED",
						},
						Context:      "pull-jub",
						Trigger:      `/test jub`,
						RerunCommand: `/test jub`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jub",
		},
		{
			name:   "/test all of run_if_changed job that has passed and doesn't need to run",
			Author: "t",
			Body:   "/test all",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jub",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED2",
						},
						Context:      "pull-jub",
						Trigger:      `/test jub`,
						RerunCommand: `/test jub`,
					},
				},
			},
			ShouldReport: true,
		},
		{
			name:        "accept /test all from trusted user",
			Author:      "t",
			PRAuthor:    "t",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		{
			name:        `Non-trusted member after "/lgtm" and "/approve"`,
			Author:      "u",
			PRAuthor:    "u",
			Body:        "/retest",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
			IssueLabels: issueLabels(labels.LGTM, labels.Approved),
		},
	}
	for _, tc := range testcases {
		t.Logf("running scenario %q", tc.name)
		if tc.Branch == "" {
			tc.Branch = "master"
		}
		g := &fakegithub.FakeClient{
			CreatedStatuses: map[string][]github.Status{},
			IssueComments:   map[int][]github.IssueComment{},
			OrgMembers:      map[string][]string{"org": {"t"}},
			PullRequests: map[int]*github.PullRequest{
				0: {
					User:   github.User{Login: tc.PRAuthor},
					Number: 0,
					Head: github.PullRequestBranch{
						SHA: "cafe",
					},
					Base: github.PullRequestBranch{
						Ref: tc.Branch,
						Repo: github.Repo{
							Owner: github.User{Login: "org"},
							Name:  "repo",
						},
					},
				},
			},
			IssueLabelsExisting: tc.IssueLabels,
			PullRequestChanges:  map[int][]github.PullRequestChange{0: {{Filename: "CHANGED"}}},
			CombinedStatuses: map[string]*github.CombinedStatus{
				"cafe": {
					Statuses: []github.Status{
						{State: "pending", Context: "pull-job"},
						{State: "failure", Context: "pull-jib"},
						{State: "success", Context: "pull-jub"},
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
		presubmits := tc.Presubmits
		if presubmits == nil {
			presubmits = map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "job",
						},
						AlwaysRun:    true,
						Context:      "pull-job",
						Trigger:      `/test all`,
						RerunCommand: `/test all`,
						Brancher:     config.Brancher{Branches: []string{"master"}},
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						AlwaysRun:    false,
						Context:      "pull-jib",
						Trigger:      `/test jib`,
						RerunCommand: `/test jib`,
					},
				},
			}
		}
		if err := c.Config.SetPresubmits(presubmits); err != nil {
			t.Fatalf("failed to set presubmits: %v", err)
		}

		event := github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			Repo: github.Repo{
				Owner:    github.User{Login: "org"},
				Name:     "repo",
				FullName: "org/repo",
			},
			Body:        tc.Body,
			User:        github.User{Login: tc.Author},
			IssueAuthor: github.User{Login: tc.PRAuthor},
			IssueState:  tc.State,
			IsPR:        tc.IsPR,
		}

		trigger := plugins.Trigger{
			IgnoreOkToTest: tc.IgnoreOkToTest,
		}

		// In some cases handleGenericComment can be called twice for the same event.
		// For instance on Issue/PR creation and modification.
		// Let's call it twice to ensure idempotency.
		if err := handleGenericComment(c, &trigger, event); err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		validate(kc, g, tc, t)
		if err := handleGenericComment(c, &trigger, event); err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		validate(kc, g, tc, t)
	}
}

func validate(kc *fkc, g *fakegithub.FakeClient, tc testcase, t *testing.T) {
	if len(kc.started) > 0 && !tc.ShouldBuild {
		t.Errorf("Built but should not have: %+v", tc)
	} else if len(kc.started) == 0 && tc.ShouldBuild {
		t.Errorf("Not built but should have: %+v", tc)
	}
	if tc.StartsExactly != "" && (len(kc.started) != 1 || kc.started[0] != tc.StartsExactly) {
		t.Errorf("Didn't build expected context %v, instead built %v", tc.StartsExactly, kc.started)
	}
	if tc.ShouldReport && len(g.CreatedStatuses) == 0 {
		t.Error("Expected report to github")
	} else if !tc.ShouldReport && len(g.CreatedStatuses) > 0 {
		t.Errorf("Expected no reports to github, but got %d", len(g.CreatedStatuses))
	}
	if !reflect.DeepEqual(g.IssueLabelsAdded, tc.AddedLabels) {
		t.Errorf("expected %q to be added, got %q", tc.AddedLabels, g.IssueLabelsAdded)
	}
	if !reflect.DeepEqual(g.IssueLabelsRemoved, tc.RemovedLabels) {
		t.Errorf("expected %q to be removed, got %q", tc.RemovedLabels, g.IssueLabelsRemoved)
	}
}

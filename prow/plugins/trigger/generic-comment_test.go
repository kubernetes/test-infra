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
	"k8s.io/test-infra/prow/pjutil"
	"log"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	clienttesting "k8s.io/client-go/testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

func issueLabels(labels ...string) []string {
	var ls []string
	for _, label := range labels {
		ls = append(ls, fmt.Sprintf("org/repo#0:%s", label))
	}
	return ls
}

type testcase struct {
	name string

	Author               string
	PRAuthor             string
	Body                 string
	State                string
	IsPR                 bool
	Branch               string
	ShouldBuild          bool
	ShouldReport         bool
	AddedLabels          []string
	RemovedLabels        []string
	StartsExactly        string
	Presubmits           map[string][]config.Presubmit
	IssueLabels          []string
	IgnoreOkToTest       bool
	ElideSkippedContexts bool
}

func TestHandleGenericComment(t *testing.T) {
	var testcases = []testcase{
		{
			name: "Not a PR.",

			Author:      "trusted-member",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        false,
			ShouldBuild: false,
		},
		{
			name: "Closed PR.",

			Author:      "trusted-member",
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
			name: "Irrelevant comment leads to no action.",

			Author:      "trusted-member",
			Body:        "Nice weather outside, right?",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name: "Non-trusted member's ok to test.",

			Author:      "untrusted-member",
			Body:        "/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name:        "accept /test from non-trusted member if PR author is trusted",
			Author:      "untrusted-member",
			PRAuthor:    "trusted-member",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		{
			name:        "reject /test from non-trusted member when PR author is untrusted",
			Author:      "untrusted-member",
			PRAuthor:    "untrusted-member",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name: `Non-trusted member after "/ok-to-test".`,

			Author:      "untrusted-member",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
			IssueLabels: issueLabels(labels.OkToTest),
		},
		{
			name: `Non-trusted member after "/ok-to-test", needs-ok-to-test label wasn't deleted.`,

			Author:        "untrusted-member",
			Body:          "/test all",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			IssueLabels:   issueLabels(labels.NeedsOkToTest, labels.OkToTest),
			RemovedLabels: issueLabels(labels.NeedsOkToTest),
		},
		{
			name: "Trusted member's ok to test, IgnoreOkToTest",

			Author:         "trusted-member",
			Body:           "/ok-to-test",
			State:          "open",
			IsPR:           true,
			ShouldBuild:    false,
			IgnoreOkToTest: true,
		},
		{
			name: "Trusted member's ok to test",

			Author:      "trusted-member",
			Body:        "looks great, thanks!\n/ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
			AddedLabels: issueLabels(labels.OkToTest),
		},
		{
			name: "Trusted member's ok to test, trailing space.",

			Author:      "trusted-member",
			Body:        "looks great, thanks!\n/ok-to-test \r",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
			AddedLabels: issueLabels(labels.OkToTest),
		},
		{
			name: "Trusted member's not ok to test.",

			Author:      "trusted-member",
			Body:        "not /ok-to-test",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
		},
		{
			name: "Trusted member's test this.",

			Author:      "trusted-member",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		{
			name: "Wrong branch.",

			Author:       "trusted-member",
			Body:         "/test all",
			State:        "open",
			IsPR:         true,
			Branch:       "other",
			ShouldBuild:  false,
			ShouldReport: true,
		},
		{
			name: "Wrong branch. Skipped statuses elided.",

			Author:               "trusted-member",
			Body:                 "/test all",
			State:                "open",
			IsPR:                 true,
			Branch:               "other",
			ShouldBuild:          false,
			ElideSkippedContexts: true,
			ShouldReport:         false,
		},
		{
			name: "Retest with one running and one failed",

			Author:        "trusted-member",
			Body:          "/retest",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		{
			name: "Retest with one running and one failed, trailing space.",

			Author:        "trusted-member",
			Body:          "/retest \r",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		{
			name:   "test of silly regex job",
			Author: "trusted-member",
			Body:   "Nice weather outside, right?",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher: config.Brancher{Branches: []string{"master"}},
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      "Nice weather outside, right?",
						RerunCommand: "Nice weather outside, right?",
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jab",
		},
		{
			name: "needs-ok-to-test label is removed when no presubmit runs by default",

			Author:      "trusted-member",
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
						AlwaysRun: false,
						Reporter: config.Reporter{
							Context: "pull-job",
						},
						Trigger:      `(?m)^/test (?:.*? )?job(?: .*?)?$`,
						RerunCommand: `/test job`,
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						AlwaysRun: false,
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
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
			Author: "trusted-member",
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
						AlwaysRun: true,
						Reporter: config.Reporter{
							SkipReport: true,
							Context:    "pull-job",
						},
						Trigger:      `(?m)^/test (?:.*? )?job(?: .*?)?$`,
						RerunCommand: `/test job`,
						Brancher:     config.Brancher{Branches: []string{"master"}},
					},
				},
			},
		},
		{
			name:   "Retest of run_if_changed job that hasn't run. Changes require job",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							SkipReport: true,
							Context:    "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jab",
		},
		{
			name:   "Retest of run_if_changed job that failed. Changes require job",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
						RerunCommand: `/test jib`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		{
			name:   "/test of run_if_changed job that has passed",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jub",
						},
						Trigger:      `(?m)^/test (?:.*? )?jub(?: .*?)?$`,
						RerunCommand: `/test jub`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jub",
		},
		{
			name:   "Retest of run_if_changed job that failed. Changes do not require the job",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
						RerunCommand: `/test jib`,
					},
				},
			},
			ShouldBuild:  false,
			ShouldReport: true,
		},
		{
			name:   "Retest of run_if_changed job that failed. Changes do not require the job. Skipped statuses elided.",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
						RerunCommand: `/test jib`,
					},
				},
			},
			ShouldBuild:          false,
			ElideSkippedContexts: true,
			ShouldReport:         false,
		},
		{
			name:   "Run if changed job triggered by /ok-to-test",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
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
			Author: "trusted-member",
			Body:   "/test jab",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher: config.Brancher{Branches: []string{"master"}},
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
					},
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher: config.Brancher{Branches: []string{"release"}},
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jab",
		},
		{
			name:   "branch-sharded job. no shard matches base branch",
			Author: "trusted-member",
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
						Brancher: config.Brancher{Branches: []string{"master"}},
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
					},
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher: config.Brancher{Branches: []string{"release"}},
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
					},
				},
			},
			ShouldReport: true,
		},
		{
			name:   "branch-sharded job. no shard matches base branch. Skipped statuses elided.",
			Author: "trusted-member",
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
						Brancher: config.Brancher{Branches: []string{"master"}},
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
					},
					{
						JobBase: config.JobBase{
							Name: "jab",
						},
						Brancher: config.Brancher{Branches: []string{"release"}},
						Reporter: config.Reporter{
							Context: "pull-jab",
						},
						Trigger:      `(?m)^/test (?:.*? )?jab(?: .*?)?$`,
						RerunCommand: `/test jab`,
					},
				},
			},
			ElideSkippedContexts: true,
			ShouldReport:         false,
		},
		{
			name: "/retest of RunIfChanged job that doesn't need to run and hasn't run",

			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jeb",
						},
						Trigger:      `(?m)^/test (?:.*? )?jeb(?: .*?)?$`,
						RerunCommand: `/test jeb`,
					},
				},
			},
			ShouldReport: true,
		},
		{
			name: "/retest of RunIfChanged job that doesn't need to run and hasn't run. Skipped statuses elided.",

			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jeb",
						},
						Trigger:      `(?m)^/test (?:.*? )?jeb(?: .*?)?$`,
						RerunCommand: `/test jeb`,
					},
				},
			},
			ElideSkippedContexts: true,
			ShouldReport:         false,
		},
		{
			name: "explicit /test for RunIfChanged job that doesn't need to run",

			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jeb",
						},
						Trigger:      `(?m)^/test (?:.*? )?jeb(?: .*?)?$`,
						RerunCommand: `/test jeb`,
					},
				},
			},
			ShouldBuild: false,
		},
		{
			name:   "/test all of run_if_changed job that has passed and needs to run",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jub",
						},
						Trigger:      `(?m)^/test (?:.*? )?jub(?: .*?)?$`,
						RerunCommand: `/test jub`,
					},
				},
			},
			ShouldBuild:   true,
			StartsExactly: "pull-jub",
		},
		{
			name:   "/test all of run_if_changed job that has passed and doesn't need to run",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jub",
						},
						Trigger:      `(?m)^/test (?:.*? )?jub(?: .*?)?$`,
						RerunCommand: `/test jub`,
					},
				},
			},
			ShouldReport: true,
		},
		{
			name:   "/test all of run_if_changed job that has passed and doesn't need to run. Skipped statuses elided.",
			Author: "trusted-member",
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
						Reporter: config.Reporter{
							Context: "pull-jub",
						},
						Trigger:      `(?m)^/test (?:.*? )?jub(?: .*?)?$`,
						RerunCommand: `/test jub`,
					},
				},
			},
			ElideSkippedContexts: true,
			ShouldReport:         false,
		},
		{
			name:        "accept /test all from trusted user",
			Author:      "trusted-member",
			PRAuthor:    "trusted-member",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			ShouldBuild: true,
		},
		{
			name:        `Non-trusted member after "/lgtm" and "/approve"`,
			Author:      "untrusted-member",
			PRAuthor:    "untrusted-member",
			Body:        "/retest",
			State:       "open",
			IsPR:        true,
			ShouldBuild: false,
			IssueLabels: issueLabels(labels.LGTM, labels.Approved),
		},
	}
	for _, tc := range testcases {
		if tc.Branch == "" {
			tc.Branch = "master"
		}
		g := &fakegithub.FakeClient{
			CreatedStatuses: map[string][]github.Status{},
			IssueComments:   map[int][]github.IssueComment{},
			OrgMembers:      map[string][]string{"org": {"trusted-member"}},
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
						{State: github.StatusPending, Context: "pull-job"},
						{State: github.StatusFailure, Context: "pull-jib"},
						{State: github.StatusSuccess, Context: "pull-jub"},
					},
				},
			},
		}
		fakeConfig := &config.Config{ProwConfig: config.ProwConfig{ProwJobNamespace: "prowjobs"}}
		fakeProwJobClient := fake.NewSimpleClientset()
		c := Client{
			GitHubClient:  g,
			ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs(fakeConfig.ProwJobNamespace),
			Config:        fakeConfig,
			Logger:        logrus.WithField("plugin", PluginName),
		}
		presubmits := tc.Presubmits
		if presubmits == nil {
			presubmits = map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "job",
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context: "pull-job",
						},
						Trigger:      `(?m)^/test (?:.*? )?job(?: .*?)?$`,
						RerunCommand: `/test job`,
						Brancher:     config.Brancher{Branches: []string{"master"}},
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						AlwaysRun: false,
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
						RerunCommand: `/test jib`,
					},
				},
			}
		}
		if err := c.Config.SetPresubmits(presubmits); err != nil {
			t.Fatalf("%s: failed to set presubmits: %v", tc.name, err)
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
			IgnoreOkToTest:       tc.IgnoreOkToTest,
			ElideSkippedContexts: tc.ElideSkippedContexts,
		}

		log.Printf("running case %s", tc.name)
		// In some cases handleGenericComment can be called twice for the same event.
		// For instance on Issue/PR creation and modification.
		// Let's call it twice to ensure idempotency.
		if err := handleGenericComment(c, trigger, event); err != nil {
			t.Fatalf("%s: didn't expect error: %s", tc.name, err)
		}
		validate(tc.name, fakeProwJobClient.Fake.Actions(), g, tc, t)
		if err := handleGenericComment(c, trigger, event); err != nil {
			t.Fatalf("%s: didn't expect error: %s", tc.name, err)
		}
		validate(tc.name, fakeProwJobClient.Fake.Actions(), g, tc, t)
	}
}

func validate(name string, actions []clienttesting.Action, g *fakegithub.FakeClient, tc testcase, t *testing.T) {
	startedContexts := sets.NewString()
	for _, action := range actions {
		switch action := action.(type) {
		case clienttesting.CreateActionImpl:
			if prowJob, ok := action.Object.(*prowapi.ProwJob); ok {
				startedContexts.Insert(prowJob.Spec.Context)
			}
		}
	}
	if len(startedContexts) > 0 && !tc.ShouldBuild {
		t.Errorf("Built but should not have: %+v", tc)
	} else if len(startedContexts) == 0 && tc.ShouldBuild {
		t.Errorf("Not built but should have: %+v", tc)
	}
	if tc.StartsExactly != "" && (startedContexts.Len() != 1 || !startedContexts.Has(tc.StartsExactly)) {
		t.Errorf("Didn't build expected context %v, instead built %v", tc.StartsExactly, startedContexts)
	}
	if tc.ShouldReport && len(g.CreatedStatuses) == 0 {
		t.Errorf("%s: Expected report to github", name)
	} else if !tc.ShouldReport && len(g.CreatedStatuses) > 0 {
		t.Errorf("%s: Expected no reports to github, but got %d: %v", name, len(g.CreatedStatuses), g.CreatedStatuses)
	}
	if !reflect.DeepEqual(g.IssueLabelsAdded, tc.AddedLabels) {
		t.Errorf("%s: expected %q to be added, got %q", name, tc.AddedLabels, g.IssueLabelsAdded)
	}
	if !reflect.DeepEqual(g.IssueLabelsRemoved, tc.RemovedLabels) {
		t.Errorf("%s: expected %q to be removed, got %q", name, tc.RemovedLabels, g.IssueLabelsRemoved)
	}
}

type orgRepoRef struct {
	org, repo, ref string
}

type fakeStatusGetter struct {
	status map[orgRepoRef]*github.CombinedStatus
	errors map[orgRepoRef]error
}

func (f *fakeStatusGetter) GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error) {
	key := orgRepoRef{org: org, repo: repo, ref: ref}
	if err, exists := f.errors[key]; exists {
		return nil, err
	}
	status, exists := f.status[key]
	if !exists {
		return nil, fmt.Errorf("failed to find status for %s/%s@%s", org, repo, ref)
	}
	return status, nil
}

func TestPresubmitFilter(t *testing.T) {
	statuses := &github.CombinedStatus{Statuses: []github.Status{
		{
			Context: "existing-successful",
			State:   github.StatusSuccess,
		},
		{
			Context: "existing-pending",
			State:   github.StatusPending,
		},
		{
			Context: "existing-error",
			State:   github.StatusError,
		},
		{
			Context: "existing-failure",
			State:   github.StatusFailure,
		},
	}}
	var testCases = []struct {
		name                 string
		honorOkToTest        bool
		body, org, repo, ref string
		presubmits           []config.Presubmit
		expected             [][]bool
		statusErr, expectErr bool
	}{
		{
			name: "test all comment selects all tests that don't need an explicit trigger",
			body: "/test all",
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
			},
			expected: [][]bool{{true, false, false}, {true, false, false}, {false, false, false}},
		},
		{
			name:          "honored ok-to-test comment selects all tests that don't need an explicit trigger",
			body:          "/ok-to-test",
			honorOkToTest: true,
			org:           "org",
			repo:          "repo",
			ref:           "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
			},
			expected: [][]bool{{true, false, false}, {true, false, false}, {false, false, false}},
		},
		{
			name:          "not honored ok-to-test comment selects no tests",
			body:          "/ok-to-test",
			honorOkToTest: false,
			org:           "org",
			repo:          "repo",
			ref:           "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
			},
			expected: [][]bool{{false, false, false}, {false, false, false}, {false, false, false}},
		},
		{
			name:       "statuses are not gathered unless retest is specified (will error but we should not see it)",
			body:       "not a command",
			org:        "org",
			repo:       "repo",
			ref:        "ref",
			presubmits: []config.Presubmit{},
			expected:   [][]bool{},
			statusErr:  true,
			expectErr:  false,
		},
		{
			name:       "statuses are gathered when retest is specified and gather error is propagated",
			body:       "/retest",
			org:        "org",
			repo:       "repo",
			ref:        "ref",
			presubmits: []config.Presubmit{},
			expected:   [][]bool{},
			statusErr:  true,
			expectErr:  true,
		},
		{
			name: "retest command selects for errored or failed contexts and required but missing contexts",
			body: "/retest",
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "successful-job",
					},
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "pending-job",
					},
					Reporter: config.Reporter{
						Context: "existing-pending",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "failure-job",
					},
					Reporter: config.Reporter{
						Context: "existing-failure",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "error-job",
					},
					Reporter: config.Reporter{
						Context: "existing-error",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "missing-always-runs",
					},
					Reporter: config.Reporter{
						Context: "missing-always-runs",
					},
					AlwaysRun: true,
				},
			},
			expected: [][]bool{{false, false, false}, {false, false, false}, {true, false, true}, {true, false, true}, {true, false, true}},
		},
		{
			name: "explicit test command filters for jobs that match",
			body: "/test trigger",
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
			},
			expected: [][]bool{{true, true, true}, {true, true, true}, {true, true, true}, {false, false, false}, {false, false, false}, {false, false, false}},
		},
		{
			name: "comments matching more than one case will select the union of presubmits",
			body: `/test trigger
/test all
/retest`,
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "successful-job",
					},
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "pending-job",
					},
					Reporter: config.Reporter{
						Context: "existing-pending",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "failure-job",
					},
					Reporter: config.Reporter{
						Context: "existing-failure",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "error-job",
					},
					Reporter: config.Reporter{
						Context: "existing-error",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "missing-always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "missing-always-runs",
					},
				},
			},
			expected: [][]bool{{true, false, false}, {true, false, false}, {true, true, true}, {false, false, false}, {false, false, false}, {true, false, true}, {true, false, true}, {true, false, true}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.presubmits) != len(testCase.expected) {
				t.Fatalf("%s: have %d presubmits but only %d expected filter outputs", testCase.name, len(testCase.presubmits), len(testCase.expected))
			}
			if err := config.SetPresubmitRegexes(testCase.presubmits); err != nil {
				t.Fatalf("%s: could not set presubmit regexes: %v", testCase.name, err)
			}
			fsg := &fakeStatusGetter{
				errors: map[orgRepoRef]error{},
				status: map[orgRepoRef]*github.CombinedStatus{},
			}
			key := orgRepoRef{org: testCase.org, repo: testCase.repo, ref: testCase.ref}
			if testCase.statusErr {
				fsg.errors[key] = errors.New("failure")
			} else {
				fsg.status[key] = statuses
			}
			filter, err := presubmitFilter(testCase.honorOkToTest, fsg, testCase.body, testCase.org, testCase.repo, testCase.ref, logrus.WithField("test-case", testCase.name))
			if testCase.expectErr && err == nil {
				t.Errorf("%s: expected an error creating the filter, but got none", testCase.name)
			}
			if !testCase.expectErr && err != nil {
				t.Errorf("%s: expected no error creating the filter, but got one: %v", testCase.name, err)
			}
			for i, presubmit := range testCase.presubmits {
				actualFiltered, actualForced, actualDefault := filter(presubmit)
				expectedFiltered, expectedForced, expectedDefault := testCase.expected[i][0], testCase.expected[i][1], testCase.expected[i][2]
				if actualFiltered != expectedFiltered {
					t.Errorf("%s: filter did not evaluate correctly, expected %v but got %v for %v", testCase.name, expectedFiltered, actualFiltered, presubmit.Name)
				}
				if actualForced != expectedForced {
					t.Errorf("%s: filter did not determine forced correctly, expected %v but got %v for %v", testCase.name, expectedForced, actualForced, presubmit.Name)
				}
				if actualDefault != expectedDefault {
					t.Errorf("%s: filter did not determine default correctly, expected %v but got %v for %v", testCase.name, expectedDefault, actualDefault, presubmit.Name)
				}
			}
		})
	}
}

func TestRetestFilter(t *testing.T) {
	var testCases = []struct {
		name           string
		failedContexts sets.String
		allContexts    sets.String
		presubmits     []config.Presubmit
		expected       [][]bool
	}{
		{
			name:           "retest filter matches jobs that produce contexts which have failed",
			failedContexts: sets.NewString("failed"),
			allContexts:    sets.NewString("failed", "succeeded"),
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "failed",
					},
					Reporter: config.Reporter{
						Context: "failed",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "succeeded",
					},
					Reporter: config.Reporter{
						Context: "succeeded",
					},
				},
			},
			expected: [][]bool{{true, false, true}, {false, false, true}},
		},
		{
			name:           "retest filter matches jobs that would run automatically and haven't yet ",
			failedContexts: sets.NewString(),
			allContexts:    sets.NewString("finished"),
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "finished",
					},
					Reporter: config.Reporter{
						Context: "finished",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "not-yet-run",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "not-yet-run",
					},
				},
			},
			expected: [][]bool{{false, false, true}, {true, false, true}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.presubmits) != len(testCase.expected) {
				t.Fatalf("%s: have %d presubmits but only %d expected filter outputs", testCase.name, len(testCase.presubmits), len(testCase.expected))
			}
			if err := config.SetPresubmitRegexes(testCase.presubmits); err != nil {
				t.Fatalf("%s: could not set presubmit regexes: %v", testCase.name, err)
			}
			filter := retestFilter(testCase.failedContexts, testCase.allContexts)
			for i, presubmit := range testCase.presubmits {
				actualFiltered, actualForced, actualDefault := filter(presubmit)
				expectedFiltered, expectedForced, expectedDefault := testCase.expected[i][0], testCase.expected[i][1], testCase.expected[i][2]
				if actualFiltered != expectedFiltered {
					t.Errorf("%s: filter did not evaluate correctly, expected %v but got %v for %v", testCase.name, expectedFiltered, actualFiltered, presubmit.Name)
				}
				if actualForced != expectedForced {
					t.Errorf("%s: filter did not determine forced correctly, expected %v but got %v for %v", testCase.name, expectedForced, actualForced, presubmit.Name)
				}
				if actualDefault != expectedDefault {
					t.Errorf("%s: filter did not determine default correctly, expected %v but got %v for %v", testCase.name, expectedDefault, actualDefault, presubmit.Name)
				}
			}
		})
	}
}

func TestDetermineSkippedPresubmits(t *testing.T) {
	var testCases = []struct {
		name                      string
		toTrigger, toSkipSuperset []config.Presubmit
		expectedToSkip            []config.Presubmit
	}{
		{
			name:           "no inputs leads to no output",
			toTrigger:      []config.Presubmit{},
			toSkipSuperset: []config.Presubmit{},
			expectedToSkip: nil,
		},
		{
			name:           "no superset of skips to choose from leads to no output",
			toTrigger:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}},
			toSkipSuperset: []config.Presubmit{},
			expectedToSkip: nil,
		},
		{
			name:           "disjoint sets of contexts leads to full skip set",
			toTrigger:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "bar"}}},
			toSkipSuperset: []config.Presubmit{{Reporter: config.Reporter{Context: "oof"}}, {Reporter: config.Reporter{Context: "rab"}}},
			expectedToSkip: []config.Presubmit{{Reporter: config.Reporter{Context: "oof"}}, {Reporter: config.Reporter{Context: "rab"}}},
		},
		{
			name:           "overlaps on context removes from skip set",
			toTrigger:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "bar"}}},
			toSkipSuperset: []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "rab"}}},
			expectedToSkip: []config.Presubmit{{Reporter: config.Reporter{Context: "rab"}}},
		},
		{
			name:           "full set of overlaps on context removes everything from skip set",
			toTrigger:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "bar"}}},
			toSkipSuperset: []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "bar"}}},
			expectedToSkip: nil,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := determineSkippedPresubmits(testCase.toTrigger, testCase.toSkipSuperset, logrus.WithField("test-case", testCase.name)), testCase.expectedToSkip; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: incorrect skipped presubmits determined: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

type fakeChangesGetter struct {
	shouldError bool
}

func (c *fakeChangesGetter) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	if c.shouldError {
		return nil, errors.New("error getting changes")
	}
	return nil, nil
}

func TestFilterPresubmits(t *testing.T) {
	var testCases = []struct {
		name                              string
		filter                            pjutil.Filter
		presubmits                        []config.Presubmit
		changesError                      bool
		expectedToTrigger, expectedToSkip []config.Presubmit
		expectErr                         bool
	}{
		{
			name: "nothing matches, nothing to run or skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return false, false, false
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError:      false,
			expectedToTrigger: nil,
			expectedToSkip:    nil,
			expectErr:         false,
		},
		{
			name: "everything matches and is forced to run, nothing to skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, true, true
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			expectedToSkip: nil,
			expectErr:      false,
		},
		{
			name: "error detecting if something should run, nothing to run or skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, false, false
			},
			presubmits: []config.Presubmit{{
				JobBase:             config.JobBase{Name: "errors"},
				Reporter:            config.Reporter{Context: "first"},
				RegexpChangeMatcher: config.RegexpChangeMatcher{RunIfChanged: "oopsie"},
			}, {
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError:      true,
			expectedToTrigger: nil,
			expectedToSkip:    nil,
			expectErr:         true,
		},
		{
			name: "some things match and are forced to run, nothing to skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return p.Name == "should-trigger", true, true
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}},
			expectedToSkip: nil,
			expectErr:      false,
		},
		{
			name: "everything matches and some things are forced to run, others should be skipped",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, p.Name == "should-trigger", p.Name == "should-trigger"
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "third"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			expectedToSkip: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "third"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			expectErr: false,
		},
		{
			name: "everything matches and some things are forced to run, others should be skipped, overlapping contexts filtered out",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, p.Name == "should-trigger", p.Name == "should-trigger"
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			expectedToSkip: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			expectErr: false,
		},
		{
			name: "everything matches and some things are forced to run, others should be skipped, overlapping contexts filtered out even if skipped comes first in the list",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, p.Name == "should-trigger", p.Name == "should-trigger"
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			expectedToSkip: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			expectErr: false,
		},
	}

	pr := &github.PullRequest{
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: "org",
				},
				Name: "repo",
			},
			Ref: "foobar",
		},
		Number: 1,
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualToTrigger, actualToSkip, err := filterPresubmits(testCase.filter, &fakeChangesGetter{shouldError: testCase.changesError}, pr, testCase.presubmits, logrus.WithField("test-case", testCase.name))
			if testCase.expectErr && err == nil {
				t.Errorf("%s: expected an error filtering presubmits, but got none", testCase.name)
			}
			if !testCase.expectErr && err != nil {
				t.Errorf("%s: expected no error filtering presubmits, but got one: %v", testCase.name, err)
			}
			if !reflect.DeepEqual(actualToTrigger, testCase.expectedToTrigger) {
				t.Errorf("%s: incorrect set of presubmits to skip: %s", testCase.name, diff.ObjectReflectDiff(actualToTrigger, testCase.expectedToTrigger))
			}
			if !reflect.DeepEqual(actualToSkip, testCase.expectedToSkip) {
				t.Errorf("%s: incorrect set of presubmits to skip: %s", testCase.name, diff.ObjectReflectDiff(actualToSkip, testCase.expectedToSkip))
			}
		})
	}
}

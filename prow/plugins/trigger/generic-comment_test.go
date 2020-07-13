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
	"log"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	clienttesting "k8s.io/client-go/testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pjutil"
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

	Author         string
	PRAuthor       string
	Body           string
	State          string
	IsPR           bool
	Branch         string
	ShouldBuild    bool
	AddedLabels    []string
	RemovedLabels  []string
	StartsExactly  string
	Presubmits     map[string][]config.Presubmit
	IssueLabels    []string
	IgnoreOkToTest bool
	AddedComment   string
}

func TestHandleGenericComment(t *testing.T) {
	helpComment := "The following commands are available to trigger jobs:\n* `/test job`\n* `/test jib`\n\n"
	helpTestAllWithJobsComment := fmt.Sprintf("Use `/test all` to run the following jobs:%s\n\n", "\n* `job`")
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

			Author:      "k8s-ci-robot",
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
			name: "Wrong branch",

			Author:      "trusted-member",
			Body:        "/test all",
			State:       "open",
			IsPR:        true,
			Branch:      "other",
			ShouldBuild: false,
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
			ShouldBuild: false,
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
		{
			name:   `help command "/test ?" lists available presubmits`,
			Author: "trusted-member",
			Body:   "/test ?",
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
							Context: "pull-job",
						},
						Trigger:      `(?m)^/test (?:.*? )?job(?: .*?)?$`,
						RerunCommand: `/test job`,
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
						RerunCommand: `/test jib`,
					},
				},
			},
			AddedComment: helpComment + "Use `/test all` to run all jobs.",
		},
		{
			name:   `help command "/test ?" uses RerunCommand field of presubmits`,
			Author: "trusted-member",
			Body:   "/test ?",
			State:  "open",
			IsPR:   true,
			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jub",
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context: "pull-jub",
						},
						Trigger:      `/rerun_command`,
						RerunCommand: `/rerun_command`,
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `/command_foo`,
						RerunCommand: `/command_foo`,
					},
				},
			},
			AddedComment: "@trusted-member: The following commands are available to trigger jobs:\n* `/rerun_command`\n* `/command_foo`\n\nUse `/test all` to run all jobs.",
		},
		{
			name:         "/test with no target results in a help message",
			Author:       "trusted-member",
			Body:         "/test",
			State:        "open",
			IsPR:         true,
			AddedComment: testWithoutTargetNote + helpComment + helpTestAllWithJobsComment,
		},
		{
			name:         "/test with no target but ? in the next line results in an invalid test command message",
			Author:       "trusted-member",
			Body:         "/test \r\n?",
			State:        "open",
			IsPR:         true,
			AddedComment: testWithoutTargetNote + helpComment + helpTestAllWithJobsComment,
		},
		{
			name:         "/retest with trailing words results in a help message",
			Author:       "trusted-member",
			Body:         "/retest FOO",
			State:        "open",
			IsPR:         true,
			AddedComment: retestWithTargetNote + helpComment + helpTestAllWithJobsComment,
		},
		{
			name:          "/retest without target but with lines following it, is valid",
			Author:        "trusted-member",
			Body:          "/retest \r\n/other-command",
			State:         "open",
			IsPR:          true,
			ShouldBuild:   true,
			StartsExactly: "pull-jib",
		},
		{
			name:         "/test with unknown target results in a help message",
			Author:       "trusted-member",
			Body:         "/test FOO",
			State:        "open",
			IsPR:         true,
			AddedComment: targetNotFoundNote + helpComment + helpTestAllWithJobsComment,
		},
		{
			name:   "help comment should list only eligible jobs under '/test all'",
			Author: "trusted-member",
			Body:   "/test ?",
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
							Context: "pull-job",
						},
						Trigger:      `(?m)^/test job$`,
						RerunCommand: `/test job`,
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
						RerunCommand: `/test jib`,
					},
				},
			},
			AddedComment: helpComment + helpTestAllWithJobsComment,
		},
		{
			name:   "when no jobs can be run with /test all, respond accordingly",
			Author: "trusted-member",
			Body:   "/test all",
			State:  "open",
			IsPR:   true,
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
						Trigger:      `(?m)^/test job$`,
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
						Trigger:      `(?m)^/test jib$`,
						RerunCommand: `/test jib`,
					},
				},
			},
			AddedComment: thereAreNoTestAllJobsNote + helpComment,
		},
		{
			name:   "available presubmits should not list those excluded by branch",
			Author: "trusted-member",
			Body:   "/test ?",
			State:  "open",
			IsPR:   true,

			Presubmits: map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "job-excluded-by-brancher",
						},
						Brancher: config.Brancher{
							SkipBranches: []string{"master"},
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context: "pull-job-excluded-by-brancher",
						},
						Trigger:      `(?m)^/test job-excluded$`,
						RerunCommand: `/test job-excluded`,
					},
					{
						JobBase: config.JobBase{
							Name: "job",
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context: "pull-job",
						},
						Trigger:      `(?m)^/test job$`,
						RerunCommand: `/test job`,
					},
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						Reporter: config.Reporter{
							Context: "pull-jib",
						},
						Trigger:      `(?m)^/test (?:.*? )?jib(?: .*?)?$`,
						RerunCommand: `/test jib`,
					},
				},
			},
			AddedComment: helpComment + helpTestAllWithJobsComment,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.Branch == "" {
				tc.Branch = "master"
			}
			g := &fakegithub.FakeClient{
				IssueComments: map[int][]github.IssueComment{},
				OrgMembers:    map[string][]string{"org": {"trusted-member"}},
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
				Collaborators: []string{"k8s-ci-robot"},
			}
			fakeConfig := &config.Config{ProwConfig: config.ProwConfig{ProwJobNamespace: "prowjobs"}}
			fakeProwJobClient := fake.NewSimpleClientset()
			c := Client{
				GitHubClient:  g,
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs(fakeConfig.ProwJobNamespace),
				Config:        fakeConfig,
				Logger:        logrus.WithField("plugin", PluginName),
				GitClient:     nil,
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
				IgnoreOkToTest: tc.IgnoreOkToTest,
			}
			trigger.SetDefaults()

			log.Printf("running case %s", tc.name)
			// In some cases handleGenericComment can be called twice for the same event.
			// For instance on Issue/PR creation and modification.
			// Let's call it twice to ensure idempotency.
			if err := handleGenericComment(c, trigger, event); err != nil {
				t.Fatalf("%s: didn't expect error: %s", tc.name, err)
			}
			validate(t, fakeProwJobClient.Fake.Actions(), g, tc)
			if err := handleGenericComment(c, trigger, event); err != nil {
				t.Fatalf("%s: didn't expect error: %s", tc.name, err)
			}
			validate(t, fakeProwJobClient.Fake.Actions(), g, tc)
		})
	}
}

func validate(t *testing.T, actions []clienttesting.Action, g *fakegithub.FakeClient, tc testcase) {
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
		t.Errorf("didn't build expected context %v, instead built %v", tc.StartsExactly, startedContexts)
	}
	if !reflect.DeepEqual(g.IssueLabelsAdded, tc.AddedLabels) {
		t.Errorf("expected %q to be added, got %q", tc.AddedLabels, g.IssueLabelsAdded)
	}
	if !reflect.DeepEqual(g.IssueLabelsRemoved, tc.RemovedLabels) {
		t.Errorf("expected %q to be removed, got %q", tc.RemovedLabels, g.IssueLabelsRemoved)
	}
	if tc.AddedComment != "" {
		if len(g.IssueComments[0]) == 0 {
			t.Errorf("expected the comments to contain %s, got no comments", tc.AddedComment)
		}
		for _, c := range g.IssueComments[0] {
			if !strings.Contains(c.Body, tc.AddedComment) {
				t.Errorf("expected the comment to contain %s, got %s", tc.AddedComment, c.Body)
			}
		}
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
			filter := pjutil.RetestFilter(testCase.failedContexts, testCase.allContexts)
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

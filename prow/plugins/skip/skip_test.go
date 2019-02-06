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

package skip

import (
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestSkipStatus(t *testing.T) {
	tests := []struct {
		name string

		presubmits []config.Presubmit
		sha        string
		event      *github.GenericCommentEvent
		prChanges  map[int][]github.PullRequestChange
		existing   []github.Status

		expected []github.Status
	}{
		{
			name: "Skip some tests",

			presubmits: []config.Presubmit{
				{
					AlwaysRun: true,
					Context:   "unit-tests",
				},
				{
					AlwaysRun: false,
					Context:   "extended-tests",
				},
				{
					AlwaysRun: false,
					Context:   "integration-tests",
				},
			},
			sha: "shalala",
			event: &github.GenericCommentEvent{
				IsPR:       true,
				IssueState: "open",
				Action:     github.GenericCommentActionCreated,
				Body:       "/skip",
				Number:     1,
				Repo:       github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			},
			existing: []github.Status{
				{
					State:   github.StatusSuccess,
					Context: "unit-tests",
				},
				{
					State:   github.StatusFailure,
					Context: "extended-tests",
				},
				{
					State:   github.StatusPending,
					Context: "integration-tests",
				},
			},

			expected: []github.Status{
				{
					State:   github.StatusSuccess,
					Context: "unit-tests",
				},
				{
					State:       github.StatusSuccess,
					Description: "Skipped",
					Context:     "extended-tests",
				},
				{
					State:       github.StatusSuccess,
					Description: "Skipped",
					Context:     "integration-tests",
				},
			},
		},
		{
			name: "Do not skip tests with PR changes that need to run",

			presubmits: []config.Presubmit{
				{
					AlwaysRun: true,
					Context:   "unit-tests",
				},
				{
					AlwaysRun: false,
					Context:   "extended-tests",
				},
				{
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^(test/integration)",
					},
					Context: "integration-tests",
				},
			},
			sha: "shalala",
			event: &github.GenericCommentEvent{
				IsPR:       true,
				IssueState: "open",
				Action:     github.GenericCommentActionCreated,
				Body:       "/skip",
				Number:     1,
				Repo:       github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			},
			existing: []github.Status{
				{
					State:   github.StatusSuccess,
					Context: "unit-tests",
				},
				{
					State:   github.StatusFailure,
					Context: "extended-tests",
				},
				{
					State:   github.StatusPending,
					Context: "integration-tests",
				},
			},
			prChanges: map[int][]github.PullRequestChange{
				1: {
					{
						Filename: "test/integration/main.go",
					},
					{
						Filename: "README.md",
					},
				},
			},

			expected: []github.Status{
				{
					State:   github.StatusSuccess,
					Context: "unit-tests",
				},
				{
					State:       github.StatusSuccess,
					Description: "Skipped",
					Context:     "extended-tests",
				},
				{
					State:   github.StatusPending,
					Context: "integration-tests",
				},
			},
		},
		{
			name: "Skip tests with PR changes that do not need to run",

			presubmits: []config.Presubmit{
				{
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^(test/integration)",
					},
					Context: "integration-tests",
				},
			},
			sha: "shalala",
			event: &github.GenericCommentEvent{
				IsPR:       true,
				IssueState: "open",
				Action:     github.GenericCommentActionCreated,
				Body:       "/skip",
				Number:     1,
				Repo:       github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			},
			existing: []github.Status{
				{
					State:   github.StatusPending,
					Context: "integration-tests",
				},
			},
			prChanges: map[int][]github.PullRequestChange{
				1: {
					{
						Filename: "build/core.sh",
					},
					{
						Filename: "README.md",
					},
				},
			},

			expected: []github.Status{
				{
					State:       github.StatusSuccess,
					Description: "Skipped",
					Context:     "integration-tests",
				},
			},
		},
		{
			name: "Skip broken but skippable tests",

			presubmits: []config.Presubmit{
				{
					SkipReport: true,
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^(test/integration)",
					},
					Context: "integration-tests",
				},
			},
			sha: "shalala",
			event: &github.GenericCommentEvent{
				IsPR:       true,
				IssueState: "open",
				Action:     github.GenericCommentActionCreated,
				Body:       "/skip",
				Number:     1,
				Repo:       github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			},
			existing: []github.Status{
				{
					State:   github.StatusPending,
					Context: "integration-tests",
				},
			},
			prChanges: map[int][]github.PullRequestChange{
				1: {
					{
						Filename: "test/integration/main.go",
					},
					{
						Filename: "README.md",
					},
				},
			},

			expected: []github.Status{
				{
					State:       github.StatusSuccess,
					Description: "Skipped",
					Context:     "integration-tests",
				},
			},
		},
	}

	for _, test := range tests {
		t.Logf("running scenario %q", test.name)
		if err := config.SetPresubmitRegexes(test.presubmits); err != nil {
			t.Fatal(err)
		}

		fghc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
			PullRequests: map[int]*github.PullRequest{
				test.event.Number: {
					Head: github.PullRequestBranch{
						SHA: test.sha,
					},
				},
			},
			PullRequestChanges: test.prChanges,
			CreatedStatuses: map[string][]github.Status{
				test.sha: test.existing,
			},
		}
		l := logrus.WithField("plugin", pluginName)

		if err := handle(fghc, l, test.event, test.presubmits); err != nil {
			t.Errorf("unexpected error: %v.", err)
			continue
		}

		// Check that the correct statuses have been updated.
		created := fghc.CreatedStatuses[test.sha]
		if len(test.expected) != len(created) {
			t.Errorf("status mismatch: expected:\n%+v\ngot:\n%+v", test.expected, created)
			continue
		}
	out:
		for _, got := range created {
			var found bool
			for _, exp := range test.expected {
				if exp.Context == got.Context {
					found = true
					if !reflect.DeepEqual(exp, got) {
						t.Errorf("expected status: %v, got: %v", exp, got)
						break out
					}
				}
			}
			if !found {
				t.Errorf("expected context %q in the results: %v", got.Context, created)
				break
			}
		}
	}
}

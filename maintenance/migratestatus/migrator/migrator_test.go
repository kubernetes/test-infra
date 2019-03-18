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

package migrator

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/scallywag"
)

type modeTest struct {
	name          string
	start         []scallywag.Status
	expectedDiffs []scallywag.Status
}

// compareDiffs checks if a list of status updates matches an expected list of status updates.
func compareDiffs(diffs []scallywag.Status, expectedDiffs []scallywag.Status) error {
	if len(diffs) != len(expectedDiffs) {
		return fmt.Errorf("failed because the returned diff had %d changes instead of %d", len(diffs), len(expectedDiffs))
	}
	for _, diff := range diffs {
		if diff.Context == "" {
			return fmt.Errorf("failed because the returned diff contained a Status with an empty Context field")
		}
		if diff.Description == "" {
			return fmt.Errorf("failed because the returned diff contained a Status with an empty Description field")
		}
		if diff.State == "" {
			return fmt.Errorf("failed because the returned diff contained a Status with an empty State field")
		}
		var match scallywag.Status
		var found bool
		for _, expected := range expectedDiffs {
			if expected.Context == diff.Context {
				match = expected
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("failed because the returned diff contained an unexpected change to context '%s'", diff.Context)
		}
		// Found a matching context. Make sure that fields are equal.
		if match.Description != diff.Description {
			return fmt.Errorf("failed because the returned diff for context '%s' had Description '%s' instead of '%s'", diff.Context, diff.Description, match.Description)
		}
		if match.State != diff.State {
			return fmt.Errorf("failed because the returned diff for context '%s' had State '%s' instead of '%s'", diff.Context, diff.State, match.State)
		}

		if match.TargetURL == "" {
			if diff.TargetURL != "" {
				return fmt.Errorf("failed because the returned diff for context '%s' had a non-empty TargetURL", diff.Context)
			}
		} else if diff.TargetURL == "" {
			return fmt.Errorf("failed because the returned diff for context '%s' had an empty TargetURL", diff.Context)
		} else if match.TargetURL != diff.TargetURL {
			return fmt.Errorf("failed because the returned diff for context '%s' had TargetURL '%s' instead of '%s'", diff.Context, diff.TargetURL, match.TargetURL)
		}
	}
	return nil
}

func TestMoveMode(t *testing.T) {
	contextA := "context A"
	contextB := "context B"
	desc := "Context retired. Status moved to \"context B\"."

	tests := []*modeTest{
		{
			name: "simple",
			start: []scallywag.Status{
				makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextA, "success", desc, ""),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "unrelated contexts",
			start: []scallywag.Status{
				makeStatus("also not related", "error", "description 4", "url 4"),
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus("unrelated context", "success", "description 2", "url 2"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextA, "success", desc, ""),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "unrelated contexts; missing context A",
			start: []scallywag.Status{
				makeStatus("also not related", "error", "description 4", "url 4"),
				makeStatus("unrelated context", "success", "description 2", "url 2"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "unrelated contexts; already have context A and B",
			start: []scallywag.Status{
				makeStatus("also not related", "error", "description 4", "url 4"),
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "unrelated contexts; already have context B; no context A",
			start: []scallywag.Status{
				makeStatus("also not related", "error", "description 4", "url 4"),
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name:          "no contexts",
			start:         []scallywag.Status{},
			expectedDiffs: []scallywag.Status{},
		},
	}

	m := *MoveMode(contextA, contextB, "")
	for _, test := range tests {
		diff := m.processStatuses(&scallywag.CombinedStatus{Statuses: test.start})
		if err := compareDiffs(diff, test.expectedDiffs); err != nil {
			t.Errorf("MoveMode test '%s' %v\n", test.name, err)
		}
	}
}

func TestCopyMode(t *testing.T) {
	contextA := "context A"
	contextB := "context B"

	tests := []*modeTest{
		{
			name: "simple",
			start: []scallywag.Status{
				makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "unrelated contexts",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "already have context B",
			start: []scallywag.Status{
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "already have updated context B",
			start: []scallywag.Status{
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus(contextB, "success", "description 2", "url 2"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "unrelated contexts already have updated context B",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus("also not related", "error", "description 4", "url 4"),
				makeStatus(contextB, "error", "description 3", "url 3"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "only have context B",
			start: []scallywag.Status{
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "unrelated contexts; context B but not A",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextB, "failure", "description 1", "url 1"),
				makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name:          "no contexts",
			start:         []scallywag.Status{},
			expectedDiffs: []scallywag.Status{},
		},
	}

	m := *CopyMode(contextA, contextB)
	for _, test := range tests {
		diff := m.processStatuses(&scallywag.CombinedStatus{Statuses: test.start})
		if err := compareDiffs(diff, test.expectedDiffs); err != nil {
			t.Errorf("CopyMode test '%s' %v\n", test.name, err)
		}
	}
}

func TestRetireModeReplacement(t *testing.T) {
	contextA := "context A"
	contextB := "context B"
	desc := "Context retired. Status moved to \"context B\"."

	tests := []*modeTest{
		{
			name: "simple",
			start: []scallywag.Status{
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name: "unrelated contexts;updated context B",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus("also not related", "error", "description 4", "url 4"),
				makeStatus(contextB, "success", "description 3", "url 3"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name: "missing context B",
			start: []scallywag.Status{
				makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "unrelated contexts;missing context B",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "missing context A",
			start: []scallywag.Status{
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "unrelated contexts;missing context A",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus("also not related", "error", "description 4", "url 4"),
				makeStatus(contextB, "success", "description 3", "url 3"),
			},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name:          "no contexts",
			start:         []scallywag.Status{},
			expectedDiffs: []scallywag.Status{},
		},
	}

	m := *RetireMode(contextA, contextB, "")
	for _, test := range tests {
		diff := m.processStatuses(&scallywag.CombinedStatus{Statuses: test.start})
		if err := compareDiffs(diff, test.expectedDiffs); err != nil {
			t.Errorf("RetireMode(Replacement) test '%s' %v\n", test.name, err)
		}
	}
}

func TestRetireModeNoReplacement(t *testing.T) {
	contextA := "context A"
	desc := "Context retired without replacement."

	tests := []*modeTest{
		{
			name: "simple",
			start: []scallywag.Status{
				makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name: "unrelated contexts",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus(contextA, "failure", "description 1", "url 1"),
				makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []scallywag.Status{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name:          "missing context A",
			start:         []scallywag.Status{},
			expectedDiffs: []scallywag.Status{},
		},
		{
			name: "unrelated contexts;missing context A",
			start: []scallywag.Status{
				makeStatus("unrelated context", "success", "description 2", "url 2"),
				makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []scallywag.Status{},
		},
	}

	m := *RetireMode(contextA, "", "")
	for _, test := range tests {
		diff := m.processStatuses(&scallywag.CombinedStatus{Statuses: test.start})
		if err := compareDiffs(diff, test.expectedDiffs); err != nil {
			t.Errorf("RetireMode(NoReplace) test '%s' %v\n", test.name, err)
		}
	}
}

// makeStatus returns a new Status struct with the specified fields.
// targetURL=="" means TargetURL==nil
func makeStatus(context, state, description, targetURL string) scallywag.Status {
	var url string
	if targetURL != "" {
		url = targetURL
	}
	return scallywag.Status{
		Context:     context,
		State:       state,
		Description: description,
		TargetURL:   url,
	}
}

type refID struct {
	org, repo, ref string
}

type fakeGitHubClient struct {
	statusesRetrieved map[refID]interface{}
}

func (c *fakeGitHubClient) GetCombinedStatus(org, repo, ref string) (*scallywag.CombinedStatus, error) {
	c.statusesRetrieved[refID{org: org, repo: repo, ref: ref}] = nil
	return nil, errors.New("return error to stop execution early")
}

func (c *fakeGitHubClient) CreateStatus(org, repo, SHA string, s scallywag.Status) error {
	return nil
}

func (c *fakeGitHubClient) GetPullRequests(org, repo string) ([]scallywag.PullRequest, error) {
	return []scallywag.PullRequest{}, nil
}

func TestProcessPR(t *testing.T) {
	var testCases = []struct {
		name    string
		matches bool
	}{
		{
			name:    "branch matching filter should proceed",
			matches: true,
		},
		{
			name:    "branch not matching filter should not proceed",
			matches: false,
		},
	}

	for _, testCase := range testCases {
		client := fakeGitHubClient{statusesRetrieved: map[refID]interface{}{}}
		var filteredBranch string
		migrator := Migrator{
			org:  "org",
			repo: "repo",
			targetBranchFilter: func(branch string) bool {
				filteredBranch = branch
				return testCase.matches
			},
			client: &client,
		}
		migrator.processPR(scallywag.PullRequest{Base: scallywag.PullRequestBranch{Ref: "branch"}, Head: scallywag.PullRequestBranch{SHA: "fake"}})
		if filteredBranch != "branch" {
			t.Errorf("%s: failed to use filter on branch", testCase.name)
		}

		_, retrieved := client.statusesRetrieved[refID{org: "org", repo: "repo", ref: "fake"}]
		if testCase.matches && !retrieved {
			t.Errorf("%s: failed to process a PR that matched", testCase.name)
		}
		if !testCase.matches && retrieved {
			t.Errorf("%s: processed a PR that didn't match", testCase.name)
		}
	}
}

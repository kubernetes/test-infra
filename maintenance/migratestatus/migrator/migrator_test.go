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

	"github.com/google/go-github/github"
)

type modeTest struct {
	name          string
	start         []github.RepoStatus
	expectedDiffs []*github.RepoStatus
}

// compareDiffs checks if a list of status updates matches an expected list of status updates.
func compareDiffs(diffs []*github.RepoStatus, expectedDiffs []*github.RepoStatus) error {
	if len(diffs) != len(expectedDiffs) {
		return fmt.Errorf("failed because the returned diff had %d changes instead of %d", len(diffs), len(expectedDiffs))
	}
	for _, diff := range diffs {
		if diff == nil {
			return fmt.Errorf("failed because the returned diff contained a nil RepoStatus")
		}
		if diff.Context == nil {
			return fmt.Errorf("failed because the returned diff contained a RepoStatus with a nil Context field")
		}
		if diff.Description == nil {
			return fmt.Errorf("failed because the returned diff contained a RepoStatus with a nil Description field")
		}
		if diff.State == nil {
			return fmt.Errorf("failed because the returned diff contained a RepoStatus with a nil State field")
		}
		var match *github.RepoStatus
		for _, expected := range expectedDiffs {
			if *expected.Context == *diff.Context {
				match = expected
				break
			}
		}
		if match == nil {
			return fmt.Errorf("failed because the returned diff contained an unexpected change to context '%s'", *diff.Context)
		}
		// Found a matching context. Make sure that fields are equal.
		if *match.Description != *diff.Description {
			return fmt.Errorf("failed because the returned diff for context '%s' had Description '%s' instead of '%s'", *diff.Context, *diff.Description, *match.Description)
		}
		if *match.State != *diff.State {
			return fmt.Errorf("failed because the returned diff for context '%s' had State '%s' instead of '%s'", *diff.Context, *diff.State, *match.State)
		}

		if match.TargetURL == nil {
			if diff.TargetURL != nil {
				return fmt.Errorf("failed because the returned diff for context '%s' had a non-nil TargetURL", *diff.Context)
			}
		} else if diff.TargetURL == nil {
			return fmt.Errorf("failed because the returned diff for context '%s' had a nil TargetURL", *diff.Context)
		} else if *match.TargetURL != *diff.TargetURL {
			return fmt.Errorf("failed because the returned diff for context '%s' had TargetURL '%s' instead of '%s'", *diff.Context, *diff.TargetURL, *match.TargetURL)
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
			start: []github.RepoStatus{
				*makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextA, "success", desc, ""),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "unrelated contexts",
			start: []github.RepoStatus{
				*makeStatus("also not related", "error", "description 4", "url 4"),
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextA, "success", desc, ""),
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "unrelated contexts; missing context A",
			start: []github.RepoStatus{
				*makeStatus("also not related", "error", "description 4", "url 4"),
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "unrelated contexts; already have context A and B",
			start: []github.RepoStatus{
				*makeStatus("also not related", "error", "description 4", "url 4"),
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "unrelated contexts; already have context B; no context A",
			start: []github.RepoStatus{
				*makeStatus("also not related", "error", "description 4", "url 4"),
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name:          "no contexts",
			start:         []github.RepoStatus{},
			expectedDiffs: []*github.RepoStatus{},
		},
	}

	m := *MoveMode(contextA, contextB)
	for _, test := range tests {
		diff := m.processStatuses(&github.CombinedStatus{Statuses: test.start})
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
			start: []github.RepoStatus{
				*makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "unrelated contexts",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextB, "failure", "description 1", "url 1"),
			},
		},
		{
			name: "already have context B",
			start: []github.RepoStatus{
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "already have updated context B",
			start: []github.RepoStatus{
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus(contextB, "success", "description 2", "url 2"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "unrelated contexts already have updated context B",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
				*makeStatus(contextB, "error", "description 3", "url 3"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "only have context B",
			start: []github.RepoStatus{
				*makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "unrelated contexts; context B but not A",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextB, "failure", "description 1", "url 1"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name:          "no contexts",
			start:         []github.RepoStatus{},
			expectedDiffs: []*github.RepoStatus{},
		},
	}

	m := *CopyMode(contextA, contextB)
	for _, test := range tests {
		diff := m.processStatuses(&github.CombinedStatus{Statuses: test.start})
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
			start: []github.RepoStatus{
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name: "unrelated contexts;updated context B",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
				*makeStatus(contextB, "success", "description 3", "url 3"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name: "missing context B",
			start: []github.RepoStatus{
				*makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "unrelated contexts;missing context B",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "missing context A",
			start: []github.RepoStatus{
				*makeStatus(contextB, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "unrelated contexts;missing context A",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
				*makeStatus(contextB, "success", "description 3", "url 3"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name:          "no contexts",
			start:         []github.RepoStatus{},
			expectedDiffs: []*github.RepoStatus{},
		},
	}

	m := *RetireMode(contextA, contextB)
	for _, test := range tests {
		diff := m.processStatuses(&github.CombinedStatus{Statuses: test.start})
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
			start: []github.RepoStatus{
				*makeStatus(contextA, "failure", "description 1", "url 1"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name: "unrelated contexts",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus(contextA, "failure", "description 1", "url 1"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []*github.RepoStatus{
				makeStatus(contextA, "success", desc, ""),
			},
		},
		{
			name:          "missing context A",
			start:         []github.RepoStatus{},
			expectedDiffs: []*github.RepoStatus{},
		},
		{
			name: "unrelated contexts;missing context A",
			start: []github.RepoStatus{
				*makeStatus("unrelated context", "success", "description 2", "url 2"),
				*makeStatus("also not related", "error", "description 4", "url 4"),
			},
			expectedDiffs: []*github.RepoStatus{},
		},
	}

	m := *RetireMode(contextA, "")
	for _, test := range tests {
		diff := m.processStatuses(&github.CombinedStatus{Statuses: test.start})
		if err := compareDiffs(diff, test.expectedDiffs); err != nil {
			t.Errorf("RetireMode(NoReplace) test '%s' %v\n", test.name, err)
		}
	}
}

// makeStatus returns a new RepoStatus struct with the specified fields.
// targetURL=="" means TargetURL==nil
func makeStatus(context, state, description, targetURL string) *github.RepoStatus {
	var url *string
	if targetURL != "" {
		url = &targetURL
	}
	return &github.RepoStatus{
		Context:     &context,
		State:       &state,
		Description: &description,
		TargetURL:   url,
	}
}

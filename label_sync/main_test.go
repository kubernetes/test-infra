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

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// Tests for getting data from GitHub are not needed:
// The would have to use real API point or test stubs

// Test func (c Configuration) validate(orgs string) error
// Input: Configuration list
func TestValidate(t *testing.T) {
	var testcases = []struct {
		name          string
		config        Configuration
		expectedError bool
	}{
		{
			name: "All empty",
		},
		{
			name: "Duplicate wanted label",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				{Name: "lab1", Description: "Test Label 1", Color: "befade"},
			}}},
			expectedError: true,
		},
		{
			name: "Required label has non unique labels when downcased",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				{Name: "LAB1", Description: "Test Label 2", Color: "deadbe"},
			}}},
			expectedError: true,
		},
		{
			name: "Required label defined in default and repo1",
			config: Configuration{
				Default: RepoConfig{Labels: []Label{
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				}},
				Repos: map[string]RepoConfig{
					"org/repo1": {Labels: []Label{
						{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					}},
				},
			},
			expectedError: true,
		},
		{
			name: "Org2 not in orgs, should warn in logs",
			config: Configuration{
				Default: RepoConfig{Labels: []Label{
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				}},
				Repos: map[string]RepoConfig{
					"org2/repo1": {Labels: []Label{
						{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
					}},
				},
			},
			expectedError: false,
		},
		{
			name: "Required label defined in default and org",
			config: Configuration{
				Default: RepoConfig{Labels: []Label{
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				}},
				Orgs: map[string]RepoConfig{
					"org": {Labels: []Label{
						{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					}},
				},
			},
			expectedError: true,
		},
		{
			name: "Required label defined in org and repo",
			config: Configuration{
				Orgs: map[string]RepoConfig{
					"org": {Labels: []Label{
						{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					}},
				},
				Repos: map[string]RepoConfig{
					"org/repo1": {Labels: []Label{
						{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					}},
				},
			},
			expectedError: true,
		},
	}
	// Do tests
	for _, tc := range testcases {
		err := tc.config.validate("org")
		if err == nil && tc.expectedError {
			t.Errorf("%s: failed to raise error", tc.name)
		} else if err != nil && !tc.expectedError {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
	}
}

// Test syncLabels(config *Configuration, curr *RepoLabels) (updates RepoUpdates, err error)
// Input: Configuration list and Current labels list on multiple repos
// Output: list of wanted label updates (update due to name or color) addition due to missing labels
// This is main testing for this program
func TestSyncLabels(t *testing.T) {
	var testcases = []struct {
		name            string
		config          Configuration
		current         RepoLabels
		expectedUpdates RepoUpdates
		expectedError   bool
		now             time.Time
	}{
		{
			name: "Required label defined in repo1 and repo2 - no update",
			config: Configuration{
				Default: RepoConfig{Labels: []Label{
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				}},
				Repos: map[string]RepoConfig{
					"org/repo1": {Labels: []Label{
						{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
					}},
					"org/repo2": {Labels: []Label{
						{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
					}},
				},
			},
			current: RepoLabels{
				"repo1": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
				},
				"repo2": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
				},
			},
		},
		{
			name: "Required label defined in repo1 and repo2 - update required",
			config: Configuration{
				Default: RepoConfig{Labels: []Label{
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				}},
				Repos: map[string]RepoConfig{
					"org/repo1": {Labels: []Label{
						{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
					}},
					"org/repo2": {Labels: []Label{
						{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
					}},
				},
			},
			current: RepoLabels{
				"repo1": {
					{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
				},
				"repo2": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
				},
			},
			expectedUpdates: RepoUpdates{
				"repo1": {
					{repo: "repo1", Why: "missing", Wanted: &Label{Name: "lab1", Description: "Test Label 1", Color: "deadbe"}}},
			},
		},
		{
			name: "Required label defined on org-level - update required on one repo",
			config: Configuration{
				Default: RepoConfig{Labels: []Label{
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				}},
				Orgs: map[string]RepoConfig{
					"org": {Labels: []Label{
						{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
					}},
				},
			},
			current: RepoLabels{
				"repo1": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					{Name: "lab2", Description: "Test Label 2", Color: "deadbe"},
				},
				"repo2": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				},
			},
			expectedUpdates: RepoUpdates{
				"repo2": {
					{repo: "repo2", Why: "missing", Wanted: &Label{Name: "lab2", Description: "Test Label 2", Color: "deadbe"}}},
			},
		},
		{
			name: "Duplicate label on repo1",
			current: RepoLabels{
				"repo1": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					{Name: "lab1", Description: "Test Label 1", Color: "befade"},
				},
			},
			expectedError: true,
		},
		{
			name: "Non unique label on repo1 when downcased",
			current: RepoLabels{
				"repo1": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
					{Name: "LAB1", Description: "Test Label 2", Color: "deadbe"},
				},
			},
			expectedError: true,
		},
		{
			name: "Non unique label but on different repos - allowed",
			current: RepoLabels{
				"repo1": {{Name: "lab1", Description: "Test Label 1", Color: "deadbe"}},
				"repo2": {{Name: "lab1", Description: "Test Label 1", Color: "deadbe"}},
			},
		},
		{
			name: "Repo has exactly all wanted labels",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
			}}},
			current: RepoLabels{
				"repo1": {
					{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
				},
			},
		},
		{
			name: "Repo has label with wrong color",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
			}}},
			current: RepoLabels{
				"repo1": {
					{Name: "lab1", Description: "Test Label 1", Color: "bebeef"},
				},
			},
			expectedUpdates: RepoUpdates{
				"repo1": {
					{Why: "change", Current: &Label{Name: "lab1", Description: "Test Label 1", Color: "deadbe"}, Wanted: &Label{Name: "lab1", Description: "Test Label 1", Color: "deadbe"}},
				},
			},
		},
		{
			name: "Repo has label with wrong description",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "lab1", Description: "Test Label 1", Color: "deadbe"},
			}}},
			current: RepoLabels{
				"repo1": {
					{Name: "lab1", Description: "Test Label 5", Color: "deadbe"},
				},
			},
			expectedUpdates: RepoUpdates{
				"repo1": {
					{Why: "change", Current: &Label{Name: "lab1", Description: "Test Label 1", Color: "deadbe"}, Wanted: &Label{Name: "lab1", Description: "Test Label 1", Color: "deadbe"}},
				},
			},
		},
		{
			name: "Repo has label with wrong name (different case)",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "Lab1", Description: "Test Label 1", Color: "deadbe"},
			}}},
			current: RepoLabels{
				"repo1": {
					{Name: "laB1", Description: "Test Label 1", Color: "deadbe"},
				},
			},
			expectedUpdates: RepoUpdates{
				"repo1": {
					{Why: "rename", Wanted: &Label{Name: "Lab1", Description: "Test Label 1", Color: "deadbe"}, Current: &Label{Name: "laB1", Description: "Test Label 1", Color: "deadbe"}},
				},
			},
		},
		{
			name: "old name",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "current", Description: "Test Label 1", Color: "blue", Previously: []Label{{Name: "old", Description: "Test Label 1", Color: "gray"}}},
			}}},
			current: RepoLabels{
				"no current": {{Name: "old", Description: "Test Label 1", Color: "much gray"}},
				"has current": {
					{Name: "old", Description: "Test Label 1", Color: "gray"},
					{Name: "current", Description: "Test Label 1", Color: "blue"},
				},
			},
			expectedUpdates: RepoUpdates{
				"no current": {
					{Why: "rename", Current: &Label{Name: "old", Description: "Test Label 1", Color: "much gray"}, Wanted: &Label{Name: "current", Description: "Test Label 1", Color: "blue"}},
				},
				"has current": {
					{Why: "migrate", Current: &Label{Name: "old", Description: "Test Label 1", Color: "gray"}, Wanted: &Label{Name: "current", Description: "Test Label 1", Color: "blue"}},
				},
			},
		},
		{
			name: "Repo is missing a label",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "Lab1", Description: "Test Label 1", Color: "deadbe"},
			}}},
			current: RepoLabels{
				"repo1": {},
			},
			expectedUpdates: RepoUpdates{
				"repo1": {
					{Why: "missing", Wanted: &Label{Name: "Lab1", Description: "Test Label 1", Color: "deadbe"}},
				},
			},
		},
		{
			name: "Repo is missing multiple labels, and expected labels order is changed",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "Lab1", Description: "Test Label 1", Color: "deadbe"},
				{Name: "Lab2", Description: "Test Label 2", Color: "000000"},
				{Name: "Lab3", Description: "Test Label 3", Color: "ffffff"},
			}}},
			current: RepoLabels{
				"repo1": {},
				"repo2": {{Name: "Lab2", Description: "Test Label 2", Color: "000000"}},
			},
			expectedUpdates: RepoUpdates{
				"repo2": {
					{Why: "missing", Wanted: &Label{Name: "Lab3", Description: "Test Label 3", Color: "ffffff"}},
					{Why: "missing", Wanted: &Label{Name: "Lab1", Description: "Test Label 1", Color: "deadbe"}},
				},
				"repo1": {
					{Why: "missing", Wanted: &Label{Color: "000000", Name: "Lab2", Description: "Test Label 2"}},
					{Why: "missing", Wanted: &Label{Name: "Lab3", Description: "Test Label 3", Color: "ffffff"}},
					{Why: "missing", Wanted: &Label{Name: "Lab1", Description: "Test Label 1", Color: "deadbe"}},
				},
			},
		},
		{
			name: "Multiple repos complex case",
			config: Configuration{Default: RepoConfig{Labels: []Label{
				{Name: "priority/P0", Description: "P0 Priority", Color: "ff0000"},
				{Name: "lgtm", Description: "LGTM", Color: "00ff00"},
			}}},
			current: RepoLabels{
				"repo1": {
					{Name: "Priority/P0", Description: "P0 Priority", Color: "ee3333"},
					{Name: "LGTM", Description: "LGTM", Color: "00ff00"},
				},
				"repo2": {
					{Name: "priority/P0", Description: "P0 Priority", Color: "ee3333"},
					{Name: "lgtm", Description: "LGTM", Color: "00ff00"},
				},
				"repo3": {
					{Name: "PRIORITY/P0", Description: "P0 Priority", Color: "ff0000"},
					{Name: "lgtm", Description: "LGTM", Color: "0000ff"},
				},
				"repo4": {
					{Name: "priority/P0", Description: "P0 Priority", Color: "ff0000"},
				},
				"repo5": {
					{Name: "lgtm", Description: "LGTM", Color: "00ff00"},
				},
			},
			expectedUpdates: RepoUpdates{
				"repo1": {
					{Why: "rename", Wanted: &Label{Name: "priority/P0", Description: "P0 Priority", Color: "ff0000"}, Current: &Label{Name: "Priority/P0", Description: "P0 Priority", Color: "ee3333"}},
					{Why: "rename", Wanted: &Label{Name: "lgtm", Description: "LGTM", Color: "00ff00"}, Current: &Label{Name: "LGTM", Description: "LGTM", Color: "00ff00"}},
				},
				"repo2": {
					{Why: "change", Current: &Label{Name: "priority/P0", Description: "P0 Priority", Color: "ff0000"}, Wanted: &Label{Name: "priority/P0", Description: "P0 Priority", Color: "ff0000"}},
				},
				"repo3": {
					{Why: "rename", Wanted: &Label{Name: "priority/P0", Description: "P0 Priority", Color: "ff0000"}, Current: &Label{Name: "PRIORITY/P0", Description: "P0 Priority", Color: "ff0000"}},
					{Why: "change", Current: &Label{Name: "lgtm", Description: "LGTM", Color: "00ff00"}, Wanted: &Label{Name: "lgtm", Description: "LGTM", Color: "00ff00"}},
				},
				"repo4": {
					{Why: "missing", Wanted: &Label{Name: "lgtm", Description: "LGTM", Color: "00ff00"}},
				},
				"repo5": {
					{Why: "missing", Wanted: &Label{Name: "priority/P0", Description: "P0 Priority", Color: "ff0000"}},
				},
			},
		},
	}

	// Do tests
	for _, tc := range testcases {
		actualUpdates, err := syncLabels(tc.config, "org", tc.current)
		if err == nil && tc.expectedError {
			t.Errorf("%s: failed to raise error", tc.name)
		} else if err != nil && !tc.expectedError {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		} else if !tc.expectedError && !equalUpdates(actualUpdates, tc.expectedUpdates, t) {
			t.Errorf("%s: expected updates:\n%+v\ngot:\n%+v", tc.name, tc.expectedUpdates, actualUpdates)
		}
	}
}

// This is needed to compare Update sets, two update sets are equal
// only if their maps have the same lists (but order can be different)
// Using standard `reflect.DeepEqual` for entire structures makes tests flaky
func equalUpdates(updates1, updates2 RepoUpdates, t *testing.T) bool {
	if len(updates1) != len(updates2) {
		t.Errorf("ERROR: expected and actual update sets have different repo sets")
		return false
	}
	// Iterate per repository differences
	for repo, list1 := range updates1 {
		list2, ok := updates2[repo]
		if !ok || len(list1) != len(list2) {
			t.Errorf("ERROR: expected and actual update lists for repo %s have different lengths", repo)
			return false
		}
		items1 := make(map[string]bool)
		for _, item := range list1 {
			j, err := json.Marshal(item)
			if err != nil {
				t.Errorf("ERROR: internal test error: unable to json.Marshal test item: %+v", item)
				return false
			}
			items1[string(j)] = true
		}
		items2 := make(map[string]bool)
		for _, item := range list2 {
			j, err := json.Marshal(item)
			if err != nil {
				t.Errorf("ERROR: internal test error: unable to json.Marshal test item: %+v", item)
				return false
			}
			items2[string(j)] = true
		}
		// Iterate list of label differences
		for key := range items1 {
			_, ok := items2[key]
			if !ok {
				t.Errorf("ERROR: difference: repo: %s, key: %s not found", repo, key)
				return false
			}
		}
	}
	return true
}

// Test loading YAML file (labels.yaml)
func TestLoadYAML(t *testing.T) {
	d := time.Date(2017, 1, 1, 13, 0, 0, 0, time.UTC)
	var testcases = []struct {
		path     string
		expected Configuration
		ok       bool
		errMsg   string
	}{
		{
			path: "labels_example.yaml",
			expected: Configuration{
				Default: RepoConfig{Labels: []Label{
					{Name: "lgtm", Description: "LGTM", Color: "green"},
					{Name: "priority/P0", Description: "P0 Priority", Color: "red", Previously: []Label{{Name: "P0", Description: "P0 Priority", Color: "blue"}}},
					{Name: "dead-label", Description: "Delete Me :)", DeleteAfter: &d},
				}},
				Orgs:  map[string]RepoConfig{"org": {Labels: []Label{{Name: "sgtm", Description: "Sounds Good To Me", Color: "green"}}}},
				Repos: map[string]RepoConfig{"org/repo": {Labels: []Label{{Name: "tgtm", Description: "Tastes Good To Me", Color: "blue"}}}},
			},
			ok: true,
		},
		{
			path:     "syntax_error_example.yaml",
			expected: Configuration{},
			ok:       false,
			errMsg:   "error converting",
		},
		{
			path:     "no_such_file.yaml",
			expected: Configuration{},
			ok:       false,
			errMsg:   "no such file",
		},
	}
	for i, tc := range testcases {
		actual, err := LoadConfig(tc.path, "org")
		errNil := err == nil
		if errNil != tc.ok {
			t.Errorf("TestLoadYAML: test case number %d, expected ok: %v, got %v (error=%v)", i+1, tc.ok, err == nil, err)
		}
		if !errNil && !strings.Contains(err.Error(), tc.errMsg) {
			t.Errorf("TestLoadYAML: test case number %d, expected error '%v' to contain '%v'", i+1, err.Error(), tc.errMsg)
		}
		if diff := cmp.Diff(actual, &tc.expected, cmpopts.IgnoreUnexported(Label{})); errNil && diff != "" {
			t.Errorf("TestLoadYAML: test case number %d, labels differ:%s", i+1, diff)
		}
	}
}

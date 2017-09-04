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
	"errors"
	"reflect"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
)

// Tests for getting data from GitHub are not needed:
// The would have to use real API point or test stubs

// Test SyncLabels(required *RequiredLabels, curr *RepoLabels) (updates UpdateData, err error)
// Input: RequiredLabels list and Current labels list on multiple repos
// Output: list of required label updates (update due to name or color) addition due to missing labels
// This is main testing for this program
func TestSyncLabels(t *testing.T) {
	var testcases = []struct {
		name            string
		requiredLabels  RequiredLabels
		currentLabels   RepoLabels
		expectedUpdates UpdateData
		expectedError   error
	}{
		{
			name:            "All empty",
			requiredLabels:  RequiredLabels{},
			currentLabels:   RepoLabels{},
			expectedUpdates: UpdateData{},
		},
		{
			name: "Duplicate required label",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "lab1", Color: "deadbe"},
				{Name: "lab1", Color: "befade"},
			}},
			currentLabels:   RepoLabels{},
			expectedUpdates: UpdateData{},
			expectedError:   errors.New("label lab1 is not unique when downcased in input config"),
		},
		{
			name: "Required label has non unique labels when downcased",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "lab1", Color: "deadbe"},
				{Name: "LAB1", Color: "deadbe"},
			}},
			currentLabels:   RepoLabels{},
			expectedUpdates: UpdateData{},
			expectedError:   errors.New("label LAB1 is not unique when downcased in input config"),
		},
		{
			name:           "Duplicate label on repo1",
			requiredLabels: RequiredLabels{},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {
					{Name: "lab1", Color: "deadbe"},
					{Name: "lab1", Color: "befade"},
				},
			}},
			expectedUpdates: UpdateData{},
			expectedError:   errors.New("repository: repo1, label lab1 is not unique when downcased in input config"),
		},
		{
			name:           "Non unique label on repo1 when downcased",
			requiredLabels: RequiredLabels{},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {
					{Name: "lab1", Color: "deadbe"},
					{Name: "LAB1", Color: "deadbe"},
				},
			}},
			expectedUpdates: UpdateData{},
			expectedError:   errors.New("repository: repo1, label LAB1 is not unique when downcased in input config"),
		},
		{
			name:           "Non unique label but on different repos - allowed",
			requiredLabels: RequiredLabels{},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {{Name: "lab1", Color: "deadbe"}},
				"repo2": {{Name: "lab1", Color: "deadbe"}},
			}},
			expectedUpdates: UpdateData{},
		},
		{
			name: "Repo has exactly all required labels",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "lab1", Color: "deadbe"},
			}},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {
					{Name: "lab1", Color: "deadbe"},
				},
			}},
			expectedUpdates: UpdateData{},
		},
		{
			name: "Repo has label with wrong color",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "lab1", Color: "deadbe"},
			}},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {
					{Name: "lab1", Color: "bebeef"},
				},
			}},
			expectedUpdates: UpdateData{ReposToUpdate: map[string][]UpdateItem{
				"repo1": {
					{Why: "invalid_color", RequiredLabel: Label{Name: "lab1", Color: "deadbe"}, CurrentLabel: Label{Name: "lab1", Color: "bebeef"}},
				},
			}},
		},
		{
			name: "Repo has label with wrong name (different case)",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "Lab1", Color: "deadbe"},
			}},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {
					{Name: "laB1", Color: "deadbe"},
				},
			}},
			expectedUpdates: UpdateData{ReposToUpdate: map[string][]UpdateItem{
				"repo1": {
					{Why: "invalid_name", RequiredLabel: Label{Name: "Lab1", Color: "deadbe"}, CurrentLabel: Label{Name: "laB1", Color: "deadbe"}},
				},
			}},
		},
		{
			name: "Repo is missing a label",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "Lab1", Color: "deadbe"},
			}},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {},
			}},
			expectedUpdates: UpdateData{ReposToUpdate: map[string][]UpdateItem{
				"repo1": {
					{Why: "missing", RequiredLabel: Label{Name: "Lab1", Color: "deadbe"}, CurrentLabel: Label{}},
				},
			}},
		},
		{
			name: "Repo is missing multiple labels, and expected labels order is changed",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "Lab1", Color: "deadbe"},
				{Name: "Lab2", Color: "000000"},
				{Name: "Lab3", Color: "ffffff"},
			}},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {},
				"repo2": {
					{Name: "Lab2", Color: "000000"},
				},
			}},
			expectedUpdates: UpdateData{ReposToUpdate: map[string][]UpdateItem{
				"repo2": {
					{Why: "missing", RequiredLabel: Label{Name: "Lab3", Color: "ffffff"}, CurrentLabel: Label{}},
					{CurrentLabel: Label{}, Why: "missing", RequiredLabel: Label{Name: "Lab1", Color: "deadbe"}},
				},
				"repo1": {
					{Why: "missing", RequiredLabel: Label{Color: "000000", Name: "Lab2"}, CurrentLabel: Label{}},
					{Why: "missing", RequiredLabel: Label{Name: "Lab3", Color: "ffffff"}, CurrentLabel: Label{}},
					{Why: "missing", RequiredLabel: Label{Name: "Lab1", Color: "deadbe"}, CurrentLabel: Label{}},
				},
			}},
		},
		{
			name: "Multiple repos complex case",
			requiredLabels: RequiredLabels{Labels: []Label{
				{Name: "priority/P0", Color: "ff0000"},
				{Name: "lgtm", Color: "00ff00"},
			}},
			currentLabels: RepoLabels{Labels: map[string][]github.Label{
				"repo1": {
					{Name: "Priority/P0", Color: "ee3333"},
					{Name: "LGTM", Color: "00ff00"},
				},
				"repo2": {
					{Name: "priority/P0", Color: "ee3333"},
					{Name: "lgtm", Color: "00ff00"},
				},
				"repo3": {
					{Name: "PRIORITY/P0", Color: "ff0000"},
					{Name: "lgtm", Color: "0000ff"},
				},
				"repo4": {
					{Name: "priority/P0", Color: "ff0000"},
				},
				"repo5": {
					{Name: "lgtm", Color: "00ff00"},
				},
			}},
			expectedUpdates: UpdateData{ReposToUpdate: map[string][]UpdateItem{
				"repo1": {
					{Why: "invalid_name", RequiredLabel: Label{Name: "priority/P0", Color: "ff0000"}, CurrentLabel: Label{Name: "Priority/P0", Color: "ee3333"}},
					{Why: "invalid_name", RequiredLabel: Label{Name: "lgtm", Color: "00ff00"}, CurrentLabel: Label{Name: "LGTM", Color: "00ff00"}},
				},
				"repo2": {
					{Why: "invalid_color", RequiredLabel: Label{Name: "priority/P0", Color: "ff0000"}, CurrentLabel: Label{Name: "priority/P0", Color: "ee3333"}},
				},
				"repo3": {
					{Why: "invalid_name", RequiredLabel: Label{Name: "priority/P0", Color: "ff0000"}, CurrentLabel: Label{Name: "PRIORITY/P0", Color: "ff0000"}},
					{Why: "invalid_color", RequiredLabel: Label{Name: "lgtm", Color: "00ff00"}, CurrentLabel: Label{Name: "lgtm", Color: "0000ff"}},
				},
				"repo4": {
					{Why: "missing", RequiredLabel: Label{Name: "lgtm", Color: "00ff00"}, CurrentLabel: Label{}},
				},
				"repo5": {
					{Why: "missing", RequiredLabel: Label{Name: "priority/P0", Color: "ff0000"}, CurrentLabel: Label{}},
				},
			}},
		},
	}

	// Do tests
	for i, tc := range testcases {
		actualUpdates, err := SyncLabels(&tc.requiredLabels, &tc.currentLabels)
		if err == nil && tc.expectedError != nil || err != nil && tc.expectedError == nil {
			t.Errorf("TestSyncLabel: test case number %d: '%s': expected err '%+v', got '%+v'", i+1, tc.name, tc.expectedError, err)
		}
		if err != nil && tc.expectedError != nil && err.Error() != tc.expectedError.Error() {
			t.Errorf("TestSyncLabel: test case number %d: '%s: expected err '%+v', got '%+v'", i+1, tc.name, tc.expectedError.Error(), err.Error())
		}
		if !equalUpdates(&actualUpdates, &tc.expectedUpdates, t) {
			t.Errorf("TestSyncLabel: test case number %d: '%s': expected updates:\n%+v\ngot:\n%+v", i+1, tc.name, tc.expectedUpdates, actualUpdates)
		}
	}
}

// This is needed to compare Update sets, two update sets are equal
// only if their maps have the same lists (but order can be different)
// Using standard `reflect.DeepEqual` for entire structures makes tests flaky
func equalUpdates(updateData1, updateData2 *UpdateData, t *testing.T) bool {
	updates1, updates2 := updateData1.ReposToUpdate, updateData2.ReposToUpdate
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
	var testcases = []struct {
		path     string
		expected RequiredLabels
		ok       bool
		errMsg   string
	}{
		{
			path: "labels_example.yaml",
			expected: RequiredLabels{Labels: []Label{
				{Name: "lgtm", Color: "green"},
				{Name: "priority/P0", Color: "red"},
			}},
			ok: true,
		},
		{
			path:     "syntax_error_example.yaml",
			expected: RequiredLabels{},
			ok:       false,
			errMsg:   "error converting",
		},
		{
			path:     "no_such_file.yaml",
			expected: RequiredLabels{},
			ok:       false,
			errMsg:   "no such file",
		},
	}
	for i, tc := range testcases {
		actual := RequiredLabels{}
		err := actual.Load(tc.path)
		errNil := (err == nil)
		if errNil != tc.ok {
			t.Errorf("TestLoadYAML: test case number %d, expected ok: %v, got %v (error=%v)", i+1, tc.ok, err == nil, err)
		}
		if !errNil && !strings.Contains(err.Error(), tc.errMsg) {
			t.Errorf("TestLoadYAML: test case number %d, expected error '%v' to contain '%v'", i+1, err.Error(), tc.errMsg)
		}
		if errNil && !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("TestLoadYAML: test case number %d, expected labels %v, got %v", i+1, tc.expected, actual)
		}
	}
}

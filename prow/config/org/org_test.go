/*
Copyright 2018 The Kubernetes Authors.

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

package org

import (
	"encoding/json"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

func TestPrivacy(t *testing.T) {
	get := func(v Privacy) *Privacy {
		return &v
	}
	cases := []struct {
		input    string
		expected *Privacy
	}{
		{
			"secret",
			get(Secret),
		},
		{
			"closed",
			get(Closed),
		},
		{
			"",
			nil,
		},
		{
			"unknown",
			nil,
		},
	}
	for _, tc := range cases {
		var actual Privacy
		err := json.Unmarshal([]byte("\""+tc.input+"\""), &actual)
		switch {
		case err == nil && tc.expected == nil:
			t.Errorf("%s: failed to receive an error", tc.input)
		case err != nil && tc.expected != nil:
			t.Errorf("%s: unexpected error: %v", tc.input, err)
		case err == nil && *tc.expected != actual:
			t.Errorf("%s: actual %v != expected %v", tc.input, tc.expected, actual)
		}
	}
}

func TestPruneRepoDefaults(t *testing.T) {
	empty := ""
	nonEmpty := "string that is not empty"
	yes := true
	no := false
	master := "master"
	notMaster := "not-master"
	testCases := []struct {
		description string
		repo        Repo
		expected    Repo
	}{
		{
			description: "default values are pruned",
			repo: Repo{
				Description:      &empty,
				HomePage:         &empty,
				Private:          &no,
				HasIssues:        &yes,
				HasProjects:      &yes,
				HasWiki:          &yes,
				AllowSquashMerge: &yes,
				AllowMergeCommit: &yes,
				AllowRebaseMerge: &yes,
				DefaultBranch:    &master,
				Archived:         &no,
			},
			expected: Repo{HasProjects: &yes},
		},
		{
			description: "non-default values are not pruned",
			repo: Repo{
				Description:      &nonEmpty,
				HomePage:         &nonEmpty,
				Private:          &yes,
				HasIssues:        &no,
				HasProjects:      &no,
				HasWiki:          &no,
				AllowSquashMerge: &no,
				AllowMergeCommit: &no,
				AllowRebaseMerge: &no,
				DefaultBranch:    &notMaster,
				Archived:         &yes,
			},
			expected: Repo{Description: &nonEmpty,
				HomePage:         &nonEmpty,
				Private:          &yes,
				HasIssues:        &no,
				HasProjects:      &no,
				HasWiki:          &no,
				AllowSquashMerge: &no,
				AllowMergeCommit: &no,
				AllowRebaseMerge: &no,
				DefaultBranch:    &notMaster,
				Archived:         &yes,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			pruned := PruneRepoDefaults(tc.repo)
			if !reflect.DeepEqual(tc.expected, pruned) {
				t.Errorf("%s: result differs from expected:\n", diff.ObjectReflectDiff(tc.expected, pruned))
			}
		})
	}
}

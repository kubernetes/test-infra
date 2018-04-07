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

package main

import (
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/config/branchprotection"
	"k8s.io/test-infra/prow/github"
)

func TestMakeBool(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		input    *bool
		expected bool
	}{
		{
			input:    nil,
			expected: false,
		},
		{
			input:    &no,
			expected: false,
		},
		{
			input:    &yes,
			expected: true,
		},
	}
	for _, tc := range cases {
		if actual := makeBool(tc.input); actual != tc.expected {
			t.Errorf("%v: actual %v != expected %t", tc.input, actual, tc.expected)
		}
	}
}

func TestMakeReviews(t *testing.T) {
	zero := 0
	three := 3
	one := 1
	yes := true
	cases := []struct {
		name     string
		input    *branchprotection.ReviewPolicy
		expected *github.RequiredPullRequestReviews
	}{
		{
			name: "nil returns nil",
		},
		{
			name: "nil apporvals returns nil",
			input: &branchprotection.ReviewPolicy{
				Approvals: nil,
			},
		},
		{
			name: "0 approvals returns nil",
			input: &branchprotection.ReviewPolicy{
				Approvals: &zero,
			},
		},
		{
			name: "approvals set",
			input: &branchprotection.ReviewPolicy{
				Approvals: &three,
			},
			expected: &github.RequiredPullRequestReviews{
				RequireApprovingReviewCount: 3,
			},
		},
		{
			name: "set all",
			input: &branchprotection.ReviewPolicy{
				Approvals:     &one,
				RequireOwners: &yes,
				DismissStale:  &yes,
				Restrictions: &branchprotection.Restrictions{
					Users: []string{"fred", "jane"},
					Teams: []string{"megacorp", "startup"},
				},
			},
			expected: &github.RequiredPullRequestReviews{
				RequireApprovingReviewCount: 1,
				RequireCodeOwnerReviews:     true,
				DismissStaleReviews:         true,
				DismissalRestrictions: github.Restrictions{
					Teams: []string{"megacorp", "startup"},
					Users: []string{"fred", "jane"},
				},
			},
		},
	}

	for _, tc := range cases {
		actual := makeReviews(tc.input)
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("%s: actual %v != expected %v", tc.name, actual, tc.expected)
		}
	}
}

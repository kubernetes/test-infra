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

	branchprotection "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

func TestMakeRequest(t *testing.T) {

	cases := []struct {
		name     string
		policy   branchprotection.Policy
		expected github.BranchProtectionRequest
	}{
		{
			name: "Empty works",
		},
		{
			name:   "teams != nil => users != nil",
			policy: branchprotection.Policy{Pushers: []string{"hello"}},
			expected: github.BranchProtectionRequest{
				Restrictions: &github.Restrictions{
					Teams: []string{"hello"},
					Users: []string{},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := makeRequest(tc.policy)
			expected := tc.expected
			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("actual %+v != expected %+v", actual, expected)
			}
		})
	}
}

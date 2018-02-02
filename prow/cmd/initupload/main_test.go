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

import "testing"

func TestValidatePathOptions(t *testing.T) {
	var testCases = []struct {
		name        string
		strategy    string
		org         string
		repo        string
		expectedErr bool
	}{
		{
			name:        "invalid strategy",
			strategy:    "whatever",
			expectedErr: true,
		},
		{
			name:        "explicit strategy, no defaults",
			strategy:    "explicit",
			expectedErr: false,
		},
		{
			name:        "legacy strategy, no defaults",
			strategy:    "legacy",
			expectedErr: true,
		},
		{
			name:        "legacy strategy, no default repo",
			strategy:    "legacy",
			org:         "org",
			expectedErr: true,
		},
		{
			name:        "legacy strategy, no default org",
			strategy:    "legacy",
			repo:        "repo",
			expectedErr: true,
		},
		{
			name:        "legacy strategy, with defaults",
			strategy:    "legacy",
			org:         "org",
			repo:        "repo",
			expectedErr: false,
		},
		{
			name:        "single strategy, no defaults",
			strategy:    "single",
			expectedErr: true,
		},
		{
			name:        "single strategy, no default repo",
			strategy:    "single",
			org:         "org",
			expectedErr: true,
		},
		{
			name:        "single strategy, no default org",
			strategy:    "single",
			repo:        "repo",
			expectedErr: true,
		},
		{
			name:        "single strategy, with defaults",
			strategy:    "single",
			org:         "org",
			repo:        "repo",
			expectedErr: false,
		},
	}

	for _, testCase := range testCases {
		err := validatePathOptions(&testCase.strategy, &testCase.org, &testCase.repo)
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no err but got %v", testCase.name, err)
		}
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected err but got none", testCase.name)
		}
	}
}

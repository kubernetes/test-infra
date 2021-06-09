/*
Copyright 2019 The Kubernetes Authors.

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
	"testing"
)

func TestCommitToRef(t *testing.T) {
	cases := []struct {
		name     string
		commit   string
		expected string
	}{
		{
			name: "basically works",
		},
		{
			name:     "just tag works",
			commit:   "v0.0.30",
			expected: "v0.0.30",
		},
		{
			name:     "just commit works",
			commit:   "deadbeef",
			expected: "deadbeef",
		},
		{
			name:     "commits past tag works",
			commit:   "v0.0.30-14-gdeadbeef",
			expected: "deadbeef",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual, expected := commitToRef(tc.commit), tc.expected; actual != tc.expected {
				t.Errorf("commitToRef(%q) got %q want %q", tc.commit, actual, expected)
			}
		})
	}
}

func TestIsUnderPath(t *testing.T) {
	cases := []struct {
		description string
		paths       []string
		file        string
		expected    bool
	}{
		{
			description: "file is under the direct path",
			paths:       []string{"config/prow/"},
			file:        "config/prow/config.yaml",
			expected:    true,
		},
		{
			description: "file is under the indirect path",
			paths:       []string{"config/prow-staging/"},
			file:        "config/prow-staging/jobs/config.yaml",
			expected:    true,
		},
		{
			description: "file is under one path but not others",
			paths:       []string{"config/prow/", "config/prow-staging/"},
			file:        "config/prow-staging/jobs/whatever-repo/whatever-file",
			expected:    true,
		},
		{
			description: "file is not under the path but having the same prefix",
			paths:       []string{"config/prow/"},
			file:        "config/prow-staging/config.yaml",
			expected:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			actual := isUnderPath(tc.file, tc.paths)
			if actual != tc.expected {
				t.Errorf("expected to be %t but actual is %t", tc.expected, actual)
			}
		})
	}
}

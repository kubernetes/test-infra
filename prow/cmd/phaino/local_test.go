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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepo(t *testing.T) {
	cases := []struct {
		name     string
		goal     string
		wd       string
		dirs     []string
		expected string
		err      bool
	}{
		{
			name:     "match full repo",
			goal:     "k8s.io/test-infra",
			wd:       "go/src/k8s.io/test-infra",
			expected: "go/src/k8s.io/test-infra",
		},
		{
			name: "repo not found",
			goal: "k8s.io/test-infra",
			wd:   "random",
			dirs: []string{"k8s.io/repo-infra", "github.com/fejta/test-infra"},
			err:  true,
		},
		{
			name:     "convert github to k8s vanity",
			goal:     "github.com/kubernetes/test-infra",
			wd:       "go/src/k8s.io/test-infra",
			expected: "go/src/k8s.io/test-infra",
		},
		{
			name:     "match sibling base",
			goal:     "k8s.io/repo-infra",
			wd:       "src/test-infra",
			dirs:     []string{"src/repo-infra"},
			expected: "src/repo-infra",
		},
		{
			name:     "match full sibling",
			goal:     "k8s.io/repo-infra",
			wd:       "go/src/k8s.io/test-infra",
			dirs:     []string{"go/src/k8s.io/repo-infra"},
			expected: "go/src/k8s.io/repo-infra",
		},
		{
			name:     "match just repo",
			goal:     "k8s.io/test-infra",
			wd:       "src/test-infra",
			expected: "src/test-infra",
		},
		{
			name:     "match base of repo",
			goal:     "k8s.io/test-infra",
			wd:       "src/test-infra/prow/cmd/mkpj",
			expected: "src/test-infra",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "find-repo-"+tc.name)
			if err != nil {
				t.Fatalf("Cannot create temp dir: %v", err)
			}
			defer os.RemoveAll(dir)

			tc.dirs = append(tc.dirs, tc.wd)
			for _, d := range tc.dirs {
				full := filepath.Join(dir, d)
				if err := os.MkdirAll(full, 0755); err != nil {
					t.Fatalf("Cannot create subdir %q: %v", full, err)
				}
			}

			wd := filepath.Join(dir, tc.wd)
			expected := filepath.Join(dir, tc.expected)

			actual, err := findRepo(wd, tc.goal)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Error("Failed to get an error")
			case actual != expected:
				t.Errorf("Actual %q != expected %q", actual, expected)
			}
		})
	}
}

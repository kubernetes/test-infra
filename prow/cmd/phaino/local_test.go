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
	"context"
	"go/build"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestReadRepo(t *testing.T) {
	dir, err := ioutil.TempDir("", "read-repo")
	if err != nil {
		t.Fatalf("Cannot create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	cases := []struct {
		name      string
		goal      string
		wd        string
		gopath    string
		dirs      []string
		userInput string
		expected  string
		err       bool
	}{
		{
			name:     "find from local",
			goal:     "k8s.io/test-infra2",
			wd:       path.Join(dir, "find_from_local", "go/src/k8s.io/test-infra2"),
			expected: path.Join(dir, "find_from_local", "go/src/k8s.io/test-infra2"),
		},
		{
			name:   "find from explicit gopath",
			goal:   "k8s.io/test-infra2",
			gopath: path.Join(dir, "find_from_explicit_gopath"),
			wd:     path.Join(dir, "find_from_explicit_gopath_random", "random"),
			dirs: []string{
				path.Join(dir, "find_from_explicit_gopath", "src", "k8s.io/test-infra2"),
			},
			expected: path.Join(dir, "find_from_explicit_gopath", "src", "k8s.io/test-infra2"),
		},
		{
			name:   "not exist",
			goal:   "k8s.io/test-infra2",
			gopath: path.Join(dir, "not_exist", "random1"),
			wd:     path.Join(dir, "not_exist", "random2"),
			err:    true,
		},
		{
			name:      "not exist due to user error",
			goal:      "k8s.io/test-infra2",
			wd:        path.Join(dir, "not_exist_due_to_user_error", "go/src/k8s.io/test-infra2"),
			expected:  "/random/other/path",
			userInput: "/random/other/path",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.dirs = append(tc.dirs, tc.wd)
			for _, d := range tc.dirs {
				if err := os.MkdirAll(d, 0755); err != nil {
					t.Fatalf("Cannot create subdir %q: %v", d, err)
				}
			}

			// build.Default was loaded while imported, override it directly.
			oldGopath := build.Default.GOPATH
			defer func() {
				build.Default.GOPATH = oldGopath
			}()
			build.Default.GOPATH = tc.gopath

			// Trick the system to think it's running in bazel and wd is tc.wd.
			oldPwd := os.Getenv("BUILD_WORKING_DIRECTORY")
			defer os.Setenv("BUILD_WORKING_DIRECTORY", oldPwd)
			os.Setenv("BUILD_WORKING_DIRECTORY", tc.wd)

			actual, err := readRepo(context.Background(), tc.goal, func(path, def string) (string, error) {
				return tc.userInput, nil
			})
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Error("Failed to get an error")
			case actual != tc.expected:
				t.Errorf("Actual %q != expected %q", actual, tc.expected)
			}
		})
	}
}

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

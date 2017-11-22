/*
Copyright 2015 The Kubernetes Authors.

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

package features

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	_ = fmt.Printf
	_ = glog.Errorf
)

const (
	baseDir        = ""
	leafDir        = "a/b/c"
	nonExistentDir = "DELETED_DIR"
)

func getTestRepo() *RepoInfo {
	testRepo := RepoInfo{enableMdYaml: false, useReviewers: false}
	approvers := map[string]sets.String{}
	baseApprovers := sets.NewString("Alice", "Bob")
	leafApprovers := sets.NewString("Carl", "Dave")
	approvers[baseDir] = baseApprovers
	approvers[leafDir] = leafApprovers
	testRepo.approvers = approvers
	testRepo.labels = map[string]sets.String{
		baseDir: sets.NewString("sig/godzilla"),
		leafDir: sets.NewString("wg/save-tokyo"),
	}

	return &testRepo
}

func TestGetApprovers(t *testing.T) {
	testFile0 := filepath.Join(baseDir, "testFile.md")
	testFile1 := filepath.Join(leafDir, "testFile.md")
	testFile2 := filepath.Join(nonExistentDir, "testFile.md")
	TestRepo := getTestRepo()
	tests := []struct {
		testName           string
		testFile           *github.CommitFile
		expectedOwnersPath string
		expectedLeafOwners sets.String
		expectedAllOwners  sets.String
	}{
		{
			testName:           "Modified Base Dir Only",
			testFile:           &github.CommitFile{Filename: &testFile0},
			expectedOwnersPath: baseDir,
			expectedLeafOwners: TestRepo.approvers[baseDir],
			expectedAllOwners:  TestRepo.approvers[baseDir],
		},
		{
			testName:           "Modified Leaf Dir Only",
			testFile:           &github.CommitFile{Filename: &testFile1},
			expectedOwnersPath: leafDir,
			expectedLeafOwners: TestRepo.approvers[leafDir],
			expectedAllOwners:  TestRepo.approvers[leafDir].Union(TestRepo.approvers[baseDir]),
		},
		{
			testName:           "Modified Nonexistent Dir (Default to Base)",
			testFile:           &github.CommitFile{Filename: &testFile2},
			expectedOwnersPath: baseDir,
			expectedLeafOwners: TestRepo.approvers[baseDir],
			expectedAllOwners:  TestRepo.approvers[baseDir],
		},
	}
	for testNum, test := range tests {
		foundLeafApprovers := TestRepo.LeafApprovers(*test.testFile.Filename)
		foundApprovers := TestRepo.Approvers(*test.testFile.Filename)
		foundOwnersPath := TestRepo.FindApproverOwnersForPath(*test.testFile.Filename)
		if !foundLeafApprovers.Equal(test.expectedLeafOwners) {
			t.Errorf("The Leaf Approvers Found Do Not Match Expected For Test %d: %s", testNum, test.testName)
			t.Errorf("\tExpected Owners: %v\tFound Owners: %v ", test.expectedLeafOwners, foundLeafApprovers)
		}
		if !foundApprovers.Equal(test.expectedAllOwners) {
			t.Errorf("The Approvers Found Do Not Match Expected For Test %d: %s", testNum, test.testName)
			t.Errorf("\tExpected Owners: %v\tFound Owners: %v ", test.expectedAllOwners, foundApprovers)
		}
		if foundOwnersPath != test.expectedOwnersPath {
			t.Errorf("The Owners Path Found Does Not Match Expected For Test %d: %s", testNum, test.testName)
			t.Errorf("\tExpected Owners: %v\tFound Owners: %v ", test.expectedOwnersPath, foundOwnersPath)
		}
	}
}

func TestLabels(t *testing.T) {
	tests := []struct {
		testName       string
		inputPath      string
		expectedLabels sets.String
	}{
		{
			testName:       "base 1",
			inputPath:      "foo.txt",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			testName:       "base 2",
			inputPath:      "./foo.txt",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			testName:       "base 3",
			inputPath:      "",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			testName:       "base 4",
			inputPath:      ".",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			testName:       "leaf 1",
			inputPath:      "a/b/c/foo.txt",
			expectedLabels: sets.NewString("sig/godzilla", "wg/save-tokyo"),
		}, {
			testName:       "leaf 2",
			inputPath:      "a/b/foo.txt",
			expectedLabels: sets.NewString("sig/godzilla"),
		},
	}

	TestRepo := getTestRepo()
	for _, tt := range tests {
		got := TestRepo.FindLabelsForPath(tt.inputPath)
		if !got.Equal(tt.expectedLabels) {
			t.Errorf("%v: expected %v, got %v", tt.testName, tt.expectedLabels, got)
		}
	}
}

func TestCanonical(t *testing.T) {

	tests := []struct {
		testName     string
		inputPath    string
		expectedPath string
	}{
		{
			testName:     "Empty String",
			inputPath:    "",
			expectedPath: "",
		},
		{
			testName:     "Dot (.) as Path",
			inputPath:    ".",
			expectedPath: "",
		},
		{
			testName:     "Github Style Input (No Root)",
			inputPath:    "a/b/c/d.txt",
			expectedPath: "a/b/c/d.txt",
		},
		{
			testName:     "Preceding Slash and Trailing Slash",
			inputPath:    "/a/b/",
			expectedPath: "/a/b",
		},
		{
			testName:     "Trailing Slash",
			inputPath:    "foo/bar/baz/",
			expectedPath: "foo/bar/baz",
		},
	}
	for _, test := range tests {
		if test.expectedPath != canonicalize(test.inputPath) {
			t.Errorf("Expected the canonical path for %v to be %v.  Found %v instead", test.inputPath, test.expectedPath, canonicalize(test.inputPath))
		}
	}
}

var (
	lowerCaseAliases = []byte(`
aliases:
  team/t1:
    - u1
    - u2
  team/t2:
    - u1
    - u3`)
	mixedCaseAliases = []byte(`
aliases:
  TEAM/T1:
    - U1
    - U2`)
)

type aliasTest struct {
	data []byte
}

func (a *aliasTest) read() ([]byte, error) {
	return a.data, nil
}

func TestCalculateAliasDelta(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name      string
		aliasData []byte
		approvers sets.String
		reviewers sets.String
		expectedA sets.String
		expectedR sets.String
	}{
		{
			name:      "No expansions.",
			aliasData: lowerCaseAliases,
			approvers: sets.NewString("abc", "def"),
			expectedA: sets.NewString("abc", "def"),
			reviewers: sets.NewString("abcr", "defr"),
			expectedR: sets.NewString("abcr", "defr"),
		},
		{
			name:      "One alias to be expanded",
			aliasData: lowerCaseAliases,
			approvers: sets.NewString("abc", "team/t1"),
			expectedA: sets.NewString("abc", "u1", "u2"),
			reviewers: sets.NewString("abcr", "team/t2"),
			expectedR: sets.NewString("abcr", "u1", "u3"),
		},
		{
			name:      "Duplicates inside and outside alias.",
			aliasData: lowerCaseAliases,
			approvers: sets.NewString("u1", "team/t1"),
			expectedA: sets.NewString("u1", "u2"),
			reviewers: sets.NewString("u3", "team/t2"),
			expectedR: sets.NewString("u3", "u1"),
		},
		{
			name:      "Duplicates in multiple aliases.",
			aliasData: lowerCaseAliases,
			approvers: sets.NewString("u1", "team/t1", "team/t2"),
			expectedA: sets.NewString("u1", "u2", "u3"),
			reviewers: sets.NewString("u1", "team/t1", "team/t2"),
			expectedR: sets.NewString("u1", "u2", "u3"),
		},
		{
			name:      "Mixed casing in aliases.",
			aliasData: mixedCaseAliases,
			approvers: sets.NewString("team/t1"),
			expectedA: sets.NewString("u1", "u2"),
			reviewers: sets.NewString("team/t1"),
			expectedR: sets.NewString("u1", "u2"),
		},
	}

	for _, test := range tests {
		info := RepoInfo{
			aliasFile: "dummy.file",
			aliasData: &aliasData{},
			aliasReader: &aliasTest{
				data: test.aliasData,
			},
			approvers: map[string]sets.String{
				"fake": test.approvers,
			},
			reviewers: map[string]sets.String{
				"fake": test.reviewers,
			},
		}

		if err := info.updateRepoAliases(); err != nil {
			t.Fatalf("%v", err)
		}

		info.expandAllAliases()
		if expected, got := test.expectedA, info.approvers["fake"]; !reflect.DeepEqual(expected, got) {
			t.Errorf("%s: expected approvers: %#v, got: %#v", test.name, expected, got)
		}
		if expected, got := test.expectedR, info.reviewers["fake"]; !reflect.DeepEqual(expected, got) {
			t.Errorf("%s: expected reviewers: %#v, got: %#v", test.name, expected, got)
		}
	}
}

func TestResolveAlias(t *testing.T) {
	tests := []struct {
		name         string
		knownAliases map[string][]string
		user         string
		expected     []string
	}{
		{
			name:         "no known aliases",
			knownAliases: map[string][]string{},
			user:         "bob",
			expected:     []string{},
		},
		{
			name:         "no applicable aliases",
			knownAliases: map[string][]string{"jim": {"james"}},
			user:         "bob",
			expected:     []string{},
		},
		{
			name:         "applicable aliases",
			knownAliases: map[string][]string{"bob": {"robert"}},
			user:         "bob",
			expected:     []string{"robert"},
		},
	}

	for _, test := range tests {
		info := RepoInfo{
			aliasData: &aliasData{
				AliasMap: test.knownAliases,
			},
		}

		if expected, got := test.expected, info.resolveAlias(test.user); !reflect.DeepEqual(expected, got) {
			t.Errorf("%s: expected: %#v, got: %#v", test.name, expected, got)
		}
	}
}

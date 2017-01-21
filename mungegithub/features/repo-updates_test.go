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
	"testing"

	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
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

// Commit returns a filled out github.Commit which happened at time.Unix(t, 0)
func getTestRepo() *RepoInfo {
	testRepo := RepoInfo{BaseDir: baseDir, EnableMdYaml: false, UseReviewers: false}
	approvers := map[string]sets.String{}
	baseApprovers := sets.NewString("Alice", "Bob")
	leafApprovers := sets.NewString("Carl", "Dave")
	approvers[baseDir] = baseApprovers
	approvers[leafDir] = leafApprovers
	testRepo.approvers = approvers

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
		foundOwnersPath := TestRepo.FindOwnersForPath(*test.testFile.Filename)
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

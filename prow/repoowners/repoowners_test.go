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

package repoowners

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func getTestClient(enableMdYaml, includeAliases bool) (*Client, func(), error) {
	testFiles := map[string][]byte{
		"foo":                        []byte("approvers:\n- bob"),
		"OWNERS":                     []byte("approvers: \n- cjwagner\nreviewers:\n- Alice\n- bob\nlabels:\n - EVERYTHING"),
		"src/OWNERS":                 []byte("approvers:\n- Best-Approvers"),
		"src/dir/OWNERS":             []byte("assignees:\n - bob\nreviewers:\n- alice\n- CJWagner\nlabels:\n- src-code"),
		"src/dir/conformance/OWNERS": []byte("options:\n no_parent_owners: true\napprovers:\n - mml"),
		"docs/file.md":               []byte("---\napprovers: \n- ALICE\n\nlabels:\n- docs\n---"),
	}
	testAliasesFile := map[string][]byte{
		"OWNERS_ALIASES": []byte("aliases:\n  Best-approvers:\n  - carl\n  - cjwagner\n  best-reviewers:\n  - Carl\n  - BOB"),
	}

	localGit, git, err := localgit.New()
	if err != nil {
		return nil, nil, err
	}
	if err := localGit.MakeFakeRepo("org", "repo"); err != nil {
		return nil, nil, fmt.Errorf("Error making fake repo: %v", err)
	}
	if err := localGit.AddCommit("org", "repo", testFiles); err != nil {
		return nil, nil, fmt.Errorf("Error adding initial commit: %v", err)
	}
	if includeAliases {
		if err := localGit.AddCommit("org", "repo", testAliasesFile); err != nil {
			return nil, nil, fmt.Errorf("Error adding OWNERS_ALIASES commit: %v", err)
		}
	}

	return &Client{
			git:    git,
			ghc:    &fakegithub.FakeClient{Collaborators: []string{"cjwagner", "k8s-ci-robot", "alice", "bob", "carl", "mml"}},
			Logger: logrus.WithField("client", "repoowners"),
			cache:  make(map[string]cacheEntry),

			mdYAMLEnabled: func(org, repo string) bool {
				return enableMdYaml
			},
		},
		// Clean up function
		func() {
			git.Clean()
			localGit.Clean()
		},
		nil
}

func TestLoadRepoOwners(t *testing.T) {
	tests := []struct {
		name              string
		mdEnabled         bool
		aliasesFileExists bool

		expectedApprovers, expectedReviewers, expectedLabels map[string]sets.String

		expectedOptions map[string]dirOptions
	}{
		{
			name: "no alias, no md",
			expectedApprovers: map[string]sets.String{
				"":                    sets.NewString("cjwagner"),
				"src":                 sets.NewString(),
				"src/dir":             sets.NewString("bob"),
				"src/dir/conformance": sets.NewString("mml"),
			},
			expectedReviewers: map[string]sets.String{
				"":        sets.NewString("alice", "bob"),
				"src/dir": sets.NewString("alice", "cjwagner"),
			},
			expectedLabels: map[string]sets.String{
				"":        sets.NewString("EVERYTHING"),
				"src/dir": sets.NewString("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners: true,
				},
			},
		},
		{
			name:              "alias, no md",
			aliasesFileExists: true,
			expectedApprovers: map[string]sets.String{
				"":                    sets.NewString("cjwagner"),
				"src":                 sets.NewString("carl", "cjwagner"),
				"src/dir":             sets.NewString("bob"),
				"src/dir/conformance": sets.NewString("mml"),
			},
			expectedReviewers: map[string]sets.String{
				"":        sets.NewString("alice", "bob"),
				"src/dir": sets.NewString("alice", "cjwagner"),
			},
			expectedLabels: map[string]sets.String{
				"":        sets.NewString("EVERYTHING"),
				"src/dir": sets.NewString("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners: true,
				},
			},
		},
		{
			name:              "alias, md",
			aliasesFileExists: true,
			mdEnabled:         true,
			expectedApprovers: map[string]sets.String{
				"":                    sets.NewString("cjwagner"),
				"src":                 sets.NewString("carl", "cjwagner"),
				"src/dir":             sets.NewString("bob"),
				"src/dir/conformance": sets.NewString("mml"),
				"docs/file.md":        sets.NewString("alice"),
			},
			expectedReviewers: map[string]sets.String{
				"":        sets.NewString("alice", "bob"),
				"src/dir": sets.NewString("alice", "cjwagner"),
			},
			expectedLabels: map[string]sets.String{
				"":             sets.NewString("EVERYTHING"),
				"src/dir":      sets.NewString("src-code"),
				"docs/file.md": sets.NewString("docs"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners: true,
				},
			},
		},
	}

	for _, test := range tests {
		client, cleanup, err := getTestClient(test.mdEnabled, test.aliasesFileExists)
		if err != nil {
			t.Errorf("[%s] Error creating test client: %v.", test.name, err)
			continue
		}
		defer cleanup()

		ro, err := client.LoadRepoOwners("org", "repo")
		if err != nil {
			t.Errorf("[%s] Unexpected error loading RepoOwners: %v.", test.name, err)
			continue
		}

		if ro.baseDir == "" {
			t.Errorf("[%s] Expected 'baseDir' to be populated.", test.name)
			continue
		}
		if (ro.RepoAliases != nil) != test.aliasesFileExists {
			t.Errorf("[%s] Expected 'RepoAliases' to be poplulated: %t, but got %t.", test.name, test.aliasesFileExists, ro.RepoAliases != nil)
			continue
		}
		if ro.enableMDYAML != test.mdEnabled {
			t.Errorf("[%s] Expected 'enableMdYaml' to be: %t, but got %t.", test.name, test.mdEnabled, ro.enableMDYAML)
			continue
		}

		check := func(field string, expected, got map[string]sets.String) {
			if !reflect.DeepEqual(expected, got) {
				t.Errorf("[%s] Expected %s to be %#v, but got %#v.", test.name, field, expected, got)
			}
		}
		check("approvers", test.expectedApprovers, ro.approvers)
		check("reviewers", test.expectedReviewers, ro.reviewers)
		check("labels", test.expectedLabels, ro.labels)
		if !reflect.DeepEqual(test.expectedOptions, ro.options) {
			t.Errorf("[%s] Expected %s to be %#v, but got %#v.", test.name, "options", test.expectedOptions, ro.options)
		}
	}
}

func TestLoadRepoAliases(t *testing.T) {
	tests := []struct {
		name                string
		aliasFileExists     bool
		expectedRepoAliases RepoAliases
	}{
		{
			name:                "No aliases file",
			aliasFileExists:     false,
			expectedRepoAliases: nil,
		},
		{
			name:            "Normal aliases file",
			aliasFileExists: true,
			expectedRepoAliases: RepoAliases{
				"best-approvers": sets.NewString("carl", "cjwagner"),
				"best-reviewers": sets.NewString("carl", "bob"),
			},
		},
	}
	for _, test := range tests {
		client, cleanup, err := getTestClient(false, test.aliasFileExists)
		if err != nil {
			t.Errorf("[%s] Error creating test client: %v.", test.name, err)
			continue
		}

		got, err := client.LoadRepoAliases("org", "repo")
		if err != nil {
			t.Errorf("[%s] Unexpected error loading RepoAliases: %v.", test.name, err)
			cleanup()
			continue
		}
		if !reflect.DeepEqual(got, test.expectedRepoAliases) {
			t.Errorf("[%s] Expected RepoAliases: %#v, but got: %#v.", test.name, test.expectedRepoAliases, got)
		}
		cleanup()
	}
}

const (
	baseDir        = ""
	leafDir        = "a/b/c"
	noParentsDir   = "d"
	nonExistentDir = "DELETED_DIR"
)

func TestGetApprovers(t *testing.T) {
	ro := &RepoOwners{
		approvers: map[string]sets.String{
			baseDir:      sets.NewString("alice", "bob"),
			leafDir:      sets.NewString("carl", "dave"),
			noParentsDir: sets.NewString("mml"),
		},
		options: map[string]dirOptions{
			noParentsDir: {
				NoParentOwners: true,
			},
		},
	}
	tests := []struct {
		name               string
		filePath           string
		expectedOwnersPath string
		expectedLeafOwners sets.String
		expectedAllOwners  sets.String
	}{
		{
			name:               "Modified Base Dir Only",
			filePath:           filepath.Join(baseDir, "testFile.md"),
			expectedOwnersPath: baseDir,
			expectedLeafOwners: ro.approvers[baseDir],
			expectedAllOwners:  ro.approvers[baseDir],
		},
		{
			name:               "Modified Leaf Dir Only",
			filePath:           filepath.Join(leafDir, "testFile.md"),
			expectedOwnersPath: leafDir,
			expectedLeafOwners: ro.approvers[leafDir],
			expectedAllOwners:  ro.approvers[leafDir].Union(ro.approvers[baseDir]),
		},
		{
			name:               "Modified NoParentOwners Dir Only",
			filePath:           filepath.Join(noParentsDir, "testFile.go"),
			expectedOwnersPath: noParentsDir,
			expectedLeafOwners: ro.approvers[noParentsDir],
			expectedAllOwners:  ro.approvers[noParentsDir],
		},
		{
			name:               "Modified Nonexistent Dir (Default to Base)",
			filePath:           filepath.Join(nonExistentDir, "testFile.md"),
			expectedOwnersPath: baseDir,
			expectedLeafOwners: ro.approvers[baseDir],
			expectedAllOwners:  ro.approvers[baseDir],
		},
	}
	for testNum, test := range tests {
		foundLeafApprovers := ro.LeafApprovers(test.filePath)
		foundApprovers := ro.Approvers(test.filePath)
		foundOwnersPath := ro.FindApproverOwnersForPath(test.filePath)
		if !foundLeafApprovers.Equal(test.expectedLeafOwners) {
			t.Errorf("The Leaf Approvers Found Do Not Match Expected For Test %d: %s", testNum, test.name)
			t.Errorf("\tExpected Owners: %v\tFound Owners: %v ", test.expectedLeafOwners, foundLeafApprovers)
		}
		if !foundApprovers.Equal(test.expectedAllOwners) {
			t.Errorf("The Approvers Found Do Not Match Expected For Test %d: %s", testNum, test.name)
			t.Errorf("\tExpected Owners: %v\tFound Owners: %v ", test.expectedAllOwners, foundApprovers)
		}
		if foundOwnersPath != test.expectedOwnersPath {
			t.Errorf("The Owners Path Found Does Not Match Expected For Test %d: %s", testNum, test.name)
			t.Errorf("\tExpected Owners: %v\tFound Owners: %v ", test.expectedOwnersPath, foundOwnersPath)
		}
	}
}

func TestFindLabelsForPath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedLabels sets.String
	}{
		{
			name:           "base 1",
			path:           "foo.txt",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			name:           "base 2",
			path:           "./foo.txt",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			name:           "base 3",
			path:           "",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			name:           "base 4",
			path:           ".",
			expectedLabels: sets.NewString("sig/godzilla"),
		}, {
			name:           "leaf 1",
			path:           "a/b/c/foo.txt",
			expectedLabels: sets.NewString("sig/godzilla", "wg/save-tokyo"),
		}, {
			name:           "leaf 2",
			path:           "a/b/foo.txt",
			expectedLabels: sets.NewString("sig/godzilla"),
		},
	}

	testOwners := &RepoOwners{
		labels: map[string]sets.String{
			baseDir: sets.NewString("sig/godzilla"),
			leafDir: sets.NewString("wg/save-tokyo"),
		},
	}
	for _, test := range tests {
		got := testOwners.FindLabelsForPath(test.path)
		if !got.Equal(test.expectedLabels) {
			t.Errorf(
				"[%s] Expected labels %q for path %q, but got %q.",
				test.name,
				test.expectedLabels.List(),
				test.path,
				got.List(),
			)
		}
	}
}

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		expectedPath string
	}{
		{
			name:         "Empty String",
			path:         "",
			expectedPath: "",
		},
		{
			name:         "Dot (.) as Path",
			path:         ".",
			expectedPath: "",
		},
		{
			name:         "Github Style Input (No Root)",
			path:         "a/b/c/d.txt",
			expectedPath: "a/b/c/d.txt",
		},
		{
			name:         "Preceding Slash and Trailing Slash",
			path:         "/a/b/",
			expectedPath: "/a/b",
		},
		{
			name:         "Trailing Slash",
			path:         "foo/bar/baz/",
			expectedPath: "foo/bar/baz",
		},
	}
	for _, test := range tests {
		if got := canonicalize(test.path); test.expectedPath != got {
			t.Errorf(
				"[%s] Expected the canonical path for %v to be %v.  Found %v instead",
				test.name,
				test.path,
				test.expectedPath,
				got,
			)
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

func TestExpandAliases(t *testing.T) {
	testAliases := RepoAliases{
		"team/t1": sets.NewString("u1", "u2"),
		"team/t2": sets.NewString("u1", "u3"),
	}
	tests := []struct {
		name             string
		unexpanded       sets.String
		expectedExpanded sets.String
	}{
		{
			name:             "No expansions.",
			unexpanded:       sets.NewString("abc", "def"),
			expectedExpanded: sets.NewString("abc", "def"),
		},
		{
			name:             "One alias to be expanded",
			unexpanded:       sets.NewString("abc", "team/t1"),
			expectedExpanded: sets.NewString("abc", "u1", "u2"),
		},
		{
			name:             "Duplicates inside and outside alias.",
			unexpanded:       sets.NewString("u1", "team/t1"),
			expectedExpanded: sets.NewString("u1", "u2"),
		},
		{
			name:             "Duplicates in multiple aliases.",
			unexpanded:       sets.NewString("u1", "team/t1", "team/t2"),
			expectedExpanded: sets.NewString("u1", "u2", "u3"),
		},
		{
			name:             "Mixed casing in aliases.",
			unexpanded:       sets.NewString("Team/T1"),
			expectedExpanded: sets.NewString("u1", "u2"),
		},
	}

	for _, test := range tests {
		if got := testAliases.ExpandAliases(test.unexpanded); !test.expectedExpanded.Equal(got) {
			t.Errorf(
				"[%s] Expected %q to expand to %q, but got %q.",
				test.name,
				test.unexpanded.List(),
				test.expectedExpanded.List(),
				got.List(),
			)
		}
	}
}

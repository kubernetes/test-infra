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
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	prowConf "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins/ownersconfig"
)

var (
	defaultBranch = "master" // TODO(fejta): localgit.DefaultBranch()
	testFiles     = map[string][]byte{
		"foo": []byte(`approvers:
- bob`),
		"OWNERS": []byte(`approvers:
- cjwagner
reviewers:
- Alice
- bob
required_reviewers:
- chris
labels:
- EVERYTHING`),
		"src/OWNERS": []byte(`approvers:
- Best-Approvers`),
		"src/dir/OWNERS": []byte(`approvers:
- bob
reviewers:
- alice
- "@CJWagner"
- jakub
required_reviewers:
- ben
labels:
- src-code`),
		"src/dir/subdir/OWNERS": []byte(`approvers:
- bob
- alice
reviewers:
- bob
- alice`),
		"src/dir/conformance/OWNERS": []byte(`options:
  no_parent_owners: true
  auto_approve_unowned_subfolders: true
approvers:
- mml`),
		"docs/file.md": []byte(`---
approvers:
- ALICE

labels:
- docs
---`),
		"vendor/OWNERS": []byte(`approvers:
- alice`),
		"vendor/k8s.io/client-go/OWNERS": []byte(`approvers:
- bob`),
	}

	testFilesRe = map[string][]byte{
		// regexp filtered
		"re/OWNERS": []byte(`filters:
  ".*":
    labels:
    - re/all
  "\\.go$":
    labels:
    - re/go`),
		"re/a/OWNERS": []byte(`filters:
  "\\.md$":
    labels:
    - re/md-in-a
  "\\.go$":
    labels:
    - re/go-in-a`),
	}
)

// regexpAll is used to construct a default {regexp -> values} mapping for ".*"
func regexpAll(values ...string) map[*regexp.Regexp]sets.String {
	return map[*regexp.Regexp]sets.String{nil: sets.NewString(values...)}
}

// patternAll is used to construct a default {regexp string -> values} mapping for ".*"
func patternAll(values ...string) map[string]sets.String {
	// use "" to represent nil and distinguish it from a ".*" regexp (which shouldn't exist).
	return map[string]sets.String{"": sets.NewString(values...)}
}

type cacheOptions struct {
	hasAliases bool

	mdYaml                   bool
	commonFileChanged        bool
	mdFileChanged            bool
	ownersAliasesFileChanged bool
	ownersFileChanged        bool
}

type fakeGitHubClient struct {
	Collaborators []string
	ref           string
}

func (f *fakeGitHubClient) ListCollaborators(org, repo string) ([]github.User, error) {
	result := make([]github.User, 0, len(f.Collaborators))
	for _, login := range f.Collaborators {
		result = append(result, github.User{Login: login})
	}
	return result, nil
}

func (f *fakeGitHubClient) GetRef(org, repo, ref string) (string, error) {
	return f.ref, nil
}

func getTestClient(
	files map[string][]byte,
	enableMdYaml,
	skipCollab,
	includeAliases bool,
	ignorePreconfiguredDefaults bool,
	ownersDirDenylistDefault []string,
	ownersDirDenylistByRepo map[string][]string,
	extraBranchesAndFiles map[string]map[string][]byte,
	cacheOptions *cacheOptions,
	clients localgit.Clients,
) (*Client, func(), error) {
	testAliasesFile := map[string][]byte{
		"OWNERS_ALIASES": []byte("aliases:\n  Best-approvers:\n  - carl\n  - cjwagner\n  best-reviewers:\n  - Carl\n  - BOB"),
	}

	localGit, git, err := clients()
	if err != nil {
		return nil, nil, err
	}

	if localgit.DefaultBranch("") != defaultBranch {
		localGit.InitialBranch = defaultBranch
	}

	if err := localGit.MakeFakeRepo("org", "repo"); err != nil {
		return nil, nil, fmt.Errorf("cannot make fake repo: %v", err)
	}

	if err := localGit.AddCommit("org", "repo", files); err != nil {
		return nil, nil, fmt.Errorf("cannot add initial commit: %v", err)
	}
	if includeAliases {
		if err := localGit.AddCommit("org", "repo", testAliasesFile); err != nil {
			return nil, nil, fmt.Errorf("cannot add OWNERS_ALIASES commit: %v", err)
		}
	}
	if len(extraBranchesAndFiles) > 0 {
		for branch, extraFiles := range extraBranchesAndFiles {
			if err := localGit.CheckoutNewBranch("org", "repo", branch); err != nil {
				return nil, nil, err
			}
			if len(extraFiles) > 0 {
				if err := localGit.AddCommit("org", "repo", extraFiles); err != nil {
					return nil, nil, fmt.Errorf("cannot add commit: %v", err)
				}
			}
		}
		if err := localGit.Checkout("org", "repo", defaultBranch); err != nil {
			return nil, nil, err
		}
	}
	cache := newCache()
	if cacheOptions != nil {
		var entry cacheEntry
		entry.sha, err = localGit.RevParse("org", "repo", "HEAD")
		if err != nil {
			return nil, nil, fmt.Errorf("cannot get commit SHA: %v", err)
		}
		if cacheOptions.hasAliases {
			entry.aliases = make(map[string]sets.String)
		}
		entry.owners = &RepoOwners{
			enableMDYAML: cacheOptions.mdYaml,
		}
		if cacheOptions.commonFileChanged {
			md := map[string][]byte{"common": []byte(`---
This file could be anything
---`)}
			if err := localGit.AddCommit("org", "repo", md); err != nil {
				return nil, nil, fmt.Errorf("cannot add commit: %v", err)
			}
		}
		if cacheOptions.mdFileChanged {
			md := map[string][]byte{"docs/file.md": []byte(`---
approvers:
- ALICE


labels:
- docs
---`)}
			if err := localGit.AddCommit("org", "repo", md); err != nil {
				return nil, nil, fmt.Errorf("cannot add commit: %v", err)
			}
		}
		if cacheOptions.ownersAliasesFileChanged {
			testAliasesFile = map[string][]byte{
				"OWNERS_ALIASES": []byte("aliases:\n  Best-approvers:\n\n  - carl\n  - cjwagner\n  best-reviewers:\n  - Carl\n  - BOB"),
			}
			if err := localGit.AddCommit("org", "repo", testAliasesFile); err != nil {
				return nil, nil, fmt.Errorf("cannot add commit: %v", err)
			}
		}
		if cacheOptions.ownersFileChanged {
			owners := map[string][]byte{
				"OWNERS": []byte(`approvers:
- cjwagner
reviewers:
- "@Alice"
- bob

required_reviewers:
- chris
labels:
- EVERYTHING`),
			}
			if err := localGit.AddCommit("org", "repo", owners); err != nil {
				return nil, nil, fmt.Errorf("cannot add commit: %v", err)
			}
		}
		cache.data["org"+"/"+"repo:master"] = entry
		// mark this entry is cache
		entry.owners.baseDir = "cache"
	}
	ghc := &fakeGitHubClient{Collaborators: []string{"cjwagner", "k8s-ci-robot", "alice", "bob", "carl", "mml", "maggie"}}
	ghc.ref, err = localGit.RevParse("org", "repo", "HEAD")
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get commit SHA: %v", err)
	}
	return &Client{
			logger: logrus.WithField("client", "repoowners"),
			ghc:    ghc,
			delegate: &delegate{
				git:   git,
				cache: cache,

				mdYAMLEnabled: func(org, repo string) bool {
					return enableMdYaml
				},
				skipCollaborators: func(org, repo string) bool {
					return skipCollab
				},
				ownersDirDenylist: func() *prowConf.OwnersDirDenylist {
					return &prowConf.OwnersDirDenylist{
						Repos:                       ownersDirDenylistByRepo,
						Default:                     ownersDirDenylistDefault,
						IgnorePreconfiguredDefaults: ignorePreconfiguredDefaults,
					}
				},
				filenames: ownersconfig.FakeResolver,
			},
		},
		// Clean up function
		func() {
			git.Clean()
			localGit.Clean()
		},
		nil
}

func TestOwnersDirDenylist(t *testing.T) {
	testOwnersDirDenylist(localgit.New, t)
}

func TestOwnersDirDenylistV2(t *testing.T) {
	testOwnersDirDenylist(localgit.NewV2, t)
}

func testOwnersDirDenylist(clients localgit.Clients, t *testing.T) {
	getRepoOwnersWithDenylist := func(t *testing.T, defaults []string, byRepo map[string][]string, ignorePreconfiguredDefaults bool) *RepoOwners {
		client, cleanup, err := getTestClient(testFiles, true, false, true, ignorePreconfiguredDefaults, defaults, byRepo, nil, nil, clients)
		if err != nil {
			t.Fatalf("Error creating test client: %v.", err)
		}
		defer cleanup()

		ro, err := client.LoadRepoOwners("org", "repo", defaultBranch)
		if err != nil {
			t.Fatalf("Unexpected error loading RepoOwners: %v.", err)
		}

		return ro.(*RepoOwners)
	}

	type testConf struct {
		denylistDefault             []string
		denylistByRepo              map[string][]string
		ignorePreconfiguredDefaults bool
		includeDirs                 []string
		excludeDirs                 []string
	}

	tests := map[string]testConf{}

	tests["denylist by org"] = testConf{
		denylistByRepo: map[string][]string{
			"org": {"src"},
		},
		includeDirs: []string{""},
		excludeDirs: []string{"src", "src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["denylist by org/repo"] = testConf{
		denylistByRepo: map[string][]string{
			"org/repo": {"src"},
		},
		includeDirs: []string{""},
		excludeDirs: []string{"src", "src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["denylist by default"] = testConf{
		denylistDefault: []string{"src"},
		includeDirs:     []string{""},
		excludeDirs:     []string{"src", "src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["subdir denylist"] = testConf{
		denylistDefault: []string{"dir"},
		includeDirs:     []string{"", "src"},
		excludeDirs:     []string{"src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["no denylist setup"] = testConf{
		includeDirs: []string{"", "src", "src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["denylist setup but not matching this repo"] = testConf{
		denylistByRepo: map[string][]string{
			"not_org/not_repo": {"src"},
			"not_org":          {"src"},
		},
		includeDirs: []string{"", "src", "src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["non-matching denylist"] = testConf{
		denylistDefault: []string{"sr$"},
		includeDirs:     []string{"", "src", "src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["path denylist"] = testConf{
		denylistDefault: []string{"src/dir"},
		includeDirs:     []string{"", "src"},
		excludeDirs:     []string{"src/dir", "src/dir/conformance", "src/dir/subdir"},
	}
	tests["regexp denylist path"] = testConf{
		denylistDefault: []string{"src/dir/."},
		includeDirs:     []string{"", "src", "src/dir"},
		excludeDirs:     []string{"src/dir/conformance", "src/dir/subdir"},
	}
	tests["path substring"] = testConf{
		denylistDefault: []string{"/c"},
		includeDirs:     []string{"", "src", "src/dir", "src/dir/subdir"},
		excludeDirs:     []string{"src/dir/conformance"},
	}
	tests["exclude preconfigured defaults"] = testConf{
		includeDirs: []string{"", "src", "src/dir", "src/dir/subdir", "vendor"},
		excludeDirs: []string{"vendor/k8s.io/client-go"},
	}
	tests["ignore preconfigured defaults"] = testConf{
		includeDirs:                 []string{"", "src", "src/dir", "src/dir/subdir", "vendor", "vendor/k8s.io/client-go"},
		ignorePreconfiguredDefaults: true,
	}

	for name, conf := range tests {
		t.Run(name, func(t *testing.T) {
			ro := getRepoOwnersWithDenylist(t, conf.denylistDefault, conf.denylistByRepo, conf.ignorePreconfiguredDefaults)

			includeDirs := sets.NewString(conf.includeDirs...)
			excludeDirs := sets.NewString(conf.excludeDirs...)
			for dir := range ro.approvers {
				if excludeDirs.Has(dir) {
					t.Errorf("Expected directory %s to be excluded from the approvers map", dir)
				}
				includeDirs.Delete(dir)
			}
			for dir := range ro.reviewers {
				if excludeDirs.Has(dir) {
					t.Errorf("Expected directory %s to be excluded from the reviewers map", dir)
				}
				includeDirs.Delete(dir)
			}

			for _, dir := range includeDirs.List() {
				t.Errorf("Expected to find approvers or reviewers for directory %s", dir)
			}
		})
	}
}

func TestOwnersRegexpFiltering(t *testing.T) {
	testOwnersRegexpFiltering(localgit.New, t)
}

func TestOwnersRegexpFilteringV2(t *testing.T) {
	testOwnersRegexpFiltering(localgit.NewV2, t)
}

func testOwnersRegexpFiltering(clients localgit.Clients, t *testing.T) {
	tests := map[string]sets.String{
		"re/a/go.go":   sets.NewString("re/all", "re/go", "re/go-in-a"),
		"re/a/md.md":   sets.NewString("re/all", "re/md-in-a"),
		"re/a/txt.txt": sets.NewString("re/all"),
		"re/go.go":     sets.NewString("re/all", "re/go"),
		"re/txt.txt":   sets.NewString("re/all"),
		"re/b/md.md":   sets.NewString("re/all"),
	}

	client, cleanup, err := getTestClient(testFilesRe, true, false, true, false, nil, nil, nil, nil, clients)
	if err != nil {
		t.Fatalf("Error creating test client: %v.", err)
	}
	defer cleanup()

	r, err := client.LoadRepoOwners("org", "repo", defaultBranch)
	if err != nil {
		t.Fatalf("Unexpected error loading RepoOwners: %v.", err)
	}
	ro := r.(*RepoOwners)
	t.Logf("labels: %#v\n\n", ro.labels)
	for file, expected := range tests {
		if got := ro.FindLabelsForFile(file); !got.Equal(expected) {
			t.Errorf("For file %q expected labels %q, but got %q.", file, expected.List(), got.List())
		}
	}
}

func strP(str string) *string {
	return &str
}

func TestLoadRepoOwners(t *testing.T) {
	testLoadRepoOwners(localgit.New, t)
}

func TestLoadRepoOwnersV2(t *testing.T) {
	testLoadRepoOwners(localgit.NewV2, t)
}

func testLoadRepoOwners(clients localgit.Clients, t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		mdEnabled         bool
		aliasesFileExists bool
		skipCollaborators bool
		// used for testing OWNERS from a branch different from master
		branch                *string
		extraBranchesAndFiles map[string]map[string][]byte

		expectedApprovers, expectedReviewers, expectedRequiredReviewers, expectedLabels map[string]map[string]sets.String

		expectedOptions  map[string]dirOptions
		cacheOptions     *cacheOptions
		expectedReusable bool
	}{
		{
			name: "no alias, no md",
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll(),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":        patternAll("EVERYTHING"),
				"src/dir": patternAll("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
		},
		{
			name:              "alias, no md",
			aliasesFileExists: true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("carl", "cjwagner"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":        patternAll("EVERYTHING"),
				"src/dir": patternAll("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
		},
		{
			name:              "alias, md",
			aliasesFileExists: true,
			mdEnabled:         true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("carl", "cjwagner"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"docs/file.md":        patternAll("alice"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":             patternAll("EVERYTHING"),
				"src/dir":      patternAll("src-code"),
				"docs/file.md": patternAll("docs"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
		},
		{
			name:   "OWNERS from non-default branch",
			branch: strP("release-1.10"),
			extraBranchesAndFiles: map[string]map[string][]byte{
				"release-1.10": {
					"src/doc/OWNERS": []byte("approvers:\n - maggie\n"),
				},
			},
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll(),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"src/doc":             patternAll("maggie"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":        patternAll("EVERYTHING"),
				"src/dir": patternAll("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
		},
		{
			name:   "OWNERS from master branch while release branch diverges",
			branch: strP(defaultBranch),
			extraBranchesAndFiles: map[string]map[string][]byte{
				"release-1.10": {
					"src/doc/OWNERS": []byte("approvers:\n - maggie\n"),
				},
			},
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll(),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":        patternAll("EVERYTHING"),
				"src/dir": patternAll("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
		},
		{
			name:              "Skip collaborator checks, use only OWNERS files",
			skipCollaborators: true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("best-approvers"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner", "jakub"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":        patternAll("EVERYTHING"),
				"src/dir": patternAll("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
		},
		{
			name:              "cache reuses, base sha equals to cache sha",
			skipCollaborators: true,
			cacheOptions: &cacheOptions{
				hasAliases: true,
			},
			expectedReusable: true,
		},
		{
			name:              "cache reuses, only change common files",
			skipCollaborators: true,
			cacheOptions: &cacheOptions{
				hasAliases:        true,
				commonFileChanged: true,
			},
			expectedReusable: true,
		},
		{
			name:              "cache does not reuse, mdYaml changed",
			aliasesFileExists: true,
			mdEnabled:         true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("carl", "cjwagner"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"docs/file.md":        patternAll("alice"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":             patternAll("EVERYTHING"),
				"src/dir":      patternAll("src-code"),
				"docs/file.md": patternAll("docs"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
			cacheOptions: &cacheOptions{},
		},
		{
			name:              "cache does not reuse, aliases is nil",
			aliasesFileExists: true,
			mdEnabled:         true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("carl", "cjwagner"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"docs/file.md":        patternAll("alice"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":             patternAll("EVERYTHING"),
				"src/dir":      patternAll("src-code"),
				"docs/file.md": patternAll("docs"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
			cacheOptions: &cacheOptions{
				commonFileChanged: true,
			},
		},
		{
			name:              "cache does not reuse, changes files contains OWNERS",
			aliasesFileExists: true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("carl", "cjwagner"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":        patternAll("EVERYTHING"),
				"src/dir": patternAll("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
			cacheOptions: &cacheOptions{
				hasAliases:        true,
				ownersFileChanged: true,
			},
		},
		{
			name:              "cache does not reuse, changes files contains OWNERS_ALIASES",
			aliasesFileExists: true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("carl", "cjwagner"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":        patternAll("EVERYTHING"),
				"src/dir": patternAll("src-code"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
			cacheOptions: &cacheOptions{
				hasAliases:               true,
				ownersAliasesFileChanged: true,
			},
		},
		{
			name:              "cache reuses, changes files contains .md, but mdYaml is false",
			skipCollaborators: true,
			cacheOptions: &cacheOptions{
				hasAliases:    true,
				mdFileChanged: true,
			},
			expectedReusable: true,
		},
		{
			name:              "cache does not reuse, changes files contains .md, and mdYaml is true",
			aliasesFileExists: true,
			mdEnabled:         true,
			expectedApprovers: map[string]map[string]sets.String{
				"":                    patternAll("cjwagner"),
				"src":                 patternAll("carl", "cjwagner"),
				"src/dir":             patternAll("bob"),
				"src/dir/conformance": patternAll("mml"),
				"src/dir/subdir":      patternAll("alice", "bob"),
				"docs/file.md":        patternAll("alice"),
				"vendor":              patternAll("alice"),
			},
			expectedReviewers: map[string]map[string]sets.String{
				"":               patternAll("alice", "bob"),
				"src/dir":        patternAll("alice", "cjwagner"),
				"src/dir/subdir": patternAll("alice", "bob"),
			},
			expectedRequiredReviewers: map[string]map[string]sets.String{
				"":        patternAll("chris"),
				"src/dir": patternAll("ben"),
			},
			expectedLabels: map[string]map[string]sets.String{
				"":             patternAll("EVERYTHING"),
				"src/dir":      patternAll("src-code"),
				"docs/file.md": patternAll("docs"),
			},
			expectedOptions: map[string]dirOptions{
				"src/dir/conformance": {
					NoParentOwners:               true,
					AutoApproveUnownedSubfolders: true,
				},
			},
			cacheOptions: &cacheOptions{
				hasAliases:    true,
				mdYaml:        true,
				mdFileChanged: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("Running scenario %q", test.name)
			client, cleanup, err := getTestClient(testFiles, test.mdEnabled, test.skipCollaborators, test.aliasesFileExists, false, nil, nil, test.extraBranchesAndFiles, test.cacheOptions, clients)
			if err != nil {
				t.Fatalf("Error creating test client: %v.", err)
			}
			t.Cleanup(cleanup)

			base := defaultBranch
			defer cleanup()

			if test.branch != nil {
				base = *test.branch
			}
			r, err := client.LoadRepoOwners("org", "repo", base)
			if err != nil {
				t.Fatalf("Unexpected error loading RepoOwners: %v.", err)
			}
			ro := r.(*RepoOwners)
			if test.expectedReusable {
				if ro.baseDir != "cache" {
					t.Fatalf("expected cache must be reused, but got baseDir %q", ro.baseDir)
				}
				return
			} else {
				if ro.baseDir == "cache" {
					t.Fatal("expected cache should not be reused, but reused")
				}
			}
			if ro.baseDir == "" {
				t.Fatal("Expected 'baseDir' to be populated.")
			}
			if (ro.RepoAliases != nil) != test.aliasesFileExists {
				t.Fatalf("Expected 'RepoAliases' to be poplulated: %t, but got %t.", test.aliasesFileExists, ro.RepoAliases != nil)
			}
			if ro.enableMDYAML != test.mdEnabled {
				t.Fatalf("Expected 'enableMdYaml' to be: %t, but got %t.", test.mdEnabled, ro.enableMDYAML)
			}

			check := func(field string, expected map[string]map[string]sets.String, got map[string]map[*regexp.Regexp]sets.String) {
				converted := map[string]map[string]sets.String{}
				for path, m := range got {
					converted[path] = map[string]sets.String{}
					for re, s := range m {
						var pattern string
						if re != nil {
							pattern = re.String()
						}
						converted[path][pattern] = s
					}
				}
				if !reflect.DeepEqual(expected, converted) {
					t.Errorf("Expected %s to be:\n%+v\ngot:\n%+v.", field, expected, converted)
				}
			}
			check("approvers", test.expectedApprovers, ro.approvers)
			check("reviewers", test.expectedReviewers, ro.reviewers)
			check("required_reviewers", test.expectedRequiredReviewers, ro.requiredReviewers)
			check("labels", test.expectedLabels, ro.labels)
			if !reflect.DeepEqual(test.expectedOptions, ro.options) {
				t.Errorf("Expected options to be:\n%#v\ngot:\n%#v.", test.expectedOptions, ro.options)
			}
		})
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
		approvers: map[string]map[*regexp.Regexp]sets.String{
			baseDir:      regexpAll("alice", "bob"),
			leafDir:      regexpAll("carl", "dave"),
			noParentsDir: regexpAll("mml"),
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
			expectedLeafOwners: ro.approvers[baseDir][nil],
			expectedAllOwners:  ro.approvers[baseDir][nil],
		},
		{
			name:               "Modified Leaf Dir Only",
			filePath:           filepath.Join(leafDir, "testFile.md"),
			expectedOwnersPath: leafDir,
			expectedLeafOwners: ro.approvers[leafDir][nil],
			expectedAllOwners:  ro.approvers[baseDir][nil].Union(ro.approvers[leafDir][nil]),
		},
		{
			name:               "Modified NoParentOwners Dir Only",
			filePath:           filepath.Join(noParentsDir, "testFile.go"),
			expectedOwnersPath: noParentsDir,
			expectedLeafOwners: ro.approvers[noParentsDir][nil],
			expectedAllOwners:  ro.approvers[noParentsDir][nil],
		},
		{
			name:               "Modified Nonexistent Dir (Default to Base)",
			filePath:           filepath.Join(nonExistentDir, "testFile.md"),
			expectedOwnersPath: baseDir,
			expectedLeafOwners: ro.approvers[baseDir][nil],
			expectedAllOwners:  ro.approvers[baseDir][nil],
		},
	}
	for testNum, test := range tests {
		foundLeafApprovers := ro.LeafApprovers(test.filePath)
		foundApprovers := ro.Approvers(test.filePath).Set()
		foundOwnersPath := ro.FindApproverOwnersForFile(test.filePath)
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
		labels: map[string]map[*regexp.Regexp]sets.String{
			baseDir: regexpAll("sig/godzilla"),
			leafDir: regexpAll("wg/save-tokyo"),
		},
	}
	for _, test := range tests {
		got := testOwners.FindLabelsForFile(test.path)
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
			name:         "GitHub Style Input (No Root)",
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

func TestExpandAliases(t *testing.T) {
	testAliases := RepoAliases{
		"team/t1": sets.NewString("u1", "u2"),
		"team/t2": sets.NewString("u1", "u3"),
		"team/t3": sets.NewString(),
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
		{
			name:             "Empty team.",
			unexpanded:       sets.NewString("Team/T3"),
			expectedExpanded: sets.NewString(),
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

func TestSaveSimpleConfig(t *testing.T) {
	dir, err := ioutil.TempDir("", "simpleConfig")
	if err != nil {
		t.Errorf("unexpected error when creating temp dir")
	}
	defer os.RemoveAll(dir)

	tests := []struct {
		name     string
		given    SimpleConfig
		expected string
	}{
		{
			name: "No expansions.",
			given: SimpleConfig{
				Config: Config{
					Approvers: []string{"david", "sig-alias", "Alice"},
					Reviewers: []string{"adam", "sig-alias"},
				},
			},
			expected: `approvers:
- david
- sig-alias
- Alice
options: {}
reviewers:
- adam
- sig-alias
`,
		},
	}

	for _, test := range tests {
		file := filepath.Join(dir, fmt.Sprintf("%s.yaml", test.name))
		err := SaveSimpleConfig(test.given, file)
		if err != nil {
			t.Errorf("unexpected error when writing simple config")
		}
		b, err := ioutil.ReadFile(file)
		if err != nil {
			t.Errorf("unexpected error when reading file: %s", file)
		}
		s := string(b)
		if test.expected != s {
			t.Errorf("result '%s' is differ from expected: '%s'", s, test.expected)
		}
		simple, err := LoadSimpleConfig(b)
		if err != nil {
			t.Errorf("unexpected error when load simple config: %v", err)
		}
		if !reflect.DeepEqual(simple, test.given) {
			t.Errorf("unexpected error when loading simple config from: '%s'", diff.ObjectReflectDiff(simple, test.given))
		}
	}
}

func TestSaveFullConfig(t *testing.T) {
	dir, err := ioutil.TempDir("", "fullConfig")
	if err != nil {
		t.Errorf("unexpected error when creating temp dir")
	}
	defer os.RemoveAll(dir)

	tests := []struct {
		name     string
		given    FullConfig
		expected string
	}{
		{
			name: "No expansions.",
			given: FullConfig{
				Filters: map[string]Config{
					".*": {
						Approvers: []string{"alice", "bob", "carol", "david"},
						Reviewers: []string{"adam", "bob", "carol"},
					},
				},
			},
			expected: `filters:
  .*:
    approvers:
    - alice
    - bob
    - carol
    - david
    reviewers:
    - adam
    - bob
    - carol
options: {}
`,
		},
	}

	for _, test := range tests {
		file := filepath.Join(dir, fmt.Sprintf("%s.yaml", test.name))
		err := SaveFullConfig(test.given, file)
		if err != nil {
			t.Errorf("unexpected error when writing full config")
		}
		b, err := ioutil.ReadFile(file)
		if err != nil {
			t.Errorf("unexpected error when reading file: %s", file)
		}
		s := string(b)
		if test.expected != s {
			t.Errorf("result '%s' is differ from expected: '%s'", s, test.expected)
		}
		full, err := LoadFullConfig(b)
		if err != nil {
			t.Errorf("unexpected error when load full config: %v", err)
		}
		if !reflect.DeepEqual(full, test.given) {
			t.Errorf("unexpected error when loading simple config from: '%s'", diff.ObjectReflectDiff(full, test.given))
		}
	}
}

func TestTopLevelApprovers(t *testing.T) {
	expectedApprovers := []string{"alice", "bob"}
	ro := &RepoOwners{
		approvers: map[string]map[*regexp.Regexp]sets.String{
			baseDir: regexpAll(expectedApprovers...),
			leafDir: regexpAll("carl", "dave"),
		},
	}

	foundApprovers := ro.TopLevelApprovers()
	if !foundApprovers.Equal(sets.NewString(expectedApprovers...)) {
		t.Errorf("Expected Owners: %v\tFound Owners: %v ", expectedApprovers, foundApprovers)
	}
}

func TestCacheDoesntRace(t *testing.T) {
	key := "key"
	cache := newCache()

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() { cache.setEntry(key, cacheEntry{}); wg.Done() }()
	go func() { cache.getEntry(key); wg.Done() }()

	wg.Wait()
}

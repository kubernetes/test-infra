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

package verifyowners

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pkg/layeredsets"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/ownersconfig"
	"k8s.io/test-infra/prow/repoowners"
)

var ownerFiles = map[string][]byte{
	"emptyApprovers": []byte(`approvers:
reviewers:
- alice
- bob
labels:
- label1
`),
	"emptyApproversFilters": []byte(`filters:
  ".*":
    approvers:
    reviewers:
    - alice
    - bob
    labels:
    - label1
`),
	"invalidSyntax": []byte(`approvers:
- jdoe
reviewers:
- alice
- bob
labels
- label1
`),
	"invalidSyntaxFilters": []byte(`filters:
  ".*":
    approvers:
    - jdoe
    reviewers:
    - alice
    - bob
    labels
    - label1
`),
	"invalidLabels": []byte(`approvers:
- jdoe
reviewers:
- alice
- bob
labels:
- lgtm
`),
	"invalidLabelsFilters": []byte(`filters:
  ".*":
    approvers:
    - jdoe
    reviewers:
    - alice
    - bob
    labels:
    - lgtm
`),
	"noApprovers": []byte(`reviewers:
- alice
- bob
labels:
- label1
`),
	"noApproversFilters": []byte(`filters:
  ".*":
    reviewers:
    - alice
    - bob
    labels:
    - label1
`),
	"valid": []byte(`approvers:
- jdoe
reviewers:
- alice
- bob
labels:
- label1
`),
	"validFilters": []byte(`filters:
  ".*":
    approvers:
    - jdoe
    reviewers:
    - alice
    - bob
    labels:
    - label1
`),
	"referencesToBeAddedAlias": []byte(`approvers:
- not-yet-existing-alias
`),
}

var patches = map[string]string{
	"referencesToBeAddedAlias": `
+ - not-yet-existing-alias
`,
}

var ownerAliasesFiles = map[string][]byte{
	"toBeAddedAlias": []byte(`aliases:
  not-yet-existing-alias:
  - bob
`),
}

func IssueLabelsContain(arr []string, str string) bool {
	for _, a := range arr {
		// IssueLabels format is owner/repo#number:label
		b := strings.Split(a, ":")
		if b[len(b)-1] == str {
			return true
		}
	}
	return false
}

func ownersFilePatch(files []string, ownersFile string, useEmptyPatch bool) map[string]string {
	changes := emptyPatch(files)
	if useEmptyPatch {
		return changes
	}
	for _, file := range files {
		if strings.Contains(file, "OWNERS") && !strings.Contains(file, "OWNERS_ALIASES") {
			changes[file] = patches[ownersFile]
		}
	}
	return changes
}

func emptyPatch(files []string) map[string]string {
	changes := make(map[string]string, len(files))
	for _, f := range files {
		changes[f] = ""
	}
	return changes
}

func newFakeGitHubClient(changed map[string]string, removed []string, pr int) *fakegithub.FakeClient {
	var changes []github.PullRequestChange
	for file, patch := range changed {
		changes = append(changes, github.PullRequestChange{Filename: file, Patch: patch})
	}
	for _, file := range removed {
		changes = append(changes, github.PullRequestChange{Filename: file, Status: github.PullRequestFileRemoved})
	}
	return &fakegithub.FakeClient{
		PullRequestChanges: map[int][]github.PullRequestChange{pr: changes},
		Reviews:            map[int][]github.Review{},
		Collaborators:      []string{"alice", "bob", "jdoe"},
		IssueComments:      map[int][]github.IssueComment{},
	}
}

type fakePruner struct{}

func (fp *fakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}

type fakeRepoownersClient struct {
	foc *fakeOwnersClient
}

func (froc fakeRepoownersClient) LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error) {
	return froc.foc, nil
}

type fakeOwnersClient struct {
	owners            map[string]string
	approvers         map[string]layeredsets.String
	leafApprovers     map[string]sets.String
	reviewers         map[string]layeredsets.String
	requiredReviewers map[string]sets.String
	leafReviewers     map[string]sets.String
	dirIgnorelist     []*regexp.Regexp
}

func (foc *fakeOwnersClient) Filenames() ownersconfig.Filenames {
	return ownersconfig.FakeFilenames
}

func (foc *fakeOwnersClient) Approvers(path string) layeredsets.String {
	return foc.approvers[path]
}

func (foc *fakeOwnersClient) LeafApprovers(path string) sets.String {
	return foc.leafApprovers[path]
}

func (foc *fakeOwnersClient) FindApproverOwnersForFile(path string) string {
	return foc.owners[path]
}

func (foc *fakeOwnersClient) Reviewers(path string) layeredsets.String {
	return foc.reviewers[path]
}

func (foc *fakeOwnersClient) RequiredReviewers(path string) sets.String {
	return foc.requiredReviewers[path]
}

func (foc *fakeOwnersClient) LeafReviewers(path string) sets.String {
	return foc.leafReviewers[path]
}

func (foc *fakeOwnersClient) FindReviewersOwnersForFile(path string) string {
	return foc.owners[path]
}

func (foc *fakeOwnersClient) FindLabelsForFile(path string) sets.String {
	return sets.String{}
}

func (foc *fakeOwnersClient) IsNoParentOwners(path string) bool {
	return false
}

func (foc *fakeOwnersClient) ParseSimpleConfig(path string) (repoowners.SimpleConfig, error) {
	dir := filepath.Dir(path)
	for _, re := range foc.dirIgnorelist {
		if re.MatchString(dir) {
			return repoowners.SimpleConfig{}, filepath.SkipDir
		}
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return repoowners.SimpleConfig{}, err
	}
	full := new(repoowners.SimpleConfig)
	err = yaml.Unmarshal(b, full)
	return *full, err
}

func (foc *fakeOwnersClient) ParseFullConfig(path string) (repoowners.FullConfig, error) {
	dir := filepath.Dir(path)
	for _, re := range foc.dirIgnorelist {
		if re.MatchString(dir) {
			return repoowners.FullConfig{}, filepath.SkipDir
		}
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return repoowners.FullConfig{}, err
	}
	full := new(repoowners.FullConfig)
	err = yaml.Unmarshal(b, full)
	return *full, err
}

func (foc *fakeOwnersClient) TopLevelApprovers() sets.String {
	return sets.String{}
}

func makeFakeRepoOwnersClient() fakeRepoownersClient {
	return fakeRepoownersClient{
		foc: &fakeOwnersClient{},
	}
}

func addFilesToRepo(lg *localgit.LocalGit, paths []string, ownersFile string) error {
	origFiles := map[string][]byte{}
	for _, file := range paths {
		if strings.Contains(file, "OWNERS_ALIASES") {
			origFiles[file] = ownerAliasesFiles[ownersFile]
		} else if strings.Contains(file, "OWNERS") {
			origFiles[file] = ownerFiles[ownersFile]
		} else {
			origFiles[file] = []byte("foo")
		}
	}
	return lg.AddCommit("org", "repo", origFiles)
}

func TestHandle(t *testing.T) {
	testHandle(localgit.New, t)
}

func TestHandleV2(t *testing.T) {
	testHandle(localgit.NewV2, t)
}

func testHandle(clients localgit.Clients, t *testing.T) {
	var tests = []struct {
		name                string
		filesChanged        []string
		usePatch            bool
		filesRemoved        []string
		ownersFile          string
		filesChangedAfterPR []string
		addedContent        string
		shouldLabel         bool
	}{
		{
			name:         "no OWNERS file",
			filesChanged: []string{"a.go", "b.go"},
			ownersFile:   "valid",
			shouldLabel:  false,
		},
		{
			name:         "no OWNERS file with filters",
			filesChanged: []string{"a.go", "b.go"},
			ownersFile:   "validFilters",
			shouldLabel:  false,
		},
		{
			name:         "good OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "valid",
			shouldLabel:  false,
		},
		{
			name:         "good OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "validFilters",
			shouldLabel:  false,
		},
		{
			name:         "invalid syntax OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntax",
			shouldLabel:  true,
		},
		{
			name:         "invalid syntax OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntaxFilters",
			shouldLabel:  true,
		},
		{
			name:         "forbidden labels in OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidLabels",
			shouldLabel:  true,
		},
		{
			name:         "forbidden labels in OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidLabelsFilters",
			shouldLabel:  true,
		},
		{
			name:         "empty approvers in OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "emptyApprovers",
			shouldLabel:  true,
		},
		{
			name:         "empty approvers in OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "emptyApproversFilters",
			shouldLabel:  true,
		},
		{
			name:         "no approvers in OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "noApprovers",
			shouldLabel:  true,
		},
		{
			name:         "no approvers in OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "noApproversFilters",
			shouldLabel:  true,
		},
		{
			name:         "no approvers in pkg/OWNERS file",
			filesChanged: []string{"pkg/OWNERS", "b.go"},
			ownersFile:   "noApprovers",
			shouldLabel:  false,
		},
		{
			name:         "no approvers in pkg/OWNERS file with filters",
			filesChanged: []string{"pkg/OWNERS", "b.go"},
			ownersFile:   "noApproversFilters",
			shouldLabel:  false,
		},
		{
			name:         "OWNERS file was removed",
			filesRemoved: []string{"pkg/OWNERS"},
			ownersFile:   "valid",
			shouldLabel:  false,
		},
		{
			name:         "OWNERS_ALIASES file was removed",
			filesRemoved: []string{"OWNERS_ALIASES"},
			shouldLabel:  false,
		},
		{
			name:                "new alias added after a PR references that alias",
			filesChanged:        []string{"OWNERS"},
			usePatch:            true,
			ownersFile:          "referencesToBeAddedAlias",
			filesChangedAfterPR: []string{"OWNERS_ALIASES"},
			addedContent:        "toBeAddedAlias",
			shouldLabel:         false,
		},
	}
	lg, c, err := clients()
	if err != nil {
		t.Fatalf("Making localgit: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Cleaning up localgit: %v", err)
		}
		if err := c.Clean(); err != nil {
			t.Errorf("Cleaning up client: %v", err)
		}
	}()
	if err := lg.MakeFakeRepo("org", "repo"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pr := i + 1
			// make sure we're on master before branching
			if err := lg.Checkout("org", "repo", "master"); err != nil {
				t.Fatalf("Switching to master branch: %v", err)
			}
			if len(test.filesRemoved) > 0 {
				if err := addFilesToRepo(lg, test.filesRemoved, test.ownersFile); err != nil {
					t.Fatalf("Adding base commit: %v", err)
				}
			}

			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}

			if len(test.filesChanged) > 0 {
				if err := addFilesToRepo(lg, test.filesChanged, test.ownersFile); err != nil {
					t.Fatalf("Adding PR commit: %v", err)
				}
			}
			if len(test.filesRemoved) > 0 {
				if err := lg.RmCommit("org", "repo", test.filesRemoved); err != nil {
					t.Fatalf("Adding PR commit (removing files): %v", err)
				}
			}

			sha, err := lg.RevParse("org", "repo", "HEAD")
			if err != nil {
				t.Fatalf("Getting commit SHA: %v", err)
			}
			if len(test.filesChangedAfterPR) > 0 {
				if err := lg.Checkout("org", "repo", "master"); err != nil {
					t.Fatalf("Switching to master branch: %v", err)
				}
				if err := addFilesToRepo(lg, test.filesChangedAfterPR, test.addedContent); err != nil {
					t.Fatalf("Adding commit to master: %v", err)
				}
			}
			pre := &github.PullRequestEvent{
				PullRequest: github.PullRequest{
					User: github.User{Login: "author"},
					Base: github.PullRequestBranch{
						Ref: "master",
					},
					Head: github.PullRequestBranch{
						SHA: sha,
					},
				},
			}
			changes := ownersFilePatch(test.filesChanged, test.ownersFile, !test.usePatch)
			fghc := newFakeGitHubClient(changes, test.filesRemoved, pr)
			fghc.PullRequests = map[int]*github.PullRequest{}
			fghc.PullRequests[pr] = &github.PullRequest{
				Base: github.PullRequestBranch{
					Ref: "master",
				},
			}

			prInfo := info{
				org:          "org",
				repo:         "repo",
				repoFullName: "org/repo",
				number:       pr,
			}

			if err := handle(fghc, c, makeFakeRepoOwnersClient(), logrus.WithField("plugin", PluginName), &pre.PullRequest, prInfo, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, false, &fakePruner{}, ownersconfig.FakeResolver); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			if !test.shouldLabel && IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
				t.Fatalf("%s: didn't expect label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			} else if test.shouldLabel && !IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
				t.Fatalf("%s: expected label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			}
		})
	}
}

func TestParseOwnersFile(t *testing.T) {
	testParseOwnersFile(localgit.New, t)
}

func TestParseOwnersFileV2(t *testing.T) {
	testParseOwnersFile(localgit.NewV2, t)
}

func testParseOwnersFile(clients localgit.Clients, t *testing.T) {
	tests := []struct {
		name     string
		document []byte
		patch    string
		errLine  int
	}{
		{
			name:     "emptyApprovers",
			document: ownerFiles["emptyApprovers"],
			errLine:  1,
		},
		{
			name:     "emptyApproversFilters",
			document: ownerFiles["emptyApproversFilters"],
			errLine:  1,
		},
		{
			name:     "invalidSyntax",
			document: ownerFiles["invalidSyntax"],
			errLine:  7,
		},
		{
			name:     "invalidSyntaxFilters",
			document: ownerFiles["invalidSyntaxFilters"],
			errLine:  9,
		},
		{
			name:     "invalidSyntax edit",
			document: ownerFiles["invalidSyntax"],
			patch: `@@ -3,6 +3,6 @@ approvers:
 reviewers:
 - alice
 - bob
-labels:
+labels
 - label1
 `,
			errLine: 1,
		},
		{
			name:     "noApprovers",
			document: ownerFiles["noApprovers"],
			errLine:  1,
		},
		{
			name:     "noApproversFilters",
			document: ownerFiles["noApproversFilters"],
			errLine:  1,
		},
		{
			name:     "valid",
			document: ownerFiles["valid"],
		},
		{
			name:     "validFilters",
			document: ownerFiles["validFilters"],
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pr := i + 1
			lg, c, err := clients()
			if err != nil {
				t.Fatalf("Making localgit: %v", err)
			}
			defer func() {
				if err := lg.Clean(); err != nil {
					t.Errorf("Cleaning up localgit: %v", err)
				}
				if err := c.Clean(); err != nil {
					t.Errorf("Cleaning up client: %v", err)
				}
			}()
			if err := lg.MakeFakeRepo("org", "repo"); err != nil {
				t.Fatalf("Making fake repo: %v", err)
			}
			// make sure we're on master before branching
			if err := lg.Checkout("org", "repo", "master"); err != nil {
				t.Fatalf("Switching to master branch: %v", err)
			}
			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}
			pullFiles := map[string][]byte{}
			pullFiles["OWNERS"] = test.document
			if err := lg.AddCommit("org", "repo", pullFiles); err != nil {
				t.Fatalf("Adding PR commit: %v", err)
			}

			if test.patch == "" {
				test.patch = makePatch(test.document)
			}
			change := github.PullRequestChange{
				Filename: "OWNERS",
				Patch:    test.patch,
			}

			r, err := c.ClientFor("org", "repo")
			if err != nil {
				t.Fatalf("error cloning the repo: %v", err)
			}
			defer func() {
				if err := r.Clean(); err != nil {
					t.Fatalf("error cleaning up repo: %v", err)
				}
			}()

			path := filepath.Join(r.Directory(), "OWNERS")
			message, _ := parseOwnersFile(&fakeOwnersClient{}, path, change, &logrus.Entry{}, []string{}, ownersconfig.FakeFilenames)
			if message != nil {
				if test.errLine == 0 {
					t.Errorf("%s: expected no error, got one: %s", test.name, message.message)
				}
				if message.line != test.errLine {
					t.Errorf("%s: wrong line for message, expected %d, got %d", test.name, test.errLine, message.line)
				}
			} else if test.errLine != 0 {
				t.Errorf("%s: expected an error, got none", test.name)
			}
		})
	}
}

func makePatch(b []byte) string {
	p := bytes.Replace(b, []byte{'\n'}, []byte{'\n', '+'}, -1)
	nbLines := bytes.Count(p, []byte{'+'}) + 1
	return fmt.Sprintf("@@ -0,0 +1,%d @@\n+%s", nbLines, p)
}

func TestHelpProvider(t *testing.T) {
	enabledRepos := []config.OrgRepo{
		{Org: "org1", Repo: "repo"},
		{Org: "org2", Repo: "repo"},
	}
	cases := []struct {
		name         string
		config       *plugins.Configuration
		enabledRepos []config.OrgRepo
		err          bool
		expected     *pluginhelp.PluginHelp
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: enabledRepos,
			expected: &pluginhelp.PluginHelp{
				Description: "The verify-owners plugin validates OWNERS and OWNERS_ALIASES files (by default) and ensures that they always contain collaborators of the org, if they are modified in a PR. On validation failure it automatically adds the 'do-not-merge/invalid-owners-file' label to the PR, and a review comment on the incriminating file(s). Per-repo configuration for filenames is possible.",
				Config: map[string]string{
					"default": "OWNERS and OWNERS_ALIASES files are validated.",
				},
				Commands: []pluginhelp.Command{{
					Usage:       "/verify-owners",
					Description: "do-not-merge/invalid-owners-file",
					Examples:    []string{"/verify-owners"},
					WhoCanUse:   "Anyone",
				}},
			},
		},
		{
			name: "ReviewerCount specified",
			config: &plugins.Configuration{
				Owners: plugins.Owners{
					LabelsBlackList: []string{"label1", "label2"},
				},
			},
			enabledRepos: enabledRepos,
			expected: &pluginhelp.PluginHelp{
				Description: "The verify-owners plugin validates OWNERS and OWNERS_ALIASES files (by default) and ensures that they always contain collaborators of the org, if they are modified in a PR. On validation failure it automatically adds the 'do-not-merge/invalid-owners-file' label to the PR, and a review comment on the incriminating file(s). Per-repo configuration for filenames is possible.",
				Config: map[string]string{
					"default": "OWNERS and OWNERS_ALIASES files are validated. The verify-owners plugin will complain if OWNERS files contain any of the following banned labels: label1, label2.",
				},
				Commands: []pluginhelp.Command{{
					Usage:       "/verify-owners",
					Description: "do-not-merge/invalid-owners-file",
					Examples:    []string{"/verify-owners"},
					WhoCanUse:   "Anyone",
				}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pluginHelp, err := helpProvider(c.config, c.enabledRepos)
			if err != nil && !c.err {
				t.Fatalf("helpProvider error: %v", err)
			}
			if diff := cmp.Diff(pluginHelp, c.expected); diff != "" {
				t.Errorf("%s: did not get correct help: %v", c.name, diff)
			}
		})
	}
}

var ownersFiles = map[string][]byte{
	"nonCollaborators": []byte(`reviewers:
- phippy
- alice
approvers:
- zee
- bob
`),
	"collaboratorsWithAliases": []byte(`reviewers:
- foo-reviewers
- alice
approvers:
- bob
`),
	"nonCollaboratorsWithAliases": []byte(`reviewers:
- foo-reviewers
- alice
- goldie
approvers:
- bob
`),
}

var ownersPatch = map[string]string{
	"collaboratorAdditions": `@@ -1,4 +1,6 @@
 reviewers:
 - phippy
+- alice
 approvers:
 - zee
+- bob
`,
	"collaboratorRemovals": `@@ -1,8 +1,6 @@
 reviewers:
 - phippy
 - alice
-- bob
 approvers:
 - zee
 - bob
-- alice
`,
	"nonCollaboratorAdditions": `@@ -1,4 +1,6 @@
 reviewers:
+- phippy
 - alice
 approvers:
+- zee
 - bob
`,
	"nonCollaboratorRemovals": `@@ -1,8 +1,6 @@
 reviewers:
 - phippy
 - alice
-- al
 approvers:
 - zee
 - bob
-- bo
`,
	"nonCollaboratorsWithAliases": `@@ -1,4 +1,6 @@
 reviewers:
+- foo-reviewers
 - alice
+- goldie
 approvers:
 - bob
`,
	"collaboratorsWithAliases": `@@ -1,2 +1,5 @@
+reviewers:
+- foo-reviewers
+- alice
 approvers:
 - bob
`,
}

var ownersAliases = map[string][]byte{
	"nonCollaborators": []byte(`aliases:
  foo-reviewers:
  - alice
  - phippy
  - zee
`),
	"collaborators": []byte(`aliases:
  foo-reviewers:
  - alice
`),
}

var ownersAliasesPatch = map[string]string{
	"nonCollaboratorAdditions": `@@ -1,3 +1,5 @@
 aliases:
   foo-reviewers:
   - alice
+  - phippy
+  - zee
`,
	"nonCollaboratorRemovals": `@@ -1,6 +1,5 @@
 aliases:
   foo-reviewers:
   - alice
-  - al
   - phippy
   - zee
`,
	"collaboratorAdditions": `@@ -1,4 +1,5 @@
 aliases:
   foo-reviewers:
+  - alice
   - phippy
   - zee
`,
	"collaboratorRemovals": `@@ -1,6 +1,5 @@
 aliases:
   foo-reviewers:
   - alice
-  - bob
   - phippy
   - zee
`,
}

func TestNonCollaborators(t *testing.T) {
	testNonCollaborators(localgit.New, t)
}

func TestNonCollaboratorsV2(t *testing.T) {
	testNonCollaborators(localgit.NewV2, t)
}

func testNonCollaborators(clients localgit.Clients, t *testing.T) {
	const nonTrustedNotMemberNotCollaborator = "User is not a member of the org. User is not a collaborator."
	var tests = []struct {
		name                 string
		filesChanged         []string
		ownersFile           string
		ownersPatch          string
		ownersAliasesFile    string
		ownersAliasesPatch   string
		includeVendorOwners  bool
		skipTrustedUserCheck bool
		shouldLabel          bool
		shouldComment        bool
		commentShouldContain string
	}{
		{
			name:          "collaborators additions in OWNERS file",
			filesChanged:  []string{"OWNERS"},
			ownersFile:    "nonCollaborators",
			ownersPatch:   "collaboratorAdditions",
			shouldLabel:   false,
			shouldComment: false,
		},
		{
			name:          "collaborators removals in OWNERS file",
			filesChanged:  []string{"OWNERS"},
			ownersFile:    "nonCollaborators",
			ownersPatch:   "collaboratorRemovals",
			shouldLabel:   false,
			shouldComment: false,
		},
		{
			name:                 "non-collaborators additions in OWNERS file",
			filesChanged:         []string{"OWNERS"},
			ownersFile:           "nonCollaborators",
			ownersPatch:          "nonCollaboratorAdditions",
			shouldLabel:          true,
			shouldComment:        true,
			commentShouldContain: nonTrustedNotMemberNotCollaborator,
		},
		{
			name:          "non-collaborators removal in OWNERS file",
			filesChanged:  []string{"OWNERS"},
			ownersFile:    "nonCollaborators",
			ownersPatch:   "nonCollaboratorRemovals",
			shouldLabel:   false,
			shouldComment: false,
		},
		{
			name:                 "non-collaborators additions in OWNERS file, with skipTrustedUserCheck=true",
			filesChanged:         []string{"OWNERS"},
			ownersFile:           "nonCollaborators",
			ownersPatch:          "nonCollaboratorAdditions",
			skipTrustedUserCheck: true,
			shouldLabel:          false,
			shouldComment:        false,
		},
		{
			name:                 "non-collaborators additions in OWNERS_ALIASES file",
			filesChanged:         []string{"OWNERS_ALIASES"},
			ownersFile:           "collaboratorsWithAliases",
			ownersAliasesFile:    "nonCollaborators",
			ownersAliasesPatch:   "nonCollaboratorAdditions",
			shouldLabel:          true,
			shouldComment:        true,
			commentShouldContain: nonTrustedNotMemberNotCollaborator,
		},
		{
			name:               "non-collaborators removals in OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS_ALIASES"},
			ownersFile:         "collaboratorsWithAliases",
			ownersAliasesFile:  "nonCollaborators",
			ownersAliasesPatch: "nonCollaboratorRemovals",
			shouldLabel:        false,
			shouldComment:      false,
		},
		{
			name:               "collaborators additions in OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS_ALIASES"},
			ownersFile:         "collaboratorsWithAliases",
			ownersAliasesFile:  "nonCollaborators",
			ownersAliasesPatch: "collaboratorAdditions",
			shouldLabel:        false,
			shouldComment:      false,
		},
		{
			name:               "collaborators removals in OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS_ALIASES"},
			ownersFile:         "collaboratorsWithAliases",
			ownersAliasesFile:  "nonCollaborators",
			ownersAliasesPatch: "collaboratorRemovals",
			shouldLabel:        false,
			shouldComment:      false,
		},
		{
			name:                 "non-collaborators additions in OWNERS_ALIASES file, with skipTrustedUserCheck=true",
			filesChanged:         []string{"OWNERS_ALIASES"},
			ownersFile:           "collaboratorsWithAliases",
			ownersAliasesFile:    "nonCollaborators",
			ownersAliasesPatch:   "nonCollaboratorAdditions",
			skipTrustedUserCheck: true,
			shouldLabel:          false,
			shouldComment:        false,
		},
		{
			name:                 "non-collaborators additions in both OWNERS and OWNERS_ALIASES file",
			filesChanged:         []string{"OWNERS", "OWNERS_ALIASES"},
			ownersFile:           "nonCollaboratorsWithAliases",
			ownersPatch:          "nonCollaboratorsWithAliases",
			ownersAliasesFile:    "nonCollaborators",
			ownersAliasesPatch:   "nonCollaboratorAdditions",
			shouldLabel:          true,
			shouldComment:        true,
			commentShouldContain: nonTrustedNotMemberNotCollaborator,
		},
		{
			name:               "collaborator additions in both OWNERS and OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS", "OWNERS_ALIASES"},
			ownersFile:         "collaboratorsWithAliases",
			ownersPatch:        "collaboratorsWithAliases",
			ownersAliasesFile:  "collaborators",
			ownersAliasesPatch: "collaboratorAdditions",
			shouldLabel:        false,
			shouldComment:      false,
		},
		{
			name:          "non-collaborators additions in OWNERS file in vendor subdir",
			filesChanged:  []string{"vendor/k8s.io/client-go/OWNERS"},
			ownersFile:    "nonCollaborators",
			ownersPatch:   "nonCollaboratorAdditions",
			shouldLabel:   false,
			shouldComment: false,
		},
		{
			name:                 "non-collaborators additions in OWNERS file in vendor subdir, but include it",
			filesChanged:         []string{"vendor/k8s.io/client-go/OWNERS"},
			ownersFile:           "nonCollaborators",
			ownersPatch:          "nonCollaboratorAdditions",
			includeVendorOwners:  true,
			shouldLabel:          true,
			shouldComment:        true,
			commentShouldContain: nonTrustedNotMemberNotCollaborator,
		},
		{
			name:                 "non-collaborators additions in OWNERS file in vendor dir",
			filesChanged:         []string{"vendor/OWNERS"},
			ownersFile:           "nonCollaborators",
			ownersPatch:          "nonCollaboratorAdditions",
			shouldLabel:          true,
			shouldComment:        true,
			commentShouldContain: nonTrustedNotMemberNotCollaborator,
		},
	}
	lg, c, err := clients()
	if err != nil {
		t.Fatalf("Making localgit: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Cleaning up localgit: %v", err)
		}
		if err := c.Clean(); err != nil {
			t.Errorf("Cleaning up client: %v", err)
		}
	}()
	if err := lg.MakeFakeRepo("org", "repo"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pr := i + 1
			// make sure we're on master before branching
			if err := lg.Checkout("org", "repo", "master"); err != nil {
				t.Fatalf("Switching to master branch: %v", err)
			}
			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}
			pullFiles := map[string][]byte{}
			changes := []github.PullRequestChange{}

			for _, file := range test.filesChanged {
				if strings.Contains(file, "OWNERS_ALIASES") {
					pullFiles[file] = ownersAliases[test.ownersAliasesFile]
					changes = append(changes, github.PullRequestChange{
						Filename: file,
						Patch:    ownersAliasesPatch[test.ownersAliasesPatch],
					})
				} else if strings.Contains(file, "OWNERS") {
					pullFiles[file] = ownersFiles[test.ownersFile]
					changes = append(changes, github.PullRequestChange{
						Filename: file,
						Patch:    ownersPatch[test.ownersPatch],
					})
				}
			}

			if err := lg.AddCommit("org", "repo", pullFiles); err != nil {
				t.Fatalf("Adding PR commit: %v", err)
			}
			sha, err := lg.RevParse("org", "repo", "HEAD")
			if err != nil {
				t.Fatalf("Getting commit SHA: %v", err)
			}
			pre := &github.PullRequestEvent{
				PullRequest: github.PullRequest{
					User: github.User{Login: "author"},
					Base: github.PullRequestBranch{
						Ref: "master",
					},
					Head: github.PullRequestBranch{
						SHA: sha,
					},
				},
			}
			fghc := newFakeGitHubClient(emptyPatch(test.filesChanged), nil, pr)
			fghc.PullRequestChanges[pr] = changes

			fghc.PullRequests = map[int]*github.PullRequest{}
			fghc.PullRequests[pr] = &github.PullRequest{
				Base: github.PullRequestBranch{
					Ref: fakegithub.TestRef,
				},
			}

			froc := makeFakeRepoOwnersClient()
			if !test.includeVendorOwners {
				var ignorePatterns []*regexp.Regexp
				re, err := regexp.Compile("vendor/.*/.*$")
				if err != nil {
					t.Fatalf("error compiling regex: %v", err)
				}
				ignorePatterns = append(ignorePatterns, re)
				froc.foc.dirIgnorelist = ignorePatterns
			}

			prInfo := info{
				org:          "org",
				repo:         "repo",
				repoFullName: "org/repo",
				number:       pr,
			}

			if err := handle(fghc, c, froc, logrus.WithField("plugin", PluginName), &pre.PullRequest, prInfo, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, test.skipTrustedUserCheck, &fakePruner{}, ownersconfig.FakeResolver); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			if !test.shouldLabel && IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
				t.Errorf("%s: didn't expect label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			}
			if test.shouldLabel && !IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
				t.Errorf("%s: expected label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			}
			if !test.shouldComment && len(fghc.IssueComments[pr]) > 0 {
				t.Errorf("%s: didn't expect comment", test.name)
			}
			if test.shouldComment && len(fghc.IssueComments[pr]) == 0 {
				t.Errorf("%s: expected comment but didn't receive", test.name)
			}
			if test.shouldComment && len(test.commentShouldContain) > 0 && !strings.Contains(fghc.IssueComments[pr][0].Body, test.commentShouldContain) {
				t.Errorf("%s: expected comment to contain\n%s\nbut it was actually\n%s", test.name, test.commentShouldContain, fghc.IssueComments[pr][0].Body)
			}
		})
	}
}

func TestHandleGenericComment(t *testing.T) {
	testHandleGenericComment(localgit.New, t)
}

func TestHandleGenericCommentV2(t *testing.T) {
	testHandleGenericComment(localgit.NewV2, t)
}

func testHandleGenericComment(clients localgit.Clients, t *testing.T) {
	var tests = []struct {
		name         string
		commentEvent github.GenericCommentEvent
		filesChanged []string
		filesRemoved []string
		ownersFile   string
		shouldLabel  bool
	}{
		{
			name: "no OWNERS file",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/verify-owners",
			},
			filesChanged: []string{"a.go", "b.go"},
			ownersFile:   "valid",
			shouldLabel:  false,
		},
		{
			name: "good OWNERS file",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/verify-owners",
			},
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "valid",
			shouldLabel:  false,
		},
		{
			name: "invalid syntax OWNERS file",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/verify-owners",
			},
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntax",
			shouldLabel:  true,
		},
		{
			name: "invalid syntax OWNERS file, unrelated comment",
			commentEvent: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/verify owners",
			},
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntax",
			shouldLabel:  false,
		},
		{
			name: "invalid syntax OWNERS file, comment edited",
			commentEvent: github.GenericCommentEvent{
				Action: github.GenericCommentActionEdited,
				IsPR:   true,
				Body:   "/verify-owners",
			},
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntax",
			shouldLabel:  false,
		},
		{
			name: "invalid syntax OWNERS file, comment on an issue",
			commentEvent: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   false,
				Body:   "/verify-owners",
			},
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntax",
			shouldLabel:  false,
		},
	}

	lg, c, err := clients()
	if err != nil {
		t.Fatalf("Making localgit: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Cleaning up localgit: %v", err)
		}
		if err := c.Clean(); err != nil {
			t.Errorf("Cleaning up client: %v", err)
		}
	}()
	if err := lg.MakeFakeRepo("org", "repo"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pr := i + 1
			// make sure we're on master before branching
			if err := lg.Checkout("org", "repo", "master"); err != nil {
				t.Fatalf("Switching to master branch: %v", err)
			}
			if len(test.filesRemoved) > 0 {
				if err := addFilesToRepo(lg, test.filesRemoved, test.ownersFile); err != nil {
					t.Fatalf("Adding base commit: %v", err)
				}
			}

			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}

			if len(test.filesChanged) > 0 {
				if err := addFilesToRepo(lg, test.filesChanged, test.ownersFile); err != nil {
					t.Fatalf("Adding PR commit: %v", err)
				}
			}
			if len(test.filesRemoved) > 0 {
				if err := lg.RmCommit("org", "repo", test.filesRemoved); err != nil {
					t.Fatalf("Adding PR commit (removing files): %v", err)
				}
			}

			sha, err := lg.RevParse("org", "repo", "HEAD")
			if err != nil {
				t.Fatalf("Getting commit SHA: %v", err)
			}

			test.commentEvent.Repo.Owner.Login = "org"
			test.commentEvent.Repo.Name = "repo"
			test.commentEvent.Repo.FullName = "org/repo"
			test.commentEvent.Number = pr

			fghc := newFakeGitHubClient(emptyPatch(test.filesChanged), test.filesRemoved, pr)
			fghc.PullRequests = map[int]*github.PullRequest{}
			fghc.PullRequests[pr] = &github.PullRequest{
				User: github.User{Login: "author"},
				Head: github.PullRequestBranch{
					SHA: sha,
				},
				Base: github.PullRequestBranch{
					Ref: "master",
				},
			}

			if err := handleGenericComment(fghc, c, makeFakeRepoOwnersClient(), logrus.WithField("plugin", PluginName), &test.commentEvent, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, false, &fakePruner{}, ownersconfig.FakeResolver); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			if !test.shouldLabel && IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
				t.Errorf("%s: didn't expect label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			} else if test.shouldLabel && !IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
				t.Errorf("%s: expected label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			}
		})
	}
}

func testOwnersRemoval(clients localgit.Clients, t *testing.T) {
	var tests = []struct {
		name              string
		ownersRestored    bool
		aliasesRestored   bool
		shouldRemoveLabel bool
	}{
		{
			name:              "OWNERS and OWNERS_ALIASES files restored",
			ownersRestored:    true,
			aliasesRestored:   true,
			shouldRemoveLabel: true,
		},
		{
			name:              "OWNERS file restored, OWNERS_ALIASES left",
			ownersRestored:    true,
			aliasesRestored:   false,
			shouldRemoveLabel: true,
		},
		{
			name:              "OWNERS file left, OWNERS_ALIASES left",
			ownersRestored:    false,
			aliasesRestored:   false,
			shouldRemoveLabel: false,
		},
	}
	lg, c, err := clients()
	if err != nil {
		t.Fatalf("Making localgit: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Cleaning up localgit: %v", err)
		}
		if err := c.Clean(); err != nil {
			t.Errorf("Cleaning up client: %v", err)
		}
	}()
	if err := lg.MakeFakeRepo("org", "repo"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}

	if err := addFilesToRepo(lg, []string{"OWNERS"}, "valid"); err != nil {
		t.Fatalf("Adding base commit: %v", err)
	}

	if err := addFilesToRepo(lg, []string{"OWNERS_ALIASES"}, "collaborators"); err != nil {
		t.Fatalf("Adding base commit: %v", err)
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pr := i + 1
			// make sure we're on master before branching
			if err := lg.Checkout("org", "repo", "master"); err != nil {
				t.Fatalf("Switching to master branch: %v", err)
			}
			pullFiles := map[string][]byte{}
			pullFiles["a.go"] = []byte("foo")

			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}

			if test.ownersRestored == false {
				if err := addFilesToRepo(lg, []string{"OWNERS"}, "invalidSyntax"); err != nil {
					t.Fatalf("Adding OWNERS file: %v", err)
				}
			}

			if test.aliasesRestored == false {
				if err := addFilesToRepo(lg, []string{"OWNERS_ALIASES"}, "toBeAddedAlias"); err != nil {
					t.Fatalf("Adding OWNERS_ALIASES file: %v", err)
				}
			}

			if err := lg.AddCommit("org", "repo", pullFiles); err != nil {
				t.Fatalf("Adding PR commit: %v", err)
			}
			sha, err := lg.RevParse("org", "repo", "HEAD")
			if err != nil {
				t.Fatalf("Getting commit SHA: %v", err)
			}
			pre := &github.PullRequestEvent{
				PullRequest: github.PullRequest{
					User: github.User{Login: "author"},
					Base: github.PullRequestBranch{
						Ref: "master",
					},
					Head: github.PullRequestBranch{
						SHA: sha,
					},
				},
			}
			files := make([]string, 3)
			files = append(files, "a.go")
			if !test.ownersRestored {
				files = append(files, "OWNERS")
			}
			if !test.aliasesRestored {
				files = append(files, "OWNERS_ALIASES")
			}
			fghc := newFakeGitHubClient(emptyPatch(files), nil, pr)

			fghc.PullRequests = map[int]*github.PullRequest{}
			fghc.PullRequests[pr] = &github.PullRequest{
				Base: github.PullRequestBranch{
					Ref: fakegithub.TestRef,
				},
			}

			prInfo := info{
				org:          "org",
				repo:         "repo",
				repoFullName: "org/repo",
				number:       pr,
			}
			fghc.AddLabel(prInfo.org, prInfo.repo, prInfo.number, labels.InvalidOwners)

			froc := makeFakeRepoOwnersClient()

			if err := handle(fghc, c, froc, logrus.WithField("plugin", PluginName), &pre.PullRequest, prInfo, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, false, &fakePruner{}, ownersconfig.FakeResolver); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			if test.shouldRemoveLabel && !IssueLabelsContain(fghc.IssueLabelsRemoved, labels.InvalidOwners) {
				t.Errorf("%s: expected label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsRemoved)
			}
			if !test.shouldRemoveLabel && IssueLabelsContain(fghc.IssueLabelsRemoved, labels.InvalidOwners) {
				t.Errorf("%s: didn't expect label %q in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsRemoved)
			}
		})
	}
}

func TestOwnersRemoval(t *testing.T) {
	testOwnersRemoval(localgit.New, t)
}

func TestOwnersRemovalV2(t *testing.T) {
	testOwnersRemoval(localgit.NewV2, t)
}

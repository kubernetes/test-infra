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

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
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
}

func IssueLabelsAddedContain(arr []string, str string) bool {
	for _, a := range arr {
		// IssueLabelsAdded format is owner/repo#number:label
		b := strings.Split(a, ":")
		if b[len(b)-1] == str {
			return true
		}
	}
	return false
}

func newFakeGitHubClient(files []string, pr int) *fakegithub.FakeClient {
	var changes []github.PullRequestChange
	for _, file := range files {
		changes = append(changes, github.PullRequestChange{Filename: file})
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
	approvers         map[string]sets.String
	leafApprovers     map[string]sets.String
	reviewers         map[string]sets.String
	requiredReviewers map[string]sets.String
	leafReviewers     map[string]sets.String
	dirBlacklist      []*regexp.Regexp
}

func (foc *fakeOwnersClient) Approvers(path string) sets.String {
	return foc.approvers[path]
}

func (foc *fakeOwnersClient) LeafApprovers(path string) sets.String {
	return foc.leafApprovers[path]
}

func (foc *fakeOwnersClient) FindApproverOwnersForFile(path string) string {
	return foc.owners[path]
}

func (foc *fakeOwnersClient) Reviewers(path string) sets.String {
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
	for _, re := range foc.dirBlacklist {
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
	for _, re := range foc.dirBlacklist {
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

func makeFakeRepoOwnersClient() fakeRepoownersClient {
	return fakeRepoownersClient{
		foc: &fakeOwnersClient{},
	}
}

func TestHandle(t *testing.T) {
	var tests = []struct {
		name         string
		filesChanged []string
		ownersFile   string
		shouldLabel  bool
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
	}
	lg, c, err := localgit.New()
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
		pr := i + 1
		// make sure we're on master before branching
		if err := lg.Checkout("org", "repo", "master"); err != nil {
			t.Fatalf("Switching to master branch: %v", err)
		}
		if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
			t.Fatalf("Checking out pull branch: %v", err)
		}
		pullFiles := map[string][]byte{}
		for _, file := range test.filesChanged {
			if strings.Contains(file, "OWNERS") {
				pullFiles[file] = ownerFiles[test.ownersFile]
			} else {
				pullFiles[file] = []byte("foo")
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
			Number: pr,
			PullRequest: github.PullRequest{
				User: github.User{Login: "author"},
				Head: github.PullRequestBranch{
					SHA: sha,
				},
			},
			Repo: github.Repo{FullName: "org/repo"},
		}
		fghc := newFakeGitHubClient(test.filesChanged, pr)
		fghc.PullRequests = map[int]*github.PullRequest{}
		fghc.PullRequests[pr] = &github.PullRequest{
			Base: github.PullRequestBranch{
				Ref: fakegithub.TestRef,
			},
		}

		if err := handle(fghc, c, makeFakeRepoOwnersClient(), logrus.WithField("plugin", PluginName), pre, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, false, &fakePruner{}); err != nil {
			t.Fatalf("Handle PR: %v", err)
		}
		if !test.shouldLabel && IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
			t.Errorf("%s: didn't expect label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			continue
		} else if test.shouldLabel && !IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
			t.Errorf("%s: expected label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			continue
		}
	}
}

func TestParseOwnersFile(t *testing.T) {
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
			lg, c, err := localgit.New()
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

			r, err := c.Clone("org/repo")
			if err != nil {
				t.Fatalf("error cloning the repo: %v", err)
			}
			defer func() {
				if err := r.Clean(); err != nil {
					t.Fatalf("error cleaning up repo: %v", err)
				}
			}()

			path := filepath.Join(r.Dir, "OWNERS")
			message, _ := parseOwnersFile(&fakeOwnersClient{}, path, change, &logrus.Entry{}, []string{})
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
	cases := []struct {
		name               string
		config             *plugins.Configuration
		enabledRepos       []string
		err                bool
		configInfoIncludes []string
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo"},
		},
		{
			name:         "Overlapping org and org/repo",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org2", "org2/repo"},
		},
		{
			name:         "Invalid enabledRepos",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo/extra"},
			err:          true,
		},
		{
			name: "ReviewerCount specified",
			config: &plugins.Configuration{
				Owners: plugins.Owners{
					LabelsBlackList: []string{"label1", "label2"},
				},
			},
			enabledRepos:       []string{"org1", "org2/repo"},
			configInfoIncludes: []string{"label1, label2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pluginHelp, err := helpProvider(c.config, c.enabledRepos)
			if err != nil && !c.err {
				t.Fatalf("helpProvider error: %v", err)
			}
			for _, msg := range c.configInfoIncludes {
				if !strings.Contains(pluginHelp.Config[""], msg) {
					t.Fatalf("helpProvider.Config error mismatch: didn't get %v, but wanted it", msg)
				}
			}
		})
	}
}

var ownersFiles = map[string][]byte{
	"nonCollaboratorAdditions": []byte(`reviewers:
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
	"nonCollaboratorAdditions": `@@ -1,4 +1,6 @@
reviewers:
+- phippy
- alice
approvers:
+- zee
- bob
`,
	"nonCollaboratorsWithAliases": `@@ -1,4 +1,6 @@
reviewers:
+- foo-reviewers
- alice
+- goldie
approvers:
- bob
`,
	"collaboratorsWtihAliases": `@@ -1,2 +1,5 @@
+reviewers:
+- foo-reviewers
+- alice
approvers:
- bob
`,
}

var ownersAliases = map[string][]byte{
	"nonCollaboratorAdditions": []byte(`aliases:
  foo-reviewers:
  - alice
  - phippy
  - zee
`),
	"collaboratorAdditions": []byte(`aliases:
  foo-reviewers:
  - alice
`),
}

var ownersAliasesPatch = map[string]string{
	"nonCollaboratorAdditions": `@@ -1,3 +1,5 @@
aliases:
  foo-reviewers:
  - alice
+ - phippy
+ - zee
`,
	"collaboratorAdditions": `@@ -0,0 +1,3 @@
+aliases:
+ foo-reviewers:
+ - alice
`,
}

func TestNonCollaborators(t *testing.T) {
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
	}{
		{
			name:          "non-collaborators additions in OWNERS file",
			filesChanged:  []string{"OWNERS"},
			ownersFile:    "nonCollaboratorAdditions",
			ownersPatch:   "nonCollaboratorAdditions",
			shouldLabel:   true,
			shouldComment: true,
		},
		{
			name:                 "non-collaborators additions in OWNERS file, with skipTrustedUserCheck=true",
			filesChanged:         []string{"OWNERS"},
			ownersFile:           "nonCollaboratorAdditions",
			ownersPatch:          "nonCollaboratorAdditions",
			skipTrustedUserCheck: true,
			shouldLabel:          false,
			shouldComment:        false,
		},
		{
			name:               "non-collaborators additions in OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS_ALIASES"},
			ownersFile:         "collaboratorsWithAliases",
			ownersAliasesFile:  "nonCollaboratorAdditions",
			ownersAliasesPatch: "nonCollaboratorAdditions",
			shouldLabel:        true,
			shouldComment:      true,
		},
		{
			name:                 "non-collaborators additions in OWNERS_ALIASES file, with skipTrustedUserCheck=true",
			filesChanged:         []string{"OWNERS_ALIASES"},
			ownersFile:           "collaboratorsWithAliases",
			ownersAliasesFile:    "nonCollaboratorAdditions",
			ownersAliasesPatch:   "nonCollaboratorAdditions",
			skipTrustedUserCheck: true,
			shouldLabel:          false,
			shouldComment:        false,
		},
		{
			name:               "non-collaborators additions in both OWNERS and OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS", "OWNERS_ALIASES"},
			ownersFile:         "nonCollaboratorsWithAliases",
			ownersPatch:        "nonCollaboratorsWithAliases",
			ownersAliasesFile:  "nonCollaboratorAdditions",
			ownersAliasesPatch: "nonCollaboratorAdditions",
			shouldLabel:        true,
			shouldComment:      true,
		},
		{
			name:               "collaborator additions in both OWNERS and OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS", "OWNERS_ALIASES"},
			ownersFile:         "collaboratorsWithAliases",
			ownersPatch:        "collaboratorsWtihAliases",
			ownersAliasesFile:  "collaboratorAdditions",
			ownersAliasesPatch: "collaboratorAdditions",
			shouldLabel:        false,
			shouldComment:      false,
		},
		{
			name:          "non-collaborators additions in OWNERS file in vendor subdir",
			filesChanged:  []string{"vendor/k8s.io/client-go/OWNERS"},
			ownersFile:    "nonCollaboratorAdditions",
			ownersPatch:   "nonCollaboratorAdditions",
			shouldLabel:   false,
			shouldComment: false,
		},
		{
			name:                "non-collaborators additions in OWNERS file in vendor subdir, but include it",
			filesChanged:        []string{"vendor/k8s.io/client-go/OWNERS"},
			ownersFile:          "nonCollaboratorAdditions",
			ownersPatch:         "nonCollaboratorAdditions",
			includeVendorOwners: true,
			shouldLabel:         true,
			shouldComment:       true,
		},
		{
			name:          "non-collaborators additions in OWNERS file in vendor dir",
			filesChanged:  []string{"vendor/OWNERS"},
			ownersFile:    "nonCollaboratorAdditions",
			ownersPatch:   "nonCollaboratorAdditions",
			shouldLabel:   true,
			shouldComment: true,
		},
	}
	lg, c, err := localgit.New()
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
			Number: pr,
			PullRequest: github.PullRequest{
				User: github.User{Login: "author"},
				Head: github.PullRequestBranch{
					SHA: sha,
				},
			},
			Repo: github.Repo{FullName: "org/repo"},
		}
		fghc := newFakeGitHubClient(test.filesChanged, pr)
		fghc.PullRequestChanges[pr] = changes

		fghc.PullRequests = map[int]*github.PullRequest{}
		fghc.PullRequests[pr] = &github.PullRequest{
			Base: github.PullRequestBranch{
				Ref: fakegithub.TestRef,
			},
		}

		froc := makeFakeRepoOwnersClient()
		if !test.includeVendorOwners {
			var blacklist []*regexp.Regexp
			re, err := regexp.Compile("vendor/.*/.*$")
			if err != nil {
				t.Fatalf("error compiling regex: %v", err)
			}
			blacklist = append(blacklist, re)
			froc.foc.dirBlacklist = blacklist
		}

		if err := handle(fghc, c, froc, logrus.WithField("plugin", PluginName), pre, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, test.skipTrustedUserCheck, &fakePruner{}); err != nil {
			t.Fatalf("Handle PR: %v", err)
		}
		if !test.shouldLabel && IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
			t.Errorf("%s: didn't expect label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
		}
		if test.shouldLabel && !IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
			t.Errorf("%s: expected label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
		}
		if !test.shouldComment && len(fghc.IssueComments[pr]) > 0 {
			t.Errorf("%s: didn't expect comment", test.name)
		}
		if test.shouldComment && len(fghc.IssueComments[pr]) == 0 {
			t.Errorf("%s: expected comment but didn't receive", test.name)
		}
	}
}

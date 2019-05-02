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
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
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
		if err := handle(fghc, c, logrus.WithField("plugin", PluginName), pre, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, &fakePruner{}); err != nil {
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
	cases := []struct {
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
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.patch == "" {
				c.patch = makePatch(c.document)
			}
			change := github.PullRequestChange{
				Filename: "OWNERS",
				Patch:    c.patch,
			}
			message, _ := parseOwnersFile(c.document, change, &logrus.Entry{}, []string{})
			if message != nil {
				if c.errLine == 0 {
					t.Errorf("%s: expected no error, got one: %s", c.name, message.message)
				}
				if message.line != c.errLine {
					t.Errorf("%s: wrong line for message, expected %d, got %d", c.name, c.errLine, message.line)
				}
			} else if c.errLine != 0 {
				t.Errorf("%s: expected an error, got none", c.name)
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
		name               string
		filesChanged       []string
		ownersFile         string
		ownersPatch        string
		ownersAliasesFile  string
		ownersAliasesPatch string
		shouldLabel        bool
		shouldComment      bool
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
			name:               "non-collaborators additions in OWNERS_ALIASES file",
			filesChanged:       []string{"OWNERS_ALIASES"},
			ownersFile:         "collaboratorsWithAliases",
			ownersAliasesFile:  "nonCollaboratorAdditions",
			ownersAliasesPatch: "nonCollaboratorAdditions",
			shouldLabel:        true,
			shouldComment:      true,
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
			if file == "OWNERS" {
				pullFiles[file] = ownersFiles[test.ownersFile]
				changes = append(changes, github.PullRequestChange{
					Filename: "OWNERS",
					Patch:    ownersPatch[test.ownersPatch],
				})
			}

			if file == "OWNERS_ALIASES" {
				pullFiles[file] = ownersAliases[test.ownersAliasesFile]
				changes = append(changes, github.PullRequestChange{
					Filename: "OWNERS_ALIASES",
					Patch:    ownersAliasesPatch[test.ownersAliasesPatch],
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

		if err := handle(fghc, c, logrus.WithField("plugin", PluginName), pre, []string{labels.Approved, labels.LGTM}, plugins.Trigger{}, &fakePruner{}); err != nil {
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

/*
Copyright 2020 The Kubernetes Authors.

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

package rewardowners

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/plugins/ownersconfig"

	"k8s.io/test-infra/prow/github/fakegithub"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

var patches = map[string]string{
	"addAlias": `
+ - alias2
`,
	"addUser": `
+ - bob
`,
	"removeAlias": `
- - alias2
`,
	"removeUser": `
- - bob
`,
}

var ownerAliasesFile = []byte(`aliases:
  alias2:
  - alice
`)

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

func ownersFilePatch(files []string, p []string) map[string]string {
	changes := emptyPatch(files)
	for i, file := range files {
		if i < len(p) {
			changes[file] = patches[p[i]]
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

func newFakeGitHubClient(changed map[string]string, pr int) *fakegithub.FakeClient {
	var changes []github.PullRequestChange
	for file, patch := range changed {
		changes = append(changes, github.PullRequestChange{Filename: file, Patch: patch})
	}
	return &fakegithub.FakeClient{
		PullRequestChanges: map[int][]github.PullRequestChange{pr: changes},
		Reviews:            map[int][]github.Review{},
		Collaborators:      []string{"alice", "bob", "jdoe"},
		IssueComments:      map[int][]github.IssueComment{},
	}
}

func addFilesToRepo(lg *localgit.LocalGit, paths []string) error {
	origFiles := map[string][]byte{}
	for _, file := range paths {
		origFiles[file] = []byte("foo")
	}
	origFiles["OWNERS_ALIASES"] = ownerAliasesFile
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
		name         string
		filesChanged []string
		patches      []string
		shouldLabel  bool
	}{
		{
			name:         "no OWNERS file",
			filesChanged: []string{"a.go", "b.go"},
			shouldLabel:  false,
		},
		{
			name:         "add user to OWNERS",
			filesChanged: []string{"OWNERS", "b.go"},
			patches:      []string{"addUser"},
			shouldLabel:  true,
		},
		{
			name:         "add alias to OWNERS",
			filesChanged: []string{"OWNERS", "b.go"},
			patches:      []string{"addAlias"},
			shouldLabel:  false,
		},
		{
			name:         "add user to OWNERS_ALIASES",
			filesChanged: []string{"OWNERS_ALIASES", "b.go"},
			patches:      []string{"addUser"},
			shouldLabel:  true,
		},
		{
			name:         "remove user from OWNERS",
			filesChanged: []string{"OWNERS", "b.go"},
			patches:      []string{"removeUser"},
			shouldLabel:  false,
		},
		{
			name:         "remove user from OWNERS_ALIASES",
			filesChanged: []string{"OWNERS_ALIASES", "b.go"},
			patches:      []string{"removeUser"},
			shouldLabel:  false,
		},
		{
			name:         "move user from OWNERS_ALIASES to OWNERS",
			filesChanged: []string{"OWNERS", "OWNERS_ALIASES"},
			patches:      []string{"addUser", "removeUser"},
			shouldLabel:  false,
		},
		{
			name:         "move user from OWNERS to OWNERS_ALIASES",
			filesChanged: []string{"OWNERS", "OWNERS_ALIASES"},
			patches:      []string{"removeUser", "addUser"},
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

			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}

			if len(test.filesChanged) > 0 {
				if err := addFilesToRepo(lg, test.filesChanged); err != nil {
					t.Fatalf("Adding PR commit: %v", err)
				}
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
			changes := ownersFilePatch(test.filesChanged, test.patches)
			fghc := newFakeGitHubClient(changes, pr)
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

			if err := handle(fghc, c, logrus.WithField("plugin", PluginName), &pre.PullRequest, prInfo, plugins.Trigger{}, false, ownersconfig.FakeResolver); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			if !test.shouldLabel && IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.Welcome) {
				t.Fatalf("%s: didn't expect label %s in %s", test.name, labels.Welcome, fghc.IssueLabelsAdded)
			} else if test.shouldLabel && !IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.Welcome) {
				t.Fatalf("%s: expected label %s in %s", test.name, labels.Welcome, fghc.IssueLabelsAdded)
			}
		})
	}
}

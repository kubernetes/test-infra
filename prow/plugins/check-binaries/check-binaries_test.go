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

package checkbinaries

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var defaultBranch = localgit.DefaultBranch("")

var patches = map[string]string{
	"referencesToBeAdded": `
+ - not-yet-existing
`,
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

func filePatch(files []string, useEmptyPatch bool) map[string]string {
	changes := emptyPatch(files)
	if useEmptyPatch {
		return changes
	}
	for _, file := range files {
		changes[file] = patches["referencesToBeAdded"]

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
	fgc := fakegithub.NewFakeClient()
	fgc.PullRequestChanges = map[int][]github.PullRequestChange{pr: changes}
	fgc.Reviews = map[int][]github.Review{}
	fgc.Collaborators = []string{"alice", "bob", "jdoe"}
	fgc.IssueComments = map[int][]github.IssueComment{}
	return fgc
}

type fakePruner struct{}

func (fp *fakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}

func addFilesToRepo(lg *localgit.LocalGit, paths []string) error {
	origFiles := map[string][]byte{}
	for _, file := range paths {
		origFiles[file] = []byte("foo")
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
		filesChangedAfterPR []string
		addedContent        string
		shouldLabel         bool
		reString            string
	}{
		{
			name:         "debian/source/include-binaries file",
			filesChanged: []string{"debian/source/include-binaries", "b.go"},
			shouldLabel:  true,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
		},
		{
			name:         "no invalid binaries file",
			filesChanged: []string{"a.go", "b.go"},
			shouldLabel:  false,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
		},
		{
			name:         ".so file",
			filesChanged: []string{"a.so", "b.go"},
			shouldLabel:  true,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
		},
		{
			name:         ".so.1 file",
			filesChanged: []string{"a.so.1", "b.go"},
			shouldLabel:  true,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
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
			if err := lg.Checkout("org", "repo", defaultBranch); err != nil {
				t.Fatalf("Switching to master branch: %v", err)
			}
			if len(test.filesRemoved) > 0 {
				if err := addFilesToRepo(lg, test.filesRemoved); err != nil {
					t.Fatalf("Adding base commit: %v", err)
				}
			}

			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}

			if len(test.filesChanged) > 0 {
				if err := addFilesToRepo(lg, test.filesChanged); err != nil {
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
				if err := lg.Checkout("org", "repo", defaultBranch); err != nil {
					t.Fatalf("Switching to master branch: %v", err)
				}
				if err := addFilesToRepo(lg, test.filesChangedAfterPR); err != nil {
					t.Fatalf("Adding commit to master: %v", err)
				}
			}
			pre := &github.PullRequestEvent{
				PullRequest: github.PullRequest{
					User: github.User{Login: "author"},
					Base: github.PullRequestBranch{
						Ref: defaultBranch,
					},
					Head: github.PullRequestBranch{
						SHA: sha,
					},
				},
			}
			changes := filePatch(test.filesChanged, !test.usePatch)
			fghc := newFakeGitHubClient(changes, test.filesRemoved, pr)
			fghc.PullRequests = map[int]*github.PullRequest{}
			fghc.PullRequests[pr] = &github.PullRequest{
				Base: github.PullRequestBranch{
					Ref: defaultBranch,
				},
			}

			prInfo := info{
				org:          "org",
				repo:         "repo",
				repoFullName: "org/repo",
				number:       pr,
			}

			if err := handle(fghc, logrus.WithField("plugin", PluginName), &pre.PullRequest, prInfo, &fakePruner{}, test.reString); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			if !test.shouldLabel && IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidBinaries) {
				t.Fatalf("%s: didn't expect label %s in %s", test.name, labels.InvalidBinaries, fghc.IssueLabelsAdded)
			} else if test.shouldLabel && !IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidBinaries) {
				t.Fatalf("%s: expected label %s in %s", test.name, labels.InvalidBinaries, fghc.IssueLabelsAdded)
			}
		})
	}
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
				Description: "The check binaries plugin check source whether include binaries.",
				Config: map[string]string{
					"org1/repo": `The check-binaries plugin configured to check PRs binaries with re: debian/source/include-binaries|.*\.so(\.\d+)?`,
					"org2/repo": `The check-binaries plugin configured to check PRs binaries with re: debian/source/include-binaries|.*\.so(\.\d+)?`,
				},
				Commands: []pluginhelp.Command{{
					Usage:       "/check-binaries",
					Description: "do-not-merge/source-include-binaries",
					Examples:    []string{"/check-binaries"},
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
		shouldLabel  bool
		reString     string
	}{
		{
			name: "no invalid binaries file",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/check-binaries",
			},
			filesChanged: []string{"a.go", "b.go"},
			shouldLabel:  false,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
		},
		{
			name: "debian/source/include-binaries",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/check-binaries",
			},
			filesChanged: []string{"debian/source/include-binaries", "b.go"},
			shouldLabel:  true,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
		},
		{
			name: ".so file",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/check-binaries",
			},
			filesChanged: []string{"a.so", "b.go"},
			shouldLabel:  true,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
		},
		{
			name: ".so.1 file",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/check-binaries",
			},
			filesChanged: []string{"a.so.1", "b.go"},
			shouldLabel:  true,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
		},
		{
			name: ".so.1 file",
			commentEvent: github.GenericCommentEvent{
				Action:     github.GenericCommentActionCreated,
				IssueState: "open",
				IsPR:       true,
				Body:       "/check-binaries",
			},
			filesRemoved: []string{"a.so.1", "b.go"},
			shouldLabel:  false,
			reString:     `debian/source/include-binaries|.*\.so(\.\d+)?`,
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
			if err := lg.Checkout("org", "repo", defaultBranch); err != nil {
				t.Fatalf("Switching to master branch: %v", err)
			}
			if len(test.filesRemoved) > 0 {
				if err := addFilesToRepo(lg, test.filesRemoved); err != nil {
					t.Fatalf("Adding base commit: %v", err)
				}
			}

			if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
				t.Fatalf("Checking out pull branch: %v", err)
			}

			if len(test.filesChanged) > 0 {
				if err := addFilesToRepo(lg, test.filesChanged); err != nil {
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
					Ref: defaultBranch,
				},
			}

			if err := handleGenericComment(fghc, logrus.WithField("plugin", PluginName), &test.commentEvent, &fakePruner{}, test.reString); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			if !test.shouldLabel && IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidBinaries) {
				t.Errorf("%s: didn't expect label %s in %s", test.name, labels.InvalidBinaries, fghc.IssueLabelsAdded)
			} else if test.shouldLabel && !IssueLabelsContain(fghc.IssueLabelsAdded, labels.InvalidBinaries) {
				t.Errorf("%s: expected label %s in %s", test.name, labels.InvalidBinaries, fghc.IssueLabelsAdded)
			}
		})
	}
}

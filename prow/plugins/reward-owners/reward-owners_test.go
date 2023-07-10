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
	"testing"

	"k8s.io/test-infra/prow/github/fakegithub"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pkg/layeredsets"

	"k8s.io/test-infra/prow/plugins/ownersconfig"
	"k8s.io/test-infra/prow/repoowners"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
)

type fakeOwnersClient struct{}

func (f *fakeOwnersClient) LoadRepoOwnersSha(org, repo, base, sha string, updateCache bool) (repoowners.RepoOwner, error) {
	return &fakeRepoOwners{sha: sha}, nil
}

type fakeRepoOwners struct {
	sha string
}

func (f *fakeRepoOwners) Filenames() ownersconfig.Filenames               { return ownersconfig.Filenames{} }
func (f *fakeRepoOwners) FindApproverOwnersForFile(path string) string    { return "" }
func (f *fakeRepoOwners) FindReviewersOwnersForFile(path string) string   { return "" }
func (f *fakeRepoOwners) FindLabelsForFile(path string) sets.Set[string]  { return nil }
func (f *fakeRepoOwners) IsNoParentOwners(path string) bool               { return false }
func (f *fakeRepoOwners) IsAutoApproveUnownedSubfolders(path string) bool { return false }
func (f *fakeRepoOwners) LeafApprovers(path string) sets.Set[string]      { return nil }
func (f *fakeRepoOwners) Approvers(path string) layeredsets.String        { return layeredsets.String{} }
func (f *fakeRepoOwners) LeafReviewers(path string) sets.Set[string]      { return nil }
func (f *fakeRepoOwners) ParseSimpleConfig(path string) (repoowners.SimpleConfig, error) {
	return repoowners.SimpleConfig{}, nil
}
func (f *fakeRepoOwners) ParseFullConfig(path string) (repoowners.FullConfig, error) {
	return repoowners.FullConfig{}, nil
}
func (f *fakeRepoOwners) Reviewers(path string) layeredsets.String       { return layeredsets.String{} }
func (f *fakeRepoOwners) RequiredReviewers(path string) sets.Set[string] { return nil }
func (f *fakeRepoOwners) TopLevelApprovers() sets.Set[string]            { return nil }

func (f *fakeRepoOwners) AllApprovers() sets.Set[string] {
	return ownersBySha[f.sha]
}

func (f *fakeRepoOwners) AllOwners() sets.Set[string] {
	return ownersBySha[f.sha]
}

func (f *fakeRepoOwners) AllReviewers() sets.Set[string] {
	return ownersBySha[f.sha]
}

var ownersBySha = map[string]sets.Set[string]{
	"base":         sets.New[string]("alice", "bob"),
	"add cole":     sets.New[string]("alice", "bob", "cole"),
	"remove alice": sets.New[string]("bob"),
}

func makeChanges(files []string) map[string]string {
	changes := make(map[string]string, len(files))
	for _, f := range files {
		changes[f] = "foo"
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

func TestHandle(t *testing.T) {
	testHandle(localgit.New, t)
}

func TestHandleV2(t *testing.T) {
	testHandle(localgit.NewV2, t)
}

func testHandle(clients localgit.Clients, t *testing.T) {
	var tests = []struct {
		name          string
		filesChanged  []string
		sha           string
		expectComment bool
	}{
		{
			name:          "no OWNERS file",
			filesChanged:  []string{"a.go", "b.go"},
			expectComment: false,
		},
		{
			name:          "add user to OWNERS",
			filesChanged:  []string{"OWNERS", "b.go"},
			sha:           "add cole",
			expectComment: true,
		},
		{
			name:          "add user to OWNERS_ALIASES",
			filesChanged:  []string{"OWNERS_ALIASES", "b.go"},
			sha:           "add cole",
			expectComment: true,
		},
		{
			name:          "remove user from OWNERS",
			filesChanged:  []string{"OWNERS", "b.go"},
			sha:           "remove alice",
			expectComment: false,
		},
		{
			name:          "remove user from OWNERS_ALIASES",
			filesChanged:  []string{"OWNERS_ALIASES", "b.go"},
			sha:           "remove alice",
			expectComment: false,
		},
		{
			name:          "move user from OWNERS_ALIASES to OWNERS",
			filesChanged:  []string{"OWNERS", "OWNERS_ALIASES"},
			sha:           "head",
			expectComment: false,
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

			changes := makeChanges(test.filesChanged)
			fghc := newFakeGitHubClient(changes, pr)
			oc := &fakeOwnersClient{}

			prInfo := info{
				base: github.PullRequestBranch{
					Ref: "master",
					SHA: "base",
				},
				head: github.PullRequestBranch{
					Ref: "master",
					SHA: test.sha,
				},
				number:       pr,
				org:          "org",
				repo:         "repo",
				repoFullName: "org/repo",
			}

			if err := handle(fghc, oc, logrus.WithField("plugin", PluginName), prInfo, ownersconfig.FakeResolver); err != nil {
				t.Fatalf("Handle PR: %v", err)
			}
			numComments := len(fghc.IssueCommentsAdded)
			if numComments > 1 {
				t.Fatalf("did not expect multiple comments for any test case and got %d comments", numComments)
			}
			if numComments == 0 && test.expectComment {
				t.Fatalf("expected a comment for case '%s' and got none", test.name)
			} else if numComments > 0 && !test.expectComment {
				t.Fatalf("did not expect comments for case '%s' and got %d comments", test.name, numComments)
			}
		})
	}
}

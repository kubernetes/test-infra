/*
Copyright 2019 The Kubernetes Authors.

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

package mergecommitblocker

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/commentpruner"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
)

type strSet map[string]struct{}

type fakeGHClient struct {
	labels   strSet
	comments map[int]string
}

func (f *fakeGHClient) AddLabel(org, repo string, number int, label string) error {
	f.labels[label] = struct{}{}
	return nil
}

func (f *fakeGHClient) RemoveLabel(org, repo string, number int, label string) error {
	delete(f.labels, label)
	return nil
}

func (f *fakeGHClient) GetIssueLabels(org, repo string, number int) ([]github.Label, error) {
	var labels []github.Label
	for l := range f.labels {
		labels = append(labels, github.Label{Name: l})
	}
	return labels, nil
}

func (f *fakeGHClient) CreateComment(org, repo string, number int, comment string) error {
	if _, ok := f.comments[number]; ok {
		return fmt.Errorf("comment id %d already exists", number)
	}
	f.comments[number] = comment
	return nil
}

func (f *fakeGHClient) DeleteComment(org, repo string, id int) error {
	delete(f.comments, id)
	return nil
}

func (f *fakeGHClient) BotUserChecker() (func(candidate string) bool, error) {
	return func(candidate string) bool {
		return candidate == "foo"
	}, nil
}

func (f *fakeGHClient) ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) {
	var ghComments []github.IssueComment
	for id, c := range f.comments {
		ghComment := github.IssueComment{
			ID:   id,
			Body: c,
			User: github.User{Login: "foo"},
		}
		ghComments = append(ghComments, ghComment)
	}
	return ghComments, nil
}

func TestHandle(t *testing.T) {
	testHandle(localgit.New, t)
}

func TestHandleV2(t *testing.T) {
	testHandle(localgit.NewV2, t)
}

func testHandle(clients localgit.Clients, t *testing.T) {
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
	if err := lg.MakeFakeRepo("foo", "bar"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	var (
		checkoutPR = func(prNum int) {
			if err := lg.CheckoutNewBranch("foo", "bar", fmt.Sprintf("pull/%d/head", prNum)); err != nil {
				t.Fatalf("Creating & checking out pull branch pull/%d/head: %v", prNum, err)
			}
		}
		checkoutBranch = func(branch string) {
			if err := lg.Checkout("foo", "bar", branch); err != nil {
				t.Fatalf("Checking out branch %s: %v", branch, err)
			}
		}
		addCommit = func(file string) {
			if err := lg.AddCommit("foo", "bar", map[string][]byte{file: {}}); err != nil {
				t.Fatalf("Adding commit: %v", err)
			}
		}
		mergeMaster = func() {
			if _, err := lg.Merge("foo", "bar", "master"); err != nil {
				t.Fatalf("Rebasing commit: %v", err)
			}
		}
		rebaseMaster = func() {
			if _, err := lg.Rebase("foo", "bar", "master"); err != nil {
				t.Fatalf("Rebasing commit: %v", err)
			}
		}
	)

	type testCase struct {
		name          string
		fakeGHClient  *fakeGHClient
		prNum         int
		checkout      func()
		mergeOrRebase func()
		wantLabel     bool
		wantComment   bool
	}
	testcases := []testCase{
		{
			name: "merge commit label not exist, PR has merge commits",
			fakeGHClient: &fakeGHClient{
				labels:   strSet{},
				comments: make(map[int]string),
			},
			prNum:         11,
			checkout:      func() { checkoutBranch("pull/11/head") },
			mergeOrRebase: mergeMaster,
			wantLabel:     true,
			wantComment:   true,
		},
		{
			name: "merge commit label exists, PR has merge commits",
			fakeGHClient: &fakeGHClient{
				labels:   strSet{labels.MergeCommits: struct{}{}},
				comments: map[int]string{12: commentBody},
			},
			prNum:         12,
			checkout:      func() { checkoutBranch("pull/12/head") },
			mergeOrRebase: mergeMaster,
			wantLabel:     true,
			wantComment:   true,
		},
		{
			name: "merge commit label not exists, PR doesn't have merge commits",
			fakeGHClient: &fakeGHClient{
				labels:   strSet{},
				comments: make(map[int]string),
			},
			prNum:         13,
			checkout:      func() { checkoutBranch("pull/13/head") },
			mergeOrRebase: rebaseMaster,
			wantLabel:     false,
			wantComment:   false,
		},
		{
			name: "merge commit label exists, PR doesn't have merge commits",
			fakeGHClient: &fakeGHClient{
				labels:   strSet{labels.MergeCommits: struct{}{}},
				comments: map[int]string{14: commentBody},
			},
			prNum:         14,
			checkout:      func() { checkoutBranch("pull/14/head") },
			mergeOrRebase: rebaseMaster,
			wantLabel:     false,
			wantComment:   false,
		},
	}

	addCommit("wow")
	// preparation work: branch off all prs upon commit 'wow'
	for _, tt := range testcases {
		checkoutPR(tt.prNum)
	}
	// switch back to master and create a new commit 'ouch'
	checkoutBranch("master")
	addCommit("ouch")
	masterSHA, err := lg.RevParse("foo", "bar", "HEAD")
	if err != nil {
		t.Fatalf("Fetching SHA: %v", err)
	}

	for _, tt := range testcases {
		tt.checkout()
		tt.mergeOrRebase()
		prSHA, err := lg.RevParse("foo", "bar", "HEAD")
		if err != nil {
			t.Fatalf("Fetching SHA: %v", err)
		}
		pre := &github.PullRequestEvent{
			Action: github.PullRequestActionOpened,
			PullRequest: github.PullRequest{
				Number: tt.prNum,
				Base: github.PullRequestBranch{
					Repo: github.Repo{
						Owner: github.User{Login: "foo"},
						Name:  "bar",
					},
					SHA: masterSHA,
				},
				Head: github.PullRequestBranch{
					Repo: github.Repo{
						Owner: github.User{Login: "foo"},
						Name:  "bar",
					},
					SHA: prSHA,
				},
			},
		}
		log := logrus.NewEntry(logrus.New())
		fakePruner := commentpruner.NewEventClient(tt.fakeGHClient, log, "foo", "bar", tt.prNum)
		if err := handle(tt.fakeGHClient, c, fakePruner, log, pre); err != nil {
			t.Errorf("Expect err is nil, but got %v", err)
		}
		// verify if MergeCommits label as expected
		if _, got := tt.fakeGHClient.labels[labels.MergeCommits]; got != tt.wantLabel {
			t.Errorf("Case: %v. Expect MergeCommits=%v, but got %v", tt.name, tt.wantLabel, got)
		}
		// verify if github comment is created/pruned as expected
		if _, got := tt.fakeGHClient.comments[tt.prNum]; got != tt.wantComment {
			t.Errorf("Case: %v. Expect wantComment=%v, but got %v", tt.name, tt.wantComment, got)
		}
	}
}

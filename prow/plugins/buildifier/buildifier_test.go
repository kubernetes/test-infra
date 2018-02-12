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

package buildifier

import (
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
)

var initialFiles = map[string][]byte{
	"BUILD":     []byte(`package(default_visibility = ["//visibility:public"])`),
	"WORKSPACE": []byte(`workspace(name = "io_foo_bar")`),
}

var pullFiles = map[string][]byte{
	"BUILD": []byte(`package(default_visibility = ["//visibility:public"])


docker_build(
    name = "blah",
)
`),
	"WORKSPACE": []byte(`workspace(name = "io_foo_bar")

foo_repositories()
`),
	"blah.bzl": []byte(`def foo():
  print("bar")
`),
}

type ghc struct {
	genfile     []byte
	pr          github.PullRequest
	changes     []github.PullRequestChange
	oldComments []github.ReviewComment
	comment     github.DraftReview
}

func (g *ghc) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	return g.changes, nil
}

func (g *ghc) CreateReview(org, repo string, number int, r github.DraftReview) error {
	g.comment = r
	return nil
}

func (g *ghc) ListPullRequestComments(org, repo string, number int) ([]github.ReviewComment, error) {
	return g.oldComments, nil
}

func (g *ghc) GetFile(org, repo, filepath, commit string) ([]byte, error) {
	return g.genfile, nil
}

func (g *ghc) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	return &g.pr, nil
}

var e = &github.GenericCommentEvent{
	Action:     github.GenericCommentActionCreated,
	IssueState: "open",
	Body:       "/buildify",
	User:       github.User{Login: "mattmoor"},
	Number:     42,
	IsPR:       true,
	Repo: github.Repo{
		Owner:    github.User{Login: "foo"},
		Name:     "bar",
		FullName: "foo/bar",
	},
}

func TestBuildify(t *testing.T) {
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
	if err := lg.MakeFakeRepo("foo", "bar"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	if err := lg.AddCommit("foo", "bar", initialFiles); err != nil {
		t.Fatalf("Adding initial commit: %v", err)
	}
	if err := lg.CheckoutNewBranch("foo", "bar", "pull/42/head"); err != nil {
		t.Fatalf("Checking out pull branch: %v", err)
	}
	if err := lg.AddCommit("foo", "bar", pullFiles); err != nil {
		t.Fatalf("Adding PR commit: %v", err)
	}

	gh := &ghc{
		changes: []github.PullRequestChange{
			{
				Filename: "BUILD",
			},
			{
				Filename: "WORKSPACE",
			},
			{
				Filename: "blah.bzl",
			},
		},
	}
	if err := handle(gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
		t.Fatalf("Got error from handle: %v", err)
	}
	if len(gh.comment.Comments) != 1 {
		t.Fatalf("Expected one comment, got %d: %v.", len(gh.comment.Comments), gh.comment.Comments)
	}
	for _, c := range gh.comment.Comments {
		pos := c.Position
		gh.oldComments = append(gh.oldComments, github.ReviewComment{
			Path:     c.Path,
			Position: &pos,
			Body:     c.Body,
		})
	}
}

func TestModifiedBazelFiles(t *testing.T) {
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
	if err := lg.MakeFakeRepo("foo", "bar"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	if err := lg.AddCommit("foo", "bar", initialFiles); err != nil {
		t.Fatalf("Adding initial commit: %v", err)
	}
	if err := lg.CheckoutNewBranch("foo", "bar", "pull/42/head"); err != nil {
		t.Fatalf("Checking out pull branch: %v", err)
	}
	if err := lg.AddCommit("foo", "bar", pullFiles); err != nil {
		t.Fatalf("Adding PR commit: %v", err)
	}

	var testcases = []struct {
		name                  string
		gh                    *ghc
		expectedModifiedFiles map[string]string
	}{
		{
			name: "modified files include BUILD",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "BUILD",
					},
					{
						Filename: "foo.go",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"BUILD": "",
			},
		},
		{
			name: "modified files include BUILD.bazel",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "BUILD.bazel",
					},
					{
						Filename: "foo.go",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"BUILD.bazel": "",
			},
		},
		{
			name: "modified files include WORKSPACE",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "WORKSPACE",
					},
					{
						Filename: "foo.md",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"WORKSPACE": "",
			},
		},
		{
			name: "modified files include .bzl file",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.bzl",
					},
					{
						Filename: "blah.md",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"qux.bzl": "",
			},
		},
		{
			name: "modified files include removed BUILD file",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
					},
					{
						Filename: "BUILD",
						Status:   github.PullRequestFileRemoved,
					},
				},
			},
			expectedModifiedFiles: map[string]string{},
		},
		{
			name: "modified files include renamed file",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
					},
					{
						Filename: "BUILD",
						Status:   github.PullRequestFileRenamed,
					},
				},
			},
			expectedModifiedFiles: map[string]string{},
		},
		{
			name: "added and modified files",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "foo/BUILD.bazel",
						Status:   github.PullRequestFileAdded,
					},
					{
						Filename: "bar/blah.bzl",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"foo/BUILD.bazel": "",
				"bar/blah.bzl":    "",
			},
		},
	}
	for _, tc := range testcases {
		actualModifiedFiles, _ := modifiedBazelFiles(tc.gh, "foo", "bar", 9527, "0ebb33b")
		if !reflect.DeepEqual(tc.expectedModifiedFiles, actualModifiedFiles) {
			t.Errorf("Expected: %#v, Got %#v in case %s.", tc.expectedModifiedFiles, actualModifiedFiles, tc.name)
		}
	}
}

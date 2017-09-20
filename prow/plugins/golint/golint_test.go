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

package golint

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
)

var initialFiles = map[string][]byte{
	"bar.go": []byte(`// Package bar does an interesting thing.
package bar

// Foo does a thing.
func Foo(wow int) int {
	return 42 + wow
}
`),
}

var pullFiles = map[string][]byte{
	"qux.go": []byte(`package bar

func Qux() error {
	return nil
}
`),
	"zz_generated.wowza.go": []byte(`package bar

func Qux() error {
	return nil
}
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
	Body:       "/lint",
	User:       github.User{Login: "cjwagner"},
	Number:     42,
	IsPR:       true,
	Repo: github.Repo{
		Owner:    github.User{Login: "foo"},
		Name:     "bar",
		FullName: "foo/bar",
	},
}

func TestLint(t *testing.T) {
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
		genfile: []byte("file-prefix zz_generated"),
		changes: []github.PullRequestChange{
			{
				Filename: "qux.go",
				Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			},
			{
				Filename: "zz_generated.wowza.go",
				Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux2() error {\n+   return nil\n+}",
			},
		},
	}
	if err := handle(gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
		t.Fatalf("Got error from handle: %v", err)
	}
	if len(gh.comment.Comments) != 2 {
		t.Fatalf("Expected two comments, got %d: %v.", len(gh.comment.Comments), gh.comment.Comments)
	}
	for _, c := range gh.comment.Comments {
		pos := c.Position
		gh.oldComments = append(gh.oldComments, github.ReviewComment{
			Path:     c.Path,
			Position: &pos,
			Body:     c.Body,
		})
	}
	if err := handle(gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
		t.Fatalf("Got error from handle on second try: %v", err)
	}
	if len(gh.comment.Comments) != 0 {
		t.Fatalf("Expected no comments, got %d: %v", len(gh.comment.Comments), gh.comment.Comments)
	}

	// Test that we limit comments.
	badFileLines := []string{"package baz", ""}
	for i := 0; i < maxComments+5; i++ {
		badFileLines = append(badFileLines, fmt.Sprintf("type PublicType%d int", i))
	}
	gh.changes = append(gh.changes, github.PullRequestChange{
		Filename: "baz.go",
		Patch:    fmt.Sprintf("@@ -0,0 +1,%d @@\n+%s", len(badFileLines), strings.Join(badFileLines, "\n+")),
	})
	if err := lg.AddCommit("foo", "bar", map[string][]byte{"baz.go": []byte(strings.Join(badFileLines, "\n"))}); err != nil {
		t.Fatalf("Adding PR commit: %v", err)
	}
	gh.oldComments = nil
	if err := handle(gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
		t.Fatalf("Got error from handle on third try: %v", err)
	}
	if len(gh.comment.Comments) != maxComments {
		t.Fatalf("Expected %d comments, got %d: %v", maxComments, len(gh.comment.Comments), gh.comment.Comments)
	}
}

func TestAddedLines(t *testing.T) {
	var testcases = []struct {
		patch string
		lines map[int]int
		err   bool
	}{
		{
			patch: "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			lines: map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5},
		},
		{
			patch: "@@ -29,12 +29,14 @@ import (\n \t\"github.com/sirupsen/logrus\"\n \t\"github.com/ghodss/yaml\"\n \n+\t\"k8s.io/test-infra/prow/config\"\n \t\"k8s.io/test-infra/prow/jenkins\"\n \t\"k8s.io/test-infra/prow/kube\"\n \t\"k8s.io/test-infra/prow/plank\"\n )\n \n var (\n+\tconfigPath   = flag.String(\"config-path\", \"/etc/config/config\", \"Path to config.yaml.\")\n \tbuildCluster = flag.String(\"build-cluster\", \"\", \"Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.\")\n \n \tjenkinsURL       = flag.String(\"jenkins-url\", \"\", \"Jenkins URL\")\n@@ -47,18 +49,22 @@ var objReg = regexp.MustCompile(`^[\\w-]+$`)\n \n func main() {\n \tflag.Parse()\n-\n \tlogrus.SetFormatter(&logrus.JSONFormatter{})\n \n-\tkc, err := kube.NewClientInCluster(kube.ProwNamespace)\n+\tconfigAgent := &config.Agent{}\n+\tif err := configAgent.Start(*configPath); err != nil {\n+\t\tlogrus.WithError(err).Fatal(\"Error starting config agent.\")\n+\t}\n+\n+\tkc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)\n \tif err != nil {\n \t\tlogrus.WithError(err).Fatal(\"Error getting client.\")\n \t}\n \tvar pkc *kube.Client\n \tif *buildCluster == \"\" {\n-\t\tpkc = kc.Namespace(kube.TestPodNamespace)\n+\t\tpkc = kc.Namespace(configAgent.Config().PodNamespace)\n \t} else {\n-\t\tpkc, err = kube.NewClientFromFile(*buildCluster, kube.TestPodNamespace)\n+\t\tpkc, err = kube.NewClientFromFile(*buildCluster, configAgent.Config().PodNamespace)\n \t\tif err != nil {\n \t\t\tlogrus.WithError(err).Fatal(\"Error getting kube client to build cluster.\")\n \t\t}",
			lines: map[int]int{4: 32, 11: 39, 23: 54, 24: 55, 25: 56, 26: 57, 27: 58, 28: 59, 35: 65, 38: 67},
		},
		{
			patch: "@@ -1 +0,0 @@\n-such",
		},
		{
			patch: "@@ -1,3 +0,0 @@\n-such\n-a\n-doge",
		},
		{
			patch: "@@ -0,0 +1 @@\n+wow",
			lines: map[int]int{1: 1},
		},
		{
			patch: "@@ -1 +1 @@\n-doge\n+wow",
			lines: map[int]int{2: 1},
		},
		{
			patch: "something strange",
			err:   true,
		},
		{
			patch: "@@ -a,3 +0,0 @@\n-wow",
			err:   true,
		},
		{
			patch: "@@ -1 +1 @@",
			err:   true,
		},
		{
			patch: "",
		},
	}
	for _, tc := range testcases {
		als, err := addedLines(tc.patch)
		if err == nil == tc.err {
			t.Errorf("For patch %s\nExpected error %v, got error %v", tc.patch, tc.err, err)
			continue
		}
		if len(als) != len(tc.lines) {
			t.Errorf("For patch %s\nAdded lines has wrong length. Got %v, expected %v", tc.patch, als, tc.lines)
		}
		for pl, l := range tc.lines {
			if als[l] != pl {
				t.Errorf("For patch %s\nExpected added line %d to be %d, but got %d", tc.patch, l, pl, als[l])
			}
		}
	}
}

func TestModifiedGoFiles(t *testing.T) {
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
			name: "modified files include vendor file",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
					},
					{
						Filename: "vendor/foo/bar.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux2() error {\n+   return nil\n+}",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"qux.go": "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			},
		},
		{
			name: "modified files include non go file",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
					},
					{
						Filename: "foo.md",
						Patch:    "@@ -1,3 +1,4 @@\n+TODO",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"qux.go": "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			},
		},
		{
			name: "modified files include generated file",
			gh: &ghc{
				genfile: []byte("file-prefix zz_generated"),
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
					},
					{
						Filename: "zz_generated.wowza.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux2() error {\n+   return nil\n+}",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"qux.go": "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			},
		},
		{
			name: "modified files include removed file",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
					},
					{
						Filename: "bar.go",
						Status:   github.PullRequestFileRemoved,
						Patch:    "@@ -1,5 +0,0 @@\n-package bar\n-\n-func Qux() error {\n-   return nil\n-}",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"qux.go": "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			},
		},
		{
			name: "modified files include renamed file",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
					},
					{
						Filename: "bar.go",
						Status:   github.PullRequestFileRenamed,
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"qux.go": "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			},
		},
		{
			name: "added and modified files",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
						Status:   github.PullRequestFileAdded,
						Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
					},
					{
						Filename: "bar.go",
						Patch:    "@@ -0,0 +1,5 @@\n+package baz\n+\n+func Bar() error {\n+   return nil\n+}",
					},
				},
			},
			expectedModifiedFiles: map[string]string{
				"qux.go": "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
				"bar.go": "@@ -0,0 +1,5 @@\n+package baz\n+\n+func Bar() error {\n+   return nil\n+}",
			},
		},
		{
			name: "removed and renamed files",
			gh: &ghc{
				changes: []github.PullRequestChange{
					{
						Filename: "qux.go",
						Status:   github.PullRequestFileRemoved,
						Patch:    "@@ -1,5 +0,0 @@\n-package bar\n-\n-func Qux() error {\n-   return nil\n-}",
					},
					{
						Filename: "bar.go",
						Status:   github.PullRequestFileRenamed,
					},
				},
			},
			expectedModifiedFiles: map[string]string{},
		},
	}
	for _, tc := range testcases {
		actualModifiedFiles, _ := modifiedGoFiles(tc.gh, "foo", "bar", 9527, "0ebb33b")
		if !reflect.DeepEqual(tc.expectedModifiedFiles, actualModifiedFiles) {
			t.Errorf("Expected: %#v, Got %#v in case %s.", tc.expectedModifiedFiles, actualModifiedFiles, tc.name)
		}
	}
}

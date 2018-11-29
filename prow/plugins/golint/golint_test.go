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
	"k8s.io/test-infra/prow/plugins"
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

func TestMinConfidence(t *testing.T) {
	zero := float64(0)
	half := 0.5
	cases := []struct {
		name     string
		golint   *plugins.Golint
		expected float64
	}{
		{
			name:     "nothing set",
			expected: defaultConfidence,
		},
		{
			name:     "no confidence set",
			golint:   &plugins.Golint{},
			expected: defaultConfidence,
		},
		{
			name:     "confidence set to zero",
			golint:   &plugins.Golint{MinimumConfidence: &zero},
			expected: zero,
		},
		{
			name:     "confidence set positive",
			golint:   &plugins.Golint{MinimumConfidence: &half},
			expected: half,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := minConfidence(tc.golint)
			if actual != tc.expected {
				t.Errorf("minimum confidence %f != expected %f", actual, tc.expected)
			}
		})
	}
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
	if err := handle(0, gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
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
	if err := handle(0, gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
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
	if err := handle(0, gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
		t.Fatalf("Got error from handle on third try: %v", err)
	}
	if len(gh.comment.Comments) != maxComments {
		t.Fatalf("Expected %d comments, got %d: %v", maxComments, len(gh.comment.Comments), gh.comment.Comments)
	}
}

func TestLintCodeSuggestion(t *testing.T) {

	var testcases = []struct {
		name       string
		codeChange string
		pullFiles  map[string][]byte
		comment    string
	}{
		{
			name:       "Check names with underscore",
			codeChange: "@@ -0,0 +1,7 @@\n+// Package bar comment\n+package bar\n+\n+// Qux_1 comment\n+func Qux_1_Func() error {\n+   return nil\n+}",
			pullFiles: map[string][]byte{
				"qux.go": []byte("// Package bar comment\npackage bar\n\n// Qux_1 comment\nfunc Qux_1() error {\n	return nil\n}\n"),
			},
			comment: "```suggestion\nfunc Qux1() error {\n```\nGolint naming: don't use underscores in Go names; func Qux_1 should be Qux1. [More info](http://golang.org/doc/effective_go.html#mixed-caps). <!-- golint -->",
		},
		{
			name:       "Check names with all caps",
			codeChange: "@@ -0,0 +1,7 @@\n+// Package bar comment\n+package bar\n+\n+// QUX_FUNC comment\n+func QUX_FUNC() error {\n+   return nil\n+}",
			pullFiles: map[string][]byte{
				"qux.go": []byte("// Package bar comment\npackage bar\n\n// QUX_FUNC comment\nfunc QUX_FUNC() error {\n       return nil\n}\n"),
			},
			comment: "```suggestion\nfunc QuxFunc() error {\n```\nGolint naming: don't use ALL_CAPS in Go names; use CamelCase. [More info](https://golang.org/wiki/CodeReviewComments#mixed-caps). <!-- golint -->",
		},
		{
			name:       "Correct function name",
			codeChange: "@@ -0,0 +1,7 @@\n+// Package bar comment\n+package bar\n+\n+// QuxFunc comment\n+func QuxFunc() error {\n+   return nil\n+}",
			pullFiles: map[string][]byte{
				"qux.go": []byte("// Package bar comment\npackage bar\n\n// QuxFunc comment\nfunc QuxFunc() error {\n       return nil\n}\n"),
			},
			comment: "",
		},
		{
			name:       "Check stutter in function names",
			codeChange: "@@ -0,0 +1,9 @@\n+/*\n+Package bar comment\n+*/\n+package bar\n+\n+// BarFunc comment\n+func BarFunc() error {\n+   return nil\n+}",
			pullFiles: map[string][]byte{
				"qux.go": []byte("/*\nPackage bar comment\n*/\npackage bar\n\n// BarFunc comment\nfunc BarFunc() error {\n   return nil\n}"),
			},
			comment: "```suggestion\nfunc Func() error {\n```\nGolint naming: func name will be used as bar.BarFunc by other packages, and that stutters; consider calling this Func. [More info](https://golang.org/wiki/CodeReviewComments#package-names). <!-- golint -->",
		},
		{
			name:       "Check stutter in type names",
			codeChange: "@@ -0,0 +1,8 @@\n+/*\n+Package bar comment\n+*/\n+package bar\n+\n+// BarMaker comment\n+type BarMaker struct{}\n+",
			pullFiles: map[string][]byte{
				"qux.go": []byte("/*\nPackage bar comment\n*/\npackage bar\n\n// BarMaker comment\ntype BarMaker struct{}\n"),
			},
			comment: "```suggestion\ntype Maker struct{}\n```\nGolint naming: type name will be used as bar.BarMaker by other packages, and that stutters; consider calling this Maker. [More info](https://golang.org/wiki/CodeReviewComments#package-names). <!-- golint -->",
		},
		{
			name:       "Check stutter: no stutter",
			codeChange: "@@ -0,0 +1,8 @@\n+/*\n+Package bar comment\n+*/\n+package bar\n+\n+// barMaker comment\n+type barMaker struct{}\n+",
			pullFiles: map[string][]byte{
				"qux.go": []byte("/*\nPackage bar comment\n*/\npackage bar\n\n// barMaker comment\ntype barMaker struct{}\n"),
			},
			comment: "",
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
	if err := lg.MakeFakeRepo("foo", "bar"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	if err := lg.AddCommit("foo", "bar", initialFiles); err != nil {
		t.Fatalf("Adding initial commit: %v", err)
	}
	if err := lg.CheckoutNewBranch("foo", "bar", "pull/42/head"); err != nil {
		t.Fatalf("Checking out pull branch: %v", err)
	}

	for _, test := range testcases {
		t.Logf("Running test case %q...", test.name)
		if err := lg.AddCommit("foo", "bar", test.pullFiles); err != nil {
			t.Fatalf("Adding PR commit: %v", err)
		}
		gh := &ghc{
			changes: []github.PullRequestChange{
				{
					Filename: "qux.go",
					Patch:    test.codeChange,
				},
			},
		}
		if err := handle(0, gh, c, logrus.NewEntry(logrus.New()), e); err != nil {
			t.Fatalf("Got error from handle: %v", err)
		}

		if test.comment == "" {
			if len(gh.comment.Comments) > 0 {
				t.Fatalf("Expected no comment, got %d: %v.", len(gh.comment.Comments), gh.comment.Comments)
			}
		} else {
			if len(gh.comment.Comments) != 1 {
				t.Fatalf("Expected one comments, got %d: %v.", len(gh.comment.Comments), gh.comment.Comments)
			}
			if test.comment != gh.comment.Comments[0].Body {
				t.Fatalf("Expected\n" + test.comment + "\n but got\n" + gh.comment.Comments[0].Body)
			}
		}
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
			patch: "@@ -29,12 +29,14 @@ import (\n \t\"github.com/sirupsen/logrus\"\n \t\"sigs.k8s.io/yaml\"\n \n+\t\"k8s.io/test-infra/prow/config\"\n \t\"k8s.io/test-infra/prow/jenkins\"\n \t\"k8s.io/test-infra/prow/kube\"\n \t\"k8s.io/test-infra/prow/plank\"\n )\n \n var (\n+\tconfigPath   = flag.String(\"config-path\", \"/etc/config/config\", \"Path to config.yaml.\")\n \tbuildCluster = flag.String(\"build-cluster\", \"\", \"Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.\")\n \n \tjenkinsURL       = flag.String(\"jenkins-url\", \"\", \"Jenkins URL\")\n@@ -47,18 +49,22 @@ var objReg = regexp.MustCompile(`^[\\w-]+$`)\n \n func main() {\n \tflag.Parse()\n-\n \tlogrus.SetFormatter(&logrus.JSONFormatter{})\n \n-\tkc, err := kube.NewClientInCluster(kube.ProwNamespace)\n+\tconfigAgent := &config.Agent{}\n+\tif err := configAgent.Start(*configPath); err != nil {\n+\t\tlogrus.WithError(err).Fatal(\"Error starting config agent.\")\n+\t}\n+\n+\tkc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)\n \tif err != nil {\n \t\tlogrus.WithError(err).Fatal(\"Error getting client.\")\n \t}\n \tvar pkc *kube.Client\n \tif *buildCluster == \"\" {\n-\t\tpkc = kc.Namespace(kube.TestPodNamespace)\n+\t\tpkc = kc.Namespace(configAgent.Config().PodNamespace)\n \t} else {\n-\t\tpkc, err = kube.NewClientFromFile(*buildCluster, kube.TestPodNamespace)\n+\t\tpkc, err = kube.NewClientFromFile(*buildCluster, configAgent.Config().PodNamespace)\n \t\tif err != nil {\n \t\t\tlogrus.WithError(err).Fatal(\"Error getting kube client to build cluster.\")\n \t\t}",
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
			patch: "@@ -0,0 +1 @@\n+wow\n\\ No newline at end of file",
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
		als, err := AddedLines(tc.patch)
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

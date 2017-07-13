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
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"

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
}

type ghc struct {
	changes []github.PullRequestChange
	comment string
}

func (g *ghc) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	return g.changes, nil
}

func (g *ghc) CreateComment(org, repo string, number int, body string) error {
	g.comment = body
	return nil
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
		changes: []github.PullRequestChange{
			{
				Filename: "qux.go",
				Patch:    "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			},
		},
	}
	if err := handle(gh, c, logrus.NewEntry(logrus.New()), github.IssueCommentEvent{
		Action: "created",
		Issue: github.Issue{
			State:       "open",
			Number:      42,
			PullRequest: &struct{}{},
		},
		Comment: github.IssueComment{
			Body: "/lint",
		},
		Repo: github.Repo{
			Owner:    github.User{Login: "foo"},
			Name:     "bar",
			FullName: "foo/bar",
		},
	}); err != nil {
		t.Fatalf("Got error from handle: %v", err)
	}
	if !strings.Contains(gh.comment, "qux.go:3") {
		t.Errorf("Should have seen an error on line 3 of qux.go in comment: %s", gh.comment)
	}
}

func TestAddedLines(t *testing.T) {
	var testcases = []struct {
		patch string
		lines []int
		err   bool
	}{
		{
			patch: "@@ -0,0 +1,5 @@\n+package bar\n+\n+func Qux() error {\n+   return nil\n+}",
			lines: []int{1, 2, 3, 4, 5},
		},
		{
			patch: "@@ -29,12 +29,14 @@ import (\n \t\"github.com/Sirupsen/logrus\"\n \t\"github.com/ghodss/yaml\"\n \n+\t\"k8s.io/test-infra/prow/config\"\n \t\"k8s.io/test-infra/prow/jenkins\"\n \t\"k8s.io/test-infra/prow/kube\"\n \t\"k8s.io/test-infra/prow/plank\"\n )\n \n var (\n+\tconfigPath   = flag.String(\"config-path\", \"/etc/config/config\", \"Path to config.yaml.\")\n \tbuildCluster = flag.String(\"build-cluster\", \"\", \"Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.\")\n \n \tjenkinsURL       = flag.String(\"jenkins-url\", \"\", \"Jenkins URL\")\n@@ -47,18 +49,22 @@ var objReg = regexp.MustCompile(`^[\\w-]+$`)\n \n func main() {\n \tflag.Parse()\n-\n \tlogrus.SetFormatter(&logrus.JSONFormatter{})\n \n-\tkc, err := kube.NewClientInCluster(kube.ProwNamespace)\n+\tconfigAgent := &config.Agent{}\n+\tif err := configAgent.Start(*configPath); err != nil {\n+\t\tlogrus.WithError(err).Fatal(\"Error starting config agent.\")\n+\t}\n+\n+\tkc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)\n \tif err != nil {\n \t\tlogrus.WithError(err).Fatal(\"Error getting client.\")\n \t}\n \tvar pkc *kube.Client\n \tif *buildCluster == \"\" {\n-\t\tpkc = kc.Namespace(kube.TestPodNamespace)\n+\t\tpkc = kc.Namespace(configAgent.Config().PodNamespace)\n \t} else {\n-\t\tpkc, err = kube.NewClientFromFile(*buildCluster, kube.TestPodNamespace)\n+\t\tpkc, err = kube.NewClientFromFile(*buildCluster, configAgent.Config().PodNamespace)\n \t\tif err != nil {\n \t\t\tlogrus.WithError(err).Fatal(\"Error getting kube client to build cluster.\")\n \t\t}",
			lines: []int{32, 39, 54, 55, 56, 57, 58, 59, 65, 67},
		},
		{
			patch: "@@ -1 +0,0 @@\n-such",
		},
		{
			patch: "@@ -1,3 +0,0 @@\n-such\n-a\n-doge",
		},
		{
			patch: "@@ -0,0 +1 @@\n+wow",
			lines: []int{1},
		},
		{
			patch: "@@ -1 +1 @@\n-doge\n+wow",
			lines: []int{1},
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
	}
	for _, tc := range testcases {
		als, err := addedLines(tc.patch)
		if err == nil == tc.err {
			t.Errorf("For patch %s\nExpected error %v, got error %v", tc.patch, tc.err, err)
			continue
		}
		for _, l := range tc.lines {
			if !als[l] {
				t.Errorf("For patch %s\nExpected added line %d, but didn't see it in %v", tc.patch, l, als)
			} else {
				als[l] = false
			}
		}
		for l, missed := range als {
			if missed {
				t.Errorf("For patch %s\nSaw line %d but didn't expect it.", tc.patch, l)
			}
		}
	}
}

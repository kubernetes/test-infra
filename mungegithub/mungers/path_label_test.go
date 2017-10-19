/*
Copyright 2015 The Kubernetes Authors.

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

package mungers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"testing"

	github_util "k8s.io/test-infra/mungegithub/github"
	github_test "k8s.io/test-infra/mungegithub/github/testing"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var (
	_ = fmt.Printf
	_ = glog.Errorf
)

const (
	pathLabelTestContents = `# This file is used by the path-label munger and is of the form:
#  PATH REGEXP			LABEL

^docs/proposals			kind/design
^docs/design			kind/design

# examples:
# pkg/api/types.go
# pkg/api/*/types.go
^pkg/api/([^/]+/)?types.go$    kind/api-change
^pkg/api/([^/]+/)?register.go$ kind/new-api

# examples:
# pkg/apis/*/types.go
# pkg/apis/*/*/types.go
^pkg/apis/[^/]+/([^/]+/)?types.go$    kind/api-change
^pkg/apis/[^/]+/([^/]+/)?register.go$ kind/new-api

# docs which are going away with move to separate doc repo
^docs/getting-started-guides	kind/old-docs
^docs/admin			kind/old-docs
^docs/user-guide		kind/old-docs
^docs/devel             kind/old-docs
^docs/design            kind/old-docs
^docs/proposals         kind/old-docs
`
)

func docsProposalIssue(testBotName string) *github.Issue {
	return github_test.Issue(testBotName, 1, []string{cncfClaYesLabel, "kind/design"}, true)
}

// Commit returns a filled out github.Commit which happened at time.Unix(t, 0)
func commitFiles(path []string) []*github.CommitFile {
	files := []*github.CommitFile{}
	for _, p := range path {
		f := &github.CommitFile{
			Filename: stringPtr(p),
		}
		files = append(files, f)
	}
	return files
}

func BotAddedDesign(testBotName string) []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{User: testBotName, Label: "kind/design", Time: 9},
		{User: "bob", Label: "kind/design", Time: 8},
	})
}

func OtherAddedDesign(testBotName string) []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{User: testBotName, Label: "kind/design", Time: 8},
		{User: "bob", Label: "kind/design", Time: 9},
	})
}

func TestPathLabelMunge(t *testing.T) {
	const testBotName = "dummy"
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		files       []*github.CommitFile
		events      []*github.IssueEvent
		mustHave    []string
		mustNotHave []string
	}{
		{
			files:       commitFiles([]string{"docs/proposals"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{"kind/design"},
			mustNotHave: []string{"kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"docs/my/proposals"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/api/types.go"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{"kind/api-change"},
			mustNotHave: []string{"kind/design", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/api/v1/types.go"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{"kind/api-change"},
			mustNotHave: []string{"kind/design", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/api/v1/duh/types.go"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/apis/experimental/register.go"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{"kind/new-api"},
			mustNotHave: []string{"kind/api-change", "kind/design"},
		},
		{
			files:       commitFiles([]string{"pkg/apis/experimental/v1beta1/register.go"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{"kind/new-api"},
			mustNotHave: []string{"kind/api-change", "kind/design"},
		},
		{
			files:       commitFiles([]string{"pkg/apis/experiments/v1beta1/duh/register.go"}),
			events:      BotAddedDesign(testBotName),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"README"}),
			events:      OtherAddedDesign(testBotName),
			mustHave:    []string{"kind/design"},
			mustNotHave: []string{"kind/api-change", "kind/new-api"},
		},
	}
	for testNum, test := range tests {
		client, server, mux := github_test.InitServer(t, docsProposalIssue(testBotName), ValidPR(), test.events, nil, nil, nil, test.files)
		mux.HandleFunc("/repos/o/r/issues/1/labels/kind/design", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte{})
		})
		mux.HandleFunc("/repos/o/r/issues/1/labels", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := []github.Label{{}}
			data, err := json.Marshal(out)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)

		})

		config := &github_util.Config{
			Org:     "o",
			Project: "r",
		}
		config.SetClient(client)
		config.BotName = testBotName

		pathLabelTestFile, err := ioutil.TempFile("", "path-label.txt")
		if err != nil {
			t.Fatalf("open tempfile for writing: %v", err)
		}
		defer os.Remove(pathLabelTestFile.Name()) // clean up temp file
		if _, err := pathLabelTestFile.Write([]byte(pathLabelTestContents)); err != nil {
			t.Fatalf("write to %q: %v", pathLabelTestFile.Name(), err)
		}
		if err := pathLabelTestFile.Close(); err != nil {
			t.Fatalf("closing tempfile %q: %v", pathLabelTestFile.Name(), err)
		}

		p := PathLabelMunger{pathLabelFile: pathLabelTestFile.Name()}
		err = p.Initialize(config, nil)
		if err != nil {
			t.Fatalf("%v", err)
		}

		obj, err := config.GetObject(1)
		if err != nil {
			t.Fatalf("%v", err)
		}

		p.Munge(obj)

		for _, l := range test.mustHave {
			if !obj.HasLabel(l) {
				t.Errorf("%d: Did not find label %q, labels: %v", testNum, l, obj.Issue.Labels)
			}
		}
		for _, l := range test.mustNotHave {
			if obj.HasLabel(l) {
				t.Errorf("%d: Found label %q and should not have, labels: %v", testNum, l, obj.Issue.Labels)
			}
		}
		server.Close()
	}
}

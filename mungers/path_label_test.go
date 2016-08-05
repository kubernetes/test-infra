/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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
	"net/http"
	"runtime"
	"testing"

	github_util "k8s.io/contrib/mungegithub/github"
	github_test "k8s.io/contrib/mungegithub/github/testing"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var (
	_ = fmt.Printf
	_ = glog.Errorf
)

func docsProposalIssue() *github.Issue {
	return github_test.Issue(botName, 1, []string{claYesLabel, "kind/design"}, true)
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

func BotAddedDesign() []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{botName, "kind/design", 9},
		{"bob", "kind/design", 8},
	})
}

func OtherAddedDesign() []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{botName, "kind/design", 8},
		{"bob", "kind/design", 9},
	})
}

func TestPathLabelMunge(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		files       []*github.CommitFile
		events      []*github.IssueEvent
		mustHave    []string
		mustNotHave []string
	}{
		{
			files:       commitFiles([]string{"docs/proposals"}),
			events:      BotAddedDesign(),
			mustHave:    []string{"kind/design"},
			mustNotHave: []string{"kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"docs/my/proposals"}),
			events:      BotAddedDesign(),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/api/types.go"}),
			events:      BotAddedDesign(),
			mustHave:    []string{"kind/api-change"},
			mustNotHave: []string{"kind/design", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/api/v1/types.go"}),
			events:      BotAddedDesign(),
			mustHave:    []string{"kind/api-change"},
			mustNotHave: []string{"kind/design", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/api/v1/duh/types.go"}),
			events:      BotAddedDesign(),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"pkg/apis/experimental/register.go"}),
			events:      BotAddedDesign(),
			mustHave:    []string{"kind/new-api"},
			mustNotHave: []string{"kind/api-change", "kind/design"},
		},
		{
			files:       commitFiles([]string{"pkg/apis/experimental/v1beta1/register.go"}),
			events:      BotAddedDesign(),
			mustHave:    []string{"kind/new-api"},
			mustNotHave: []string{"kind/api-change", "kind/design"},
		},
		{
			files:       commitFiles([]string{"pkg/apis/experiments/v1beta1/duh/register.go"}),
			events:      BotAddedDesign(),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			files:       commitFiles([]string{"README"}),
			events:      OtherAddedDesign(),
			mustHave:    []string{"kind/design"},
			mustNotHave: []string{"kind/api-change", "kind/new-api"},
		},
	}
	for testNum, test := range tests {
		client, server, mux := github_test.InitServer(t, docsProposalIssue(), ValidPR(), test.events, nil, nil, nil, test.files)
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

		config := &github_util.Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		p := PathLabelMunger{}
		p.PathLabelFile = "../path-label.txt"
		err := p.Initialize(config, nil)
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

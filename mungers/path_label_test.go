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

// Commit returns a filled out github.Commit which happened at time.Unix(t, 0)
func pathCommits(path string) []github.RepositoryCommit {
	return []github.RepositoryCommit{
		{
			SHA: stringPtr("mysha"),
			Files: []github.CommitFile{
				{
					Filename: stringPtr(path),
				},
			},
		},
	}
}

func TestPathLabelMunge(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		commits     []github.RepositoryCommit
		mustHave    []string
		mustNotHave []string
	}{
		{
			commits:     pathCommits("docs/proposals"),
			mustHave:    []string{"kind/design"},
			mustNotHave: []string{"kind/api-change", "kind/new-api"},
		},
		{
			commits:     pathCommits("docs/my/proposals"),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			commits:     pathCommits("pkg/api/types.go"),
			mustHave:    []string{"kind/api-change"},
			mustNotHave: []string{"kind/design", "kind/new-api"},
		},
		{
			commits:     pathCommits("pkg/api/v1/types.go"),
			mustHave:    []string{"kind/api-change"},
			mustNotHave: []string{"kind/design", "kind/new-api"},
		},
		{
			commits:     pathCommits("pkg/api/v1/duh/types.go"),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
		{
			commits:     pathCommits("pkg/apis/experimental/register.go"),
			mustHave:    []string{"kind/new-api"},
			mustNotHave: []string{"kind/api-change", "kind/design"},
		},
		{
			commits:     pathCommits("pkg/apis/experimental/v1beta1/register.go"),
			mustHave:    []string{"kind/new-api"},
			mustNotHave: []string{"kind/api-change", "kind/design"},
		},
		{
			commits:     pathCommits("pkg/apis/experiments/v1beta1/duh/register.go"),
			mustHave:    []string{},
			mustNotHave: []string{"kind/design", "kind/api-change", "kind/new-api"},
		},
	}
	for testNum, test := range tests {
		client, server, mux := github_test.InitServer(t, NoOKToMergeIssue(), ValidPR(), nil, test.commits, nil)
		mux.HandleFunc("/repos/o/r/issues/1/labels", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := []github.Label{
				{
					// TODO figure out the label name from the request...
					Name: stringPtr("label"),
				},
			}
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
		p.pathLabelFile = "../path-label.txt"
		err := p.Initialize(config)
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

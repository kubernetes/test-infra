/*
Copyright 2016 The Kubernetes Authors.

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
	c "k8s.io/contrib/mungegithub/mungers/matchers/comment"

	"github.com/google/go-github/github"
)

const (
	claContext = "some/context"
)

func TestCLAMunger(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name        string
		issue       *github.Issue
		status      *github.CombinedStatus
		mustHave    []string
		mustNotHave []string
	}{
		{
			name:  "CLA status success should add cncf/cla:yes label and remove cncf/cla:no label",
			issue: github_test.Issue("user1", 1, []string{cncfClaNoLabel}, true),
			status: &github.CombinedStatus{
				Statuses: []github.RepoStatus{
					{
						Context: stringPtr(claContext),
						State:   stringPtr(contextSuccess),
					},
				},
			},
			mustHave:    []string{cncfClaYesLabel},
			mustNotHave: []string{cncfClaNoLabel},
		},
		{
			name:  "CLA status failure should add cncf/cla:no label and remove cncf/cla:yes label",
			issue: github_test.Issue("user1", 1, []string{cncfClaYesLabel}, true),
			status: &github.CombinedStatus{
				Statuses: []github.RepoStatus{
					{
						Context: stringPtr(claContext),
						State:   stringPtr(contextFailure),
					},
				},
			},
			mustHave:    []string{cncfClaNoLabel},
			mustNotHave: []string{cncfClaYesLabel},
		},
		{
			name:  "CLA status error should apply cncf/cla:no label.",
			issue: github_test.Issue("user1", 1, []string{}, true),
			status: &github.CombinedStatus{
				Statuses: []github.RepoStatus{
					{
						Context: stringPtr(claContext),
						State:   stringPtr(contextError),
					},
				},
			},
			mustHave:    []string{cncfClaNoLabel},
			mustNotHave: []string{cncfClaYesLabel},
		},
		{
			name:  "CLA status pending should not apply labels.",
			issue: github_test.Issue("user1", 1, []string{}, true),
			status: &github.CombinedStatus{
				Statuses: []github.RepoStatus{
					{
						Context: stringPtr(claContext),
						State:   stringPtr(contextPending),
					},
				},
			},
			mustHave:    []string{},
			mustNotHave: []string{cncfClaYesLabel, cncfClaNoLabel},
		},
	}

	for testNum, test := range tests {
		pr := ValidPR()
		pr.Head = &github.PullRequestBranch{}
		pr.Head.SHA = stringPtr("0")
		client, server, mux := github_test.InitServer(t, test.issue, pr, nil, nil, nil, nil, nil)
		setUpMockFunctions(mux, t, test.issue)

		path := fmt.Sprintf("/repos/o/r/commits/%s/status", *pr.Head.SHA)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := test.status
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

		cla := ClaMunger{
			CLAStatusContext: claContext,
			pinger:           c.NewPinger("[fake-ping]").SetDescription(""),
		}
		obj, err := config.GetObject(*test.issue.Number)
		if err != nil {
			t.Fatalf("%v", err)
		}
		cla.Munge(obj)

		for _, lab := range test.mustHave {
			if !obj.HasLabel(lab) {
				t.Errorf("%s:%d: Did not find label %q, labels: %v", test.name, testNum, lab, obj.Issue.Labels)
			}
		}
		for _, lab := range test.mustNotHave {
			if obj.HasLabel(lab) {
				t.Errorf("%s:%d: Found label %q and should not have, labels: %v", test.name, testNum, lab, obj.Issue.Labels)
			}
		}
		server.Close()
	}
}

func setUpMockFunctions(mux *http.ServeMux, t *testing.T, issue *github.Issue) {
	path := fmt.Sprintf("/repos/o/r/issue/%d/labels", *issue.Number)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		out := []github.Label{{}}
		data, err := json.Marshal(out)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		w.Write(data)
	})

	path = fmt.Sprintf("/repos/o/r/issues/%d/comments", *issue.Number)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		data, err := json.Marshal(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		w.Write(data)
	})
}

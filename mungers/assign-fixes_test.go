/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

func TestAssignFixes(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name       string
		assignee   string
		pr         *github.PullRequest
		prIssue    *github.Issue
		prBody     string
		fixesIssue *github.Issue
	}{
		{
			name:       "fixes an issue",
			assignee:   "dev45",
			pr:         github_test.PullRequest("dev45", false, true, true),
			prIssue:    github_test.Issue("fred", 7779, []string{}, true),
			prBody:     "does stuff and fixes #8889.",
			fixesIssue: github_test.Issue("jill", 8889, []string{}, true),
		},
	}
	for _, test := range tests {
		test.prIssue.Body = &test.prBody
		client, server, mux := github_test.InitServer(t, test.prIssue, test.pr, nil, nil, nil, nil, nil)
		path := fmt.Sprintf("/repos/o/r/issues/%d", *test.fixesIssue.Number)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			data, err := json.Marshal(test.fixesIssue)
			if err != nil {
				t.Errorf("%v", err)
			}
			if r.Method != "PATCH" && r.Method != "GET" {
				t.Errorf("Unexpected method: expected: GET/PATCH got: %s", r.Method)
			}
			if r.Method == "PATCH" {
				body, _ := ioutil.ReadAll(r.Body)

				type IssuePatch struct {
					Assignee string
				}
				var ip IssuePatch
				err := json.Unmarshal(body, &ip)
				if err != nil {
					fmt.Println("error:", err)
				}
				if ip.Assignee != test.assignee {
					t.Errorf("Patching the incorrect Assignee %v instead of %v", ip.Assignee, test.assignee)
				}
			}
			w.WriteHeader(http.StatusOK)
			w.Write(data)
		})

		config := &github_util.Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		c := AssignFixesMunger{}
		err := c.Initialize(config, nil)
		if err != nil {
			t.Fatalf("%v", err)
		}

		err = c.EachLoop()
		if err != nil {
			t.Fatalf("%v", err)
		}

		obj, err := config.GetObject(*test.prIssue.Number)
		if err != nil {
			t.Fatalf("%v", err)
		}

		c.Munge(obj)
		server.Close()
	}
}

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

	github_util "k8s.io/test-infra/mungegithub/github"
	github_test "k8s.io/test-infra/mungegithub/github/testing"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var (
	_ = fmt.Printf
	_ = glog.Errorf
)

func TestCherrypickAuthApprove(t *testing.T) {
	const testBotName = "dummy"
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name                string
		issue               *github.Issue
		issueBody           string
		prBranch            string
		parentIssue         *github.Issue
		milestone           *github.Milestone
		shouldHaveLabel     string
		shouldHaveMilestone string
		shouldNotHaveLabel  string
		shouldNotHaveMile   string
	}{
		{
			name:                "Add cpApproved and milestone",
			issue:               github_test.Issue(testBotName, 1, []string{}, true),
			issueBody:           "Cherry pick of #2 on release-1.2.",
			prBranch:            "release-1.2",
			parentIssue:         github_test.Issue(testBotName, 2, []string{cpApprovedLabel}, true),
			milestone:           &github.Milestone{Title: stringPtr("v1.2"), Number: intPtr(1)},
			shouldHaveLabel:     cpApprovedLabel,
			shouldHaveMilestone: "v1.2",
		},
		{
			name:                "Add milestone",
			issue:               github_test.Issue(testBotName, 1, []string{cpApprovedLabel}, true),
			issueBody:           "Cherry pick of #2 on release-1.2.",
			prBranch:            "release-1.2",
			parentIssue:         github_test.Issue(testBotName, 2, []string{cpApprovedLabel}, true),
			milestone:           &github.Milestone{Title: stringPtr("v1.2"), Number: intPtr(1)},
			shouldHaveLabel:     cpApprovedLabel,
			shouldHaveMilestone: "v1.2",
		},
		{
			name:               "Do not add because parent not have",
			issue:              github_test.Issue(testBotName, 1, []string{}, true),
			issueBody:          "Cherry pick of #2 on release-1.2.",
			prBranch:           "release-1.2",
			parentIssue:        github_test.Issue(testBotName, 2, []string{}, true),
			milestone:          &github.Milestone{Title: stringPtr("v1.2"), Number: intPtr(1)},
			shouldNotHaveLabel: cpApprovedLabel,
			shouldNotHaveMile:  "v1.2",
		},
		{
			name:               "PR against wrong branch",
			issue:              github_test.Issue(testBotName, 1, []string{}, true),
			issueBody:          "Cherry pick of #2 on release-1.2.",
			prBranch:           "release-1.1",
			parentIssue:        github_test.Issue(testBotName, 2, []string{cpApprovedLabel}, true),
			milestone:          &github.Milestone{Title: stringPtr("v1.2"), Number: intPtr(1)},
			shouldNotHaveLabel: cpApprovedLabel,
			shouldNotHaveMile:  "v1.2",
		},
		{
			name:               "Parent milestone against other branch",
			issue:              github_test.Issue(testBotName, 1, []string{}, true),
			issueBody:          "Cherry pick of #2 on release-1.2.",
			prBranch:           "release-1.2",
			parentIssue:        github_test.Issue(testBotName, 2, []string{cpApprovedLabel}, true),
			milestone:          &github.Milestone{Title: stringPtr("v1.1"), Number: intPtr(1)},
			shouldNotHaveLabel: cpApprovedLabel,
			shouldNotHaveMile:  "v1.1",
		},
		{
			name:               "Parent has no milestone",
			issue:              github_test.Issue(testBotName, 1, []string{}, true),
			issueBody:          "Cherry pick of #2 on release-1.2.",
			prBranch:           "release-1.2",
			parentIssue:        github_test.Issue(testBotName, 2, []string{cpApprovedLabel}, true),
			shouldNotHaveLabel: cpApprovedLabel,
			shouldNotHaveMile:  "v1.2",
		},
	}
	for testNum, test := range tests {
		test.issue.Body = &test.issueBody

		pr := ValidPR()
		pr.Base.Ref = &test.prBranch
		client, server, mux := github_test.InitServer(t, test.issue, pr, nil, nil, nil, nil, nil)

		path := fmt.Sprintf("/repos/o/r/issues/%d/labels", *test.issue.Number)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := []github.Label{{}}
			data, err := json.Marshal(out)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
		})

		path = "/repos/o/r/milestones"
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := []github.Milestone{}
			if test.milestone != nil {
				out = append(out, *test.milestone)
			}
			data, err := json.Marshal(out)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
		})

		test.parentIssue.Milestone = test.milestone
		path = fmt.Sprintf("/repos/o/r/issues/%d", *test.parentIssue.Number)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			data, err := json.Marshal(test.parentIssue)
			if err != nil {
				t.Errorf("%v", err)
			}
			if r.Method != "GET" {
				t.Errorf("Unexpected method: expected: GET got: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			w.Write(data)
		})

		config := &github_util.Config{
			Org:     "o",
			Project: "r",
		}
		config.SetClient(client)
		config.BotName = testBotName

		c := CherrypickAutoApprove{}
		err := c.Initialize(config, nil)
		if err != nil {
			t.Fatalf("%v", err)
		}

		err = c.EachLoop()
		if err != nil {
			t.Fatalf("%v", err)
		}

		obj, err := config.GetObject(*test.issue.Number)
		if err != nil {
			t.Fatalf("%v", err)
		}

		c.Munge(obj)
		if test.shouldHaveLabel != "" && !obj.HasLabel(test.shouldHaveLabel) {
			t.Errorf("%d:%q: missing label %q", testNum, test.name, test.shouldHaveLabel)
		}
		milestone, ok := obj.ReleaseMilestone()
		if !ok {
			t.Errorf("%d:%q: error getting obj.ReleaseMilestone", testNum, test.name)
		}
		if test.shouldHaveMilestone != "" && milestone != test.shouldHaveMilestone {
			t.Errorf("%d:%q: missing milestone %q", testNum, test.name, test.shouldHaveMilestone)
		}
		if test.shouldNotHaveLabel != "" && obj.HasLabel(test.shouldNotHaveLabel) {
			t.Errorf("%d:%q: extra label %q", testNum, test.name, test.shouldNotHaveLabel)
		}
		if test.shouldNotHaveMile != "" && milestone == test.shouldNotHaveMile {
			t.Errorf("%d:%q: extra milestone %q", testNum, test.name, test.shouldNotHaveMile)
		}

		server.Close()
	}
}

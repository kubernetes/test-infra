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

package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	github_test "k8s.io/contrib/mungegithub/github/testing"

	"github.com/google/go-github/github"
)

func stringPtr(val string) *string     { return &val }
func timePtr(val time.Time) *time.Time { return &val }
func intPtr(val int) *int              { return &val }

func TestHasLabel(t *testing.T) {
	tests := []struct {
		obj      MungeObject
		label    string
		hasLabel bool
	}{
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"foo"}, true),
			},
			label:    "foo",
			hasLabel: true,
		},
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"bar"}, true),
			},
			label:    "foo",
			hasLabel: false,
		},
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"bar", "foo"}, true),
			},
			label:    "foo",
			hasLabel: true,
		},
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"bar", "baz"}, true),
			},
			label:    "foo",
			hasLabel: false,
		},
	}

	for _, test := range tests {
		if test.hasLabel != test.obj.HasLabel(test.label) {
			t.Errorf("Unexpected output: %v", test)
		}
	}
}

func TestHasLabels(t *testing.T) {
	tests := []struct {
		obj        MungeObject
		seekLabels []string
		hasLabel   bool
	}{
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"foo"}, true),
			},
			seekLabels: []string{"foo"},
			hasLabel:   true,
		},
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"bar"}, true),
			},
			seekLabels: []string{"foo"},
			hasLabel:   false,
		},
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"bar", "foo"}, true),
			},
			seekLabels: []string{"foo"},
			hasLabel:   true,
		},
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"bar", "baz"}, true),
			},
			seekLabels: []string{"foo"},
			hasLabel:   false,
		},
		{
			obj: MungeObject{
				Issue: github_test.Issue("", 1, []string{"foo"}, true),
			},
			seekLabels: []string{"foo", "bar"},
			hasLabel:   false,
		},
	}

	for _, test := range tests {
		if test.hasLabel != test.obj.HasLabels(test.seekLabels) {
			t.Errorf("Unexpected output: %v", test)
		}
	}
}

func TestForEachIssueDo(t *testing.T) {
	issue1 := github_test.Issue("bob", 1, nil, true)
	issue5 := github_test.Issue("bob", 5, nil, true)
	issue6 := github_test.Issue("bob", 6, nil, true)
	issue7 := github_test.Issue("bob", 7, nil, true)
	issue20 := github_test.Issue("bob", 20, nil, true)

	user := github.User{Login: stringPtr("bob")}
	tests := []struct {
		Issues      [][]github.Issue
		Pages       []int
		ValidIssues int
	}{
		{
			Issues: [][]github.Issue{
				{*issue5},
			},
			Pages:       []int{0},
			ValidIssues: 1,
		},
		{
			Issues: [][]github.Issue{
				{*issue5},
				{*issue6},
				{*issue7},
				{
					{
						Number: intPtr(8),
						// no User, invalid
					},
				},
			},
			Pages:       []int{4, 4, 4, 0},
			ValidIssues: 3,
		},
		{
			Issues: [][]github.Issue{
				// Invalid 1 < MinPRNumber
				// Invalid 20 > MaxPRNumber
				{*issue1, *issue20},
				// two valid issues
				{*issue5, *issue6},
				{
					{
						// no Number, invalid
						User: &user,
					},
				},
			},
			Pages:       []int{3, 3, 0},
			ValidIssues: 2,
		},
	}

	for i, test := range tests {
		client, server, mux := github_test.InitServer(t, nil, nil, nil, nil, nil)
		config := &Config{
			client:      client,
			Org:         "foo",
			Project:     "bar",
			MinPRNumber: 5,
			MaxPRNumber: 15,
		}
		count := 0
		mux.HandleFunc("/repos/foo/bar/issues", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			// this means page 0, return page 1
			page := r.URL.Query().Get("page")
			if page == "" {
				t.Errorf("Should not get page 0, start with page 1")
			}
			if page != strconv.Itoa(count+1) {
				t.Errorf("Unexpected page: %s", r.URL.Query().Get("page"))
			}
			if r.URL.Query().Get("sort") != "created" {
				t.Errorf("Unexpected sort: %s", r.URL.Query().Get("sort"))
			}
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("Unexpected per_page: %s", r.URL.Query().Get("per_page"))
			}
			w.Header().Add("Link",
				fmt.Sprintf("<https://api.github.com/?page=%d>; rel=\"last\"", test.Pages[count]))
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(test.Issues[count])
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			w.Write(data)
			count++
		})
		objects := []*MungeObject{}
		handle := func(obj *MungeObject) error {
			objects = append(objects, obj)
			return nil
		}
		err := config.ForEachIssueDo(handle)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(objects) != test.ValidIssues {
			t.Errorf("Test: %d Unexpected output %d vs %d", i, len(objects), test.ValidIssues)
		}

		if count != len(test.Issues) {
			t.Errorf("Test: %d Unexpected number of fetches: %d", i, count)
		}
		server.Close()
	}
}

func TestComputeStatus(t *testing.T) {
	contextS := []string{"context"}
	otherS := []string{"other context"}
	bothS := []string{"context", "other context"}
	firstS := []string{"context", "crap"}

	tests := []struct {
		combinedStatus   *github.CombinedStatus
		requiredContexts []string
		expected         string
	}{
		// test no context specified
		{
			combinedStatus: github_test.Status("mysha", nil, nil, nil, nil),
			expected:       "success",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: stringPtr("pending"),
				SHA:   stringPtr("mysha"),
			},
			expected: "pending",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: stringPtr("failure"),
				SHA:   stringPtr("mysha"),
			},
			expected: "failure",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: stringPtr("error"),
				SHA:   stringPtr("mysha"),
			},
			expected: "error",
		},
		// test missing subcontext requested but missing
		{
			combinedStatus:   github_test.Status("mysha", otherS, nil, nil, nil),
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		{
			combinedStatus:   github_test.Status("mysha", nil, otherS, nil, nil),
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		{
			combinedStatus:   github_test.Status("mysha", nil, nil, otherS, nil),
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		{
			combinedStatus:   github_test.Status("mysha", nil, nil, nil, otherS),
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		// test subcontext present and requested
		{
			combinedStatus:   github_test.Status("mysha", contextS, nil, nil, nil),
			requiredContexts: contextS,
			expected:         "success",
		},
		{
			combinedStatus:   github_test.Status("mysha", nil, nil, contextS, nil),
			requiredContexts: contextS,
			expected:         "pending",
		},
		{
			combinedStatus:   github_test.Status("mysha", nil, nil, nil, contextS),
			requiredContexts: contextS,
			expected:         "error",
		},
		{
			combinedStatus:   github_test.Status("mysha", nil, contextS, nil, nil),
			requiredContexts: contextS,
			expected:         "failure",
		},
		// test failed PR but the one we care about is passed
		{
			combinedStatus:   github_test.Status("mysha", contextS, otherS, nil, nil),
			requiredContexts: contextS,
			expected:         "success",
		},
		// test failed because we need both, but one is failed
		{
			combinedStatus:   github_test.Status("mysha", contextS, otherS, nil, nil),
			requiredContexts: bothS,
			expected:         "failure",
		},
		// test failed because we need both, bot one isn't present
		{
			combinedStatus:   github_test.Status("mysha", firstS, nil, nil, nil),
			requiredContexts: bothS,
			expected:         "incomplete",
		},
	}

	for _, test := range tests {
		// ease of use, reduce boilerplate in test cases
		if test.requiredContexts == nil {
			test.requiredContexts = []string{}
		}
		status := computeStatus(test.combinedStatus, test.requiredContexts)
		if test.expected != status {
			t.Errorf("expected: %s, saw %s for %v", test.expected, status, test.combinedStatus)
		}
	}
}

func TestGetLastModified(t *testing.T) {
	tests := []struct {
		commits      []github.RepositoryCommit
		expectedTime *time.Time
	}{
		{
			commits:      github_test.Commits(1, 10),
			expectedTime: timePtr(time.Unix(10, 0)),
		},
		{
			// remember the order of github_test.Commits() is non-deterministic
			commits:      github_test.Commits(3, 10),
			expectedTime: timePtr(time.Unix(12, 0)),
		},
		{
			// so this is probably not quite the same test...
			commits:      github_test.Commits(3, 8),
			expectedTime: timePtr(time.Unix(10, 0)),
		},
		{
			//  We can't represent the same time in 2 commits using github_test.Commits()
			commits: []github.RepositoryCommit{
				{
					SHA: stringPtr("mysha1"),
					Commit: &github.Commit{
						SHA: stringPtr("mysha1"),
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(9, 0)),
						},
					},
				},
				{
					SHA: stringPtr("mysha2"),
					Commit: &github.Commit{
						SHA: stringPtr("mysha2"),
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(10, 0)),
						},
					},
				},
				{
					SHA: stringPtr("mysha3"),
					Commit: &github.Commit{
						SHA: stringPtr("mysha3"),
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(9, 0)),
						},
					},
				},
			},
			expectedTime: timePtr(time.Unix(10, 0)),
		},
	}
	for _, test := range tests {
		client, server, _ := github_test.InitServer(t, nil, nil, nil, test.commits, nil)
		config := &Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		obj := &MungeObject{
			config: config,
			Issue:  github_test.Issue("bob", 1, nil, true),
		}
		ts := obj.LastModifiedTime()
		if !ts.Equal(*test.expectedTime) {
			t.Errorf("expected: %v, saw: %v for: %v", test.expectedTime, ts, test)
		}
		server.Close()
	}
}

func TestRemoveLabel(t *testing.T) {
	tests := []struct {
		issue    *github.Issue
		remove   string
		expected []string
	}{
		{
			issue:    github_test.Issue("", 1, []string{"label1"}, false),
			remove:   "label1",
			expected: []string{},
		},
		{
			issue:    github_test.Issue("", 1, []string{"label2", "label1"}, false),
			remove:   "label1",
			expected: []string{"label2"},
		},
		{
			issue:    github_test.Issue("", 1, []string{"label2"}, false),
			remove:   "label1",
			expected: []string{"label2"},
		},
		{
			issue:    github_test.Issue("", 1, []string{}, false),
			remove:   "label1",
			expected: []string{},
		},
	}
	for testNum, test := range tests {
		client, server, mux := github_test.InitServer(t, test.issue, nil, nil, nil, nil)
		config := &Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)
		mux.HandleFunc(fmt.Sprintf("/repos/o/r/issues/1/labels/%s", test.remove), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		obj, err := config.GetObject(*test.issue.Number)
		if err != nil {
			t.Fatalf("%d: unable to get issue: %v", testNum, *test.issue.Number)
		}
		obj.RemoveLabel(test.remove)
		if len(test.expected) != len(obj.Issue.Labels) {
			t.Errorf("%d: len(labels) not equal, expected labels: %v but got labels: %v", testNum, test.expected, obj.Issue.Labels)
			return
		}
		for i, l := range test.expected {
			if l != *obj.Issue.Labels[i].Name {
				t.Errorf("%d: expected labels: %v but got labels: %v", testNum, test.expected, obj.Issue.Labels)
			}
		}
		server.Close()
	}
}

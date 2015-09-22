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
	"strings"
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
		labels   []github.Label
		label    string
		hasLabel bool
	}{
		{
			labels: []github.Label{
				{Name: stringPtr("foo")},
			},
			label:    "foo",
			hasLabel: true,
		},
		{
			labels: []github.Label{
				{Name: stringPtr("bar")},
			},
			label:    "foo",
			hasLabel: false,
		},
		{
			labels: []github.Label{
				{Name: stringPtr("bar")},
				{Name: stringPtr("foo")},
			},
			label:    "foo",
			hasLabel: true,
		},
		{
			labels: []github.Label{
				{Name: stringPtr("bar")},
				{Name: stringPtr("baz")},
			},
			label:    "foo",
			hasLabel: false,
		},
	}

	for _, test := range tests {
		if test.hasLabel != HasLabel(test.labels, test.label) {
			t.Errorf("Unexpected output: %v", test)
		}
	}
}

func TestHasLabels(t *testing.T) {
	tests := []struct {
		labels     []github.Label
		seekLabels []string
		hasLabel   bool
	}{
		{
			labels: []github.Label{
				{Name: stringPtr("foo")},
			},
			seekLabels: []string{"foo"},
			hasLabel:   true,
		},
		{
			labels: []github.Label{
				{Name: stringPtr("bar")},
			},
			seekLabels: []string{"foo"},
			hasLabel:   false,
		},
		{
			labels: []github.Label{
				{Name: stringPtr("bar")},
				{Name: stringPtr("foo")},
			},
			seekLabels: []string{"foo"},
			hasLabel:   true,
		},
		{
			labels: []github.Label{
				{Name: stringPtr("bar")},
				{Name: stringPtr("baz")},
			},
			seekLabels: []string{"foo"},
			hasLabel:   false,
		},
		{
			labels: []github.Label{
				{Name: stringPtr("foo")},
			},
			seekLabels: []string{"foo", "bar"},
			hasLabel:   false,
		},
	}

	for _, test := range tests {
		if test.hasLabel != HasLabels(test.labels, test.seekLabels) {
			t.Errorf("Unexpected output: %v", test)
		}
	}
}

// For getting an initializied int pointer.
func intp(i int) *int          { return &i }
func stringp(s string) *string { return &s }
func boolp(b bool) *bool       { return &b }

func PR(num int, merged bool) github.PullRequest {
	pr := github.PullRequest{
		Title:     stringp("Test PR Title"),
		Number:    intp(num),
		Merged:    boolp(merged),
		Mergeable: boolp(true),
	}
	return pr
}

func TestForEachPRDo(t *testing.T) {
	prlinks := github.PullRequestLinks{}
	user := github.User{Login: stringp("bob")}
	tests := []struct {
		Issues   [][]github.Issue
		PRs      map[int]github.PullRequest
		Pages    []int
		ValidPRs int
	}{
		{
			Issues: [][]github.Issue{
				{
					{
						PullRequestLinks: &prlinks,
						Number:           intp(1),
						User:             &user,
					},
				},
			},
			PRs: map[int]github.PullRequest{
				1: PR(1, false),
			},
			Pages:    []int{0},
			ValidPRs: 1,
		},
		{
			Issues: [][]github.Issue{
				{
					{
						Number: intp(1),
						User:   &user,
					},
				},
				{
					{
						PullRequestLinks: &prlinks,
						Number:           intp(2),
						User:             &user,
					},
				},
				{
					{
						PullRequestLinks: &prlinks,
						Number:           intp(3),
						User:             &user,
					},
				},
				{
					{
						PullRequestLinks: &prlinks,
						Number:           intp(4),
						User:             &user,
					},
				},
			},
			PRs: map[int]github.PullRequest{
				2: PR(2, false),
				3: PR(3, true),
				4: PR(4, false),
			},
			Pages:    []int{4, 4, 4, 0},
			ValidPRs: 2,
		},
		{
			Issues: [][]github.Issue{
				{
					{
						PullRequestLinks: &prlinks,
						Number:           intp(1),
						User:             &user,
					},
				},
				{
					{
						PullRequestLinks: &prlinks,
						Number:           intp(2),
						User:             &user,
					},
				},
				{
					{
						PullRequestLinks: &prlinks,
						Number:           intp(3),
						User:             &user,
					},
					{
						PullRequestLinks: &prlinks,
						Number:           intp(4),
						User:             &user,
					},
					{
						PullRequestLinks: &prlinks,
						Number:           intp(5),
						User:             &user,
					},
				},
			},
			PRs: map[int]github.PullRequest{
				1: PR(1, false),
				2: PR(2, false),
				3: PR(3, false),
				4: PR(4, true),
				5: PR(5, false),
			},
			Pages:    []int{3, 3, 0},
			ValidPRs: 4,
		},
	}

	for _, test := range tests {
		client, server, mux := github_test.InitTest()
		config := &GithubConfig{
			client:      client,
			Org:         "foo",
			Project:     "bar",
			MaxPRNumber: 32768,
		}
		mux.HandleFunc("/repos/foo/bar/pulls/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			prNumS := strings.TrimPrefix(r.URL.Path, "/repos/foo/bar/pulls/")
			prNum, _ := strconv.Atoi(prNumS)
			data, err := json.Marshal(test.PRs[prNum])
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			w.Write(data)
		})
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
			if r.URL.Query().Get("per_page") != "20" {
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
		prs := []*github.PullRequest{}
		handle := func(pr *github.PullRequest, issue *github.Issue) error {
			prs = append(prs, pr)
			return nil
		}
		err := config.ForEachPRDo([]string{}, handle)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(prs) != test.ValidPRs {
			t.Errorf("unexpected output %d vs %d", len(prs), test.ValidPRs)
		}

		if count != len(test.Issues) {
			t.Errorf("unexpected number of fetches: %d", count)
		}
		server.Close()
	}
}

func TestComputeStatus(t *testing.T) {
	success := stringPtr("success")
	failure := stringPtr("failure")
	errorp := stringPtr("error")
	pending := stringPtr("pending")
	sha := stringPtr("abcdef")
	contextS := []string{"context"}
	context := stringPtr("context")

	tests := []struct {
		combinedStatus   *github.CombinedStatus
		requiredContexts []string
		expected         string
	}{
		// test no context specified
		{
			combinedStatus: &github.CombinedStatus{
				State: success,
				SHA:   sha,
			},
			expected: "success",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: pending,
				SHA:   sha,
			},
			expected: "pending",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: failure,
				SHA:   sha,
			},
			expected: "failure",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: errorp,
				SHA:   sha,
			},
			expected: "error",
		},
		// test missing subcontext requested but missing
		{
			combinedStatus: &github.CombinedStatus{
				State: success,
				SHA:   sha,
			},
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: pending,
				SHA:   sha,
			},
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: failure,
				SHA:   sha,
			},
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: errorp,
				SHA:   sha,
			},
			requiredContexts: contextS,
			expected:         "incomplete",
		},
		// test subcontext present and requested
		{
			combinedStatus: &github.CombinedStatus{
				State: success,
				SHA:   sha,
				Statuses: []github.RepoStatus{
					{Context: context, State: success},
				},
			},
			requiredContexts: contextS,
			expected:         "success",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: pending,
				SHA:   sha,
				Statuses: []github.RepoStatus{
					{Context: context, State: pending},
				},
			},
			requiredContexts: contextS,
			expected:         "pending",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: errorp,
				SHA:   sha,
				Statuses: []github.RepoStatus{
					{Context: context, State: errorp},
				},
			},
			requiredContexts: contextS,
			expected:         "error",
		},
		{
			combinedStatus: &github.CombinedStatus{
				State: failure,
				SHA:   sha,
				Statuses: []github.RepoStatus{
					{Context: context, State: failure},
				},
			},
			requiredContexts: contextS,
			expected:         "failure",
		},
		// test failed PR but the one we care about is passed
		{
			combinedStatus: &github.CombinedStatus{
				State: failure,
				SHA:   sha,
				Statuses: []github.RepoStatus{
					{Context: context, State: success},
					{Context: stringPtr("other status"), State: failure},
				},
			},
			requiredContexts: contextS,
			expected:         "success",
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
			commits: []github.RepositoryCommit{
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(10, 0)),
						},
					},
				},
			},
			expectedTime: timePtr(time.Unix(10, 0)),
		},
		{
			commits: []github.RepositoryCommit{
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(10, 0)),
						},
					},
				},
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(11, 0)),
						},
					},
				},
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(12, 0)),
						},
					},
				},
			},
			expectedTime: timePtr(time.Unix(12, 0)),
		},
		{
			commits: []github.RepositoryCommit{
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(10, 0)),
						},
					},
				},
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(9, 0)),
						},
					},
				},
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(8, 0)),
						},
					},
				},
			},
			expectedTime: timePtr(time.Unix(10, 0)),
		},
		{
			commits: []github.RepositoryCommit{
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(9, 0)),
						},
					},
				},
				{
					Commit: &github.Commit{
						Committer: &github.CommitAuthor{
							Date: timePtr(time.Unix(10, 0)),
						},
					},
				},
				{
					Commit: &github.Commit{
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
		client, server, mux := github_test.InitTest()
		config := &GithubConfig{
			client:  client,
			Org:     "o",
			Project: "r",
		}
		mux.HandleFunc(fmt.Sprintf("/repos/o/r/pulls/1/commits"), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(test.commits)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
			ts, err := config.LastModifiedTime(1)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !ts.Equal(*test.expectedTime) {
				t.Errorf("expected: %v, saw: %v", test.expectedTime, ts)
			}
		})
		server.Close()
	}
}

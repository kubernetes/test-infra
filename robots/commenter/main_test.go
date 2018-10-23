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

package main

import (
	"errors"
	"fmt"
	"k8s.io/test-infra/prow/github"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseHTMLURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		org  string
		repo string
		num  int
		fail bool
	}{
		{
			name: "normal issue",
			url:  "https://github.com/org/repo/issues/1234",
			org:  "org",
			repo: "repo",
			num:  1234,
		},
		{
			name: "normal pull",
			url:  "https://github.com/pull-org/pull-repo/pull/5555",
			org:  "pull-org",
			repo: "pull-repo",
			num:  5555,
		},
		{
			name: "different host",
			url:  "ftp://gitlab.whatever/org/repo/issues/6666",
			org:  "org",
			repo: "repo",
			num:  6666,
		},
		{
			name: "string issue",
			url:  "https://github.com/org/repo/issues/future",
			fail: true,
		},
		{
			name: "weird issue",
			url:  "https://gubernator.k8s.io/build/kubernetes-jenkins/logs/ci-kubernetes-e2e-gci-gce/11947/",
			fail: true,
		},
	}

	for _, tc := range cases {
		org, repo, num, err := parseHTMLURL(tc.url)
		if err != nil && !tc.fail {
			t.Errorf("%s: should not have produced error: %v", tc.name, err)
		} else if err == nil && tc.fail {
			t.Errorf("%s: failed to produce an error", tc.name)
		} else {
			if org != tc.org {
				t.Errorf("%s: org %s != expected %s", tc.name, org, tc.org)
			}
			if repo != tc.repo {
				t.Errorf("%s: repo %s != expected %s", tc.name, repo, tc.repo)
			}
			if num != tc.num {
				t.Errorf("%s: num %d != expected %d", tc.name, num, tc.num)
			}
		}
	}
}

func TestMakeQuery(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		closed     bool
		dur        time.Duration
		expected   []string
		unexpected []string
		err        bool
	}{
		{
			name:       "basic query",
			query:      "hello world",
			expected:   []string{"hello world", "is:open"},
			unexpected: []string{"updated:", "openhello", "worldis"},
		},
		{
			name:       "basic closed",
			query:      "hello world",
			closed:     true,
			expected:   []string{"hello world"},
			unexpected: []string{"is:open"},
		},
		{
			name:     "basic duration",
			query:    "hello",
			dur:      1 * time.Hour,
			expected: []string{"hello", "updated:<"},
		},
		{
			name:       "weird characters not escaped",
			query:      "oh yeah!@#$&*()",
			expected:   []string{"!", "@", "#", " "},
			unexpected: []string{"%", "+"},
		},
		{
			name:   "include closed with is:open query errors",
			query:  "hello is:open",
			closed: true,
			err:    true,
		},
		{
			name:  "is:closed without includeClosed",
			query: "hello is:closed",
			err:   true,
		},
	}

	for _, tc := range cases {
		actual, err := makeQuery(tc.query, tc.closed, tc.dur)
		if err != nil && !tc.err {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		} else if err == nil && tc.err {
			t.Errorf("%s: failed to raise an error", tc.name)
		}
		for _, e := range tc.expected {
			if !strings.Contains(actual, e) {
				t.Errorf("%s: could not find %s in %s", tc.name, e, actual)
			}
		}
		for _, u := range tc.unexpected {
			if strings.Contains(actual, u) {
				t.Errorf("%s: should not have found %s in %s", tc.name, u, actual)
			}
		}
	}
}

func makeIssue(owner, repo string, number int, title string) github.Issue {
	return github.Issue{
		HTMLURL: fmt.Sprintf("fake://localhost/%s/%s/pull/%d", owner, repo, number),
		Title:   title,
	}
}

type fakeClient struct {
	comments []int
	issues   []github.Issue
}

// Fakes Creating a client, using the same signature as github.Client
func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	if strings.Contains(comment, "error") || repo == "error" {
		return errors.New(comment)
	}
	c.comments = append(c.comments, number)
	return nil
}

// Fakes searching for issues, using the same signature as github.Client
func (c *fakeClient) FindIssues(query, sort string, asc bool) ([]github.Issue, error) {
	if strings.Contains(query, "error") {
		return nil, errors.New(query)
	}
	ret := []github.Issue{}
	for _, i := range c.issues {
		if strings.Contains(i.Title, query) {
			ret = append(ret, i)
		}
	}
	return ret, nil
}

func TestRun(t *testing.T) {
	manyIssues := []github.Issue{}
	manyComments := []int{}
	for i := 0; i < 100; i++ {
		manyIssues = append(manyIssues, makeIssue("o", "r", i, "many "+strconv.Itoa(i)))
		manyComments = append(manyComments, i)
	}

	cases := []struct {
		name     string
		query    string
		comment  string
		template bool
		ceiling  int
		client   fakeClient
		expected []int
		err      bool
	}{
		{
			name:     "find all",
			query:    "many",
			comment:  "found you",
			client:   fakeClient{issues: manyIssues},
			expected: manyComments,
		},
		{
			name:     "find first 10",
			query:    "many",
			ceiling:  10,
			comment:  "hey",
			client:   fakeClient{issues: manyIssues},
			expected: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		},
		{
			name:    "find none",
			query:   "none",
			comment: "this should not happen",
			client:  fakeClient{issues: manyIssues},
		},
		{
			name:    "search error",
			query:   "this search should error",
			comment: "comment",
			client:  fakeClient{issues: manyIssues},
			err:     true,
		},
		{
			name:    "comment error",
			query:   "problematic",
			comment: "rolo tomassi",
			client: fakeClient{issues: []github.Issue{
				makeIssue("o", "r", 1, "problematic this should work"),
				makeIssue("o", "error", 2, "problematic expect an error"),
				makeIssue("o", "r", 3, "problematic works as well"),
			}},
			err:      true,
			expected: []int{1, 3},
		},
		{
			name:     "template comment",
			query:    "67",
			client:   fakeClient{issues: manyIssues},
			comment:  "https://gubernator.k8s.io/pr/{{.Org}}/{{.Repo}}/{{.Number}}",
			template: true,
			expected: []int{67},
		},
		{
			name:     "bad template errors",
			query:    "67",
			client:   fakeClient{issues: manyIssues},
			comment:  "Bad {{.UnknownField}}",
			template: true,
			err:      true,
		},
	}

	for _, tc := range cases {
		ignoreSorting := ""
		ignoreOrder := false
		err := run(&tc.client, tc.query, ignoreSorting, ignoreOrder, makeCommenter(tc.comment, tc.template), tc.ceiling)
		if tc.err && err == nil {
			t.Errorf("%s: failed to received an error", tc.name)
			continue
		}
		if !tc.err && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
			continue
		}
		if len(tc.expected) != len(tc.client.comments) {
			t.Errorf("%s: expected comments %v != actual %v", tc.name, tc.expected, tc.client.comments)
			continue
		}
		missing := []int{}
		for _, e := range tc.expected {
			found := false
			for _, cmt := range tc.client.comments {
				if cmt == e {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, e)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s: missing %v from actual comments %v", tc.name, missing, tc.client.comments)
		}
	}
}

func TestMakeCommenter(t *testing.T) {
	m := meta{
		Number: 10,
		Org:    "org",
		Repo:   "repo",
		Issue: github.Issue{
			Number:  10,
			HTMLURL: "url",
			Title:   "title",
		},
	}
	cases := []struct {
		name     string
		comment  string
		template bool
		expected string
		err      bool
	}{
		{
			name:     "string works",
			comment:  "hello world {{.Number}} {{.Invalid}}",
			expected: "hello world {{.Number}} {{.Invalid}}",
		},
		{
			name:     "template works",
			comment:  "N={{.Number}} R={{.Repo}} O={{.Org}} U={{.Issue.HTMLURL}} T={{.Issue.Title}}",
			template: true,
			expected: "N=10 R=repo O=org U=url T=title",
		},
		{
			name:     "bad template errors",
			comment:  "Bad {{.UnknownField}} Template",
			expected: "Bad ",
			template: true,
			err:      true,
		},
	}

	for _, tc := range cases {
		c := makeCommenter(tc.comment, tc.template)
		actual, err := c(m)
		if actual != tc.expected {
			t.Errorf("%s: expected '%s' != actual '%s'", tc.name, tc.expected, actual)
		}
		if err != nil && !tc.err {
			t.Errorf("%s: unexpected err: %v", tc.name, err)
		}
		if err == nil && tc.err {
			t.Errorf("%s: failed to raise an exception", tc.name)
		}
	}
}

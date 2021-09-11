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
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/test-infra/prow/github"
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
		archived   bool
		closed     bool
		locked     bool
		dur        time.Duration
		expected   []string
		unexpected []string
		err        bool
	}{
		{
			name:       "basic query",
			query:      "hello world",
			expected:   []string{"hello world", "is:open", "archived:false"},
			unexpected: []string{"updated:", "openhello", "worldis"},
		},
		{
			name:       "basic archived",
			query:      "hello world",
			archived:   true,
			expected:   []string{"hello world", "is:open", "is:unlocked"},
			unexpected: []string{"archived:false"},
		},
		{
			name:       "basic closed",
			query:      "hello world",
			closed:     true,
			expected:   []string{"hello world", "archived:false", "is:unlocked"},
			unexpected: []string{"is:open"},
		},
		{
			name:       "basic locked",
			query:      "hello world",
			locked:     true,
			expected:   []string{"hello world", "is:open", "archived:false"},
			unexpected: []string{"is:unlocked"},
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
			name:     "linebreaks are replaced by whitespaces",
			query:    "label:foo\nlabel:bar",
			expected: []string{"label:foo label:bar"},
		},
		{
			name:   "include closed with is:open query errors",
			query:  "hello is:open",
			closed: true,
			err:    true,
		},
		{
			name:     "archived:false with include-archived errors",
			query:    "hello archived:false",
			archived: true,
			err:      true,
		},
		{
			name:  "archived:true without includeArchived errors",
			query: "hello archived:true",
			err:   true,
		},
		{
			name:  "is:closed without includeClosed errors",
			query: "hello is:closed",
			err:   true,
		},
		{
			name:  "is:locked without includeLocked errors",
			query: "hello is:locked",
			err:   true,
		},
		{
			name:   "is:unlocked with includeLocked errors",
			query:  "hello is:unlocked",
			locked: true,
			err:    true,
		},
	}

	for _, tc := range cases {
		actual, err := makeQuery(tc.query, tc.archived, tc.closed, tc.locked, tc.dur)
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

const (
	fakeOrg      = "fakeOrg"
	fakeRepo     = "fakeRepo"
	fakeTitle    = "fakeTitle"
	fakeComment  = "fakeComment"
	fakePRNumber = 67
)

func makeIssue(owner, repo string, number int, title string) github.Issue {
	return github.Issue{
		HTMLURL: fmt.Sprintf("fake://localhost/%s/%s/pull/%d", owner, repo, number),
		Title:   title,
	}
}

func makeComment(comment string) github.IssueComment {
	return github.IssueComment{
		Body:      comment,
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
}

type fakeClient struct {
	comments map[int][]github.IssueComment
	issues   []github.Issue
}

// Fakes Creating a client, using the same signature as github.Client
func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	if strings.Contains(comment, "error") || repo == "error" {
		return errors.New(comment)
	}

	c.comments[number] = append(c.comments[number], makeComment(comment))

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

// Fakes searching for issues comments using the same signature as github.Client
func (c *fakeClient) ListIssueCommentsSince(org, repo string, number int, since time.Time) ([]github.IssueComment, error) {
	var comments []github.IssueComment
	for _, comment := range c.comments[number] {
		if comment.CreatedAt.Before(since) {
			continue
		}
		comments = append(comments, comment)
	}

	return comments, nil
}

func TestRun(t *testing.T) {
	createIssues := func(num int) []github.Issue {
		issues := []github.Issue{}
		for i := 0; i < num; i++ {
			issues = append(issues, makeIssue(fakeOrg, fakeRepo, i, fmt.Sprintf("%s %d", fakeTitle, i)))
		}
		return issues
	}

	createComments := func(num int, comment string, times int) map[int][]string {
		comments := map[int][]string{}
		for i := 0; i < num; i++ {
			for j := 0; j < times; j++ {
				comments[i] = append(comments[i], comment)
			}
		}
		return comments
	}

	manyIssues := createIssues(100)

	cases := []struct {
		name                  string
		query                 string
		comment               string
		template              bool
		ceiling               int
		commentsCeiling       int
		commentsCeilingMargin time.Duration
		client                fakeClient
		expected              map[int][]string
		err                   bool
	}{
		{
			name:     "find all",
			query:    fakeTitle,
			comment:  fakeComment,
			client:   fakeClient{issues: manyIssues},
			expected: createComments(len(manyIssues), fakeComment, 1),
		},
		{
			name:     "find first 10",
			query:    fakeTitle,
			ceiling:  10,
			comment:  fakeComment,
			client:   fakeClient{issues: manyIssues},
			expected: createComments(10, fakeComment, 1),
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
				makeIssue(fakeOrg, fakeRepo, 1, "problematic this should work"),
				makeIssue(fakeOrg, "error", 2, "problematic expect an error"),
				makeIssue(fakeOrg, fakeRepo, 3, "problematic works as well"),
			}},
			err:      true,
			expected: map[int][]string{1: {"rolo tomassi"}, 3: {"rolo tomassi"}},
		},
		{
			name:     "template comment",
			query:    strconv.Itoa(fakePRNumber),
			client:   fakeClient{issues: manyIssues},
			comment:  "https://gubernator.k8s.io/pr/{{.Org}}/{{.Repo}}/{{.Number}}",
			template: true,
			expected: map[int][]string{
				fakePRNumber: {fmt.Sprintf("https://gubernator.k8s.io/pr/%s/%s/%d", fakeOrg, fakeRepo, fakePRNumber)},
			},
		},
		{
			name:     "bad template errors",
			query:    strconv.Itoa(fakePRNumber),
			client:   fakeClient{issues: manyIssues},
			comment:  "Bad {{.UnknownField}}",
			template: true,
			err:      true,
		},
		{
			name:            "high comments ceiling - create new comment",
			query:           fakeTitle,
			commentsCeiling: 5,
			comment:         fakeComment,
			client: fakeClient{
				issues: createIssues(1),
				comments: map[int][]github.IssueComment{
					0: {makeComment(fakeComment), makeComment(fakeComment)},
				},
			},
			expected: createComments(1, fakeComment, 3),
		},
		{
			name:            "comments ceiling - stop creating new comments",
			query:           fakeTitle,
			commentsCeiling: 2,
			comment:         fakeComment,
			client: fakeClient{
				issues: []github.Issue{makeIssue(fakeOrg, fakeRepo, fakePRNumber, fakeTitle)},
				comments: map[int][]github.IssueComment{
					fakePRNumber: {makeComment(fakeComment), makeComment(fakeComment)},
				},
			},
			expected: map[int][]string{
				fakePRNumber: {fakeComment, fakeComment},
			},
		},
		{
			name:            "comments ceiling - don't stop when different comments",
			query:           fakeTitle,
			commentsCeiling: 2,
			comment:         fakeComment,
			client: fakeClient{
				issues: []github.Issue{makeIssue(fakeOrg, fakeRepo, fakePRNumber, fakeTitle)},
				comments: map[int][]github.IssueComment{
					fakePRNumber: {makeComment("hello"), makeComment("world")},
				},
			},
			expected: map[int][]string{
				fakePRNumber: {"hello", "world", fakeComment},
			},
		},
		{
			name:            "comments ceiling - don't stop when not in sequence",
			query:           fakeTitle,
			commentsCeiling: 2,
			comment:         fakeComment,
			client: fakeClient{
				issues: []github.Issue{makeIssue(fakeOrg, fakeRepo, fakePRNumber, fakeTitle)},
				comments: map[int][]github.IssueComment{
					fakePRNumber: {makeComment(fakeComment), makeComment("another"), makeComment(fakeComment), makeComment("one")},
				},
			},
			expected: map[int][]string{
				fakePRNumber: {fakeComment, "another", fakeComment, "one", fakeComment},
			},
		},
		{
			name:                  "comments ceiling - don't stop when small comments ceiling margin",
			query:                 fakeTitle,
			commentsCeiling:       2,
			commentsCeilingMargin: 5 * time.Minute, // comments are created an hour ago
			comment:               fakeComment,
			client: fakeClient{
				issues: []github.Issue{makeIssue(fakeOrg, fakeRepo, fakePRNumber, fakeTitle)},
				comments: map[int][]github.IssueComment{
					fakePRNumber: {makeComment(fakeComment), makeComment(fakeComment)},
				},
			},
			expected: map[int][]string{
				fakePRNumber: {fakeComment, fakeComment, fakeComment},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ignoreSorting := ""
			ignoreOrder := false
			if tc.client.comments == nil {
				tc.client.comments = make(map[int][]github.IssueComment)
			}
			if tc.commentsCeilingMargin == 0 {
				tc.commentsCeilingMargin = 24 * time.Hour
			}
			err := run(&tc.client, tc.query, ignoreSorting, ignoreOrder, false, makeCommenter(tc.comment, tc.template),
				tc.ceiling, tc.commentsCeiling, tc.commentsCeilingMargin)
			if tc.err && err == nil {
				t.Errorf("%s: failed to received an error", tc.name)
			}
			if !tc.err && err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			}
			missing := []string{}
			if len(tc.expected) != len(tc.client.comments) {
				t.Errorf("%s: expected issues with comments %v != actual %v", tc.name, tc.expected, tc.client.comments)
			}
			for number, expectedComments := range tc.expected {
				if len(tc.expected[number]) != len(tc.client.comments[number]) {
					t.Errorf("%s: expected comments %v != actual %v", tc.name, expectedComments, tc.client.comments[number])
				}
				for _, comment := range expectedComments {
					found := false
					for _, actualComment := range tc.client.comments[number] {
						if comment == actualComment.Body {
							found = true
							break
						}
					}

					if !found {
						missing = append(missing, comment)
					}
				}
			}
			if len(missing) > 0 {
				t.Errorf("%s: missing %v from actual comments %v", tc.name, missing, tc.client.comments)
			}
		})
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

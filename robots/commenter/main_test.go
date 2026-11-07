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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	githubql "github.com/shurcooL/githubv4"

	"sigs.k8s.io/prow/pkg/github"
)

func TestFlagOptions(t *testing.T) {
	for _, mode := range []string{"", dedupeNone, dedupeLatest, dedupeAny} {
		t.Run("dedupe="+mode, func(t *testing.T) {
			oldFlags, oldArgs := flag.CommandLine, os.Args
			t.Cleanup(func() {
				flag.CommandLine, os.Args = oldFlags, oldArgs
			})
			flag.CommandLine = flag.NewFlagSet("commenter", flag.ContinueOnError)
			os.Args = []string{"commenter"}
			want := mode
			if mode == "" {
				want = dedupeNone
			} else {
				os.Args = append(os.Args, "--dedupe="+mode)
			}
			if got := flagOptions().dedupe; got != want {
				t.Errorf("dedupe = %q, want %q", got, want)
			}
		})
	}
}

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
			url:  "https://gubernator.k8s.io/build/kubernetes-ci-logs/logs/ci-kubernetes-e2e-gci-gce/11947/",
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
		byActivity bool
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
			name:       "duration by activity filters on creation",
			query:      "hello",
			dur:        1 * time.Hour,
			byActivity: true,
			expected:   []string{"hello", "created:<="},
			unexpected: []string{"updated:"},
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
		actual, err := makeQuery(tc.query, tc.archived, tc.closed, tc.locked, tc.dur, tc.byActivity)
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
		NodeID:  fmt.Sprintf("%s/%s#%d", owner, repo, number),
		HTMLURL: fmt.Sprintf("fake://localhost/%s/%s/pull/%d", owner, repo, number),
		Title:   title,
	}
}

type fakeClient struct {
	comments []int
	issues   []github.Issue
	// existingComments maps an issue number to the comments already on it.
	existingComments map[int][]github.IssueComment
	// nodes maps an issue node ID to the activity GraphQL returns for it.
	nodes map[string]activityNode
	// queries records the node IDs of each GraphQL query.
	queries [][]string
	// since records the $since of each GraphQL query.
	since []time.Time
}

// Fakes Creating a client, using the same signature as github.Client
func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	if strings.Contains(comment, "error") || repo == "error" {
		return errors.New(comment)
	}
	c.comments = append(c.comments, number)
	return nil
}

// Fakes listing an issue's comments, using the same signature as github.Client
func (c *fakeClient) ListIssueComments(owner, repo string, number int) ([]github.IssueComment, error) {
	if repo == "list-error" {
		return nil, errors.New("list error")
	}
	return c.existingComments[number], nil
}

// Fakes a GraphQL query, using the same signature as github.Client
func (c *fakeClient) QueryWithGitHubAppsSupport(_ context.Context, q interface{}, vars map[string]interface{}, _ string) error {
	aq, ok := q.(*activityQuery)
	if !ok {
		return fmt.Errorf("unexpected query %T", q)
	}
	ids := []string{}
	for _, id := range vars["ids"].([]githubql.ID) {
		ids = append(ids, id.(string))
	}
	c.queries = append(c.queries, ids)
	c.since = append(c.since, vars["since"].(githubql.DateTime).Time)
	for _, id := range ids {
		if strings.Contains(id, "query-error") {
			return errors.New("query error")
		}
		if n, ok := c.nodes[id]; ok {
			aq.Nodes = append(aq.Nodes, n)
		}
	}
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
		dedupe   string
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
			name:    "dedupe=latest only considers the most recent comment",
			query:   "dup",
			comment: "found you",
			dedupe:  dedupeLatest,
			client: fakeClient{
				issues: []github.Issue{
					makeIssue("o", "r", 1, "dup one"),
					makeIssue("o", "r", 2, "dup two"),
					makeIssue("o", "r", 3, "dup three"),
					makeIssue("o", "r", 4, "dup four"),
				},
				existingComments: map[int][]github.IssueComment{
					// Latest comment is identical: skip.
					2: {{Body: "reopened"}, {Body: "found you\r\n"}},
					3: {{Body: "something else"}},
					// Old identical comment followed by newer ones (e.g. the
					// issue was reopened): comment again.
					4: {{Body: "found you"}, {Body: "reopened"}},
				},
			},
			expected: []int{1, 3, 4},
		},
		{
			name:    "dedupe=any considers all comments",
			query:   "dup",
			comment: "found you",
			dedupe:  dedupeAny,
			client: fakeClient{
				issues: []github.Issue{
					makeIssue("o", "r", 1, "dup one"),
					makeIssue("o", "r", 2, "dup two"),
					makeIssue("o", "r", 3, "dup three"),
					makeIssue("o", "r", 4, "dup four"),
				},
				existingComments: map[int][]github.IssueComment{
					2: {{Body: "reopened"}, {Body: "found you\r\n"}},
					3: {{Body: "something else"}},
					4: {{Body: "found you"}, {Body: "reopened"}},
				},
			},
			expected: []int{1, 3},
		},
		{
			name:    "skipped issues do not count towards the ceiling",
			query:   "dup",
			comment: "found you",
			ceiling: 1,
			dedupe:  dedupeLatest,
			client: fakeClient{
				issues: []github.Issue{
					makeIssue("o", "r", 1, "dup one"),
					makeIssue("o", "r", 2, "dup two"),
				},
				existingComments: map[int][]github.IssueComment{
					1: {{Body: "found you"}},
				},
			},
			expected: []int{2},
		},
		{
			name:    "dedupe=none comments even when listing comments would fail",
			query:   "dup",
			comment: "found you",
			dedupe:  dedupeNone,
			client: fakeClient{
				issues: []github.Issue{makeIssue("o", "list-error", 1, "dup one")},
			},
			expected: []int{1},
		},
		{
			name:    "list comments error",
			query:   "problematic",
			comment: "hey",
			dedupe:  dedupeLatest,
			client: fakeClient{issues: []github.Issue{
				makeIssue("o", "r", 1, "problematic this should work"),
				makeIssue("o", "list-error", 2, "problematic expect a list error"),
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
		if tc.dedupe == "" {
			tc.dedupe = dedupeNone
		}
		ignoreSorting := ""
		ignoreOrder := false
		err := run(&tc.client, tc.query, ignoreSorting, ignoreOrder, false, makeCommenter(tc.comment, tc.template), tc.ceiling, tc.dedupe, activityFilter{})
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

func TestRunCeiling(t *testing.T) {
	cases := []struct {
		dedupe   string
		expected []int
	}{
		{dedupe: dedupeNone, expected: []int{1, 2, 3, 4, 5, 6}},
		{dedupe: dedupeLatest, expected: []int{2, 4, 5, 6}},
		{dedupe: dedupeAny, expected: []int{4, 5, 6}},
	}
	for _, tc := range cases {
		for _, ceiling := range []int{0, 1, 2, 3, 4, 6} {
			t.Run(fmt.Sprintf("dedupe=%s/ceiling=%d", tc.dedupe, ceiling), func(t *testing.T) {
				c := fakeClient{
					issues: []github.Issue{
						makeIssue("o", "r", 1, "dup one"),
						makeIssue("o", "r", 2, "dup two"),
						makeIssue("o", "r", 3, "dup three"),
						makeIssue("o", "r", 4, "dup four"),
						makeIssue("o", "r", 5, "dup five"),
						makeIssue("o", "r", 6, "dup six"),
					},
					existingComments: map[int][]github.IssueComment{
						1: {{Body: "found you\r\n"}},
						// An older identical comment must not prevent commenting.
						2: {{Body: "found you"}, {Body: "reopened"}},
						// A skip between eligible issues must not consume the ceiling.
						3: {{Body: "something else"}, {Body: "found you"}},
						4: {{Body: "something else"}},
					},
				}
				want := tc.expected
				if ceiling > 0 && ceiling < len(want) {
					want = want[:ceiling]
				}
				if err := run(&c, "dup", "", false, false, makeCommenter("found you", false), ceiling, tc.dedupe, activityFilter{}); err != nil {
					t.Fatalf("run: %v", err)
				}
				if !reflect.DeepEqual(c.comments, want) {
					t.Errorf("commented on %v, want %v", c.comments, want)
				}
			})
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

var activityNow = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func daysAgo(d int) time.Time {
	return activityNow.Add(-time.Duration(d) * 24 * time.Hour)
}

func commentItem(login string, at time.Time) activityPullRequestItem {
	return activityPullRequestItem{activityIssueItem: activityIssueItem{
		Typename:     "IssueComment",
		IssueComment: activityComment{Author: activityActor{Login: login}, CreatedAt: at},
	}}
}

// prNode returns a pull request whose timeline within the --updated window is items.
func prNode(id string, truncated bool, items ...activityPullRequestItem) activityNode {
	n := activityNode{Typename: "PullRequest"}
	n.PullRequest.ID = id
	n.PullRequest.TimelineItems.PageInfo.HasNextPage = truncated
	n.PullRequest.TimelineItems.Nodes = items
	return n
}

func TestNewActivityFilter(t *testing.T) {
	cases := []struct {
		name    string
		logins  string
		enabled bool
		ignored []string
		counted []string
	}{
		{
			name: "empty disables the filter",
		},
		{
			name:   "only separators disables the filter",
			logins: " , ,",
		},
		{
			name:    "logins are case insensitive and trimmed",
			logins:  " K8s-CI-Robot ,netlify",
			enabled: true,
			ignored: []string{"k8s-ci-robot", "NETLIFY"},
			counted: []string{"k8s-triage-robot", "someone"},
		},
		{
			name:    "GitHub Apps match with and without the [bot] suffix",
			logins:  "kubernetes-prow[bot],netlify",
			enabled: true,
			ignored: []string{"kubernetes-prow", "kubernetes-prow[bot]", "netlify[bot]"},
			counted: []string{"kubernetes"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newActivityFilter(tc.logins, time.Hour, activityNow)
			if got := f.enabled(); got != tc.enabled {
				t.Errorf("enabled() = %t, want %t", got, tc.enabled)
			}
			for _, l := range tc.ignored {
				if !f.ignored[normalizeLogin(l)] {
					t.Errorf("%q is not ignored", l)
				}
			}
			for _, l := range tc.counted {
				if f.ignored[normalizeLogin(l)] {
					t.Errorf("%q is ignored", l)
				}
			}
		})
	}
}

func TestActivityNodeTimeline(t *testing.T) {
	pr := prNode("pr", true,
		commentItem("commenter", daysAgo(9)),
		activityPullRequestItem{
			activityIssueItem: activityIssueItem{Typename: "PullRequestReview"},
			PullRequestReview: activityComment{Author: activityActor{Login: "reviewer"}, CreatedAt: daysAgo(8)},
		},
	)
	pr.PullRequest.TimelineItems.Nodes = append(pr.PullRequest.TimelineItems.Nodes, activityPullRequestItem{})
	commit := &pr.PullRequest.TimelineItems.Nodes[len(pr.PullRequest.TimelineItems.Nodes)-1]
	commit.Typename = "PullRequestCommit"
	commit.PullRequestCommit.Commit.Author.User.Login = "author"
	commit.PullRequestCommit.Commit.CommittedDate = daysAgo(5)

	issue := activityNode{Typename: "Issue"}
	issue.Issue.ID = "issue"
	issue.Issue.TimelineItems.Nodes = []activityIssueItem{
		{
			Typename:     "IssueComment",
			IssueComment: activityComment{Author: activityActor{Login: "commenter"}, CreatedAt: daysAgo(4)},
		},
		// Unexpected item types count as activity by an unknown login.
		{Typename: "LabeledEvent"},
	}

	cases := []struct {
		name string
		node activityNode
		want timeline
	}{
		{
			name: "pull request",
			node: pr,
			want: timeline{
				id: "pr",
				items: []activity{
					{"commenter", daysAgo(9)},
					{"reviewer", daysAgo(8)},
					{"author", daysAgo(5)},
				},
				truncated: true,
			},
		},
		{
			name: "issue",
			node: issue,
			want: timeline{
				id:    "issue",
				items: []activity{{"commenter", daysAgo(4)}, {}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.node.timeline(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("timeline() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestActive(t *testing.T) {
	f := newActivityFilter("k8s-ci-robot,kubernetes-prow[bot]", 30*24*time.Hour, activityNow)
	cases := []struct {
		name string
		t    timeline
		want bool
	}{
		{
			name: "no activity is inactive",
		},
		{
			name: "counted activity is active",
			t:    timeline{items: []activity{{"author", daysAgo(20)}}},
			want: true,
		},
		{
			name: "counted activity followed by ignored activity is active",
			t: timeline{items: []activity{
				{"author", daysAgo(20)},
				{"k8s-ci-robot", daysAgo(10)},
				{"kubernetes-prow", daysAgo(5)},
			}},
			want: true,
		},
		{
			name: "only ignored activity is inactive",
			t: timeline{items: []activity{
				{"k8s-ci-robot", daysAgo(20)},
				{"kubernetes-prow", daysAgo(1)},
			}},
		},
		{
			name: "the triage robot counts so lifecycle steps are spaced out",
			t: timeline{items: []activity{
				{"k8s-triage-robot", daysAgo(20)},
				{"kubernetes-prow", daysAgo(20)},
			}},
			want: true,
		},
		{
			name: "activity by an unknown login is active",
			t:    timeline{items: []activity{{}}},
			want: true,
		},
		{
			name: "only ignored activity in a truncated timeline is active",
			t: timeline{truncated: true, items: []activity{
				{"k8s-ci-robot", daysAgo(20)},
				{"k8s-ci-robot", daysAgo(1)},
			}},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.active(tc.t); got != tc.want {
				t.Errorf("active() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestRunActivityFilter(t *testing.T) {
	month := 30 * 24 * time.Hour
	f := newActivityFilter("k8s-ci-robot", month, activityNow)
	issues := []github.Issue{
		makeIssue("o", "r", 1, "old one"),
		makeIssue("o", "r", 2, "old two"),
		makeIssue("o", "r", 3, "old three"),
		makeIssue("o", "r", 4, "old four"),
	}
	nodes := map[string]activityNode{
		// Inactive, only bot activity.
		"o/r#1": prNode("o/r#1", false, commentItem("k8s-ci-robot", daysAgo(1))),
		// Active.
		"o/r#2": prNode("o/r#2", false, commentItem("author", daysAgo(10))),
		// Inactive, no activity.
		"o/r#3": prNode("o/r#3", false),
		// Skipped, not returned by GitHub.
	}

	cases := []struct {
		name     string
		ceiling  int
		expected []int
	}{
		{name: "active issues are skipped", expected: []int{1, 3}},
		{name: "skipped issues do not count towards the ceiling", ceiling: 1, expected: []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeClient{issues: issues, nodes: nodes}
			if err := run(&c, "old", "", false, false, makeCommenter("stale", false), tc.ceiling, dedupeNone, f); err != nil {
				t.Fatalf("run: %v", err)
			}
			if !reflect.DeepEqual(c.comments, tc.expected) {
				t.Errorf("commented on %v, want %v", c.comments, tc.expected)
			}
			for _, since := range c.since {
				if want := daysAgo(30); !since.Equal(want) {
					t.Errorf("queried since %s, want %s", since, want)
				}
			}
		})
	}

	t.Run("activity is looked up in batches until the ceiling is reached", func(t *testing.T) {
		many := []github.Issue{}
		manyNodes := map[string]activityNode{}
		for i := 0; i < 5*activityBatchSize; i++ {
			issue := makeIssue("o", "r", i, "many "+strconv.Itoa(i))
			many = append(many, issue)
			// Only the last issue of each batch is inactive.
			if i%activityBatchSize == activityBatchSize-1 {
				manyNodes[issue.NodeID] = prNode(issue.NodeID, false)
			} else {
				manyNodes[issue.NodeID] = prNode(issue.NodeID, false, commentItem("author", daysAgo(1)))
			}
		}
		c := fakeClient{issues: many, nodes: manyNodes}
		if err := run(&c, "many", "", false, false, makeCommenter("stale", false), 2, dedupeNone, f); err != nil {
			t.Fatalf("run: %v", err)
		}
		if want := []int{activityBatchSize - 1, 2*activityBatchSize - 1}; !reflect.DeepEqual(c.comments, want) {
			t.Errorf("commented on %v, want %v", c.comments, want)
		}
		if len(c.queries) != 2 || len(c.queries[0]) != activityBatchSize || len(c.queries[1]) != activityBatchSize {
			t.Errorf("queried batches of %v, want 2 batches of %d", func() []int {
				sizes := []int{}
				for _, q := range c.queries {
					sizes = append(sizes, len(q))
				}
				return sizes
			}(), activityBatchSize)
		}
	})

	t.Run("a failing batch is skipped and later batches are processed", func(t *testing.T) {
		many := []github.Issue{}
		manyNodes := map[string]activityNode{}
		for i := 0; i < 2*activityBatchSize; i++ {
			repo := "r"
			if i == 0 {
				repo = "query-error"
			}
			issue := makeIssue("o", repo, i, "many "+strconv.Itoa(i))
			many = append(many, issue)
			manyNodes[issue.NodeID] = prNode(issue.NodeID, false)
		}
		c := fakeClient{issues: many, nodes: manyNodes}
		if err := run(&c, "many", "", false, false, makeCommenter("stale", false), 0, dedupeNone, f); err == nil {
			t.Error("expected an error")
		}
		want := []int{}
		for i := activityBatchSize; i < 2*activityBatchSize; i++ {
			want = append(want, i)
		}
		if !reflect.DeepEqual(c.comments, want) {
			t.Errorf("commented on %v, want %v", c.comments, want)
		}
	})

	t.Run("disabled filter does not query", func(t *testing.T) {
		c := fakeClient{issues: issues}
		if err := run(&c, "old", "", false, false, makeCommenter("stale", false), 0, dedupeNone, activityFilter{}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if len(c.queries) != 0 {
			t.Errorf("queried %v, want no queries", c.queries)
		}
		if want := []int{1, 2, 3, 4}; !reflect.DeepEqual(c.comments, want) {
			t.Errorf("commented on %v, want %v", c.comments, want)
		}
	})
}

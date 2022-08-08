/*
Copyright 2019 The Kubernetes Authors.

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

package tide

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/github"
)

func TestSearch(t *testing.T) {
	const q = "random search string"
	now := time.Now()
	earlier := now.Add(-5 * time.Hour)
	makePRs := func(numbers ...int) []PullRequest {
		var prs []PullRequest
		for _, n := range numbers {
			prs = append(prs, PullRequest{Number: githubql.Int(n)})
		}
		return prs
	}
	makeQuery := func(more bool, cursor string, numbers ...int) searchQuery {
		var sq searchQuery
		sq.Search.PageInfo.HasNextPage = githubql.Boolean(more)
		sq.Search.PageInfo.EndCursor = githubql.String(cursor)
		for _, pr := range makePRs(numbers...) {
			sq.Search.Nodes = append(sq.Search.Nodes, PRNode{pr})
		}
		return sq
	}

	cases := []struct {
		name     string
		start    time.Time
		end      time.Time
		q        string
		cursors  []*githubql.String
		sqs      []searchQuery
		errs     []error
		expected []PullRequest
		err      bool
	}{
		{
			name:    "single page works",
			start:   earlier,
			end:     now,
			q:       datedQuery(q, earlier, now),
			cursors: []*githubql.String{nil},
			sqs: []searchQuery{
				makeQuery(false, "", 1, 2),
			},
			errs:     []error{nil},
			expected: makePRs(1, 2),
		},
		{
			name:    "fail on first page",
			start:   earlier,
			end:     now,
			q:       datedQuery(q, earlier, now),
			cursors: []*githubql.String{nil},
			sqs: []searchQuery{
				{},
			},
			errs: []error{errors.New("injected error")},
			err:  true,
		},
		{
			name:    "set minimum start time",
			start:   time.Time{},
			end:     now,
			q:       datedQuery(q, floor(time.Time{}), now),
			cursors: []*githubql.String{nil},
			sqs: []searchQuery{
				makeQuery(false, "", 1, 2),
			},
			errs:     []error{nil},
			expected: makePRs(1, 2),
		},
		{
			name:  "can handle multiple pages of results",
			start: earlier,
			end:   now,
			q:     datedQuery(q, earlier, now),
			cursors: []*githubql.String{
				nil,
				githubql.NewString("first"),
				githubql.NewString("second"),
			},
			sqs: []searchQuery{
				makeQuery(true, "first", 1, 2),
				makeQuery(true, "second", 3, 4),
				makeQuery(false, "", 5, 6),
			},
			errs:     []error{nil, nil, nil},
			expected: makePRs(1, 2, 3, 4, 5, 6),
		},
		{
			name:  "return partial results on later page failure",
			start: earlier,
			end:   now,
			q:     datedQuery(q, earlier, now),
			cursors: []*githubql.String{
				nil,
				githubql.NewString("first"),
			},
			sqs: []searchQuery{
				makeQuery(true, "first", 1, 2),
				{},
			},
			errs:     []error{nil, errors.New("second page error")},
			expected: makePRs(1, 2),
			err:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &GitHubProvider{}
			var i int
			querier := func(_ context.Context, result interface{}, actual map[string]interface{}, _ string) error {
				expected := map[string]interface{}{
					"query":        githubql.String(tc.q),
					"searchCursor": tc.cursors[i],
				}
				if !equality.Semantic.DeepEqual(expected, actual) {
					t.Errorf("call %d vars do not match:\n%s", i, diff.ObjectReflectDiff(expected, actual))
				}
				ret := result.(*searchQuery)
				err := tc.errs[i]
				sq := tc.sqs[i]
				i++
				if err != nil {
					return err
				}
				*ret = sq
				return nil
			}
			prs, err := client.search(querier, logrus.WithField("test", tc.name), q, tc.start, tc.end, "")
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			}

			if !reflect.DeepEqual(tc.expected, prs) {
				t.Errorf("prs do not match:\n%s", diff.ObjectReflectDiff(tc.expected, prs))
			}
		})
	}
}

func TestPrepareMergeDetails(t *testing.T) {
	pr := PullRequest{
		Number:     githubql.Int(1),
		Mergeable:  githubql.MergeableStateMergeable,
		HeadRefOID: githubql.String("SHA"),
		Title:      "my commit title",
		Body:       "my commit body",
	}

	testCases := []struct {
		name        string
		tpl         config.TideMergeCommitTemplate
		pr          PullRequest
		mergeMethod types.PullRequestMergeType
		expected    github.MergeDetails
	}{{
		name:        "No commit template",
		tpl:         config.TideMergeCommitTemplate{},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:         "SHA",
			MergeMethod: "merge",
		},
	}, {
		name: "No commit template fields",
		tpl: config.TideMergeCommitTemplate{
			Title: nil,
			Body:  nil,
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:         "SHA",
			MergeMethod: "merge",
		},
	}, {
		name: "Static commit template",
		tpl: config.TideMergeCommitTemplate{
			Title: getTemplate("CommitTitle", "static title"),
			Body:  getTemplate("CommitBody", "static body"),
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:           "SHA",
			MergeMethod:   "merge",
			CommitTitle:   "static title",
			CommitMessage: "static body",
		},
	}, {
		name: "Commit template uses PullRequest fields",
		tpl: config.TideMergeCommitTemplate{
			Title: getTemplate("CommitTitle", "{{ .Number }}: {{ .Title }}"),
			Body:  getTemplate("CommitBody", "{{ .HeadRefOID }} - {{ .Body }}"),
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:           "SHA",
			MergeMethod:   "merge",
			CommitTitle:   "1: my commit title",
			CommitMessage: "SHA - my commit body",
		},
	}, {
		name: "Commit template uses nonexistent fields",
		tpl: config.TideMergeCommitTemplate{
			Title: getTemplate("CommitTitle", "{{ .Hello }}"),
			Body:  getTemplate("CommitBody", "{{ .World }}"),
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:         "SHA",
			MergeMethod: "merge",
		},
	}}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfgAgent := &config.Agent{}
			cfgAgent.Set(cfg)
			provider := &GitHubProvider{
				cfg:    cfgAgent.Config,
				ghc:    &fgc{},
				logger: logrus.WithContext(context.Background()),
			}

			actual := provider.prepareMergeDetails(test.tpl, *CodeReviewCommonFromPullRequest(&test.pr), test.mergeMethod)

			if !reflect.DeepEqual(actual, test.expected) {
				t.Errorf("Case %s failed: expected %+v, got %+v", test.name, test.expected, actual)
			}
		})
	}
}

func TestHeadContexts(t *testing.T) {
	type commitContext struct {
		// one context per commit for testing
		context string
		sha     string
	}

	win := "win"
	lose := "lose"
	headSHA := "head"
	testCases := []struct {
		name                string
		commitContexts      []commitContext
		expectAPICall       bool
		expectChecksAPICall bool
	}{
		{
			name: "first commit is head",
			commitContexts: []commitContext{
				{context: win, sha: headSHA},
				{context: lose, sha: "other"},
				{context: lose, sha: "sha"},
			},
		},
		{
			name: "last commit is head",
			commitContexts: []commitContext{
				{context: lose, sha: "shaaa"},
				{context: lose, sha: "other"},
				{context: win, sha: headSHA},
			},
		},
		{
			name: "no commit is head, falling back to v3 api and getting context via status api",
			commitContexts: []commitContext{
				{context: lose, sha: "shaaa"},
				{context: lose, sha: "other"},
				{context: lose, sha: "sha"},
			},
			expectAPICall: true,
		},
		{
			name: "no commit is head, falling back to v3 api and getting context via checks api",
			commitContexts: []commitContext{
				{context: lose, sha: "shaaa"},
				{context: lose, sha: "other"},
				{context: lose, sha: "sha"},
			},
			expectAPICall:       true,
			expectChecksAPICall: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running test case %q", tc.name)
			fgc := &fgc{}
			if !tc.expectChecksAPICall {
				fgc.combinedStatus = map[string]string{win: string(githubql.StatusStateSuccess)}
			} else {
				fgc.checkRuns = &github.CheckRunList{CheckRuns: []github.CheckRun{
					{Name: win, Status: "completed", Conclusion: "neutral"},
				}}
			}
			if tc.expectAPICall {
				fgc.expectedSHA = headSHA
			}
			provider := &GitHubProvider{
				ghc:    fgc,
				logger: logrus.WithField("component", "tide"),
			}
			pr := &PullRequest{HeadRefOID: githubql.String(headSHA)}
			for _, ctx := range tc.commitContexts {
				commit := Commit{
					Status: struct{ Contexts []Context }{
						Contexts: []Context{
							{
								Context: githubql.String(ctx.context),
							},
						},
					},
					OID: githubql.String(ctx.sha),
				}
				pr.Commits.Nodes = append(pr.Commits.Nodes, struct{ Commit Commit }{commit})
			}

			contexts, err := provider.headContexts(CodeReviewCommonFromPullRequest(pr))
			if err != nil {
				t.Fatalf("Unexpected error from headContexts: %v", err)
			}
			if len(contexts) != 1 || string(contexts[0].Context) != win {
				t.Errorf("Expected exactly 1 %q context, but got: %#v", win, contexts)
			}
		})
	}
}

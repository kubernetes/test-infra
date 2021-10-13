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
			prs, err := search(querier, logrus.WithField("test", tc.name), q, tc.start, tc.end, "")
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			}
			// Always check prs because we might return some results on error
			if !reflect.DeepEqual(tc.expected, prs) {
				t.Errorf("prs do not match:\n%s", diff.ObjectReflectDiff(tc.expected, prs))
			}
		})
	}
}

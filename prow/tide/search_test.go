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

package tide

import (
	"testing"
	"time"

	githubql "github.com/shurcooL/githubv4"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

func uniformRangeAgeFunc(start, end time.Time, count int) func(int) time.Time {
	diff := end.Sub(start)
	step := diff / time.Duration(count)
	return func(prNum int) time.Time {
		return start.Add(step * time.Duration(prNum))
	}
}

func testSearchExecutor(ageFunc func(int) time.Time, count int) searchExecutor {
	prs := make(map[time.Time]PullRequest, count)
	for i := 0; i < count; i++ {
		prs[ageFunc(i)] = PullRequest{Number: githubql.Int(i)}
	}
	return func(start, end time.Time) ([]PullRequest, int, error) {
		var res []PullRequest
		for t, pr := range prs {
			if t.Before(start) || t.After(end) {
				continue
			}
			res = append(res, pr)
		}
		return res, len(res), nil
	}
}

func TestSearch(t *testing.T) {
	month := time.Hour * time.Duration(24*30)

	now := time.Now()
	recent := now.Add(-month)
	old := now.Add(-month * time.Duration(24))
	ancient := github.FoundingYear.Add(month * time.Duration(12))

	// For each test case, create 'count' PRs using 'ageFunc' to define their
	// distribution over time. Validate that all 'count' PRs are found.
	tcs := []struct {
		name    string
		ageFunc func(prNum int) time.Time
		count   int
	}{
		{
			name:    "less than 1000, recent->now",
			ageFunc: uniformRangeAgeFunc(recent, now, 900),
			count:   900,
		},
		{
			name:    "exactly 1000, old->now",
			ageFunc: uniformRangeAgeFunc(old, now, 1000),
			count:   1000,
		},
		{
			name:    "1500, recent->now",
			ageFunc: uniformRangeAgeFunc(recent, now, 1500),
			count:   1500,
		},
		{
			name:    "3500, recent->now",
			ageFunc: uniformRangeAgeFunc(recent, now, 3500),
			count:   3500,
		},
		{
			name:    "1500, ancient->now",
			ageFunc: uniformRangeAgeFunc(ancient, now, 1500),
			count:   1500,
		},
		{
			name:    "3500, ancient->now",
			ageFunc: uniformRangeAgeFunc(ancient, now, 3500),
			count:   3500,
		},
		{
			name:    "1500, ancient->old",
			ageFunc: uniformRangeAgeFunc(ancient, old, 1500),
			count:   1500,
		},
		{
			name:    "3500, ancient->old",
			ageFunc: uniformRangeAgeFunc(ancient, old, 3500),
			count:   3500,
		},
		{
			name:    "7000, old->now",
			ageFunc: uniformRangeAgeFunc(old, now, 7000),
			count:   7000,
		},
		{
			name:  "0 PRs",
			count: 0,
		},
	}

	for _, tc := range tcs {
		prs, err := testSearchExecutor(tc.ageFunc, tc.count).search()
		if err != nil {
			t.Fatalf("Unexpected error: %v.", err)
		}

		// Validate that there are 'tc.count' unique PRs in 'prs'.
		found := sets.NewInt()
		for _, pr := range prs {
			if found.Has(int(pr.Number)) {
				t.Errorf("Found PR #%d multiple times.", int(pr.Number))
			}
			found.Insert(int(pr.Number))
		}
		if found.Len() != tc.count {
			t.Errorf("Expected to find %d PRs, but found %d instead.", tc.count, found.Len())
		}
	}
}

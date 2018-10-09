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
	"context"
	"fmt"
	"time"

	"k8s.io/test-infra/prow/github"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
)

type searchExecutor func(start, end time.Time) ([]PullRequest, int /*true match count*/, error)

func newSearchExecutor(ctx context.Context, ghc githubClient, log *logrus.Entry, q string) searchExecutor {
	return func(start, end time.Time) ([]PullRequest, int, error) {
		datedQuery := fmt.Sprintf("%s %s", q, dateToken(start, end))
		vars := map[string]interface{}{
			"query":        githubql.String(datedQuery),
			"searchCursor": (*githubql.String)(nil),
		}
		var totalCost, remaining int
		var totalMatches int
		var ret []PullRequest
		for {
			sq := searchQuery{}
			if err := ghc.Query(ctx, &sq, vars); err != nil {
				return nil, 0, fmt.Errorf("error handling query: %q, err: %v", datedQuery, err)
			}
			totalCost += int(sq.RateLimit.Cost)
			remaining = int(sq.RateLimit.Remaining)
			totalMatches = int(sq.Search.IssueCount)
			// If the search won't return all results, abort.
			if totalMatches > 1000 {
				return nil, totalMatches, nil
			}
			for _, n := range sq.Search.Nodes {
				ret = append(ret, n.PullRequest)
			}
			if !sq.Search.PageInfo.HasNextPage {
				break
			}
			vars["searchCursor"] = githubql.NewString(sq.Search.PageInfo.EndCursor)
		}
		log.WithFields(logrus.Fields{
			"query": datedQuery,
			"start": start.String(),
			"end":   start.String(),
		}).Debugf("Query returned %d PRs and cost %d point(s). %d remaining.", len(ret), totalCost, remaining)
		return ret, totalMatches, nil
	}
}

func (q searchExecutor) search() ([]PullRequest, error) {
	prs, _, err := q.searchRange(time.Time{}, time.Now())
	return prs, err
}

func (q searchExecutor) searchSince(t time.Time) ([]PullRequest, error) {
	prs, _, err := q.searchRange(t, time.Now())
	return prs, err
}

func (q searchExecutor) searchRange(start, end time.Time) ([]PullRequest, int, error) {
	// Adjust times to be after GitHub was founded to avoid querying empty time
	// ranges.
	if start.Before(github.FoundingYear) {
		start = github.FoundingYear
	}
	if end.Before(github.FoundingYear) {
		end = github.FoundingYear
	}

	prs, count, err := q(start, end)
	if err != nil {
		return nil, 0, err
	}

	if count <= 1000 {
		// The search returned all the results for the query.
		return prs, len(prs), nil
	}
	// The query returned too many results, we need to partition it.
	prs, err = q.partitionSearchRange(start, end, count)
	return prs, len(prs), err
}

func (q searchExecutor) partitionSearchRange(start, end time.Time, count int) ([]PullRequest, error) {
	partition := partitionTime(start, end, count, 900)
	// Search right side...
	rPRs, rCount, err := q.searchRange(partition, end)
	if err != nil {
		return nil, err
	}

	// Search left side...
	// For the left side we can deduce the count in advance.
	lCount := count - rCount
	// If the count is too large we can skip the initial search and go straight to
	// partitioning to save an API token.
	var lPRs []PullRequest
	if lCount <= 1000 {
		lPRs, _, err = q.searchRange(start, partition)
	} else {
		lPRs, err = q.partitionSearchRange(start, partition, lCount)
	}
	if err != nil {
		return nil, err
	}

	return append(lPRs, rPRs...), nil
}

func partitionTime(start, end time.Time, count, goalSize int) time.Time {
	duration := end.Sub(start)
	if count < goalSize*2 {
		// Choose the midpoint.
		return start.Add(duration / 2)
	}
	// Choose the point that will make the partitionTime->end range contain goalSize
	// many results assuming a uniform distribution over time.
	// Use floats to avoid duration overflow.
	// ->    end - (duration * goalSize / count)
	diff := time.Duration(-float64(duration) * (float64(goalSize) / float64(count)))
	return end.Add(diff)
}

// dateToken generates a GitHub search query token for the specified date range.
// See: https://help.github.com/articles/understanding-the-search-syntax/#query-for-dates
func dateToken(start, end time.Time) string {
	// Github's GraphQL API silently fails if you provide it with an invalid time
	// string.
	// Dates before 1970 (unix epoch) are considered invalid.
	startString, endString := "*", "*"
	if start.Year() >= 1970 {
		startString = start.Format(github.SearchTimeFormat)
	}
	if end.Year() >= 1970 {
		endString = end.Format(github.SearchTimeFormat)
	}
	return fmt.Sprintf("updated:%s..%s", startString, endString)
}

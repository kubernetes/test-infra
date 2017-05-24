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

package mungers

import (
	githubapi "github.com/google/go-github/github"
	"runtime"
	"testing"
	//"time"
)

const (
	John            = "John"
	Ken             = "Ken"
	Lisa            = "Lisa"
	Max             = "Max"
	positiveComment = "The changes look great!"
	negativeComment = "The changes break things!"
)

func TestInactiveReviewHandler(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	/*
		timeNow := time.Now()

		getLastDateTests := []struct {
			name           string
			issueCreatedAt *time.Time
			comments       []*githubapi.IssueComment
			reviewComments []*githubapi.PullRequestComment
			expected       *time.Time
		}{
			{
				name:           "no comment, pr created more than a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments:       []*githubapi.IssueComment{},
				reviewComments: []*githubapi.PullRequestComment{},
				expected:       timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
			},
			{
				name:           "no comment, pr created within a week",
				issueCreatedAt: timePtr(timeNow),
				comments:       []*githubapi.IssueComment{},
				reviewComments: []*githubapi.PullRequestComment{},
				expected:       timePtr(timeNow),
			},
			{
				name:           "an issue comment updated more than a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments: []*githubapi.IssueComment{
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				reviewComments: []*githubapi.PullRequestComment{},
				expected:       timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
			},
			{
				name:           "a pull request comment updated more than a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments:       []*githubapi.IssueComment{},
				reviewComments: []*githubapi.PullRequestComment{
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				expected: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
			},
			{
				name:           "an issue comment updated within a week",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments: []*githubapi.IssueComment{
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(timeNow),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				reviewComments: []*githubapi.PullRequestComment{},
				expected:       timePtr(timeNow),
			},
			{
				name:           "a pull request comment updated within a week",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments:       []*githubapi.IssueComment{},
				reviewComments: []*githubapi.PullRequestComment{
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(timeNow),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				expected: timePtr(timeNow),
			},
			{
				name:           "multiple issue comments, latest comment updated more than a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments: []*githubapi.IssueComment{
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 3, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Ken)}},
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 4, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(negativeComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				reviewComments: []*githubapi.PullRequestComment{},
				expected:       timePtr(time.Date(2017, 4, 1, 13, 5, 20, 0, time.UTC)),
			},
			{
				name:           "multiple issue comments, latest comment updated within a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments: []*githubapi.IssueComment{
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 3, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Ken)}},
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(timeNow),
						Body:      stringPtr(negativeComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				reviewComments: []*githubapi.PullRequestComment{},
				expected:       timePtr(timeNow),
			},

			{
				name:           "multiple pull request comments, latest comment updated more than a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments:       []*githubapi.IssueComment{},
				reviewComments: []*githubapi.PullRequestComment{
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 3, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Ken)}},
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 4, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(negativeComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				expected: timePtr(time.Date(2017, 4, 1, 13, 5, 20, 0, time.UTC)),
			},
			{
				name:           "multiple pull request comments, latest comment updated within a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments:       []*githubapi.IssueComment{},
				reviewComments: []*githubapi.PullRequestComment{
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 3, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Ken)}},
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(timeNow),
						Body:      stringPtr(negativeComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				expected: timePtr(timeNow),
			},
			{
				name:           "an issue comment, a pull request comment, latest comment updated more than a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments: []*githubapi.IssueComment{
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 3, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Ken)}},
				},
				reviewComments: []*githubapi.PullRequestComment{
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 4, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(negativeComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				expected: timePtr(time.Date(2017, 4, 1, 13, 5, 20, 0, time.UTC)),
			},
			{
				name:           "an issue comment, a pull request comment, latest comment updated within a week ago",
				issueCreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
				comments: []*githubapi.IssueComment{
					&githubapi.IssueComment{CreatedAt: timePtr(time.Date(2017, 1, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(time.Date(2017, 3, 1, 13, 5, 20, 0, time.UTC)),
						Body:      stringPtr(positiveComment),
						User:      &githubapi.User{Login: githubapi.String(Ken)}},
				},
				reviewComments: []*githubapi.PullRequestComment{
					&githubapi.PullRequestComment{CreatedAt: timePtr(time.Date(2017, 2, 1, 13, 5, 20, 0, time.UTC)),
						UpdatedAt: timePtr(timeNow),
						Body:      stringPtr(negativeComment),
						User:      &githubapi.User{Login: githubapi.String(Lisa)}},
				},
				expected: timePtr(timeNow),
			},
		}

		for testNum, test := range getLastDateTests {
			i := InactiveReviewHandler{}
			active := i.getLatestTime(test.issueCreatedAt, stringPtr(John), test.comments, test.reviewComments)

			if *test.expected != *active {
				t.Errorf("%d:%s: expected: %v, saw: %v", testNum, test.name, test.expected, active)
			}
		}
	*/
	suggestNewReviewerTests := []struct {
		name            string
		issue           *githubapi.Issue
		potentialOwners map[string]int64
		weightSum       int64
		expected        string
	}{
		{
			name: "initial len(potentialOwners) == 0",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees: []*githubapi.User{&githubapi.User{Login: githubapi.String(Ken)},
					&githubapi.User{Login: githubapi.String(Lisa)}},
			},
			potentialOwners: make(map[string]int64),
			weightSum:       0,
			expected:        "",
		},
		{
			name: "initial len(potentialOwners) > 0, but final len(potentialOwners) == 0",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees: []*githubapi.User{&githubapi.User{Login: githubapi.String(Ken)},
					&githubapi.User{Login: githubapi.String(Lisa)},
					&githubapi.User{Login: githubapi.String(Max)}},
			},
			potentialOwners: map[string]int64{"Ken": 27, "Lisa": 39, "Max": 34},
			weightSum:       100,
			expected:        "",
		},
		{
			name: "initial len(potentialOwners) == 0, issue.Assignees == nil",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees:        nil,
			},
			potentialOwners: make(map[string]int64),
			weightSum:       0,
			expected:        "",
		},
		{
			name: "initial len(potentialOwners) > 0, issue.Assignees == nil",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees:        nil,
			},
			potentialOwners: map[string]int64{"Lisa": 39},
			weightSum:       39,
			expected:        "Lisa",
		},
		{
			name: "initial len(potentialOwners) == 0, final len(issue.Assignees) == 0",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees:        []*githubapi.User{},
			},
			potentialOwners: make(map[string]int64),
			weightSum:       39,
			expected:        "",
		},
		{
			name: "initial len(potentialOwners) > 0, len(issue.Assignees) == 0",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees:        []*githubapi.User{},
			},
			potentialOwners: map[string]int64{"Lisa": 39},
			weightSum:       39,
			expected:        "Lisa",
		},
		{
			name: "len(potentialOwners) > 0, issue.Assignees != nil, len(issue.Assignees) > 0, author is not a potential owner",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees: []*githubapi.User{&githubapi.User{Login: githubapi.String(Ken)},
					&githubapi.User{Login: githubapi.String(Lisa)}},
			},
			potentialOwners: map[string]int64{"Ken": 27, "Lisa": 39, "Max": 34},
			weightSum:       100,
			expected:        "Max",
		},
		{
			name: "len(potentialOwners) > 0, issue.Assignees != nil, len(issue.Assignees) > 0, author is a potential owner",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(Max)},
				Number:           intPtr(1),
				Assignees: []*githubapi.User{&githubapi.User{Login: githubapi.String(Ken)},
					&githubapi.User{Login: githubapi.String(Lisa)}},
			},
			potentialOwners: map[string]int64{"Ken": 27, "Lisa": 39},
			weightSum:       66,
			expected:        "",
		},
		{
			name: "len(potentialOwners) > 0, issue.Assignees != nil, len(issue.Assignees) > 0, all potential owners have already been assigned",
			issue: &githubapi.Issue{
				PullRequestLinks: &githubapi.PullRequestLinks{},
				User:             &githubapi.User{Login: githubapi.String(John)},
				Number:           intPtr(1),
				Assignees: []*githubapi.User{&githubapi.User{Login: githubapi.String(Ken)},
					&githubapi.User{Login: githubapi.String(Lisa)},
					&githubapi.User{Login: githubapi.String(Max)}},
			},
			potentialOwners: map[string]int64{"Ken": 27, "Lisa": 39, "Max": 34},
			weightSum:       100,
			expected:        "",
		},
	}

	for testNum, test := range suggestNewReviewerTests {
		i := InactiveReviewHandler{}

		newReviewer := i.suggestNewReviewer(test.issue, test.potentialOwners, test.weightSum)

		if test.expected != newReviewer {
			t.Errorf("%d:%s: expected: %v, saw: %v, potentialOwners: %v", testNum, test.name, test.expected, newReviewer, test.potentialOwners)
		}
	}
}

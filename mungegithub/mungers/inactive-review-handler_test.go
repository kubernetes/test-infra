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
)

const (
	John            = "John" //author
	Ken             = "Ken"
	Lisa            = "Lisa"
	Max             = "Max"
	positiveComment = "The changes look great!"
	negativeComment = "The changes break things!"
)

func TestInactiveReviewHandler(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	haveNonAuthorHumanTests := []struct {
		name           string
		comments       []*githubapi.IssueComment
		reviewComments []*githubapi.PullRequestComment
		expected       bool
	}{
		{
			name:           "IssueComment is empty, PullRequestComment is empty",
			comments:       []*githubapi.IssueComment{},
			reviewComments: []*githubapi.PullRequestComment{},
			expected:       false,
		},
		{
			name: "IssueComment is not empty without non-author human, PullRequestComment is empty",
			comments: []*githubapi.IssueComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			reviewComments: []*githubapi.PullRequestComment{},
			expected:       false,
		},
		{
			name: "IssueComment is not empty with non-author human, PullRequestComment is empty",
			comments: []*githubapi.IssueComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			reviewComments: []*githubapi.PullRequestComment{},
			expected:       true,
		},
		{
			name:     "IssueComment is empty, PullRequestComment is not empty without non-author human",
			comments: []*githubapi.IssueComment{},
			reviewComments: []*githubapi.PullRequestComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			expected: false,
		},
		{
			name:     "IssueComment is empty, PullRequestComment is not empty with non-author human",
			comments: []*githubapi.IssueComment{},
			reviewComments: []*githubapi.PullRequestComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			expected: true,
		},
		{
			name: "IssueComment is not empty without non-author human, PullRequestComment is not empty without non-author human",
			comments: []*githubapi.IssueComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			reviewComments: []*githubapi.PullRequestComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			expected: false,
		},
		{
			name: "IssueComment is not empty without non-author human, PullRequestComment is not empty with non-author human",
			comments: []*githubapi.IssueComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			reviewComments: []*githubapi.PullRequestComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			expected: true,
		},
		{
			name: "IssueComment is not empty with non-author human, PullRequestComment is not empty without non-author human",
			comments: []*githubapi.IssueComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			reviewComments: []*githubapi.PullRequestComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			expected: true,
		},
		{
			name: "IssueComment is not empty with non-author human, PullRequestComment is not empty with non-author human",
			comments: []*githubapi.IssueComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			reviewComments: []*githubapi.PullRequestComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			expected: true,
		},
		{
			name: "IssueComment is not empty with multiple non-author humans, PullRequestComment is not empty with multiple non-author humans",
			comments: []*githubapi.IssueComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Ken)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			reviewComments: []*githubapi.PullRequestComment{
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Max)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(Lisa)}},
				{
					Body: stringPtr(positiveComment),
					User: &githubapi.User{Login: githubapi.String(John)}},
			},
			expected: true,
		},
	}

	for testNum, test := range haveNonAuthorHumanTests {
		i := InactiveReviewHandler{}
		found := i.haveNonAuthorHuman(stringPtr(John), test.comments, test.reviewComments)

		if test.expected != found {
			t.Errorf("%d:%s: expected: %t, saw: %t", testNum, test.name, test.expected, found)
		}
	}

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
				Assignees: []*githubapi.User{{Login: githubapi.String(Ken)},
					{Login: githubapi.String(Lisa)}},
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
				Assignees: []*githubapi.User{{Login: githubapi.String(Ken)},
					{Login: githubapi.String(Lisa)},
					{Login: githubapi.String(Max)}},
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
				Assignees: []*githubapi.User{{Login: githubapi.String(Ken)},
					{Login: githubapi.String(Lisa)}},
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
				Assignees: []*githubapi.User{{Login: githubapi.String(Ken)},
					{Login: githubapi.String(Lisa)}},
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
				Assignees: []*githubapi.User{{Login: githubapi.String(Ken)},
					{Login: githubapi.String(Lisa)},
					{Login: githubapi.String(Max)}},
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

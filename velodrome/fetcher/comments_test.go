/*
Copyright 2016 The Kubernetes Authors.

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
	"reflect"
	"testing"
	"time"

	"k8s.io/test-infra/velodrome/sql"
	sqltest "k8s.io/test-infra/velodrome/sql/testing"

	"github.com/google/go-github/github"
)

func TestFindLatestCommentUpdate(t *testing.T) {
	config := sqltest.SQLiteConfig{":memory:"}
	tests := []struct {
		comments       []sql.Comment
		expectedLatest time.Time
		repository     string
	}{
		// If we don't have any comment, return 1900/1/1 0:0:0 UTC
		{
			[]sql.Comment{},
			time.Time{},
			"ONE",
		},
		// There are no comment for this repository, return the min date
		{
			[]sql.Comment{
				{IssueID: "1", CommentUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "1", CommentUpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "2", CommentUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "TWO"},
			},
			time.Time{},
			"THREE",
		},
		// Only pick selected repo
		{
			[]sql.Comment{
				{IssueID: "1", CommentUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "1", CommentUpdatedAt: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "TWO"},
				{IssueID: "1", CommentUpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "2", CommentUpdatedAt: time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
			},
			time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC),
			"ONE",
		},
		// Can pick pull-request comments
		{
			[]sql.Comment{
				{IssueID: "1", PullRequest: true, CommentUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "1", PullRequest: false, CommentUpdatedAt: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "1", PullRequest: true, CommentUpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
			},
			time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
			"ONE",
		},
		// Can pick issue comments
		{
			[]sql.Comment{
				{IssueID: "1", PullRequest: false, CommentUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "1", PullRequest: true, CommentUpdatedAt: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueID: "1", PullRequest: false, CommentUpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
			},
			time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
			"ONE",
		},
	}

	for _, test := range tests {
		db, err := config.CreateDatabase()
		if err != nil {
			t.Fatal("Failed to create database:", err)
		}

		for _, comment := range test.comments {
			db.Create(&comment)
		}

		actualLatest := findLatestCommentUpdate(db, test.repository)
		if actualLatest != test.expectedLatest {
			t.Error("Actual:", actualLatest,
				"doesn't match expected:", test.expectedLatest)
		}
	}
}

func TestUpdateComments(t *testing.T) {
	config := sqltest.SQLiteConfig{":memory:"}

	tests := []struct {
		before           []sql.Comment
		newIssueComments []*github.IssueComment
		newPullComments  []*github.PullRequestComment
		after            []sql.Comment
	}{
		// No new comments
		{
			before: []sql.Comment{
				*makeComment(12, 1, "Body", "Login", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
			},
			newIssueComments: []*github.IssueComment{},
			newPullComments:  []*github.PullRequestComment{},
			after: []sql.Comment{
				*makeComment(12, 1, "Body", "Login", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
			},
		},
		// New comments, include PR
		{
			before: []sql.Comment{
				*makeComment(12, 1, "Body", "Login", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
			},
			newIssueComments: []*github.IssueComment{
				makeGithubIssueComment(2, "IssueBody", "SomeLogin", "issue/3",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
				makeGithubIssueComment(3, "AnotherBody", "AnotherLogin", "issue/3",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			},
			newPullComments: []*github.PullRequestComment{
				makeGithubPullComment(4, "Body", "Login", "issue/2",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.February, 1, 19, 30, 0, 0, time.UTC)),
				makeGithubPullComment(5, "SecondBody", "OtherLogin", "issue/3",
					time.Date(2000, time.December, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.November, 1, 19, 30, 0, 0, time.UTC)),
			},
			after: []sql.Comment{
				*makeComment(12, 1, "Body", "Login", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
				*makeComment(3, 2, "IssueBody", "SomeLogin", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), false),
				*makeComment(3, 3, "AnotherBody", "AnotherLogin", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), false),
				*makeComment(2, 4, "Body", "Login", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.February, 1, 19, 30, 0, 0, time.UTC), true),
				*makeComment(3, 5, "SecondBody", "OtherLogin", "full/repo",
					time.Date(2000, time.December, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.November, 1, 19, 30, 0, 0, time.UTC), true),
			},
		},
		// New modified comment
		{
			before: []sql.Comment{
				*makeComment(12, 1, "Body", "Login", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
			},
			newIssueComments: []*github.IssueComment{},
			newPullComments: []*github.PullRequestComment{
				makeGithubPullComment(1, "IssueBody", "SomeLogin", "issue/12",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			},
			after: []sql.Comment{
				*makeComment(12, 1, "IssueBody", "SomeLogin", "full/repo",
					time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
					time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
			},
		},
	}

	for _, test := range tests {
		db, err := config.CreateDatabase()
		if err != nil {
			t.Fatal("Failed to create database:", err)
		}

		for _, comment := range test.before {
			if err := db.Create(&comment).Error; err != nil {
				t.Fatal(err)
			}
		}

		client := FakeClient{PullComments: test.newPullComments, IssueComments: test.newIssueComments, Repository: "full/repo"}
		UpdateComments(db, client)
		var comments []sql.Comment
		if err := db.Order("ID").Find(&comments).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(comments, test.after) {
			t.Error("Actual:", comments,
				"doesn't match expected:", test.after)
		}
	}
}

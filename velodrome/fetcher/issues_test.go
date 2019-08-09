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

func TestFindLatestIssueUpdate(t *testing.T) {
	config := sqltest.SQLiteConfig{File: ":memory:"}
	tests := []struct {
		issues         []sql.Issue
		expectedLatest time.Time
		repository     string
	}{
		// If we don't have any issue, return 1900/1/1 0:0:0 UTC
		{
			[]sql.Issue{},
			time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
			"ONE",
		},
		// We have issues, but not for this repo, return 1900/1/1 0:0:0 UTC
		{
			[]sql.Issue{
				{IssueUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueUpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueUpdatedAt: time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
			},
			time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
			"TWO",
		},
		{
			[]sql.Issue{
				{IssueUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueUpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueUpdatedAt: time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "TWO"},
			},
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			"ONE",
		},
		{
			[]sql.Issue{
				{IssueUpdatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueUpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "ONE"},
				{IssueUpdatedAt: time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC), Repository: "TWO"},
			},
			time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC),
			"TWO",
		},
	}

	for _, test := range tests {
		db, err := config.CreateDatabase()
		if err != nil {
			t.Fatal("Failed to create database:", err)
		}

		tx := db.Begin()
		for _, issue := range test.issues {
			tx.Create(&issue)
		}
		tx.Commit()

		actualLatest, err := findLatestIssueUpdate(db, test.repository)
		if err != nil {
			t.Error("findLatestIssueUpdate failed:", err)
		}
		if actualLatest != test.expectedLatest {
			t.Error("Actual:", actualLatest,
				"doesn't match expected:", test.expectedLatest)
		}
	}
}

func TestUpdateIssues(t *testing.T) {
	config := sqltest.SQLiteConfig{File: ":memory:"}

	tests := []struct {
		before     []sql.Issue
		new        []*github.Issue
		after      []sql.Issue
		repository string
	}{
		// No new issues
		{
			before: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/repo",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			new: []*github.Issue{},
			after: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/repo",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			repository: "FULL/REPO",
		},
		// New issues
		{
			before: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/repo",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			new: []*github.Issue{
				makeGitHubIssue(2, "Super Title", "Body", "NoState", "Login", "", 0, false,
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			after: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/repo",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
				*makeIssue(2, "Super Title", "Body", "NoState", "Login", "",
					"full/repo", 0, false,
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			repository: "FULL/REPO",
		},
		// New issues + already existing
		{
			before: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/repo",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
				*makeIssue(2, "Title", "", "State", "User", "", "full/repo",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			new: []*github.Issue{
				makeGitHubIssue(2, "Super Title", "Body", "NoState", "Login", "",
					0, false,
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
				makeGitHubIssue(3, "Title", "Body", "State", "John", "",
					0, false,
					time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2003, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			after: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/repo", 0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
				*makeIssue(2, "Super Title", "Body", "NoState", "Login", "",
					"full/repo", 0, false,
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
				*makeIssue(3, "Title", "Body", "State", "John", "", "full/repo",
					0, false,
					time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2003, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			repository: "FULL/REPO",
		},
		// Fetch new repository
		{
			before: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/one",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			new: []*github.Issue{
				makeGitHubIssue(1, "Super Title", "Body", "NoState", "Login", "",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			after: []sql.Issue{
				*makeIssue(1, "Title", "", "State", "User", "", "full/one",
					0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
				*makeIssue(1, "Super Title", "Body", "NoState", "Login", "",
					"full/other", 0, false,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Time{}),
			},
			repository: "FULL/OTHER",
		},
	}

	for _, test := range tests {
		db, err := config.CreateDatabase()
		if err != nil {
			t.Fatal("Failed to create database:", err)
		}

		for _, issue := range test.before {
			db.Create(&issue)
		}

		UpdateIssues(db, FakeClient{Issues: test.new, Repository: test.repository})
		var issues []sql.Issue
		if err := db.Order("ID").Find(&issues).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(issues, test.after) {
			t.Error("Actual:", issues,
				"doesn't match expected:", test.after)
		}
	}
}

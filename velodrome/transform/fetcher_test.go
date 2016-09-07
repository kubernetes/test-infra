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
	"testing"
	"time"

	"k8s.io/test-infra/velodrome/sql"
	sqltest "k8s.io/test-infra/velodrome/sql/testing"
)

// Fetch doesn't download too many items, and return the proper date.

func TestFetchIssues(t *testing.T) {
	config := sqltest.SQLiteConfig{":memory:"}
	db, err := config.CreateDatabase()
	if err != nil {
		t.Fatal("Failed to create database:", err)
	}

	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)})
	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)})
	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC)})
	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC)})

	out := make(chan sql.Issue, 10)

	last, err := fetchRecentIssues(db, time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC), out)
	if err != nil {
		t.Fatal("Failed to fetch recent issues:", err)
	}
	if last != time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC) {
		t.Error(
			"Last issue should be %s, not %s",
			time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC),
			last,
		)
	}
	// Last is included in the response set
	if len(out) != 3 {
		t.Error("Only 3 issues should have been fetched, not ", len(out))
	}
}

func TestFetchEvents(t *testing.T) {
	config := sqltest.SQLiteConfig{":memory:"}
	db, err := config.CreateDatabase()
	if err != nil {
		t.Fatal("Failed to create database:", err)
	}

	db.Create(&sql.IssueEvent{ID: 1})
	db.Create(&sql.IssueEvent{ID: 2})
	db.Create(&sql.IssueEvent{ID: 3})
	db.Create(&sql.IssueEvent{ID: 4})

	out := make(chan sql.IssueEvent, 10)

	last, err := fetchRecentEvents(db, 2, out)
	if err != nil {
		t.Fatal("Failed to fetch recent events:", err)
	}
	if last != 4 {
		t.Error("Last event should be 4, not", last)
	}
	if len(out) != 2 {
		t.Error("Only 2 events should have been fetched, not ", len(out))
	}
}

func TestFetchComments(t *testing.T) {
	config := sqltest.SQLiteConfig{":memory:"}
	db, err := config.CreateDatabase()
	if err != nil {
		t.Fatal("Failed to create database:", err)
	}

	db.Create(&sql.Comment{ID: 1})
	db.Create(&sql.Comment{ID: 2})
	db.Create(&sql.Comment{ID: 3})
	db.Create(&sql.Comment{ID: 4})

	out := make(chan sql.Comment, 10)

	last, err := fetchRecentComments(db, 2, out)
	if err != nil {
		t.Fatal("Failed to fetch recent comments:", err)
	}
	if last != 4 {
		t.Error("Last comment should be 4, not", last)
	}
	if len(out) != 2 {
		t.Error("Only 2 comments should have been fetched, not ", len(out))
	}
}

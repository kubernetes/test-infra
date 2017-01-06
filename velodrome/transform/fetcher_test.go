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
	"os"
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

	last := time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)
	if err := fetchRecentIssues(db, &last, out); err != nil {
		t.Fatal("Failed to fetch recent issues:", err)
	}
	if last != time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC) {
		t.Errorf(
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

func TestFetchEventsAndComments(t *testing.T) {
	tests := []struct {
		events          []interface{}
		lastEvent       int
		lastComment     int
		wantLastEvent   int
		wantLastComment int
		wantCount       int
	}{
		// Mixed events and comments
		{
			events: []interface{}{
				&sql.IssueEvent{ID: 1},
				&sql.IssueEvent{ID: 2},
				&sql.IssueEvent{ID: 3, EventCreatedAt: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)},
				&sql.IssueEvent{ID: 4, EventCreatedAt: time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC)},
				&sql.Comment{ID: 1},
				&sql.Comment{ID: 2},
				&sql.Comment{ID: 3, CommentCreatedAt: time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)},
				&sql.Comment{ID: 4, CommentCreatedAt: time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC)},
				&sql.Comment{ID: 5, CommentCreatedAt: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC)},
			},
			lastEvent:       2,
			lastComment:     2,
			wantLastEvent:   4,
			wantLastComment: 5,
			wantCount:       5,
		},
		// Only comments
		{
			events: []interface{}{
				&sql.Comment{ID: 1},
				&sql.Comment{ID: 2},
				&sql.Comment{ID: 3, CommentCreatedAt: time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)},
				&sql.Comment{ID: 4, CommentCreatedAt: time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC)},
				&sql.Comment{ID: 5, CommentCreatedAt: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC)},
			},
			lastEvent:       10,
			lastComment:     2,
			wantLastEvent:   10,
			wantLastComment: 5,
			wantCount:       3,
		},
	}

	for _, test := range tests {
		os.Remove("test.db")
		config := sqltest.SQLiteConfig{"test.db"}
		db, err := config.CreateDatabase()
		if err != nil {
			t.Fatal("Failed to create database:", err)
		}

		for _, event := range test.events {
			db.Create(event)
		}

		out := make(chan interface{}, len(test.events))

		lastEvent := test.lastEvent
		lastComment := test.lastComment

		if err := fetchRecentEventsAndComments(db, &lastEvent, &lastComment, out); err != nil {
			t.Fatal("Failed to fetch recent events:", err)
		}
		if lastEvent != test.wantLastEvent {
			t.Errorf("LastEvent event should be %d, not %d", test.wantLastEvent, lastEvent)
		}
		if lastComment != test.wantLastComment {
			t.Errorf("LastComment event should be %d, not %d", test.wantLastComment, lastComment)
		}
		if len(out) != test.wantCount {
			t.Errorf("%d events should have been fetched, not %d", test.wantCount, len(out))
		}

		close(out)

		lastDate := time.Time{}
		for item := range out {
			date := time.Time{}
			switch item := item.(type) {
			case sql.IssueEvent:
				date = item.EventCreatedAt
			case sql.Comment:
				date = item.CommentCreatedAt
			default:
				t.Error("Received item of unknown type:", item)
			}
			if date.Before(lastDate) {
				t.Errorf("Dates are not properly sorted: %v < %v", date, lastDate)
			}
		}
		os.Remove("test.db")
	}
}

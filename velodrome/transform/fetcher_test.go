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

// Fetch doesn't download too many items, and return the proper date. And only from proper repo

func TestFetchIssues(t *testing.T) {
	config := sqltest.SQLiteConfig{File: ":memory:"}
	db, err := config.CreateDatabase()
	if err != nil {
		t.Fatal("Failed to create database:", err)
	}

	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC), Repository: "ok"})
	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC), Repository: "ok"})
	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC), Repository: "ok"})
	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC), Repository: "ok"})
	db.Create(&sql.Issue{IssueUpdatedAt: time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC), Repository: "notok"})

	fetcher := NewFetcher("ok")
	fetcher.lastIssue = time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)
	fetcher.IssuesChannel = make(chan sql.Issue, 10)

	if err := fetcher.fetchRecentIssues(db); err != nil {
		t.Fatal("Failed to fetch recent issues:", err)
	}
	if fetcher.lastIssue != time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC) {
		t.Errorf(
			"Last issue should be %s, not %s",
			time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC),
			fetcher.lastIssue,
		)
	}
	// Last is included in the response set
	if len(fetcher.IssuesChannel) != 3 {
		t.Error("Only 3 issues should have been fetched, not ", len(fetcher.IssuesChannel))
	}
}

func TestFetchEventsAndComments(t *testing.T) {
	tests := []struct {
		events          []interface{}
		lastEvent       time.Time
		lastComment     time.Time
		wantLastEvent   time.Time
		wantLastComment time.Time
		wantCount       int
	}{
		// Mixed events and comments
		{
			events: []interface{}{
				&sql.IssueEvent{ID: "1", Repository: "ok"},
				&sql.IssueEvent{ID: "2", Repository: "ok"},
				&sql.IssueEvent{ID: "3", EventCreatedAt: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.IssueEvent{ID: "4", EventCreatedAt: time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.Comment{ID: "1", Repository: "ok"},
				&sql.Comment{ID: "2", Repository: "ok"},
				&sql.Comment{ID: "3", CommentCreatedAt: time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.Comment{ID: "4", CommentCreatedAt: time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.Comment{ID: "5", CommentCreatedAt: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.Comment{ID: "6", CommentCreatedAt: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC), Repository: "notok"},
			},
			lastEvent:       time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
			lastComment:     time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC),
			wantLastEvent:   time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC),
			wantLastComment: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC),
			wantCount:       3,
		},
		// Only comments
		{
			events: []interface{}{
				&sql.Comment{ID: "1", Repository: "ok"},
				&sql.Comment{ID: "2", Repository: "ok"},
				&sql.Comment{ID: "3", CommentCreatedAt: time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.Comment{ID: "4", CommentCreatedAt: time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.Comment{ID: "5", CommentCreatedAt: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC), Repository: "ok"},
				&sql.Comment{ID: "5", CommentCreatedAt: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC), Repository: "notok"},
			},
			lastEvent:       time.Date(1990, time.January, 0, 0, 0, 0, 0, time.UTC),
			lastComment:     time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC),
			wantLastEvent:   time.Date(1990, time.January, 0, 0, 0, 0, 0, time.UTC),
			wantLastComment: time.Date(2000, time.January, 5, 0, 0, 0, 0, time.UTC),
			wantCount:       2,
		},
	}

	for _, test := range tests {
		os.Remove("test.db")
		config := sqltest.SQLiteConfig{File: "test.db"}
		db, err := config.CreateDatabase()
		if err != nil {
			t.Fatal("Failed to create database:", err)
		}

		for _, event := range test.events {
			db.Create(event)
		}

		fetcher := NewFetcher("ok")
		fetcher.lastEvent = test.lastEvent
		fetcher.lastComment = test.lastComment
		fetcher.EventsCommentsChannel = make(chan interface{}, len(test.events))

		if err := fetcher.fetchRecentEventsAndComments(db); err != nil {
			t.Fatal("Failed to fetch recent events:", err)
		}
		if !fetcher.lastEvent.Equal(test.wantLastEvent) {
			t.Errorf("LastEvent event should be %s, not %s", test.wantLastEvent, fetcher.lastEvent)
		}
		if !fetcher.lastComment.Equal(test.wantLastComment) {
			t.Errorf("LastComment event should be %s, not %s", test.wantLastComment, fetcher.lastComment)
		}
		if len(fetcher.EventsCommentsChannel) != test.wantCount {
			t.Errorf("%d events should have been fetched, not %d", test.wantCount, len(fetcher.EventsCommentsChannel))
		}

		close(fetcher.EventsCommentsChannel)

		lastDate := time.Time{}
		for item := range fetcher.EventsCommentsChannel {
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

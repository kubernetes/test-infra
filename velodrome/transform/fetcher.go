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
	"time"

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/glog"
	"github.com/jinzhu/gorm"
)

// fetchRecentIssues retrieves issues from DB, but only fetches issues modified since last call
func fetchRecentIssues(db *gorm.DB, last time.Time, out chan sql.Issue) (time.Time, error) {
	glog.Infof("Fetching issues updated after %s", last)

	var issues []sql.Issue
	query := db.Where("issue_updated_at >= ?", last).Order("issue_updated_at").Preload("Labels").Find(&issues)
	if query.Error != nil {
		return last, query.Error
	}

	count := len(issues)
	for _, issue := range issues {
		out <- issue
		last = issue.IssueUpdatedAt
	}
	glog.Infof("Found and pushed %d updated/new issues", count)

	return last, nil
}

// fetchRecentEvents retrieves events from DB, but only fetches events created since last call
func fetchRecentEvents(db *gorm.DB, last int, out chan sql.IssueEvent) (int, error) {
	glog.Infof("Fetching issue-events with id bigger than %d", last)

	rows, err := db.Model(sql.IssueEvent{}).Where("id > ?", last).Order("event_created_at asc").Rows()
	if err != nil {
		return last, err
	}

	count := 0
	for rows.Next() {
		var issueEvent sql.IssueEvent
		db.ScanRows(rows, &issueEvent)
		out <- issueEvent
		last = issueEvent.ID
		count++
	}
	glog.Infof("Found and pushed %d new events", count)

	return last, nil
}

// fetchRecentComments retrieves comments from DB, but only fetches comments created since last call
func fetchRecentComments(db *gorm.DB, last int, out chan sql.Comment) (int, error) {
	glog.Infof("Fetching comments with id bigger than %d", last)

	rows, err := db.Model(sql.Comment{}).Where("id > ?", last).Order("comment_created_at asc").Rows()
	if err != nil {
		return last, err
	}

	count := 0
	for rows.Next() {
		var comment sql.Comment
		db.ScanRows(rows, &comment)
		out <- comment
		last = comment.ID
		count++
	}
	glog.Infof("Found and pushed %d new comments", count)

	return last, nil
}

// Fetcher is a utility class used to Fetch all types of events
type Fetcher struct {
	lastIssue          time.Time
	lastEvent          int
	lastComment        int
	issuesChannel      chan sql.Issue
	issueEventsChannel chan sql.IssueEvent
	commentsChannel    chan sql.Comment
}

// NewFetcher creates a new Fetcher and initializes the output channels
func NewFetcher() *Fetcher {
	return &Fetcher{
		issuesChannel:      make(chan sql.Issue, 100),
		issueEventsChannel: make(chan sql.IssueEvent, 100),
		commentsChannel:    make(chan sql.Comment, 100),
	}
}

// GetChannels returns the list of output channels used
func (f *Fetcher) GetChannels() (chan sql.Issue, chan sql.IssueEvent, chan sql.Comment) {
	return f.issuesChannel, f.issueEventsChannel, f.commentsChannel
}

// Fetch retrieves all types of events, and push them to output channels
func (f *Fetcher) Fetch(db *gorm.DB) error {
	var err error
	if f.lastIssue, err = fetchRecentIssues(db, f.lastIssue, f.issuesChannel); err != nil {
		return err
	}
	if f.lastEvent, err = fetchRecentEvents(db, f.lastEvent, f.issueEventsChannel); err != nil {
		return err
	}
	if f.lastComment, err = fetchRecentComments(db, f.lastComment, f.commentsChannel); err != nil {
		return err
	}
	return nil
}

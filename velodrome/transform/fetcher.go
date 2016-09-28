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
	"fmt"
	"time"

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/glog"
	"github.com/jinzhu/gorm"
)

// fetchRecentIssues retrieves issues from DB, but only fetches issues modified since last call
func fetchRecentIssues(db *gorm.DB, last *time.Time, out chan sql.Issue) error {
	glog.Infof("Fetching issues updated after %s", *last)

	var issues []sql.Issue
	query := db.Where("issue_updated_at >= ?", last).Order("issue_updated_at").Preload("Labels").Find(&issues)
	if query.Error != nil {
		return query.Error
	}

	count := len(issues)
	for _, issue := range issues {
		out <- issue
		*last = issue.IssueUpdatedAt
	}
	glog.Infof("Found and pushed %d updated/new issues", count)

	return nil
}

// fetchRecentEventsAndComments retrieves events from DB, but only fetches events created since last call
func fetchRecentEventsAndComments(db *gorm.DB, lastEvent *int, lastComment *int, out chan interface{}) error {
	glog.Infof("Fetching issue-events with id bigger than %d", *lastEvent)
	glog.Infof("Fetching comments with id bigger than %d", *lastComment)

	eventRows, err := db.Model(sql.IssueEvent{}).Where("id > ?", *lastEvent).Order("event_created_at asc").Rows()
	if err != nil {
		return fmt.Errorf("Failed to query events from database: %s", err)
	}

	commentRows, err := db.Model(sql.Comment{}).Where("id > ?", *lastComment).Order("comment_created_at asc").Rows()
	if err != nil {
		return fmt.Errorf("Failed to query comments from database: %s", err)
	}

	count := 0
	comment := &sql.Comment{}
	if commentRows.Next() {
		db.ScanRows(commentRows, comment)
	} else {
		comment = nil
	}
	event := &sql.IssueEvent{}
	if eventRows.Next() {
		db.ScanRows(eventRows, event)
	} else {
		event = nil
	}

	for event != nil || comment != nil {
		if event == nil || (comment != nil && comment.CommentCreatedAt.Before(event.EventCreatedAt)) {
			out <- *comment
			*lastComment = comment.ID
			if commentRows.Next() {
				db.ScanRows(commentRows, comment)
			} else {
				comment = nil
			}
		} else {
			out <- *event
			*lastEvent = event.ID
			if eventRows.Next() {
				db.ScanRows(eventRows, event)
			} else {
				event = nil
			}
		}
		count++
	}

	glog.Infof("Found and pushed %d new events/comments", count)

	return nil
}

// Fetcher is a utility class used to Fetch all types of events
type Fetcher struct {
	lastIssue             time.Time
	lastEvent             int
	lastComment           int
	issuesChannel         chan sql.Issue
	eventsCommentsChannel chan interface{}
}

// NewFetcher creates a new Fetcher and initializes the output channels
func NewFetcher() *Fetcher {
	return &Fetcher{
		issuesChannel:         make(chan sql.Issue, 100),
		eventsCommentsChannel: make(chan interface{}, 100),
	}
}

// GetChannels returns the list of output channels used
func (f *Fetcher) GetChannels() (chan sql.Issue, chan interface{}) {
	return f.issuesChannel, f.eventsCommentsChannel
}

// Fetch retrieves all types of events, and push them to output channels
func (f *Fetcher) Fetch(db *gorm.DB) error {
	if err := fetchRecentIssues(db, &f.lastIssue, f.issuesChannel); err != nil {
		return err
	}
	if err := fetchRecentEventsAndComments(db, &f.lastEvent, &f.lastComment, f.eventsCommentsChannel); err != nil {
		return err
	}
	return nil
}

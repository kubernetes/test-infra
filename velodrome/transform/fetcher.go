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

// Issues can fetch issues from DB. It saves the time of the last issue fetched
type Issues time.Time

// Fetch retrieves issues from DB, but only fetches issues modified since last call
func (i *Issues) Fetch(db *gorm.DB, out chan sql.Issue) error {
	glog.Infof("Fetching issues updated after %s", (*time.Time)(i))

	var issues []sql.Issue
	query := db.Where("issue_updated_at >= ?", (*time.Time)(i)).Preload("Labels").Find(&issues)
	if query.Error != nil {
		return query.Error
	}

	count := len(issues)
	for _, issue := range issues {
		out <- issue
		*i = Issues(issue.IssueUpdatedAt)
	}
	glog.Infof("Found and pushed %d updated/new issues", count)

	return nil
}

// IssueEvents can fetch Events from DB. It saves the ID of the last event fetched
type IssueEvents int

// Fetch retrieves events from DB, but only fetches events created since last call
func (i *IssueEvents) Fetch(db *gorm.DB, out chan sql.IssueEvent) error {
	glog.Infof("Fetching issue-events with id bigger than %d", *i)

	rows, err := db.Model(sql.IssueEvent{}).Where("id > ?", int(*i)).Rows()
	if err != nil {
		return err
	}

	count := 0
	for rows.Next() {
		var issueEvent sql.IssueEvent
		db.ScanRows(rows, &issueEvent)
		out <- issueEvent
		*i = IssueEvents(issueEvent.ID)
		count++
	}
	glog.Infof("Found and pushed %d new events", count)

	return nil
}

// Comments can fetch Comments from DB. It saves the ID of the last comment fetched
type Comments int

// Fetch retrieves comments from DB, but only fetches comments created since last call
func (c *Comments) Fetch(db *gorm.DB, out chan sql.Comment) error {
	glog.Infof("Fetching comments with id bigger than %d", *c)

	rows, err := db.Model(sql.Comment{}).Where("id > ?", int(*c)).Rows()
	if err != nil {
		return err
	}

	count := 0
	for rows.Next() {
		var comment sql.Comment
		db.ScanRows(rows, &comment)
		out <- comment
		*c = Comments(comment.ID)
		count++
	}
	glog.Infof("Found and pushed %d new comments", count)

	return nil
}

// Fetcher is a utility class used to Fetch all types of events
type Fetcher struct {
	issues             Issues
	issueEvents        IssueEvents
	comments           Comments
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
	if err := f.issues.Fetch(db, f.issuesChannel); err != nil {
		return err
	}
	if err := f.issueEvents.Fetch(db, f.issueEventsChannel); err != nil {
		return err
	}
	if err := f.comments.Fetch(db, f.commentsChannel); err != nil {
		return err
	}
	return nil
}

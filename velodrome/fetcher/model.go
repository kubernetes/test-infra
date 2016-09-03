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

import "time"

// Issue is a pull-request or issue. Its format fits into the ORM
type Issue struct {
	ID             int
	Labels         []Label
	Title          string `gorm:"type:varchar(1000)"`
	Body           string `gorm:"type:text"`
	User           string
	Assignee       *string
	State          string
	Comments       int
	IsPR           bool
	IssueClosedAt  *time.Time
	IssueCreatedAt time.Time
	IssueUpdatedAt time.Time
}

// IssueEvent is an event associated to a specific issued.
// It's format fits into the ORM
type IssueEvent struct {
	ID             int
	Label          *string
	Event          string
	EventCreatedAt time.Time
	IssueId        int
	Assignee       *string
	Actor          *string
}

// Label is a tag on an Issue. It's format fits into the ORM.
type Label struct {
	IssueID int
	Name    string
}

// Comment is either a pull-request comment or an issue comment.
type Comment struct {
	ID               int
	IssueID          int
	Body             string `gorm:"type:text"`
	User             string
	CommentCreatedAt time.Time
	CommentUpdatedAt time.Time
	PullRequest      bool
}

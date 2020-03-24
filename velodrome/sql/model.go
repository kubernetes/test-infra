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

package sql

import (
	"regexp"
	"time"
)

// Issue is a pull-request or issue. Its format fits into the ORM
type Issue struct {
	Repository     string `gorm:"primary_key"`
	ID             string `gorm:"primary_key"`
	Labels         []Label
	Assignees      []Assignee
	Title          string `gorm:"type:varchar(1000)"`
	Body           string `gorm:"type:text"`
	User           string
	State          string
	Comments       int
	IsPR           bool
	IssueClosedAt  *time.Time
	IssueCreatedAt time.Time
	IssueUpdatedAt time.Time
}

// FindLabels returns the list of labels matching the regex
func (issue *Issue) FindLabels(regex *regexp.Regexp) []Label {
	labels := []Label{}

	for _, label := range issue.Labels {
		if regex.MatchString(label.Name) {
			labels = append(labels, label)
		}
	}

	return labels
}

// IssueEvent is an event associated to a specific issued.
// It's format fits into the ORM
type IssueEvent struct {
	Repository     string `gorm:"primary_key;index:repo_created"`
	ID             string `gorm:"primary_key"`
	Label          *string
	Event          string
	EventCreatedAt time.Time `gorm:"index:repo_created"`
	IssueID        string
	Assignee       *string
	Actor          *string
}

// Label is a tag on an Issue. It's format fits into the ORM.
type Label struct {
	Repository string
	IssueID    string
	Name       string
}

// Assignee is assigned to an issue.
type Assignee struct {
	Repository string
	IssueID    string
	Name       string
}

// Comment is either a pull-request comment or an issue comment.
type Comment struct {
	Repository       string `gorm:"primary_key;index:repo_comment_created"`
	ID               string `gorm:"primary_key"`
	IssueID          string
	Body             string `gorm:"type:text"`
	User             string
	CommentCreatedAt time.Time `gorm:"index:repo_comment_created"`
	CommentUpdatedAt time.Time
	PullRequest      bool
}

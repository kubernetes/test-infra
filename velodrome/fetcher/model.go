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

	"github.com/google/go-github/github"
)

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

// NewIssue creates a new (orm) Issue from a github Issue
func NewIssue(gIssue *github.Issue) (*Issue, error) {
	if gIssue.Number == nil ||
		gIssue.Title == nil ||
		gIssue.User == nil ||
		gIssue.User.Login == nil ||
		gIssue.State == nil ||
		gIssue.Comments == nil ||
		gIssue.CreatedAt == nil ||
		gIssue.UpdatedAt == nil {
		return nil, fmt.Errorf("Issue is missing mandatory field: %+v", gIssue)
	}
	var closedAt *time.Time
	if gIssue.ClosedAt != nil {
		closedAt = gIssue.ClosedAt
	}
	var assignee *string
	if gIssue.Assignee != nil {
		assignee = gIssue.Assignee.Login
	}
	var body string
	if gIssue.Body != nil {
		body = *gIssue.Body
	}
	isPR := (gIssue.PullRequestLinks != nil && gIssue.PullRequestLinks.URL != nil)
	labels, err := newLabels(*gIssue.Number, gIssue.Labels)
	if err != nil {
		return nil, err
	}

	return &Issue{
		ID:             *gIssue.Number,
		Labels:         labels,
		Title:          *gIssue.Title,
		Body:           body,
		User:           *gIssue.User.Login,
		Assignee:       assignee,
		State:          *gIssue.State,
		Comments:       *gIssue.Comments,
		IsPR:           isPR,
		IssueClosedAt:  closedAt,
		IssueCreatedAt: *gIssue.CreatedAt,
		IssueUpdatedAt: *gIssue.UpdatedAt,
	}, nil
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

// NewIssueEvent creates a new (orm) Issue from a github Issue
func NewIssueEvent(gIssueEvent *github.IssueEvent) (*IssueEvent, error) {
	if gIssueEvent.ID == nil ||
		gIssueEvent.Event == nil ||
		gIssueEvent.CreatedAt == nil ||
		gIssueEvent.Issue == nil ||
		gIssueEvent.Issue.Number == nil {
		return nil, fmt.Errorf("IssueEvent is missing mandatory field: %+v", gIssueEvent)
	}

	var label *string
	if gIssueEvent.Label != nil {
		label = gIssueEvent.Label.Name
	}
	var assignee *string
	if gIssueEvent.Assignee != nil {
		assignee = gIssueEvent.Assignee.Login
	}
	var actor *string
	if gIssueEvent.Actor != nil {
		actor = gIssueEvent.Actor.Login
	}

	return &IssueEvent{
		ID:             *gIssueEvent.ID,
		Label:          label,
		Event:          *gIssueEvent.Event,
		EventCreatedAt: *gIssueEvent.CreatedAt,
		IssueId:        *gIssueEvent.Issue.Number,
		Assignee:       assignee,
		Actor:          actor,
	}, nil
}

// Label is a tag on an Issue. It's format fits into the ORM.
type Label struct {
	IssueID int
	Name    string
}

// newLabels creates a new Label for each label in the issue
func newLabels(issueId int, gLabels []github.Label) ([]Label, error) {
	labels := []Label{}

	for _, label := range gLabels {
		if label.Name == nil {
			return nil, fmt.Errorf("Label is missing name field")
		}
		labels = append(labels, Label{IssueID: issueId, Name: *label.Name})
	}

	return labels, nil
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

// NewIssueComment creates a Comment from a github.IssueComment
func NewIssueComment(issueId int, gComment *github.IssueComment) (*Comment, error) {
	if gComment.ID == nil ||
		gComment.Body == nil ||
		gComment.CreatedAt == nil ||
		gComment.UpdatedAt == nil {
		return nil, fmt.Errorf("IssueComment is missing mandatory field: %s", gComment)
	}
	var login string
	if gComment.User != nil && gComment.User.Login != nil {
		login = *gComment.User.Login
	}

	return &Comment{
		ID:               *gComment.ID,
		IssueID:          issueId,
		Body:             *gComment.Body,
		User:             login,
		CommentCreatedAt: *gComment.CreatedAt,
		CommentUpdatedAt: *gComment.UpdatedAt,
		PullRequest:      false,
	}, nil
}

// NewPullComment creates a Comment from a github.PullRequestComment
func NewPullComment(issueId int, gComment *github.PullRequestComment) (*Comment, error) {
	if gComment.ID == nil ||
		gComment.Body == nil ||
		gComment.CreatedAt == nil ||
		gComment.UpdatedAt == nil {
		return nil, fmt.Errorf("PullComment is missing mandatory field: %s", gComment)
	}
	var login string
	if gComment.User != nil && gComment.User.Login != nil {
		login = *gComment.User.Login
	}
	return &Comment{
		ID:               *gComment.ID,
		IssueID:          issueId,
		Body:             *gComment.Body,
		User:             login,
		CommentCreatedAt: *gComment.CreatedAt,
		CommentUpdatedAt: *gComment.UpdatedAt,
		PullRequest:      true,
	}, nil
}

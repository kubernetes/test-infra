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

	"github.com/google/go-github/github"
)

func makeIssue(number int,
	title, body, state, user, assignee, prUrl string,
	comments int,
	isPullRequest bool,
	createdAt, updatedAt, closedAt time.Time) *Issue {

	var pAssignee *string
	if assignee != "" {
		pAssignee = &assignee
	}

	var pClosedAt *time.Time
	if !closedAt.IsZero() {
		pClosedAt = &closedAt
	}

	return &Issue{
		ID:             number,
		Title:          title,
		Body:           body,
		User:           user,
		Assignee:       pAssignee,
		State:          state,
		Comments:       comments,
		IsPR:           isPullRequest,
		IssueClosedAt:  pClosedAt,
		IssueCreatedAt: createdAt,
		IssueUpdatedAt: updatedAt,
	}
}

func makeGithubIssue(number int,
	title, body, state, user, assignee, prUrl string,
	comments int,
	isPullRequest bool,
	createdAt, updatedAt, closedAt time.Time) *github.Issue {

	var pBody *string
	if body != "" {
		pBody = &body
	}
	var gAssignee *github.User
	if assignee != "" {
		gAssignee = &github.User{Login: &assignee}
	}
	var pullRequest *github.PullRequestLinks
	if prUrl != "" {
		pullRequest = &github.PullRequestLinks{URL: &prUrl}
	}
	gUser := &github.User{Login: &user}
	var pClosedAt *time.Time
	if !closedAt.IsZero() {
		pClosedAt = &closedAt
	}

	return &github.Issue{
		Number:           &number,
		Title:            &title,
		Body:             pBody,
		State:            &state,
		User:             gUser,
		Assignee:         gAssignee,
		Comments:         &comments,
		PullRequestLinks: pullRequest,
		CreatedAt:        &createdAt,
		UpdatedAt:        &updatedAt,
		ClosedAt:         pClosedAt,
	}
}

func TestNewIssue(t *testing.T) {
	tests := []struct {
		gIssue *github.Issue
		mIssue *Issue
	}{
		// Only mandatory
		{
			gIssue: makeGithubIssue(1, "Title", "", "State", "User", "", "",
				5, false,
				time.Date(1900, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Time{}),
			mIssue: makeIssue(1, "Title", "", "State", "User", "", "",
				5, false,
				time.Date(1900, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Time{}),
		},
		// All fields
		{
			gIssue: makeGithubIssue(1, "Title", "Body", "State", "User", "Assignee",
				"PRLink", 5, true,
				time.Date(1900, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2100, time.January, 1, 19, 30, 0, 0, time.UTC)),
			mIssue: makeIssue(1, "Title", "Body", "State", "User", "Assignee",
				"PRLink", 5, true,
				time.Date(1900, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2100, time.January, 1, 19, 30, 0, 0, time.UTC)),
		},
		// Missing mandatory fields returns nil
		{
			&github.Issue{},
			nil,
		},
	}

	for _, test := range tests {
		// Ignore the error, we will compare the nil issue to expected
		actualIssue, _ := NewIssue(test.gIssue)
		if actualIssue != nil && reflect.DeepEqual(actualIssue.Labels, []Label{}) {
			actualIssue.Labels = nil
		}
		if !reflect.DeepEqual(actualIssue, test.mIssue) {
			t.Error("Actual: ", actualIssue,
				"doesn't match expected: ", test.mIssue)
		}
	}
}

func makeIssueEvent(
	eventId, issueId int,
	label, event, assignee, actor string,
	createdAt time.Time) *IssueEvent {

	var pLabel, pAssignee, pActor *string
	if label != "" {
		pLabel = &label
	}
	if actor != "" {
		pActor = &actor
	}
	if assignee != "" {
		pAssignee = &assignee
	}

	return &IssueEvent{
		ID:             eventId,
		Label:          pLabel,
		Event:          event,
		EventCreatedAt: createdAt,
		IssueId:        issueId,
		Assignee:       pAssignee,
		Actor:          pActor,
	}
}

func makeGithubIssueEvent(
	eventId, issueId int,
	label, event, assignee, actor string,
	createdAt time.Time) *github.IssueEvent {

	var gLabel *github.Label
	if label != "" {
		gLabel = &github.Label{Name: &label}
	}

	var gAssignee, gActor *github.User
	if assignee != "" {
		gAssignee = &github.User{Login: &assignee}
	}

	if actor != "" {
		gActor = &github.User{Login: &actor}
	}

	return &github.IssueEvent{
		ID:        &eventId,
		Label:     gLabel,
		Event:     &event,
		CreatedAt: &createdAt,
		Issue:     &github.Issue{Number: &issueId},
		Assignee:  gAssignee,
		Actor:     gActor,
	}
}

func TestNewIssueEvent(t *testing.T) {
	tests := []struct {
		gIssueEvent *github.IssueEvent
		mIssueEvent *IssueEvent
	}{
		// Only mandatory
		{
			gIssueEvent: makeGithubIssueEvent(1, 2, "", "Event", "", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
			mIssueEvent: makeIssueEvent(1, 2, "", "Event", "", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
		},
		// All fields
		{
			gIssueEvent: makeGithubIssueEvent(1, 2, "Label", "Event", "Assignee", "Actor",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
			mIssueEvent: makeIssueEvent(1, 2, "Label", "Event", "Assignee", "Actor",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
		},
		// Missing mandatory fields returns nil
		{
			&github.IssueEvent{},
			nil,
		},
	}

	for _, test := range tests {
		actualIssueEvent, _ := NewIssueEvent(test.gIssueEvent)
		if !reflect.DeepEqual(actualIssueEvent, test.mIssueEvent) {
			t.Error("Actual: ", actualIssueEvent,
				"doesn't match expected: ", test.mIssueEvent)
		}
	}
}

func createLabel(name string) github.Label {
	return github.Label{Name: &name}
}

func TestNewLabels(t *testing.T) {
	tests := []struct {
		gLabels        []github.Label
		issueId        int
		expectedLabels []Label
	}{
		// Empty list gives empty list
		{
			[]github.Label{},
			1,
			[]Label{},
		},
		// Broken label
		{
			[]github.Label{
				createLabel("SomeLabel"),
				{},
				createLabel("OtherLabel"),
			},
			2,
			nil,
		},
	}

	for _, test := range tests {
		actualLabels, _ := newLabels(test.issueId, test.gLabels)
		if !reflect.DeepEqual(actualLabels, test.expectedLabels) {
			t.Error("Actual: ", actualLabels,
				"doesn't match expected: ", test.expectedLabels)
		}
	}
}

func makeGithubIssueComment(id int, body, login string, createdAt, updatedAt time.Time) *github.IssueComment {
	var user *github.User
	if login != "" {
		user = &github.User{Login: &login}
	}
	return &github.IssueComment{
		ID:        &id,
		User:      user,
		Body:      &body,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
	}
}

func makeGithubPullComment(id int, body, login string, createdAt, updatedAt time.Time) *github.PullRequestComment {
	var user *github.User
	if login != "" {
		user = &github.User{Login: &login}
	}
	return &github.PullRequestComment{
		ID:        &id,
		User:      user,
		Body:      &body,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
	}
}

func makeComment(issueId, Id int, body, login string, createdAt, updatedAt time.Time, pullRequest bool) *Comment {
	return &Comment{
		ID:               Id,
		IssueID:          issueId,
		Body:             body,
		User:             login,
		CommentCreatedAt: createdAt,
		CommentUpdatedAt: updatedAt,
		PullRequest:      pullRequest,
	}
}

func TestNewIssueComment(t *testing.T) {
	tests := []struct {
		gComment        *github.IssueComment
		issueId         int
		expectedComment *Comment
	}{
		{
			gComment: makeGithubIssueComment(1, "Body", "Login",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueId: 12,
			expectedComment: makeComment(12, 1, "Body", "Login",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), false),
		},
		{
			gComment: makeGithubIssueComment(1, "Body", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueId: 12,
			expectedComment: makeComment(12, 1, "Body", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), false),
		},
	}

	for _, test := range tests {
		actualComment, _ := NewIssueComment(test.issueId, test.gComment)
		if !reflect.DeepEqual(actualComment, test.expectedComment) {
			t.Error("Actual: ", actualComment,
				"doesn't match expected: ", test.expectedComment)
		}
	}
}

func TestNewPullComment(t *testing.T) {
	tests := []struct {
		gComment        *github.PullRequestComment
		issueId         int
		expectedComment *Comment
	}{
		{
			gComment: makeGithubPullComment(1, "Body", "Login",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueId: 12,
			expectedComment: makeComment(12, 1, "Body", "Login",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
		},
		{
			gComment: makeGithubPullComment(1, "Body", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueId: 12,
			expectedComment: makeComment(12, 1, "Body", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
		},
	}

	for _, test := range tests {
		actualComment, _ := NewPullComment(test.issueId, test.gComment)
		if !reflect.DeepEqual(actualComment, test.expectedComment) {
			t.Error("Actual: ", actualComment,
				"doesn't match expected: ", test.expectedComment)
		}
	}
}

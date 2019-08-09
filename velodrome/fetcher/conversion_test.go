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
	"strconv"
	"testing"
	"time"

	"k8s.io/test-infra/velodrome/sql"

	"github.com/google/go-github/github"
)

func makeIssue(number int,
	title, body, state, user, prURL, repository string,
	comments int,
	isPullRequest bool,
	createdAt, updatedAt, closedAt time.Time) *sql.Issue {

	var pClosedAt *time.Time
	if !closedAt.IsZero() {
		pClosedAt = &closedAt
	}

	return &sql.Issue{
		ID:             strconv.Itoa(number),
		Title:          title,
		Body:           body,
		User:           user,
		State:          state,
		Comments:       comments,
		IsPR:           isPullRequest,
		IssueClosedAt:  pClosedAt,
		IssueCreatedAt: createdAt,
		IssueUpdatedAt: updatedAt,
		Repository:     repository,
	}
}

func makeGitHubIssue(number int,
	title, body, state, user, prURL string,
	comments int,
	isPullRequest bool,
	createdAt, updatedAt, closedAt time.Time) *github.Issue {

	var pBody *string
	if body != "" {
		pBody = &body
	}
	var pullRequest *github.PullRequestLinks
	if prURL != "" {
		pullRequest = &github.PullRequestLinks{URL: &prURL}
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
		mIssue *sql.Issue
	}{
		// Only mandatory
		{
			gIssue: makeGitHubIssue(1, "Title", "", "State", "User", "",
				5, false,
				time.Date(1900, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Time{}),
			mIssue: makeIssue(1, "Title", "", "State", "User", "", "full/repo",
				5, false,
				time.Date(1900, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Time{}),
		},
		// All fields
		{
			gIssue: makeGitHubIssue(1, "Title", "Body", "State", "User",
				"PRLink", 5, true,
				time.Date(1900, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2100, time.January, 1, 19, 30, 0, 0, time.UTC)),
			mIssue: makeIssue(1, "Title", "Body", "State", "User",
				"PRLink", "full/repo", 5, true,
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
		actualIssue, _ := NewIssue(test.gIssue, "FULL/REPO")
		if actualIssue != nil && reflect.DeepEqual(actualIssue.Labels, []sql.Label{}) {
			actualIssue.Labels = nil
		}
		if actualIssue != nil && reflect.DeepEqual(actualIssue.Assignees, []sql.Assignee{}) {
			actualIssue.Assignees = nil
		}
		if !reflect.DeepEqual(actualIssue, test.mIssue) {
			t.Error("Actual: ", actualIssue,
				"doesn't match expected: ", test.mIssue)
		}
	}
}

func makeIssueEvent(
	eventID, issueID int,
	label, event, assignee, actor, repository string,
	createdAt time.Time) *sql.IssueEvent {

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

	return &sql.IssueEvent{
		ID:             strconv.Itoa(eventID),
		Label:          pLabel,
		Event:          event,
		EventCreatedAt: createdAt,
		IssueID:        strconv.Itoa(issueID),
		Assignee:       pAssignee,
		Actor:          pActor,
		Repository:     repository,
	}
}

func makeGitHubIssueEvent(
	eventID int64,
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
		ID:        &eventID,
		Label:     gLabel,
		Event:     &event,
		CreatedAt: &createdAt,
		Assignee:  gAssignee,
		Actor:     gActor,
	}
}

func TestNewIssueEvent(t *testing.T) {
	tests := []struct {
		gIssueEvent *github.IssueEvent
		issueID     int
		mIssueEvent *sql.IssueEvent
	}{
		// Only mandatory
		{
			gIssueEvent: makeGitHubIssueEvent(1, "", "Event", "", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueID: 2,
			mIssueEvent: makeIssueEvent(1, 2, "", "Event", "", "", "full/repo",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
		},
		// All fields
		{
			gIssueEvent: makeGitHubIssueEvent(1, "Label", "Event", "Assignee", "Actor",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueID: 2,
			mIssueEvent: makeIssueEvent(1, 2, "Label", "Event", "Assignee", "Actor", "full/repo",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC)),
		},
		// Missing mandatory fields returns nil
		{
			&github.IssueEvent{},
			2,
			nil,
		},
	}

	for _, test := range tests {
		actualIssueEvent, _ := NewIssueEvent(test.gIssueEvent, test.issueID, "FULL/REPO")
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
		issueID        int
		expectedLabels []sql.Label
	}{
		// Empty list gives empty list
		{
			[]github.Label{},
			1,
			[]sql.Label{},
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
		actualLabels, _ := newLabels(test.issueID, test.gLabels, "FULL/REPO")
		if !reflect.DeepEqual(actualLabels, test.expectedLabels) {
			t.Error("Actual: ", actualLabels,
				"doesn't match expected: ", test.expectedLabels)
		}
	}
}

func makeGitHubIssueComment(id int64, body, login string, createdAt, updatedAt time.Time) *github.IssueComment {
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

func makeGitHubPullComment(id int64, body, login string, createdAt, updatedAt time.Time) *github.PullRequestComment {
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

func makeComment(issueID, iD int, body, login, repository string, createdAt, updatedAt time.Time, pullRequest bool) *sql.Comment {
	return &sql.Comment{
		ID:               strconv.Itoa(iD),
		IssueID:          strconv.Itoa(issueID),
		Body:             body,
		User:             login,
		CommentCreatedAt: createdAt,
		CommentUpdatedAt: updatedAt,
		PullRequest:      pullRequest,
		Repository:       repository,
	}
}

func TestNewIssueComment(t *testing.T) {
	tests := []struct {
		gComment        *github.IssueComment
		issueID         int
		expectedComment *sql.Comment
	}{
		{
			gComment: makeGitHubIssueComment(1, "Body", "Login",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueID: 12,
			expectedComment: makeComment(12, 1, "Body", "Login", "full/repo",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), false),
		},
		{
			gComment: makeGitHubIssueComment(1, "Body", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueID: 12,
			expectedComment: makeComment(12, 1, "Body", "", "full/repo",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), false),
		},
	}

	for _, test := range tests {
		actualComment, _ := NewIssueComment(test.issueID, test.gComment, "FULL/REPO")
		if !reflect.DeepEqual(actualComment, test.expectedComment) {
			t.Error("Actual: ", actualComment,
				"doesn't match expected: ", test.expectedComment)
		}
	}
}

func TestNewPullComment(t *testing.T) {
	tests := []struct {
		gComment        *github.PullRequestComment
		issueID         int
		repository      string
		expectedComment *sql.Comment
	}{
		{
			gComment: makeGitHubPullComment(1, "Body", "Login",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueID:    12,
			repository: "FULL/REPO",
			expectedComment: makeComment(12, 1, "Body", "Login", "full/repo",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
		},
		{
			gComment: makeGitHubPullComment(1, "Body", "",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC)),
			issueID:    12,
			repository: "FULL/REPO",
			expectedComment: makeComment(12, 1, "Body", "", "full/repo",
				time.Date(2000, time.January, 1, 19, 30, 0, 0, time.UTC),
				time.Date(2001, time.January, 1, 19, 30, 0, 0, time.UTC), true),
		},
	}

	for _, test := range tests {
		actualComment, _ := NewPullComment(test.issueID, test.gComment, test.repository)
		if !reflect.DeepEqual(actualComment, test.expectedComment) {
			t.Error("Actual: ", actualComment,
				"doesn't match expected: ", test.expectedComment)
		}
	}
}

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
	"regexp"
	"strconv"
	"time"

	"github.com/golang/glog"

	"k8s.io/test-infra/velodrome/sql"
)

// IssueCreation tracks IssueCreated and if they've been commented or not
type IssueCreation struct {
	Author      string
	CreatedAt   time.Time
	Pushed      bool
	PullRequest bool
}

// FirstComment plugin: Count number
type FirstComment struct {
	DB     InfluxDatabase
	last   time.Time
	issues map[int]IssueCreation
}

// NewFirstCommentPlugin initializes the plugin. Requires an
// InfluxDatabase to push the metric
func NewFirstCommentPlugin(DB InfluxDatabase) *FirstComment {
	last, err := DB.GetLastMeasurement("first_comment")
	if err != nil {
		glog.Fatal("Failed to create FirstComment plugin: ", err)
	}
	return &FirstComment{
		DB:     DB,
		last:   last,
		issues: make(map[int]IssueCreation),
	}
}

// ReceiveIssue tracks created issues
func (m *FirstComment) ReceiveIssue(issue sql.Issue) error {
	if !issue.IsPR && issue.FindLabels(regexp.MustCompile(`kind/flake`)) == nil {
		return nil
	}
	if _, ok := m.issues[issue.ID]; ok {
		return nil
	}

	m.issues[issue.ID] = IssueCreation{
		Author:      issue.User,
		CreatedAt:   issue.IssueCreatedAt,
		PullRequest: issue.IsPR,
		Pushed:      false,
	}

	return nil
}

// Process decides if the PR has received its first comment or LGTM.
// If it already was "Pushed", then it's too late, we have processed the first event.
// If it hasn't been pushed, and it's not initated by the author or a bot, we save the metric.
func (m *FirstComment) Process(issueID int, author string, date time.Time) error {
	creation, ok := m.issues[issueID]
	if !ok {
		return nil
	}
	if creation.Pushed {
		// Only measure for first action
		return nil
	}
	if creation.Author == author ||
		author == "k8s-merge-robot" ||
		author == "k8s-bot" ||
		author == "k8s-ci-robot" ||
		author == "googlebot" {
		return nil
	}

	creation.Pushed = true
	m.issues[issueID] = creation

	first_comment := date.Sub(creation.CreatedAt)

	if !date.After(m.last) {
		return nil
	}

	return m.DB.Push(
		"first_comment",
		map[string]string{
			"pr": strconv.FormatBool(creation.PullRequest),
		},
		map[string]interface{}{
			"value": int(first_comment / time.Minute),
		},
		date,
	)

}

// ReceiveComment makes sure we received the first comment on an issue
func (m *FirstComment) ReceiveComment(comment sql.Comment) error {
	return m.Process(comment.IssueID, comment.User, comment.CommentCreatedAt)
}

// ReceiveIssueEvent filters "lgtm" events, and add the measurement
func (m *FirstComment) ReceiveIssueEvent(event sql.IssueEvent) error {
	if event.Event != "labeled" ||
		event.Label == nil ||
		*event.Label != "lgtm" ||
		event.Actor == nil {
		return nil
	}

	return m.Process(event.IssueId, *event.Actor, event.EventCreatedAt)
}

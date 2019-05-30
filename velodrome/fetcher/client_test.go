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
	"testing"
	"time"

	"github.com/google/go-github/github"
)

type FakeClient struct {
	Repository    string
	Issues        []*github.Issue
	IssueEvents   map[int][]*github.IssueEvent
	IssueComments map[int][]*github.IssueComment
	PullComments  map[int][]*github.PullRequestComment
}

func (client FakeClient) RepositoryName() string {
	return client.Repository
}

func (client FakeClient) FetchIssues(latest time.Time, c chan *github.Issue) {
	for _, issue := range client.Issues {
		c <- issue
	}
	close(c)
}

func (client FakeClient) FetchIssueEvents(issueID int, latest *int, c chan *github.IssueEvent) {
	for _, event := range client.IssueEvents[issueID] {
		c <- event
	}
	close(c)
}

func (client FakeClient) FetchIssueComments(issueID int, latest time.Time, c chan *github.IssueComment) {
	for _, comment := range client.IssueComments[issueID] {
		c <- comment
	}
	close(c)
}

func (client FakeClient) FetchPullComments(issueID int, latest time.Time, c chan *github.PullRequestComment) {
	for _, comment := range client.PullComments[issueID] {
		c <- comment
	}
	close(c)
}

func createIssueEvent(ID int64) *github.IssueEvent {
	return &github.IssueEvent{ID: &ID}
}

func TestHasID(t *testing.T) {
	tests := []struct {
		events []*github.IssueEvent
		ID     int
		isIn   bool
	}{
		{
			[]*github.IssueEvent{},
			1,
			false,
		},
		{
			[]*github.IssueEvent{
				createIssueEvent(1),
			},
			1,
			true,
		},
		{
			[]*github.IssueEvent{
				createIssueEvent(0),
				createIssueEvent(2),
			},
			1,
			false,
		},
		{
			[]*github.IssueEvent{
				createIssueEvent(2),
				createIssueEvent(3),
				createIssueEvent(1),
			},
			1,
			true,
		},
	}

	for _, test := range tests {
		found := hasID(test.events, test.ID)
		if found != test.isIn {
			if found {
				t.Error(test.ID, "was found in", test.events, "but shouldn't")
			} else {
				t.Error(test.ID, "wasn't found in", test.events)
			}
		}
	}
}

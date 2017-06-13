/*
Copyright 2017 The Kubernetes Authors.

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

package matchers

import (
	"sort"
	"time"

	"github.com/google/go-github/github"
)

type Items []Item

// AddComments returns a new list with the added comments
func (i Items) AddComments(comments ...*github.IssueComment) Items {
	for _, comment := range comments {
		i = append(i, (*Comment)(comment))
	}

	sort.Sort(i)
	return i
}

// AddEvents returns a new list with the added events
func (i Items) AddEvents(events ...*github.IssueEvent) Items {
	for _, event := range events {
		i = append(i, (*Event)(event))
	}

	sort.Sort(i)
	return i
}

// AddReviewComments returns a new list with the added review comments
func (i Items) AddReviewComments(reviews ...*github.PullRequestComment) Items {
	for _, review := range reviews {
		i = append(i, (*ReviewComment)(review))
	}

	sort.Sort(i)
	return i
}

// Events returns the events from the list
func (i Items) Events() []*github.IssueEvent {
	events := []*github.IssueEvent{}

	for _, item := range i {
		events = item.AppendEvent(events)
	}

	return events
}

// Comments returns the comments from the list
func (i Items) Comments() []*github.IssueComment {
	comments := []*github.IssueComment{}

	for _, item := range i {
		comments = item.AppendComment(comments)
	}

	return comments
}

// ReviewComments returns the review comments from the list
func (i Items) ReviewComments() []*github.PullRequestComment {
	reviews := []*github.PullRequestComment{}

	for _, item := range i {
		reviews = item.AppendReviewComment(reviews)
	}

	return reviews
}

// Swap two Items
func (i Items) Swap(x, y int) {
	i[x], i[y] = i[y], i[x]
}

// Less compares two Items
func (i Items) Less(x, y int) bool {
	if i[x].Date() == nil {
		return true
	} else if i[y].Date() == nil {
		return false
	}

	return i[x].Date().Before(*i[y].Date())
}

// Len is the number of Items
func (i Items) Len() int {
	return len(i)
}

// Empty checks for emptiness
func (i Items) IsEmpty() bool {
	return len(i) == 0
}

// GetLast returns the last item from the list
func (i Items) GetLast() Item {
	return i[len(i)-1]
}

// GetLast returns the first item from the list
func (i Items) GetFirst() Item {
	return i[0]
}

// Filter will return the list of matching Items
func (i Items) Filter(matcher Matcher) Items {
	matches := Items{}

	for _, item := range i {
		if item.Match(matcher) {
			matches = append(matches, item)
		}
	}

	return matches
}

// LastDate returns the date of the last matching event, or deflt if no match
func (i Items) LastDate(deflt *time.Time) *time.Time {
	if i.IsEmpty() {
		return deflt
	}
	return i.GetLast().Date()
}

// FirstDate returns the date of the first matching event, or deflt if no match
func (i Items) FirstDate(deflt *time.Time) *time.Time {
	if i.IsEmpty() {
		return deflt
	}
	return i.GetFirst().Date()
}

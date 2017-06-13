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
	"time"

	"github.com/google/go-github/github"
)

type Item interface {
	Match(matcher Matcher) bool
	Date() *time.Time
	AppendEvent(events []*github.IssueEvent) []*github.IssueEvent
	AppendComment(comments []*github.IssueComment) []*github.IssueComment
	AppendReviewComment(comments []*github.PullRequestComment) []*github.PullRequestComment
}

type Event github.IssueEvent

var _ Item = &Event{}

func (e *Event) Match(matcher Matcher) bool {
	return matcher.MatchEvent((*github.IssueEvent)(e))
}

func (e *Event) Date() *time.Time {
	return e.CreatedAt
}

func (e *Event) AppendEvent(events []*github.IssueEvent) []*github.IssueEvent {
	return append(events, (*github.IssueEvent)(e))
}

func (e *Event) AppendComment(comments []*github.IssueComment) []*github.IssueComment {
	return comments
}

func (e *Event) AppendReviewComment(reviews []*github.PullRequestComment) []*github.PullRequestComment {
	return reviews
}

type Comment github.IssueComment

var _ Item = &Comment{}

func (c *Comment) Match(matcher Matcher) bool {
	return matcher.MatchComment((*github.IssueComment)(c))
}

func (c *Comment) Date() *time.Time {
	return c.UpdatedAt
}

func (c *Comment) AppendEvent(events []*github.IssueEvent) []*github.IssueEvent {
	return events
}

func (c *Comment) AppendComment(comments []*github.IssueComment) []*github.IssueComment {
	return append(comments, (*github.IssueComment)(c))
}

func (c *Comment) AppendReviewComment(reviews []*github.PullRequestComment) []*github.PullRequestComment {
	return reviews
}

type ReviewComment github.PullRequestComment

var _ Item = &ReviewComment{}

func (r *ReviewComment) Match(matcher Matcher) bool {
	return matcher.MatchReviewComment((*github.PullRequestComment)(r))
}

func (r *ReviewComment) Date() *time.Time {
	return r.UpdatedAt
}

func (r *ReviewComment) AppendEvent(events []*github.IssueEvent) []*github.IssueEvent {
	return events
}

func (r *ReviewComment) AppendComment(comments []*github.IssueComment) []*github.IssueComment {
	return comments
}

func (r *ReviewComment) AppendReviewComment(reviews []*github.PullRequestComment) []*github.PullRequestComment {
	return append(reviews, (*github.PullRequestComment)(r))
}

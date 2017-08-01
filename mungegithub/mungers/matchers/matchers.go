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

package matchers

// Matcher is an interface to match an event
import (
	"strings"
	"time"

	"fmt"

	"github.com/google/go-github/github"
)

// Matcher matches against a comment or an event
type Matcher interface {
	MatchEvent(event *github.IssueEvent) bool
	MatchComment(comment *github.IssueComment) bool
	MatchReviewComment(comment *github.PullRequestComment) bool
}

// CreatedAfter matches comments created after the time
type CreatedAfter time.Time

var _ Matcher = CreatedAfter{}

// MatchComment returns true if the comment is created after the time
func (c CreatedAfter) MatchComment(comment *github.IssueComment) bool {
	if comment == nil || comment.CreatedAt == nil {
		return false
	}
	return comment.CreatedAt.After(time.Time(c))
}

// MatchEvent returns true if the event is created after the time
func (c CreatedAfter) MatchEvent(event *github.IssueEvent) bool {
	if event == nil || event.CreatedAt == nil {
		return false
	}
	return event.CreatedAt.After(time.Time(c))
}

// MatchReviewComment returns true if the review comment is created after the time
func (c CreatedAfter) MatchReviewComment(review *github.PullRequestComment) bool {
	if review == nil || review.CreatedAt == nil {
		return false
	}
	return review.CreatedAt.After(time.Time(c))
}

// CreatedBefore matches Items created before the time
type CreatedBefore time.Time

var _ Matcher = CreatedBefore{}

// MatchComment returns true if the comment is created before the time
func (c CreatedBefore) MatchComment(comment *github.IssueComment) bool {
	if comment == nil || comment.CreatedAt == nil {
		return false
	}
	return comment.CreatedAt.Before(time.Time(c))
}

// MatchEvent returns true if the event is created before the time
func (c CreatedBefore) MatchEvent(event *github.IssueEvent) bool {
	if event == nil || event.CreatedAt == nil {
		return false
	}
	return event.CreatedAt.Before(time.Time(c))
}

// MatchReviewComment returns true if the review comment is created before the time
func (c CreatedBefore) MatchReviewComment(review *github.PullRequestComment) bool {
	if review == nil || review.CreatedAt == nil {
		return false
	}
	return review.CreatedAt.Before(time.Time(c))
}

// UpdatedAfter matches comments updated after the time
type UpdatedAfter time.Time

var _ Matcher = UpdatedAfter{}

// MatchComment returns true if the comment is updated after the time
func (u UpdatedAfter) MatchComment(comment *github.IssueComment) bool {
	if comment == nil || comment.UpdatedAt == nil {
		return false
	}
	return comment.UpdatedAt.After(time.Time(u))
}

// MatchEvent returns true if the event is updated after the time
func (u UpdatedAfter) MatchEvent(event *github.IssueEvent) bool {
	if event == nil || event.CreatedAt == nil {
		return false
	}
	return event.CreatedAt.Before(time.Time(u))
}

// MatchReviewComment returns true if the review comment is updated after the time
func (u UpdatedAfter) MatchReviewComment(review *github.PullRequestComment) bool {
	if review == nil || review.UpdatedAt == nil {
		return false
	}
	return review.UpdatedAt.After(time.Time(u))
}

// UpdatedBefore matches Items updated before the time
type UpdatedBefore time.Time

var _ Matcher = UpdatedBefore{}

// MatchComment returns true if the comment is created before the time
func (u UpdatedBefore) MatchComment(comment *github.IssueComment) bool {
	if comment == nil || comment.UpdatedAt == nil {
		return false
	}
	return comment.UpdatedAt.Before(time.Time(u))
}

// MatchEvent returns true if the event is created before the time
func (u UpdatedBefore) MatchEvent(event *github.IssueEvent) bool {
	if event == nil || event.CreatedAt == nil {
		return false
	}
	return event.CreatedAt.Before(time.Time(u))
}

// MatchReviewComment returns true if the review comment is created before the time
func (u UpdatedBefore) MatchReviewComment(review *github.PullRequestComment) bool {
	if review == nil || review.UpdatedAt == nil {
		return false
	}
	return review.UpdatedAt.Before(time.Time(u))
}

type validAuthorMatcher struct{}

func ValidAuthor() Matcher {
	return validAuthorMatcher{}
}

func (v validAuthorMatcher) MatchEvent(event *github.IssueEvent) bool {
	return event != nil && event.Actor != nil && event.Actor.Login != nil
}

func (v validAuthorMatcher) MatchComment(comment *github.IssueComment) bool {
	return comment != nil && comment.User != nil && comment.User.Login != nil
}

func (v validAuthorMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return review != nil && review.User != nil && review.User.Login != nil
}

type AuthorLogin string

var _ Matcher = AuthorLogin("")

func (a AuthorLogin) MatchEvent(event *github.IssueEvent) bool {
	if !(ValidAuthor()).MatchEvent(event) {
		return false
	}

	return strings.ToLower(*event.Actor.Login) == strings.ToLower(string(a))
}

func (a AuthorLogin) MatchComment(comment *github.IssueComment) bool {
	fmt.Printf("matching comment: %v\n", comment)
	if !(ValidAuthor()).MatchComment(comment) {
		fmt.Println("comment does not have a valid author")
		return false
	}

	fmt.Printf("comparing %s from comment to %s from matcher\n", strings.ToLower(*comment.User.Login), strings.ToLower(string(a)))
	return strings.ToLower(*comment.User.Login) == strings.ToLower(string(a))
}

func (a AuthorLogin) MatchReviewComment(review *github.PullRequestComment) bool {
	if !(ValidAuthor()).MatchReviewComment(review) {
		return false
	}

	return strings.ToLower(*review.User.Login) == strings.ToLower(string(a))
}

func AuthorLogins(authors ...string) Matcher {
	matchers := []Matcher{}

	for _, author := range authors {
		matchers = append(matchers, AuthorLogin(author))
	}

	return Or(matchers...)
}

func AuthorUsers(users ...*github.User) Matcher {
	authors := []string{}

	for _, user := range users {
		if user == nil || user.Login == nil {
			continue
		}
		authors = append(authors, *user.Login)
	}

	return AuthorLogins(authors...)
}

// addLabelMatcher searches for "labeled" event.
type addLabelMatcher struct{}

func AddLabel() Matcher {
	return addLabelMatcher{}
}

// Match if the event is of type "labeled"
func (a addLabelMatcher) MatchEvent(event *github.IssueEvent) bool {
	if event == nil || event.Event == nil {
		return false
	}
	return *event.Event == "labeled"
}

func (a addLabelMatcher) MatchComment(comment *github.IssueComment) bool {
	return false
}

func (a addLabelMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

// LabelName searches for event whose label starts with the string
type LabelName string

var _ Matcher = LabelName("")

// Match if the label starts with the string
func (l LabelName) MatchEvent(event *github.IssueEvent) bool {
	if event == nil || event.Label == nil || event.Label.Name == nil {
		return false
	}
	return *event.Label.Name == string(l)
}

func (l LabelName) MatchComment(comment *github.IssueComment) bool {
	return false
}

func (l LabelName) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

// LabelPrefix searches for event whose label starts with the string
type LabelPrefix string

var _ Matcher = LabelPrefix("")

// Match if the label starts with the string
func (l LabelPrefix) MatchEvent(event *github.IssueEvent) bool {
	if event == nil || event.Label == nil || event.Label.Name == nil {
		return false
	}
	return strings.HasPrefix(*event.Label.Name, string(l))
}

func (l LabelPrefix) MatchComment(comment *github.IssueComment) bool {
	return false
}

func (l LabelPrefix) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

type eventTypeMatcher struct{}

func EventType() Matcher {
	return eventTypeMatcher{}
}

func (c eventTypeMatcher) MatchEvent(event *github.IssueEvent) bool {
	return true
}

func (c eventTypeMatcher) MatchComment(comment *github.IssueComment) bool {
	return false
}

func (c eventTypeMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

type commentTypeMatcher struct{}

func CommentType() Matcher {
	return commentTypeMatcher{}
}

func (c commentTypeMatcher) MatchEvent(event *github.IssueEvent) bool {
	return false
}

func (c commentTypeMatcher) MatchComment(comment *github.IssueComment) bool {
	return true
}

func (c commentTypeMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

type reviewCommentTypeMatcher struct{}

func ReviewCommentType() Matcher {
	return reviewCommentTypeMatcher{}
}

func (c reviewCommentTypeMatcher) MatchEvent(event *github.IssueEvent) bool {
	return false
}

func (c reviewCommentTypeMatcher) MatchComment(comment *github.IssueComment) bool {
	return false
}

func (c reviewCommentTypeMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return true
}

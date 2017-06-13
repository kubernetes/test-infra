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

package comment

import (
	"strings"
	"time"

	"github.com/google/go-github/github"
)

// Comment is a struct that represents a generic text post on github.
type Comment struct {
	Body      *string
	Author    *string
	CreatedAt *time.Time
	UpdatedAt *time.Time
	HTMLURL   *string

	Source interface{}
}

func FromIssueComment(ic *github.IssueComment) *Comment {
	if ic == nil {
		return nil
	}
	var login *string = nil
	if ic.User != nil {
		login = ic.User.Login
	}
	return &Comment{
		Body:      ic.Body,
		Author:    login,
		CreatedAt: ic.CreatedAt,
		UpdatedAt: ic.UpdatedAt,
		HTMLURL:   ic.HTMLURL,
		Source:    ic,
	}
}

func FromIssueComments(ics []*github.IssueComment) []*Comment {
	comments := []*Comment{}
	for _, ic := range ics {
		comments = append(comments, FromIssueComment(ic))
	}
	return comments
}

func FromReviewComment(rc *github.PullRequestComment) *Comment {
	if rc == nil {
		return nil
	}
	var login *string = nil
	if rc.User != nil {
		login = rc.User.Login
	}
	return &Comment{
		Body:      rc.Body,
		Author:    login,
		CreatedAt: rc.CreatedAt,
		UpdatedAt: rc.UpdatedAt,
		HTMLURL:   rc.HTMLURL,
		Source:    rc,
	}
}

func FromReviewComments(rcs []*github.PullRequestComment) []*Comment {
	comments := []*Comment{}
	for _, rc := range rcs {
		comments = append(comments, FromReviewComment(rc))
	}
	return comments
}

func FromReview(review *github.PullRequestReview) *Comment {
	if review == nil {
		return nil
	}
	var login *string = nil
	if review.User != nil {
		login = review.User.Login
	}
	return &Comment{
		Body:      review.Body,
		Author:    login,
		CreatedAt: review.SubmittedAt,
		UpdatedAt: review.SubmittedAt,
		HTMLURL:   review.HTMLURL,
		Source:    review,
	}
}

func FromReviews(reviews []*github.PullRequestReview) []*Comment {
	comments := []*Comment{}
	for _, review := range reviews {
		comments = append(comments, FromReview(review))
	}
	return comments
}

// Matcher is an interface to match a comment
type Matcher interface {
	Match(comment *Comment) bool
}

// CreatedAfter matches comments created after the time
type CreatedAfter time.Time

// Match returns true if the comment is created after the time
func (c CreatedAfter) Match(comment *Comment) bool {
	if comment == nil || comment.CreatedAt == nil {
		return false
	}
	return comment.CreatedAt.After(time.Time(c))
}

// CreatedBefore matches comments created before the time
type CreatedBefore time.Time

// Match returns true if the comment is created before the time
func (c CreatedBefore) Match(comment *Comment) bool {
	if comment == nil || comment.CreatedAt == nil {
		return false
	}
	return comment.CreatedAt.Before(time.Time(c))
}

// ValidAuthor validates that a comment has the author set
type ValidAuthor struct{}

// Match if the comment has a valid author
func (ValidAuthor) Match(comment *Comment) bool {
	return comment != nil && comment.Author != nil
}

// AuthorLogin matches comment made by this Author
type AuthorLogin string

// Match if the Author is a match (ignoring case)
func (a AuthorLogin) Match(comment *Comment) bool {
	if !(ValidAuthor{}).Match(comment) {
		return false
	}

	return strings.ToLower(*comment.Author) == strings.ToLower(string(a))
}

// Author matches comment made by this github user.
type Author github.User

// Match if the Author is a match.
func (a Author) Match(comment *Comment) bool {
	if !(ValidAuthor{}).Match(comment) {
		return false
	}
	return AuthorLogin(*a.Login).Match(comment)
}

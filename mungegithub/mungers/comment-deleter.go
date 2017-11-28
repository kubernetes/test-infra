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

package mungers

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	commentDeleterName = "comment-deleter"
)

var (
	_        = glog.Infof
	deleters = []StaleIssueComment{}
)

// CommentDeleter looks for comments which are no longer useful
// and deletes them
type CommentDeleter struct{}

func init() {
	RegisterMungerOrDie(CommentDeleter{})
}

// StaleIssueComment is an interface for a munger which writes issue comments which might go stale
// and which should be cleaned up.
type StaleIssueComment interface {
	StaleIssueComments(*github.MungeObject, []*githubapi.IssueComment) []*githubapi.IssueComment
}

// RegisterStaleIssueComments is the method for a munger to register that it creates issue comments
// which might go stale and need to be cleaned up.
func RegisterStaleIssueComments(s StaleIssueComment) {
	deleters = append(deleters, s)
}

// Name is the name usable in --pr-mungers
func (CommentDeleter) Name() string { return commentDeleterName }

// RequiredFeatures is a slice of 'features' that must be provided
func (CommentDeleter) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (CommentDeleter) Initialize(config *github.Config, features *features.Features) error { return nil }

// EachLoop is called at the start of every munge loop
func (CommentDeleter) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (CommentDeleter) RegisterOptions(opts *options.Options) sets.String { return nil }

func validComment(comment *githubapi.IssueComment) bool {
	if comment.User == nil || comment.User.Login == nil {
		return false
	}
	if comment.CreatedAt == nil {
		return false
	}
	if comment.Body == nil {
		return false
	}
	return true
}

// Munge is the workhorse the will actually make updates to the Issue.
func (CommentDeleter) Munge(obj *github.MungeObject) {
	if obj.Issue == nil || obj.Issue.State == nil || *obj.Issue.State == "closed" {
		return
	}

	comments, ok := obj.ListComments()
	if !ok {
		return
	}

	validComments := []*githubapi.IssueComment{}
	for i := range comments {
		comment := comments[i]
		if !validComment(comment) {
			continue
		}
		validComments = append(validComments, comment)
	}
	for _, d := range deleters {
		stale := d.StaleIssueComments(obj, validComments)
		for _, comment := range stale {
			obj.DeleteComment(comment)
		}
	}
}

func jenkinsBotComment(comment *githubapi.IssueComment) bool {
	return *comment.User.Login == jenkinsBotName
}

// Checks each comment in `comments` and returns a slice of comments for which the `stale` function was true
func forEachCommentTest(obj *github.MungeObject, comments []*githubapi.IssueComment, stale func(*github.MungeObject, *githubapi.IssueComment) bool) []*githubapi.IssueComment {
	out := []*githubapi.IssueComment{}

	for _, comment := range comments {
		if stale(obj, comment) {
			out = append(out, comment)
		}
	}
	return out
}

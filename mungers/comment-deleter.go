/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	commentDeleterName = "comment-deleter"
)

var (
	_     = glog.Infof
	funcs = []func(*github.MungeObject, *githubapi.IssueComment) bool{}
)

// CommentDeleter looks for comments which are no longer useful
// and deletes them
type CommentDeleter struct{}

func init() {
	RegisterMungerOrDie(CommentDeleter{})
}

//
func registerShouldDeleteCommentFunc(f func(*github.MungeObject, *githubapi.IssueComment) bool) {
	funcs = append(funcs, f)
}

// Name is the name usable in --pr-mungers
func (CommentDeleter) Name() string { return commentDeleterName }

// RequiredFeatures is a slice of 'features' that must be provided
func (CommentDeleter) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (CommentDeleter) Initialize(config *github.Config, features *features.Features) error { return nil }

// EachLoop is called at the start of every munge loop
func (CommentDeleter) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (CommentDeleter) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (CommentDeleter) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	comments, err := obj.ListComments()
	if err != nil {
		return
	}

	for i := range comments {
		comment := &comments[i]
		if comment.User == nil || comment.User.Login == nil {
			continue
		}
		if *comment.User.Login != botName {
			continue
		}
		if comment.Body == nil {
			continue
		}
		for _, f := range funcs {
			if f(obj, comment) {
				obj.DeleteComment(comment)
				break
			}
		}
	}
}

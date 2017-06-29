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
	"regexp"

	"k8s.io/test-infra/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	commentDeleterJenkinsName = "comment-deleter-jenkins"
	commentRegexpStr          = `GCE e2e( test)? build/test \*\*(passed|failed)\*\* for commit [[:xdigit:]]+\.
\* \[Build Log\]\([^)]+\)
\* \[Test Artifacts\]\([^)]+\)
\* \[Internal Jenkins Results\]\([^)]+\)`
	commentRegexpStrUpdated = `GCE e2e( test)? build/test \*\*(passed|failed)\*\* for commit [[:xdigit:]]+\.
\* \[Test Results\]\([^)]+\)
\* \[Build Log\]\([^)]+\)
\* \[Test Artifacts\]\([^)]+\)
\* \[Internal Jenkins Results\]\([^)]+\)`
)

var (
	_ = glog.Infof
	//Changed so that this variable is true if it compiles old or updated
	commentRegexp        = regexp.MustCompile(commentRegexpStr)
	updatedCommentRegexp = regexp.MustCompile(commentRegexpStrUpdated)
)

// CommentDeleterJenkins looks for jenkins comments which are no longer useful
// and deletes them
type CommentDeleterJenkins struct{}

func init() {
	c := CommentDeleterJenkins{}
	RegisterStaleIssueComments(c)
}

func isJenkinsTestComment(body string) bool {
	return updatedCommentRegexp.MatchString(body) || commentRegexp.MatchString(body)
}

// StaleIssueComments returns a slice of stale issue comments.
func (CommentDeleterJenkins) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	out := []*githubapi.IssueComment{}
	var last *githubapi.IssueComment

	for i := range comments {
		comment := comments[i]
		if !jenkinsBotComment(comment) {
			continue
		}

		if !isJenkinsTestComment(*comment.Body) {
			continue
		}
		if last != nil {
			out = append(out, last)
		}
		last = comment
	}
	return out
}

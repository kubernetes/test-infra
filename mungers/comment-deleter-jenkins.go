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
	"regexp"

	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	commentDeleterJenkinsName = "comment-deleter-jenkins"
	commentRegexpStr          = `GCE e2e build/test \*\*(passed|failed)\*\* for commit [[:xdigit:]]+\.
\* \[Build Log\]\(http://pr-test\.k8s\.io/[[:digit:]]+/kubernetes-pull-build-test-e2e-gce/[[:digit:]]+/build-log\.txt\)
\* \[Test Artifacts\]\(https://console\.developers\.google\.com/storage/browser/kubernetes-jenkins/pr-logs/pull/[[:digit:]]+/kubernetes-pull-build-test-e2e-gce/[[:digit:]]+/artifacts/\)
\* \[Internal Jenkins Results\]\(http://goto\.google\.com/prkubekins/job/kubernetes-pull-build-test-e2e-gce//[[:digit:]]+\)`
)

var (
	_             = glog.Infof
	commentRegexp = regexp.MustCompile(commentRegexpStr)
)

// CommentDeleterJenkins looks for jenkins comments which are no longer useful
// and deletes them
type CommentDeleterJenkins struct{}

func init() {
	c := CommentDeleterJenkins{}
	RegisterStaleComments(c)
}

func isJenkinsTestComment(body string) bool {
	return commentRegexp.MatchString(body)
}

// StaleComments returns a slice of comments which are stale
func (CommentDeleterJenkins) StaleComments(obj *github.MungeObject, comments []githubapi.IssueComment) []githubapi.IssueComment {
	out := []githubapi.IssueComment{}
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
			out = append(out, *last)
		}
		last = &comment
	}
	return out
}

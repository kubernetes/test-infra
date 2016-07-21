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
	"testing"
	"time"

	githubapi "github.com/google/go-github/github"

	github_test "k8s.io/contrib/mungegithub/github/testing"
)

const (
	passComment = `GCE e2e build/test **passed** for commit 436b966bcb6c19b792b7918e59e4f50195224065.
* [Build Log](http://pr-test.k8s.io/23958/kubernetes-pull-build-test-e2e-gce/35810/build-log.txt)
* [Test Artifacts](https://console.developers.google.com/storage/browser/kubernetes-jenkins/pr-logs/pull/23958/kubernetes-pull-build-test-e2e-gce/35810/artifacts/)
* [Internal Jenkins Results](http://goto.google.com/prkubekins/job/kubernetes-pull-build-test-e2e-gce//35810)`
	failComment = `GCE e2e build/test **failed** for commit 436b966bcb6c19b792b7918e59e4f50195224065.
* [Build Log](http://pr-test.k8s.io/23958/kubernetes-pull-build-test-e2e-gce/35830/build-log.txt)
* [Test Artifacts](https://console.developers.google.com/storage/browser/kubernetes-jenkins/pr-logs/pull/23958/kubernetes-pull-build-test-e2e-gce/35830/artifacts/)
* [Internal Jenkins Results](http://goto.google.com/prkubekins/job/kubernetes-pull-build-test-e2e-gce//35830)

Please reference the [list of currently known flakes](https://github.com/kubernetes/kubernetes/issues?q=is:issue+label:kind/flake+is:open) when examining this failure. If you request a re-test, you must reference the issue describing the flake.`
	oldBuildLinkComment = `GCE e2e test build/test **passed** for commit c801f3fc6221e4b1a54fd08378f35009dd01f052.
* [Build Log](https://storage.cloud.google.com/kubernetes-jenkins/pr-logs/pull/13006/kubernetes-pull-build-test-e2e-gce/26147/build-log.txt)
* [Test Artifacts](https://console.developers.google.com/storage/browser/kubernetes-jenkins/pr-logs/pull/13006/kubernetes-pull-build-test-e2e-gce/26147/artifacts/)
* [Internal Jenkins Results](http://goto.google.com/prkubekins/job/kubernetes-pull-build-test-e2e-gce//26147)`
	updatedPassComment = `GCE e2e build/test **passed** for commit 4c92572bef90215de02be96436364ff06a7a5435.
* [Test Results](https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/28636/kubernetes-pull-build-test-e2e-gce/48147)
* [Build Log](http://pr-test.k8s.io/28636/kubernetes-pull-build-test-e2e-gce/48147/build-log.txt)
* [Test Artifacts](https://console.developers.google.com/storage/browser/kubernetes-jenkins/pr-logs/pull/28636/kubernetes-pull-build-test-e2e-gce/48147/artifacts/)
* [Internal Jenkins Results](http://goto.google.com/prkubekins/job/kubernetes-pull-build-test-e2e-gce//48147)`
	//Updated fail comment's links are the same as updated pass's
	updatedFailComment = `GCE e2e build/test **failed** for commit 4c92572bef90215de02be96436364ff06a7a5435.
* [Test Results](https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/28636/kubernetes-pull-build-test-e2e-gce/48147)
* [Build Log](http://pr-test.k8s.io/28636/kubernetes-pull-build-test-e2e-gce/48147/build-log.txt)
* [Test Artifacts](https://console.developers.google.com/storage/browser/kubernetes-jenkins/pr-logs/pull/28636/kubernetes-pull-build-test-e2e-gce/48147/artifacts/)
* [Internal Jenkins Results](http://goto.google.com/prkubekins/job/kubernetes-pull-build-test-e2e-gce//48147)`
)

func TestIsJenkinsTestComment(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		isJenkins bool
	}{
		{
			name:      "success comment",
			value:     updatedPassComment,
			isJenkins: true,
		},
		{
			name:      "success comment",
			value:     passComment,
			isJenkins: true,
		},
		{
			name:      "fail comment",
			value:     updatedFailComment,
			isJenkins: true,
		},
		{
			name:      "fail comment",
			value:     failComment,
			isJenkins: true,
		},
		{
			name:      "other comment",
			value:     oldBuildLinkComment,
			isJenkins: true,
		},
		{
			name:      "Empty string",
			isJenkins: false,
		},
		{
			name:      "Random string",
			value:     "Bob says do it another way, ok Brendan?!",
			isJenkins: false,
		},
	}
	for testNum, test := range tests {
		output := isJenkinsTestComment(test.value)
		if output != test.isJenkins {
			t.Errorf("%d:%s: expected: %v, saw: %v for %s", testNum, test.name, test.isJenkins, output, test.value)
		}
	}
}

func comment(id int, body string) *githubapi.IssueComment {
	return github_test.Comment(id, jenkinsBotName, time.Now(), passComment)
}

func TestJenkinsStaleComments(t *testing.T) {
	c := CommentDeleterJenkins{}

	tests := []struct {
		name     string
		comments []*githubapi.IssueComment
		expected []*githubapi.IssueComment
	}{
		{
			name: "single pass",
			comments: []*githubapi.IssueComment{
				comment(1, passComment),
			},
		},
		{
			name: "double pass",
			comments: []*githubapi.IssueComment{
				comment(1, passComment),
				comment(2, passComment),
			},
			expected: []*githubapi.IssueComment{
				comment(1, passComment),
			},
		},
		{
			name: "pass fail pass",
			comments: []*githubapi.IssueComment{
				comment(1, passComment),
				comment(2, failComment),
				comment(3, passComment),
			},
			expected: []*githubapi.IssueComment{
				comment(1, passComment),
				comment(2, failComment),
			},
		},
	}
	for testNum, test := range tests {
		out := c.StaleComments(nil, test.comments)
		if len(out) != len(test.expected) {
			t.Errorf("%d:%s: len(expected):%d, len(out):%d", testNum, test.name, len(test.expected), len(out))
		}
		for _, cexpected := range test.expected {
			found := false
			for _, cout := range out {
				if *cout.ID == *cexpected.ID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%d:%s: missing %v from output", testNum, test.name, cexpected)
			}
		}
	}
}

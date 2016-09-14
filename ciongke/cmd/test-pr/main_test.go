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

	"github.com/kubernetes/test-infra/ciongke/github"
	"github.com/kubernetes/test-infra/ciongke/github/fakegithub"
)

func TestFailureComment(t *testing.T) {
	comments := []github.IssueComment{
		{
			User: github.User{"unrelated"},
			Body: "looks nice",
			ID:   0,
		},
		{
			User: github.User{"k8s-merge-robot"},
			Body: "Jenkins test failed for commit abcdef",
			ID:   1,
		},
		{
			User: github.User{"unrelated2"},
			Body: "Jenkins test is strange, what's going on there?",
			ID:   3,
		},
		{
			User: github.User{"k8s-merge-robot"},
			Body: "Jenkins test failed for commit qwerty",
			ID:   8,
		},
	}
	ghc := &fakegithub.FakeClient{
		IssueComments: map[int][]github.IssueComment{
			5: comments,
		},
		IssueCommentID: 9,
	}
	cl := testClient{
		Job:          "test-job",
		Context:      "Jenkins test",
		PRNumber:     5,
		Commit:       "abcde",
		GitHubClient: ghc,
	}
	cl.tryCreateFailureComment("url")
	newComments, _ := ghc.ListIssueComments("", "", 5)
	if len(newComments) != 3 {
		t.Errorf("Expected 3 comments after creating failed comment, got %+v", newComments)
	}
	for _, comment := range newComments {
		if comment.ID == 1 || comment.ID == 8 {
			t.Errorf("Comment not deleted: %v", comment.ID)
		}
	}
}

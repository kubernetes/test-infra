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

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestLGTMComment(t *testing.T) {
	// "a" is the author, "a", "r1", and "r2" are reviewers.
	var testcases = []struct {
		name          string
		action        string
		body          string
		commenter     string
		hasLGTM       bool
		shouldToggle  bool
		shouldComment bool
	}{
		{
			name:         "non-lgtm comment",
			action:       "created",
			body:         "uh oh",
			commenter:    "o",
			hasLGTM:      false,
			shouldToggle: false,
		},
		{
			name:         "lgtm comment by reviewer, no lgtm on pr",
			action:       "created",
			body:         "/lgtm",
			commenter:    "r1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "LGTM comment by reviewer, no lgtm on pr",
			action:       "created",
			body:         "/LGTM",
			commenter:    "r1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "lgtm comment by reviewer, no lgtm on pr",
			action:       "edited",
			body:         "/lgtm",
			commenter:    "r1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "lgtm comment by reviewer, lgtm on pr",
			action:       "created",
			body:         "/lgtm",
			commenter:    "r1",
			hasLGTM:      true,
			shouldToggle: false,
		},
		{
			name:          "lgtm comment by author",
			action:        "created",
			body:          "/lgtm",
			commenter:     "a",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: true,
		},
		{
			name:         "lgtm cancel by author",
			action:       "created",
			body:         "/lgtm cancel",
			commenter:    "a",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:          "lgtm comment by non-reviewer",
			action:        "created",
			body:          "/lgtm",
			commenter:     "o",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: true,
		},
		{
			name:          "lgtm cancel by non-reviewer",
			action:        "created",
			body:          "/lgtm cancel",
			commenter:     "o",
			hasLGTM:       true,
			shouldToggle:  false,
			shouldComment: true,
		},
		{
			name:         "lgtm comment deleted by reviewer",
			action:       "deleted",
			body:         "/lgtm",
			commenter:    "r1",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:         "lgtm cancel comment by reviewer",
			action:       "created",
			body:         "/lgtm cancel",
			commenter:    "r1",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:         "lgtm cancel comment by reviewer, no lgtm",
			action:       "created",
			body:         "/lgtm cancel",
			commenter:    "r1",
			hasLGTM:      false,
			shouldToggle: false,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		ga := &GitHubAgent{
			GitHubClient: fc,
		}
		ice := github.IssueCommentEvent{
			Action: tc.action,
			Comment: github.IssueComment{
				Body: tc.body,
				User: github.User{tc.commenter},
			},
			Issue: github.Issue{
				User:        github.User{"a"},
				Number:      5,
				State:       "open",
				PullRequest: &struct{}{},
				Assignees:   []github.User{{"a"}, {"r1"}, {"r2"}},
			},
		}
		if tc.hasLGTM {
			ice.Issue.Labels = []github.Label{{Name: lgtmLabel}}
		}
		if err := ga.lgtmComment(ice); err != nil {
			t.Errorf("For case %s, didn't expect error from lgtmComment: %v", tc.name, err)
			continue
		}
		if tc.shouldToggle {
			if tc.hasLGTM {
				if len(fc.LabelsRemoved) == 0 {
					t.Errorf("For case %s, should have removed LGTM.", tc.name)
				} else if len(fc.LabelsAdded) > 0 {
					t.Errorf("For case %s, should not have added LGTM.", tc.name)
				}
			} else {
				if len(fc.LabelsAdded) == 0 {
					t.Errorf("For case %s, should have added LGTM.", tc.name)
				} else if len(fc.LabelsRemoved) > 0 {
					t.Errorf("For case %s, should not have removed LGTM.", tc.name)
				}
			}
		} else if len(fc.LabelsRemoved) > 0 || len(fc.LabelsAdded) > 0 {
			t.Errorf("For case %s, should not have added/removed LGTM.", tc.name)
		}
		if tc.shouldComment && len(fc.IssueComments[5]) != 1 {
			t.Errorf("For case %s, should have commented.", tc.name)
		} else if !tc.shouldComment && len(fc.IssueComments[5]) != 0 {
			t.Errorf("For case %s, should not have commented.", tc.name)
		}
	}
}

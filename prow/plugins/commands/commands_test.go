/*
Copyright 2018 The Kubernetes Authors.

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

package commands

import (
	"errors"
	"regexp"
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

func TestGenericCommentHandler(t *testing.T) {
	re := regexp.MustCompile(`foo`)

	tcs := []struct {
		name         string
		issueType    IssueType
		event        github.GenericCommentEvent
		shouldHandle bool
		shouldError  bool
	}{
		{
			name:      "issue happy case",
			issueType: IssueTypeIssue,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   false,
				Body:   "foo",
			},
			shouldHandle: true,
		},
		{
			name:      "issue handler on PR",
			issueType: IssueTypeIssue,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "foo",
			},
			shouldHandle: false,
		},
		{
			name:      "pr happy case",
			issueType: IssueTypePR,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "foo",
			},
			shouldHandle: true,
		},
		{
			name:      "pr handler on issue",
			issueType: IssueTypePR,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   false,
				Body:   "foo",
			},
			shouldHandle: false,
		},
		{
			name:      "handle both (issue), expect error",
			issueType: IssueTypeBoth,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   false,
				Body:   "foo",
			},
			shouldHandle: true,
			shouldError:  true,
		},
		{
			name:      "handle both (pr)",
			issueType: IssueTypeBoth,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "foo",
			},
			shouldHandle: true,
		},
		{
			name:      "wrong action",
			issueType: IssueTypeBoth,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionEdited,
				IsPR:   true,
				Body:   "foo",
			},
			shouldHandle: false,
		},
		{
			name:      "no match",
			issueType: IssueTypeBoth,
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "bar",
			},
			shouldHandle: false,
		},
	}

	for _, tc := range tcs {
		t.Logf("Test case: %s", tc.name)

		handled := false
		handler := func(ctx *Context) error {
			handled = true
			if tc.shouldError {
				return errors.New("Some handler error (expected by test).")
			}
			return nil
		}
		c := New(tc.issueType, re, handler)
		err := c.GenericCommentHandler(plugins.PluginClient{}, tc.event)

		if (err != nil) != tc.shouldError {
			if tc.shouldError {
				t.Error("Expected a handling error, but didn't get one.")
			} else {
				t.Errorf("Unexpected handling error: %v.", err)
			}
		}
		if handled != tc.shouldHandle {
			if tc.shouldHandle {
				t.Error("Expected to handle the event, but didn't.")
			} else {
				t.Error("Didn't expect to handle the event, but did.")
			}
		}
	}
}

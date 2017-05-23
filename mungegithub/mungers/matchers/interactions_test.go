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

import (
	"regexp"
	"testing"

	"github.com/google/go-github/github"
)

func makeCommentWithBody(body string) *github.IssueComment {
	return &github.IssueComment{
		Body: &body,
	}
}

func TestNotificationName(t *testing.T) {
	if NotificationName("MESSAGE").MatchComment(&github.IssueComment{}) {
		t.Error("Shouldn't match nil body")
	}
	if NotificationName("MESSAGE").MatchComment(makeCommentWithBody("MESSAGE WRONG FORMAT")) {
		t.Error("Shouldn't match invalid match")
	}
	if !NotificationName("MESSAGE").MatchComment(makeCommentWithBody("[MESSAGE] Valid format")) {
		t.Error("Should match valid format")
	}
	if !NotificationName("MESSAGE").MatchComment(makeCommentWithBody("[MESSAGE]")) {
		t.Error("Should match with no arguments")
	}
	if !NotificationName("MESSage").MatchComment(makeCommentWithBody("[meSSAGE]")) {
		t.Error("Should match with different case")
	}
}

func TestCommandName(t *testing.T) {
	if CommandName("COMMAND").MatchComment(&github.IssueComment{}) {
		t.Error("Shouldn't match nil body")
	}
	if CommandName("COMMAND").MatchComment(makeCommentWithBody("COMMAND WRONG FORMAT")) {
		t.Error("Shouldn't match invalid format")
	}
	if !CommandName("COMMAND").MatchComment(makeCommentWithBody("/COMMAND Valid format")) {
		t.Error("Should match valid format")
	}
	if !CommandName("COMMAND").MatchComment(makeCommentWithBody("/COMMAND")) {
		t.Error("Should match with no arguments")
	}
	if !CommandName("COMmand").MatchComment(makeCommentWithBody("/ComMAND")) {
		t.Error("Should match with different case")
	}
}

func TestCommandArgmuents(t *testing.T) {
	var testcases = []struct {
		name        string
		re          string
		comment     *github.IssueComment
		shouldMatch bool
	}{
		{
			name:        "shouldn't match nil body",
			re:          ".*",
			comment:     &github.IssueComment{},
			shouldMatch: false,
		},
		{
			name:        "shouldn't match non-command",
			re:          ".*",
			comment:     makeCommentWithBody("COMMAND WRONG FORMAT"),
			shouldMatch: false,
		},
		{
			name:        "should match from the beginning of arguments",
			re:          "^carret",
			comment:     makeCommentWithBody("/command carret is the beginning of argument"),
			shouldMatch: true,
		},
		{
			name:        "shouldn't match command name",
			re:          "command",
			comment:     makeCommentWithBody("/command name is not part of match"),
			shouldMatch: false,
		},
	}
	for _, tc := range testcases {
		ca := CommandArguments(*regexp.MustCompile(tc.re))
		if tc.shouldMatch != ca.MatchComment(tc.comment) {
			t.Error(tc.name)
		}
	}
}

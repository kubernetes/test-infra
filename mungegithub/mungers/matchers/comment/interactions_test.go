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
	if NotificationName("MESSAGE").Match(&github.IssueComment{}) {
		t.Error("Shouldn't match nil body")
	}
	if NotificationName("MESSAGE").Match(makeCommentWithBody("MESSAGE WRONG FORMAT")) {
		t.Error("Shouldn't match invalid match")
	}
	if !NotificationName("MESSAGE").Match(makeCommentWithBody("[MESSAGE] Valid format")) {
		t.Error("Should match valid format")
	}
	if !NotificationName("MESSAGE").Match(makeCommentWithBody("[MESSAGE]")) {
		t.Error("Should match with no arguments")
	}
	if !NotificationName("MESSage").Match(makeCommentWithBody("[meSSAGE]")) {
		t.Error("Should match with different case")
	}
}

func TestCommandName(t *testing.T) {
	if CommandName("COMMAND").Match(&github.IssueComment{}) {
		t.Error("Shouldn't match nil body")
	}
	if CommandName("COMMAND").Match(makeCommentWithBody("COMMAND WRONG FORMAT")) {
		t.Error("Shouldn't match invalid format")
	}
	if !CommandName("COMMAND").Match(makeCommentWithBody("/COMMAND Valid format")) {
		t.Error("Should match valid format")
	}
	if !CommandName("COMMAND").Match(makeCommentWithBody("/COMMAND")) {
		t.Error("Should match with no arguments")
	}
	if !CommandName("COMmand").Match(makeCommentWithBody("/ComMAND")) {
		t.Error("Should match with different case")
	}
}

func TestCommandArgmuents(t *testing.T) {
	if CommandArguments(*regexp.MustCompile(".*")).Match(&github.IssueComment{}) {
		t.Error("Shouldn't match nil body")
	}
	if CommandArguments(*regexp.MustCompile(".*")).Match(makeCommentWithBody("COMMAND WRONG FORMAT")) {
		t.Error("Shouldn't match non-command")
	}
	if !CommandArguments(*regexp.MustCompile("^carret")).Match(makeCommentWithBody("/command carret is the beginning of argument")) {
		t.Error("Should match from the beginning of arguments")
	}
	if CommandArguments(*regexp.MustCompile("command")).Match(makeCommentWithBody("/command name is not part of match")) {
		t.Error("Shouldn't match command name")
	}
}

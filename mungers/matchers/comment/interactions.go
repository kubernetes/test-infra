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

package comment

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
)

// Notification identifies comments with the following format:
// [NEEDS-REBASE] Optional arguments
type Notification string

// Match returns true if the comment is a notification
func (b Notification) Match(comment *github.IssueComment) bool {
	if comment.Body == nil {
		return false
	}
	match, _ := regexp.MatchString(
		fmt.Sprintf(`^\[%s\]`, strings.ToLower(string(b))),
		strings.ToLower(*comment.Body),
	)
	return match
}

// Command identifies messages sent to the bot, with the following format:
// /COMMAND Optional arguments
type Command string

// Match will return true if the comment is indeed a command
func (c Command) Match(comment *github.IssueComment) bool {
	if comment.Body == nil {
		return false
	}
	match, _ := regexp.MatchString(
		fmt.Sprintf("^/%s", strings.ToLower(string(c))),
		strings.ToLower(*comment.Body),
	)
	return match
}

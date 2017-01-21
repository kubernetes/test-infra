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
	"strings"

	"github.com/google/go-github/github"
)

// NotificationName identifies notifications by name
type NotificationName string

// Match returns true if the comment is a notification with the given name
func (b NotificationName) Match(comment *github.IssueComment) bool {
	notif := ParseNotification(comment)
	if notif == nil {
		return false
	}

	return strings.ToUpper(notif.Name) == strings.ToUpper(string(b))
}

// CommandName identifies commands by name
type CommandName string

// Match will return true if the comment is a command with the given name
func (c CommandName) Match(comment *github.IssueComment) bool {
	command := ParseCommand(comment)
	if command == nil {
		return false
	}
	return strings.ToUpper(command.Name) == strings.ToUpper(string(c))
}

// CommandArguments identifies commands by arguments (with regex)
type CommandArguments regexp.Regexp

// Match if the command arguments match the regexp
func (c CommandArguments) Match(comment *github.IssueComment) bool {
	command := ParseCommand(comment)
	if command == nil {
		return false
	}
	return (*regexp.Regexp)(&c).MatchString(command.Arguments)
}

// MungeBotAuthor creates a matcher to find mungebot comments
func MungeBotAuthor() Matcher {
	return AuthorLogin("k8s-merge-robot")
}

// JenkinsBotAuthor creates a matcher to find jenkins bot comments
func JenkinsBotAuthor() Matcher {
	return AuthorLogin("k8s-bot")
}

// BotAuthor creates a matcher to find any bot comments
func BotAuthor() Matcher {
	return Or([]Matcher{
		MungeBotAuthor(),
		JenkinsBotAuthor(),
	})
}

// HumanActor creates a matcher to find non-bot comments.
// ValidAuthor is used because a comment that doesn't have "Author" is NOT made by a human
func HumanActor() Matcher {
	return And([]Matcher{
		ValidAuthor{},
		Not{BotAuthor()},
	})
}

// MungerNotificationName finds notification posted by the munger, based on name
func MungerNotificationName(notif string) Matcher {
	return And([]Matcher{
		MungeBotAuthor(),
		NotificationName(notif),
	})
}

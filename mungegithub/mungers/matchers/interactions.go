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
	"strings"

	"github.com/google/go-github/github"
)

// NotificationName identifies notifications by name
type NotificationName string

var _ Matcher = NotificationName("")

func (NotificationName) MatchEvent(event *github.IssueEvent) bool {
	return false
}

// Match returns true if the comment is a notification with the given name
func (b NotificationName) MatchComment(comment *github.IssueComment) bool {
	notif := ParseNotification(comment)
	if notif == nil {
		return false
	}

	return strings.ToUpper(notif.Name) == strings.ToUpper(string(b))
}

// Match returns true if the comment is a notification with the given name
func (b NotificationName) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

// CommandName identifies commands by name
type CommandName string

func (CommandName) MatchEvent(event *github.IssueEvent) bool {
	return false
}

// Match if the comment contains a command with the given name
func (c CommandName) MatchComment(comment *github.IssueComment) bool {
	commands := ParseCommands(comment)
	for _, command := range commands {
		if strings.ToUpper(command.Name) == strings.ToUpper(string(c)) {
			return true
		}
	}
	return false
}

func (CommandName) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

// CommandArguments identifies commands by arguments (with regex)
type CommandArguments regexp.Regexp

func (*CommandArguments) MatchEvent(evnet *github.IssueEvent) bool {
	return false
}

// Match if the comment contains a command whose arguments match the regexp
func (c *CommandArguments) MatchComment(comment *github.IssueComment) bool {
	commands := ParseCommands(comment)
	for _, command := range commands {
		if (*regexp.Regexp)(c).MatchString(command.Arguments) {
			return true
		}
	}
	return false
}

func (*CommandArguments) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

// JenkinsBotAuthor creates a matcher to find jenkins bot comments
func JenkinsBotAuthor() Matcher {
	return AuthorLogin("k8s-bot")
}

// BotAuthor creates a matcher to find any bot comments
func BotAuthor(mungeBotName string) Matcher {
	return Or(
		AuthorLogin(mungeBotName),
		JenkinsBotAuthor(),
	)
}

// HumanActor creates a matcher to find non-bot comments.
// ValidAuthor is used because a comment that doesn't have "Author" is NOT made by a human
func HumanActor(mungeBotName string) Matcher {
	return And(
		ValidAuthor(),
		Not(BotAuthor(mungeBotName)),
	)
}

// MungerNotificationName finds notification posted by the munger, based on name
func MungerNotificationName(notif, mungeBotName string) Matcher {
	return And(
		AuthorLogin(mungeBotName),
		NotificationName(notif),
	)
}

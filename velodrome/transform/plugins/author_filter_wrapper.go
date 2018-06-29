/*
Copyright 2017 The Kubernetes Authors.

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

package plugins

import (
	"github.com/spf13/cobra"
	"k8s.io/test-infra/velodrome/sql"
)

// AuthorFilterPluginWrapper ignore comments and events from some authors
type AuthorFilterPluginWrapper struct {
	ignoredAuthors []string

	plugin Plugin
}

var _ Plugin = &AuthorFilterPluginWrapper{}

// NewAuthorFilterPluginWrapper is the constructor for AuthorFilterPluginWrapper
func NewAuthorFilterPluginWrapper(plugin Plugin) *AuthorFilterPluginWrapper {
	return &AuthorFilterPluginWrapper{
		plugin: plugin,
	}
}

// AddFlags adds "ignore-authors" <authors> to the command help
func (a *AuthorFilterPluginWrapper) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&a.ignoredAuthors, "ignore-authors", []string{}, "Name of people to ignore")
}

func (a *AuthorFilterPluginWrapper) match(author string) bool {
	for _, ignored := range a.ignoredAuthors {
		if author == ignored {
			return true
		}
	}
	return false
}

// ReceiveIssue calls plugin.ReceiveIssue() if the author is not filtered
func (a *AuthorFilterPluginWrapper) ReceiveIssue(issue sql.Issue) []Point {
	if a.match(issue.User) {
		return nil
	}
	return a.plugin.ReceiveIssue(issue)
}

// ReceiveIssueEvent calls plugin.ReceiveIssueEvent() if the author is not filtered
func (a *AuthorFilterPluginWrapper) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	if event.Actor != nil && a.match(*event.Actor) {
		return nil
	}
	return a.plugin.ReceiveIssueEvent(event)
}

// ReceiveComment calls plugin.ReceiveComment() if the author is not filtered
func (a *AuthorFilterPluginWrapper) ReceiveComment(comment sql.Comment) []Point {
	if a.match(comment.User) {
		return nil
	}
	return a.plugin.ReceiveComment(comment)
}

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

type AuthorFilterPluginWrapper struct {
	ignoredAuthors []string

	plugin Plugin
}

var _ Plugin = &AuthorFilterPluginWrapper{}

func NewAuthorFilterPluginWrapper(plugin Plugin) *AuthorFilterPluginWrapper {
	return &AuthorFilterPluginWrapper{
		plugin: plugin,
	}
}

func (a *AuthorFilterPluginWrapper) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&a.ignoredAuthors, "ignore-authors", []string{}, "Name of people to ignore")
}

func (a *AuthorFilterPluginWrapper) Match(author string) bool {
	for _, ignored := range a.ignoredAuthors {
		if author == ignored {
			return true
		}
	}
	return false
}

func (a *AuthorFilterPluginWrapper) ReceiveIssue(issue sql.Issue) []Point {
	if a.Match(issue.User) {
		return nil
	}
	return a.plugin.ReceiveIssue(issue)
}

func (a *AuthorFilterPluginWrapper) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	if event.Actor != nil && a.Match(*event.Actor) {
		return nil
	}
	return a.plugin.ReceiveIssueEvent(event)
}

func (a *AuthorFilterPluginWrapper) ReceiveComment(comment sql.Comment) []Point {
	if a.Match(comment.User) {
		return nil
	}
	return a.plugin.ReceiveComment(comment)
}

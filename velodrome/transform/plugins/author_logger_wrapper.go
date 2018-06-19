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

// AuthorLoggerPluginWrapper logs the author on all the Points returned. This is enabled by command-line.
type AuthorLoggerPluginWrapper struct {
	plugin  Plugin
	enabled bool
}

var _ Plugin = &AuthorLoggerPluginWrapper{}

// NewAuthorLoggerPluginWrapper is the constructor for AuthorLoggerPluginWrapper
func NewAuthorLoggerPluginWrapper(plugin Plugin) *AuthorLoggerPluginWrapper {
	return &AuthorLoggerPluginWrapper{
		plugin: plugin,
	}
}

// AddFlags adds "log-authors" <authors> to the command help
func (a *AuthorLoggerPluginWrapper) AddFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&a.enabled, "log-author", false, "Log the author for each metric")
}

// ReceiveIssue is a wrapper on plugin.ReceiveIssue() logging the author
func (a *AuthorLoggerPluginWrapper) ReceiveIssue(issue sql.Issue) []Point {
	points := a.plugin.ReceiveIssue(issue)
	if a.enabled {
		for i := range points {
			if points[i].Values == nil {
				points[i].Values = map[string]interface{}{}
			}
			points[i].Values["author"] = issue.User
		}
	}

	return points
}

// ReceiveIssueEvent is a wrapper on plugin.ReceiveIssueEvent() logging the author
func (a *AuthorLoggerPluginWrapper) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	points := a.plugin.ReceiveIssueEvent(event)

	if a.enabled {
		for i := range points {
			if points[i].Values == nil {
				points[i].Values = map[string]interface{}{}
			}
			if event.Actor != nil {
				points[i].Values["author"] = *event.Actor
			}
		}
	}

	return points
}

// ReceiveComment is a wrapper on plugin.ReceiveComment() logging the author
func (a *AuthorLoggerPluginWrapper) ReceiveComment(comment sql.Comment) []Point {
	points := a.plugin.ReceiveComment(comment)

	if a.enabled {
		for i := range points {
			if points[i].Values == nil {
				points[i].Values = map[string]interface{}{}
			}
			points[i].Values["author"] = comment.User
		}
	}

	return points
}

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

package plugins

import (
	"fmt"

	"k8s.io/test-infra/velodrome/sql"

	"github.com/spf13/cobra"
)

// TypeFilterWrapperPlugin allows ignoring either PR or issues from processing
type TypeFilterWrapperPlugin struct {
	pullRequests bool
	issues       bool

	plugin Plugin

	// List of issues that we should ignore
	pass map[string]bool
}

var _ Plugin = &TypeFilterWrapperPlugin{}

// NewTypeFilterWrapperPlugin is the constructor of TypeFilterWrapperPlugin
func NewTypeFilterWrapperPlugin(plugin Plugin) *TypeFilterWrapperPlugin {
	return &TypeFilterWrapperPlugin{
		plugin: plugin,
		pass:   map[string]bool{},
	}
}

// AddFlags adds "no-pull-requests" and "no-issues" to the command help
func (t *TypeFilterWrapperPlugin) AddFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&t.pullRequests, "no-pull-requests", false, "Ignore pull-requests")
	cmd.Flags().BoolVar(&t.issues, "no-issues", false, "Ignore issues")
}

// CheckFlags makes sure not both PR and issues are ignored
func (t *TypeFilterWrapperPlugin) CheckFlags() error {
	if t.pullRequests && t.issues {
		return fmt.Errorf(
			"you can't ignore both pull-requests and issues")
	}
	return nil
}

// ReceiveIssue calls plugin.ReceiveIssue() if issues are not ignored
func (t *TypeFilterWrapperPlugin) ReceiveIssue(issue sql.Issue) []Point {
	if issue.IsPR && t.pullRequests {
		return nil
	} else if !issue.IsPR && t.issues {
		return nil
	} else {
		t.pass[issue.ID] = true
		return t.plugin.ReceiveIssue(issue)
	}
}

// ReceiveIssueEvent calls plugin.ReceiveIssueEvent() if issues are not ignored
func (t *TypeFilterWrapperPlugin) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	if !t.pass[event.IssueID] {
		return nil
	}
	return t.plugin.ReceiveIssueEvent(event)
}

// ReceiveComment calls plugin.ReceiveComment() if issues are not ignored
func (t *TypeFilterWrapperPlugin) ReceiveComment(comment sql.Comment) []Point {
	if !t.pass[comment.IssueID] {
		return nil
	}
	return t.plugin.ReceiveComment(comment)
}

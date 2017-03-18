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

type TypeFilterWrapperPlugin struct {
	pullRequests bool
	issues       bool

	plugin Plugin

	// List of issues that we should ignore
	pass map[string]bool
}

var _ Plugin = &TypeFilterWrapperPlugin{}

func NewTypeFilterWrapperPlugin(plugin Plugin) *TypeFilterWrapperPlugin {
	return &TypeFilterWrapperPlugin{
		plugin: plugin,
		pass:   map[string]bool{},
	}
}

func (t *TypeFilterWrapperPlugin) AddFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&t.pullRequests, "no-pull-requests", false, "Ignore pull-requests")
	cmd.Flags().BoolVar(&t.issues, "no-issues", false, "Ignore issues")
}

func (t *TypeFilterWrapperPlugin) CheckFlags() error {
	if t.pullRequests && t.issues {
		return fmt.Errorf(
			"You can't ignore both pull-requests and issues.")
	}
	return nil
}

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

func (t *TypeFilterWrapperPlugin) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	if !t.pass[event.IssueId] {
		return nil
	}
	return t.plugin.ReceiveIssueEvent(event)
}

func (t *TypeFilterWrapperPlugin) ReceiveComment(comment sql.Comment) []Point {
	if !t.pass[comment.IssueID] {
		return nil
	}
	return t.plugin.ReceiveComment(comment)
}

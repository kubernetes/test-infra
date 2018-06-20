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
	"k8s.io/test-infra/velodrome/sql"
)

// FakeCommentPluginWrapper is a plugin wrapper creating fake "commented" events
type FakeCommentPluginWrapper struct {
	plugin Plugin
}

var _ Plugin = &FakeCommentPluginWrapper{}

// NewFakeCommentPluginWrapper is the constructor for FakeCommentPluginWrapper
func NewFakeCommentPluginWrapper(plugin Plugin) *FakeCommentPluginWrapper {
	return &FakeCommentPluginWrapper{
		plugin: plugin,
	}
}

// ReceiveIssue is a wrapper on plugin.ReceiveIssue()
func (o *FakeCommentPluginWrapper) ReceiveIssue(issue sql.Issue) []Point {
	// Pass through
	return o.plugin.ReceiveIssue(issue)
}

// ReceiveIssueEvent is a wrapper on plugin.ReceiveIssueEvent()
func (o *FakeCommentPluginWrapper) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	// Pass through
	return o.plugin.ReceiveIssueEvent(event)
}

// ReceiveComment creates a fake "commented" event
func (o *FakeCommentPluginWrapper) ReceiveComment(comment sql.Comment) []Point {
	// Create a fake "commented" event for every comment we receive.
	fakeEvent := sql.IssueEvent{
		IssueID:        comment.IssueID,
		Event:          "commented",
		EventCreatedAt: comment.CommentCreatedAt,
		Actor:          &comment.User,
	}

	return append(
		o.plugin.ReceiveComment(comment),
		o.plugin.ReceiveIssueEvent(fakeEvent)...,
	)
}

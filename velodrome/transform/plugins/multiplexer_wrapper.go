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

// MultiplexerPluginWrapper allows registering multiple plugins for events
type MultiplexerPluginWrapper struct {
	plugins []Plugin
}

var _ Plugin = &MultiplexerPluginWrapper{}

// NewMultiplexerPluginWrapper is the constructor for MultiplexerPluginWrapper
func NewMultiplexerPluginWrapper(plugins ...Plugin) *MultiplexerPluginWrapper {
	return &MultiplexerPluginWrapper{
		plugins: plugins,
	}
}

// ReceiveIssue calls plugin.ReceiveIssue() for all plugins
func (m *MultiplexerPluginWrapper) ReceiveIssue(issue sql.Issue) []Point {
	points := []Point{}

	for _, plugin := range m.plugins {
		points = append(points, plugin.ReceiveIssue(issue)...)
	}

	return points
}

// ReceiveIssueEvent calls plugin.ReceiveIssueEvent() for all plugins
func (m *MultiplexerPluginWrapper) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	points := []Point{}

	for _, plugin := range m.plugins {
		points = append(points, plugin.ReceiveIssueEvent(event)...)
	}

	return points
}

// ReceiveComment calls plugin.ReceiveComment() for all plugins
func (m *MultiplexerPluginWrapper) ReceiveComment(comment sql.Comment) []Point {
	points := []Point{}

	for _, plugin := range m.plugins {
		points = append(points, plugin.ReceiveComment(comment)...)
	}

	return points
}

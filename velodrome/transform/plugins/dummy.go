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

import "k8s.io/test-infra/velodrome/sql"

// DummyPlugin is an empty plugin
type DummyPlugin struct{}

// ReceiveIssue is needed to implement a Plugin
func (DummyPlugin) ReceiveIssue(issue sql.Issue) []Point {
	return nil
}

// ReceiveIssueEvent is needed to implement a Plugin
func (DummyPlugin) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	return nil
}

// ReceiveComment is needed to implement a Plugin
func (DummyPlugin) ReceiveComment(comment sql.Comment) []Point {
	return nil
}

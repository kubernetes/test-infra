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

type EventCounterPlugin struct {
	matcher EventMatcher
	desc    string
}

var _ Plugin = &EventCounterPlugin{}

func (e *EventCounterPlugin) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&e.desc, "event", "", "Match event (eg: `opened`)")
}

func (e *EventCounterPlugin) CheckFlags() error {
	e.matcher = NewEventMatcher(e.desc)
	return nil
}

func (e *EventCounterPlugin) ReceiveIssue(issue sql.Issue) []Point {
	return nil
}

func (e *EventCounterPlugin) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	label := ""
	if event.Label != nil {
		label = *event.Label
	}

	if !e.matcher.Match(event.Event, label) {
		return nil
	}
	return []Point{
		Point{
			Values: map[string]interface{}{"event": 1},
			Date:   event.EventCreatedAt,
		},
	}
}

func (e *EventCounterPlugin) ReceiveComment(comment sql.Comment) []Point {
	return nil
}

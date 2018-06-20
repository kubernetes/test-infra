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
	"container/heap"

	"k8s.io/test-infra/velodrome/sql"
)

// EventTimeHeap is a min-heap on Event creation time
type EventTimeHeap []sql.IssueEvent

var _ heap.Interface = &EventTimeHeap{}

func (t EventTimeHeap) Len() int           { return len(t) }
func (t EventTimeHeap) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t EventTimeHeap) Less(i, j int) bool { return t[i].EventCreatedAt.Before(t[j].EventCreatedAt) }

// Push adds event to the heap
func (t *EventTimeHeap) Push(x interface{}) {
	*t = append(*t, x.(sql.IssueEvent))
}

// Pop retrieves the last added event
func (t *EventTimeHeap) Pop() interface{} {
	old := *t
	n := len(old)
	x := old[n-1]
	*t = old[0 : n-1]
	return x
}

// FakeOpenPluginWrapper sends new "opened" event to ReceiveEvent
type FakeOpenPluginWrapper struct {
	// Min-heap of "opened" events to inject
	openEvents  EventTimeHeap
	alreadyOpen map[string]bool
	// Actual plugin
	plugin Plugin
}

var _ Plugin = &FakeOpenPluginWrapper{}

// NewFakeOpenPluginWrapper is the constructor for FakeOpenPluginWrapper
func NewFakeOpenPluginWrapper(plugin Plugin) *FakeOpenPluginWrapper {
	return &FakeOpenPluginWrapper{
		plugin:      plugin,
		alreadyOpen: map[string]bool{},
	}
}

// ReceiveIssue creates a fake "opened" event
func (o *FakeOpenPluginWrapper) ReceiveIssue(issue sql.Issue) []Point {
	if _, ok := o.alreadyOpen[issue.ID]; !ok {
		// Create/Add fake "opened" events
		heap.Push(&o.openEvents, sql.IssueEvent{
			Event:          "opened",
			IssueID:        issue.ID,
			Actor:          &issue.User,
			EventCreatedAt: issue.IssueCreatedAt,
		})
		o.alreadyOpen[issue.ID] = true
	}

	return o.plugin.ReceiveIssue(issue)
}

// ReceiveIssueEvent injects an extra "opened" event before calling plugin.ReceiveIssueEvent()
func (o *FakeOpenPluginWrapper) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	points := []Point{}

	// Inject extra "opened" events. Inject "opened" before other events at the same time.
	for o.openEvents.Len() > 0 && !o.openEvents[0].EventCreatedAt.After(event.EventCreatedAt) {
		points = append(points, o.plugin.ReceiveIssueEvent(heap.Pop(&o.openEvents).(sql.IssueEvent))...)
	}

	return append(points, o.plugin.ReceiveIssueEvent(event)...)
}

// ReceiveComment is a wrapper on plugin.ReceiveComment()
func (o *FakeOpenPluginWrapper) ReceiveComment(comment sql.Comment) []Point {
	return o.plugin.ReceiveComment(comment)
}

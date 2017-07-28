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
	"reflect"
	"testing"
	"time"

	"k8s.io/test-infra/velodrome/sql"
)

func eventPoint(t time.Time) Point {
	return Point{
		Values: map[string]interface{}{
			"event": 1,
		},
		Date: t,
	}
}

func TestEventCounter(t *testing.T) {
	tests := []struct {
		pattern  string
		events   []sql.IssueEvent
		expected []Point
	}{
		{
			pattern: "",
			events: []sql.IssueEvent{
				{
					Event:          "merged",
					EventCreatedAt: time.Unix(10, 0),
				},
				{
					Event:          "opened",
					EventCreatedAt: time.Unix(20, 0),
				},
				{
					Event:          "closed",
					EventCreatedAt: time.Unix(30, 0),
				},
			},
			expected: []Point{},
		},
		{
			pattern: "merged",
			events: []sql.IssueEvent{
				{
					Event:          "merged",
					EventCreatedAt: time.Unix(10, 0),
				},
				{
					Event:          "opened",
					EventCreatedAt: time.Unix(20, 0),
				},
				{
					Event:          "closed",
					EventCreatedAt: time.Unix(30, 0),
				},
			},
			expected: []Point{
				eventPoint(time.Unix(10, 0)),
			},
		},
	}

	for _, test := range tests {
		plugin := EventCounterPlugin{desc: test.pattern}
		if err := plugin.CheckFlags(); err != nil {
			t.Fatalf("Failed to initial event counter (%s): %s", test.pattern, err)
		}
		got := []Point{}
		for _, event := range test.events {
			got = append(got, plugin.ReceiveIssueEvent(event)...)
		}
		want := test.expected
		if !reflect.DeepEqual(got, want) {
			t.Errorf(`EventCounterPlugin{pattern: "%s".ReceiveIssueEvent = %+v, want %+v`,
				test.pattern, got, want)
		}
	}
}

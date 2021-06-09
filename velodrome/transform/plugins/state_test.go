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

func TestStatePlugin(t *testing.T) {
	tests := []struct {
		description string
		percentiles []int
		events      []sql.IssueEvent
		expected    []Point
	}{
		{
			description: "merged",
			percentiles: []int{50, 100},
			events: []sql.IssueEvent{
				// No change
				{
					IssueID:        "1",
					Event:          "opened",
					EventCreatedAt: time.Unix(10*60, 0),
				},
				// 1 is merged
				{
					IssueID:        "1",
					Event:          "merged",
					EventCreatedAt: time.Unix(20*60, 0),
				},
				// 1 is merged again, no change
				{
					IssueID:        "1",
					Event:          "merged",
					EventCreatedAt: time.Unix(30*60, 0),
				},
				// 2 is merged
				{
					IssueID:        "2",
					Event:          "merged",
					EventCreatedAt: time.Unix(40*60, 0),
				},
			},
			expected: []Point{
				{
					Date: time.Unix(20*60, 0),
					Values: map[string]interface{}{
						"count": 1,
						"sum":   0,
						"50%":   0,
						"100%":  0,
					},
				},
				{
					Date: time.Unix(40*60, 0),
					Values: map[string]interface{}{
						"count": 2,
						"sum":   20,
						"50%":   0,
						"100%":  int(20 * time.Minute),
					},
				}},
		},
	}

	for _, test := range tests {
		plugin := StatePlugin{
			desc:        test.description,
			percentiles: test.percentiles,
		}
		if err := plugin.CheckFlags(); err != nil {
			t.Fatalf("Failed to CheckFlags(): %s", err)
		}
		got := []Point{}
		for _, event := range test.events {
			got = append(got, plugin.ReceiveIssueEvent(event)...)
		}
		want := test.expected
		if !reflect.DeepEqual(got, want) {
			t.Errorf(`StatePlugin{desc: "%s".ReceiveIssueEvent = %+v, want %+v`,
				test.description, got, want)
		}
	}
}

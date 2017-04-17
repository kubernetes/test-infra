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
)

func TestNewEvent(t *testing.T) {
	tests := []struct {
		description string
		matcher     EventMatcher
	}{
		{"", FalseEvent{}},
		{"labeled:something", LabelEvent{Label: "something"}},
		{"unlabeled:something", UnlabelEvent{Label: "something"}},
		{"merged", MergeEvent{}},
		{"closed", CloseEvent{}},
		{"reopened", ReopenEvent{}},
		{"opened", OpenEvent{}},
	}

	for _, test := range tests {
		got := NewEventMatcher(test.description)
		want := test.matcher
		if !reflect.DeepEqual(got, want) {
			t.Errorf("NewEvent(%s) = %#v, want %#v",
				test.description, got, want)
		}
	}
}

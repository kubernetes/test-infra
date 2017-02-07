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

package event

import (
	"time"

	"github.com/google/go-github/github"
)

// FilteredEvents is a list of events
type FilteredEvents []*github.IssueEvent

// GetLast returns the last event in a series of events
func (f FilteredEvents) GetLast() *github.IssueEvent {
	if f.Empty() {
		return nil
	}
	return f[len(f)-1]
}

// Empty Checks to see if the list of events is empty
func (f FilteredEvents) Empty() bool {
	return len(f) == 0
}

// FilterEvents will return the list of matching events
func FilterEvents(events []*github.IssueEvent, matcher Matcher) FilteredEvents {
	matches := FilteredEvents{}

	for _, event := range events {
		if matcher.Match(event) {
			matches = append(matches, event)
		}
	}

	return matches
}

// LastEvent returns the creation date of the last event that matches. Or deflt if there is no such event.
func LastEvent(events []*github.IssueEvent, matcher Matcher, deflt *time.Time) *time.Time {
	matches := FilterEvents(events, matcher)
	if matches.Empty() {
		return deflt
	}
	return matches.GetLast().CreatedAt
}

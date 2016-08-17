/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"strings"
	"time"

	"github.com/google/go-github/github"
)

// Matcher is an interface to match an event
type Matcher interface {
	Match(event *github.IssueEvent) bool
}

// Actor searches for a specific actor
type Actor string

// Match if the event is from the specified actor
func (a Actor) Match(event *github.IssueEvent) bool {
	if event == nil || event.Actor == nil || event.Actor.Login == nil {
		return false
	}
	return strings.ToLower(*event.Actor.Login) == strings.ToLower(string(a))
}

// AddLabel searches for "labeled" event.
type AddLabel struct{}

// Match if the event is of type "labeled"
func (a AddLabel) Match(event *github.IssueEvent) bool {
	if event == nil || event.Event == nil {
		return false
	}
	return *event.Event == "labeled"
}

// LabelPrefix searches for event whose label starts with the string
type LabelPrefix string

// Match if the label starts with the string
func (l LabelPrefix) Match(event *github.IssueEvent) bool {
	if event == nil || event.Label == nil || event.Label.Name == nil {
		return false
	}
	return strings.HasPrefix(*event.Label.Name, string(l))
}

// CreatedAfter looks for event created after time
type CreatedAfter time.Time

// Match if the event is after the time
func (c CreatedAfter) Match(event *github.IssueEvent) bool {
	if event == nil || event.CreatedAt == nil {
		return false
	}
	return event.CreatedAt.After(time.Time(c))
}

// CreatedBefore looks for event created before time
type CreatedBefore time.Time

// Match if the event is before the time
func (c CreatedBefore) Match(event *github.IssueEvent) bool {
	if event == nil || event.CreatedAt == nil {
		return false
	}
	return event.CreatedAt.Before(time.Time(c))
}

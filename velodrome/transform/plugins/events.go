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
	"fmt"
	"strings"
)

// EventMatcher generates events based on name and labels
type EventMatcher interface {
	// Does eventName and label match the event
	Match(eventName, label string) bool
	// Return the opposite of this eventmatcher
	Opposite() EventMatcher
}

// FalseEvent is a false event
type FalseEvent struct{}

var _ EventMatcher = FalseEvent{}

// Match is false
func (FalseEvent) Match(eventName, label string) bool {
	return false
}

// Opposite is true
func (FalseEvent) Opposite() EventMatcher {
	return TrueEvent{}
}

// TrueEvent is a true event
type TrueEvent struct{}

var _ EventMatcher = TrueEvent{}

// Match is true
func (TrueEvent) Match(eventName, label string) bool {
	return true
}

// Opposite is false
func (TrueEvent) Opposite() EventMatcher {
	return FalseEvent{}
}

// OpenEvent is an "opened" event
type OpenEvent struct{}

var _ EventMatcher = OpenEvent{}

// Match is "opened"
func (OpenEvent) Match(eventName, label string) bool {
	return eventName == "opened"
}

// Opposite is closed
func (OpenEvent) Opposite() EventMatcher {
	return CloseEvent{}
}

// CommentEvent is a "commented" event
type CommentEvent struct{}

var _ EventMatcher = CommentEvent{}

// Match is "commented"
func (CommentEvent) Match(eventName, label string) bool {
	return eventName == "commented"
}

// Opposite is false
func (CommentEvent) Opposite() EventMatcher {
	return FalseEvent{}
}

// LabelEvent is a "labeled" event
type LabelEvent struct {
	Label string
}

var _ EventMatcher = LabelEvent{}

// Match is "labeled" with label
func (l LabelEvent) Match(eventName, label string) bool {
	return eventName == "labeled" && label == l.Label
}

// Opposite is unlabel
func (l LabelEvent) Opposite() EventMatcher {
	return UnlabelEvent(l)
}

// UnlabelEvent is an "unlabeled" event
type UnlabelEvent struct {
	Label string
}

var _ EventMatcher = UnlabelEvent{}

// Match is "unlabeled"
func (u UnlabelEvent) Match(eventName, label string) bool {
	return eventName == "unlabeled" && label == u.Label
}

// Opposite is label
func (u UnlabelEvent) Opposite() EventMatcher {
	return LabelEvent(u)
}

// CloseEvent is a "closed" event
type CloseEvent struct{}

var _ EventMatcher = CloseEvent{}

// Match is "closed"
func (CloseEvent) Match(eventName, label string) bool {
	return eventName == "closed"
}

// Opposite is reopen
func (CloseEvent) Opposite() EventMatcher {
	return ReopenEvent{}
}

// ReopenEvent is a "reopened" event
type ReopenEvent struct{}

var _ EventMatcher = ReopenEvent{}

// Match is "reopened"
func (ReopenEvent) Match(eventName, label string) bool {
	return eventName == "reopened"
}

// Opposite is close
func (ReopenEvent) Opposite() EventMatcher {
	return CloseEvent{}
}

// MergeEvent is a "merged" event
type MergeEvent struct{}

var _ EventMatcher = MergeEvent{}

// Match is "merged"
func (MergeEvent) Match(eventName, label string) bool {
	return eventName == "merged"
}

// Opposite is false
func (MergeEvent) Opposite() EventMatcher {
	// A merge can't be undone.
	return FalseEvent{}
}

// NewEventMatcher returns the correct EventMatcher based on description
// Incoming event should have the following form:
// eventName:labelName. If eventName is not label, then the second part
// can be omitted.
func NewEventMatcher(eventDescription string) EventMatcher {
	split := strings.SplitN(eventDescription, ":", 2)
	switch split[0] {
	case "":
		return FalseEvent{}
	case "commented":
		return CommentEvent{}
	case "opened":
		return OpenEvent{}
	case "reopened":
		return ReopenEvent{}
	case "merged":
		return MergeEvent{}
	case "closed":
		return CloseEvent{}
	case "labeled":
		if len(split) != 2 {
			panic(fmt.Errorf("Missing label part of the event"))
		}
		return LabelEvent{split[1]}
	case "unlabeled":
		if len(split) != 2 {
			panic(fmt.Errorf("Missing label part of the event"))
		}
		return UnlabelEvent{split[1]}
	default:
		panic(fmt.Errorf("Unknown type of event: %s", split[0]))
	}
}

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

type EventMatcher interface {
	// Does eventName and label match the event
	Match(eventName, label string) bool
	// Return the opposite of this eventmatcher
	Opposite() EventMatcher
}

type FalseEvent struct{}

var _ EventMatcher = FalseEvent{}

func (FalseEvent) Match(eventName, label string) bool {
	return false
}

func (FalseEvent) Opposite() EventMatcher {
	return TrueEvent{}
}

type TrueEvent struct{}

var _ EventMatcher = TrueEvent{}

func (TrueEvent) Match(eventName, label string) bool {
	return true
}

func (TrueEvent) Opposite() EventMatcher {
	return FalseEvent{}
}

type OpenEvent struct{}

var _ EventMatcher = OpenEvent{}

func (OpenEvent) Match(eventName, label string) bool {
	return eventName == "opened"
}

func (OpenEvent) Opposite() EventMatcher {
	return CloseEvent{}
}

type CommentEvent struct{}

var _ EventMatcher = CommentEvent{}

func (CommentEvent) Match(eventName, label string) bool {
	return eventName == "commented"
}

func (CommentEvent) Opposite() EventMatcher {
	return FalseEvent{}
}

type LabelEvent struct {
	Label string
}

var _ EventMatcher = LabelEvent{}

func (l LabelEvent) Match(eventName, label string) bool {
	return eventName == "labeled" && label == l.Label
}

func (l LabelEvent) Opposite() EventMatcher {
	return UnlabelEvent{Label: l.Label}
}

type UnlabelEvent struct {
	Label string
}

var _ EventMatcher = UnlabelEvent{}

func (u UnlabelEvent) Match(eventName, label string) bool {
	return eventName == "unlabeled" && label == u.Label
}

func (u UnlabelEvent) Opposite() EventMatcher {
	return LabelEvent{Label: u.Label}
}

type CloseEvent struct{}

var _ EventMatcher = CloseEvent{}

func (CloseEvent) Match(eventName, label string) bool {
	return eventName == "closed"
}

func (CloseEvent) Opposite() EventMatcher {
	return ReopenEvent{}
}

type ReopenEvent struct{}

var _ EventMatcher = ReopenEvent{}

func (ReopenEvent) Match(eventName, label string) bool {
	return eventName == "reopened"
}

func (ReopenEvent) Opposite() EventMatcher {
	return CloseEvent{}
}

type MergeEvent struct{}

var _ EventMatcher = MergeEvent{}

func (MergeEvent) Match(eventName, label string) bool {
	return eventName == "merged"
}

func (MergeEvent) Opposite() EventMatcher {
	// A merge can't be undone.
	return FalseEvent{}
}

// Incoming event should have the following form:
// eventName:labelName. If eventName is not label, then the second part
// can be ommitted.
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

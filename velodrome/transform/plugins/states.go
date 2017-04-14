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
	"strings"
	"time"
)

// State describes a pull-request states, based on the events we've
// seen.
type State interface {
	// Has the state been activated
	Active() bool
	// How long has the state been activated (will panic if not active)
	Age(t time.Time) time.Duration
	// Receive the event, return the new state
	ReceiveEvent(eventName, label string, t time.Time) (State, bool)
}

// ActiveState describe a states that has been enabled.
type ActiveState struct {
	startTime time.Time
	exit      EventMatcher
}

var _ State = &ActiveState{}

// Active if always true for an ActiveState
func (ActiveState) Active() bool {
	return true
}

// Age gives the time since the state has been activated.
func (a *ActiveState) Age(t time.Time) time.Duration {
	return t.Sub(a.startTime)
}

// ReceiveEvent checks if the event matches the exit criteria.
// Returns a new InactiveState or self, and true if it changed.
func (a *ActiveState) ReceiveEvent(eventName, label string, t time.Time) (State, bool) {
	if a.exit.Match(eventName, label) {
		return &InactiveState{
			entry: a.exit.Opposite(),
		}, true
	}
	return a, false
}

// InactiveState describes a state that has not enabled, or been disabled.
type InactiveState struct {
	entry EventMatcher
}

var _ State = &InactiveState{}

// Active is always false for an InactiveState
func (InactiveState) Active() bool {
	return false
}

// Age doesn't make sense for InactiveState.
func (i *InactiveState) Age(t time.Time) time.Duration {
	panic("InactiveState doesn't have an age.")
}

// ReceiveEvent checks if the event matches the entry criteria
// Returns a new ActiveState or self, and true if it changed.
func (i *InactiveState) ReceiveEvent(eventName, label string, t time.Time) (State, bool) {
	if i.entry.Match(eventName, label) {
		return &ActiveState{
			startTime: t,
			exit:      i.entry.Opposite(),
		}, true
	}
	return i, false
}

// MultiState tracks multiple individual states at the same time.
type MultiState struct {
	states []State
}

var _ State = &MultiState{}

// Active is true if all the states are active.
func (m *MultiState) Active() bool {
	for _, state := range m.states {
		if !state.Active() {
			return false
		}
	}
	return true
}

// Age returns the time since all states have been activated.
// It will panic if any of the state is not active.
func (m *MultiState) Age(t time.Time) time.Duration {
	minAge := time.Duration(1<<63 - 1)
	for _, state := range m.states {
		stateAge := state.Age(t)
		if stateAge < minAge {
			minAge = stateAge
		}
	}
	return minAge
}

// ReceiveEvent will send the event to each individual state, and update
// them if they change.
func (m *MultiState) ReceiveEvent(eventName, label string, t time.Time) (State, bool) {
	oneChanged := false
	for i := range m.states {
		state, changed := m.states[i].ReceiveEvent(eventName, label, t)
		if changed {
			oneChanged = true
		}
		m.states[i] = state

	}
	return m, oneChanged
}

// NewState creates a MultiState instance based on the statesDescription
// string. statesDescription is a comma separated list of
// events. Events can be prepended with "!" (bang) to say that the state
// will be activated only if this event doesn't happen (or is inverted).
func NewState(statesDescription string) State {
	states := []State{}

	if statesDescription == "" {
		// Create an infinite inactive state
		return &InactiveState{
			entry: FalseEvent{},
		}
	}

	splitDescription := strings.Split(statesDescription, ",")
	for _, description := range splitDescription {
		description = strings.TrimSpace(description)
		if strings.HasPrefix(description, "!") {
			states = append(states, &ActiveState{
				startTime: time.Time{},
				exit:      NewEventMatcher(description[1:]),
			})
		} else {
			states = append(states, &InactiveState{
				entry: NewEventMatcher(description),
			})
		}
	}

	return &MultiState{states: states}
}

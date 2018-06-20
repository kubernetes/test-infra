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
)

func TestActiveStateActive(t *testing.T) {
	state := ActiveState{
		startTime: time.Unix(0, 0),
		exit:      LabelEvent{Label: "test"},
	}

	got := state.Active()
	want := true

	if got != want {
		t.Errorf("%#v.Active() = %t, want %t", state, got, want)
	}
}

func TestActiveStateAge(t *testing.T) {
	state := ActiveState{
		startTime: time.Unix(0, 0),
		exit:      LabelEvent{Label: "test"},
	}

	got := state.Age(time.Unix(0, 10))
	want := time.Duration(10)
	if got != want {
		t.Errorf("%#v.Age(time.Unix(0, 10)) = %s, want %s", state, got, want)
	}
}

func TestActiveStateReceiveMatchingEvent(t *testing.T) {
	state := ActiveState{
		startTime: time.Unix(0, 0),
		exit:      LabelEvent{Label: "test"},
	}

	gotState, gotChanged := state.ReceiveEvent("labeled", "test", time.Unix(0, 10))
	wantState := &InactiveState{UnlabelEvent{Label: "test"}}
	wantChanged := true
	if !reflect.DeepEqual(gotState, wantState) || gotChanged != wantChanged {
		t.Errorf(`%#v.ReceiveEvent("labeled", "test", _) = (%#v, %t), want (%#v, %t)`,
			state,
			gotState, gotChanged,
			wantState, wantChanged)
	}
}

func TestActiveStateReceiveNonMatchingEvent(t *testing.T) {
	state := ActiveState{
		startTime: time.Unix(0, 0),
		exit:      LabelEvent{Label: "test"},
	}

	gotState, gotChanged := state.ReceiveEvent("labeled", "non-matching", time.Unix(0, 10))
	wantState := &state
	wantChanged := false
	if !reflect.DeepEqual(gotState, wantState) || gotChanged != wantChanged {
		t.Errorf(`%#v.ReceiveEvent("labeled", "non-matching", _) = (%#v, %t), want (%#v, %t)`,
			state,
			gotState, gotChanged,
			wantState, wantChanged)
	}
}

func TestInactiveStateActive(t *testing.T) {
	state := InactiveState{
		entry: LabelEvent{Label: "test"},
	}

	got := state.Active()
	want := false

	if got != want {
		t.Errorf("%#v.Active() = %t, want %t", state, got, want)
	}
}

func TestInactiveStateReceiveMatchingEvent(t *testing.T) {
	state := InactiveState{
		entry: LabelEvent{Label: "test"},
	}

	gotState, gotChanged := state.ReceiveEvent("labeled", "test", time.Unix(0, 10))
	wantState := &ActiveState{startTime: time.Unix(0, 10), exit: UnlabelEvent{Label: "test"}}
	wantChanged := true
	if !reflect.DeepEqual(gotState, wantState) || gotChanged != wantChanged {
		t.Errorf(`%#v.ReceiveEvent("labeled", "test", _) = (%#v, %t), want (%#v, %t)`,
			state,
			gotState, gotChanged,
			wantState, wantChanged)
	}
}

func TestInactiveStateReceiveNonMatchingEvent(t *testing.T) {
	state := InactiveState{
		entry: LabelEvent{Label: "test"},
	}

	gotState, gotChanged := state.ReceiveEvent("labeled", "non-matching", time.Unix(0, 10))
	wantState := &state
	wantChanged := false
	if !reflect.DeepEqual(gotState, wantState) || gotChanged != wantChanged {
		t.Errorf(`%#v.ReceiveEvent("labeled", "non-matching", _) = (%#v, %t), want (%#v, %t)`,
			state,
			gotState, gotChanged,
			wantState, wantChanged)
	}
}

func TestMultiStateActive(t *testing.T) {
	// All states are active
	state := &MultiState{
		[]State{
			&ActiveState{exit: MergeEvent{}, startTime: time.Unix(0, 10)},
			&ActiveState{exit: CloseEvent{}, startTime: time.Unix(0, 20)},
		},
	}

	got := state.Active()
	want := true
	if got != want {
		t.Errorf("%#v.Active() = %t, want %t", state, got, want)
	}
}

func TestMultiStateActiveAge(t *testing.T) {
	// All states are active, Age returns time since latest active
	state := &MultiState{
		[]State{
			&ActiveState{exit: MergeEvent{}, startTime: time.Unix(0, 10)},
			&ActiveState{exit: CloseEvent{}, startTime: time.Unix(0, 20)},
		},
	}

	got := state.Age(time.Unix(0, 30))
	want := time.Duration(10)
	if got != want {
		t.Errorf("%#v.Age(time.Unix(0, 30)) = %s, want %s", state, got, want)
	}
}

func TestMultiStateInactive(t *testing.T) {
	// One state is inactive
	state := &MultiState{
		[]State{
			&ActiveState{exit: MergeEvent{}, startTime: time.Unix(0, 10)},
			&InactiveState{entry: CloseEvent{}},
		},
	}

	got := state.Active()
	want := false
	if got != want {
		t.Errorf("%#v.Active() = %t, want %t", state, got, want)
	}
}

func TestMultiStateReceiveEvent(t *testing.T) {
	var want, got, state State
	var wantChanged, gotChanged bool
	// We are looking for "merged,!closed", i.e. "merged" but not "closed"
	state = &MultiState{
		[]State{
			&InactiveState{entry: MergeEvent{}},
			&ActiveState{exit: CloseEvent{}, startTime: time.Time{}},
		},
	}
	got, gotChanged = state.ReceiveEvent("closed", "", time.Unix(0, 10))
	want, wantChanged = &MultiState{
		[]State{
			&InactiveState{entry: MergeEvent{}},
			&InactiveState{entry: ReopenEvent{}},
		},
	}, true
	if !reflect.DeepEqual(got, want) || gotChanged != wantChanged {
		t.Errorf(`%#v.ReceiveEvent("closed", "", _) = (%#v, %t), want (%#v, %t)`,
			state, got, gotChanged, want, wantChanged)
	}

	state = got
	got, gotChanged = state.ReceiveEvent("merged", "", time.Unix(0, 20))
	want, wantChanged = &MultiState{
		[]State{
			&ActiveState{exit: FalseEvent{}, startTime: time.Unix(0, 20)},
			&InactiveState{entry: ReopenEvent{}},
		},
	}, true
	if !reflect.DeepEqual(got, want) || gotChanged != wantChanged {
		t.Errorf(`%#v.ReceiveEvent("merged", "", time.Unix(0, 20)) = (%#v, %t), want (%#v, %t)`,
			state, got, gotChanged, want, wantChanged)
	}

	state = got
	got, gotChanged = state.ReceiveEvent("reopened", "", time.Unix(0, 30))
	want, wantChanged = &MultiState{
		[]State{
			&ActiveState{exit: FalseEvent{}, startTime: time.Unix(0, 20)},
			&ActiveState{exit: CloseEvent{}, startTime: time.Unix(0, 30)},
		},
	}, true
	if !reflect.DeepEqual(got, want) || gotChanged != wantChanged {
		t.Errorf(`%#v.ReceiveEvent("merged", "", time.Unix(0, 20)) = (%#v, %t), want (%#v, %t)`,
			state, got, gotChanged, want, wantChanged)
	}
}

func TestNewState(t *testing.T) {
	tests := []struct {
		description string
		state       State
	}{
		// Empty description generates impossible state.
		{
			description: "",
			state:       &InactiveState{entry: FalseEvent{}},
		},
		// Single event state.
		{
			description: "merged",
			state: &MultiState{
				[]State{
					&InactiveState{entry: MergeEvent{}},
				},
			},
		},
		// Comma separated multi-event state with active sub-state.
		{
			description: "merged,!closed",
			state: &MultiState{
				[]State{
					&InactiveState{entry: MergeEvent{}},
					&ActiveState{exit: CloseEvent{}, startTime: time.Time{}},
				},
			},
		},
	}

	for _, test := range tests {
		got := NewState(test.description)
		want := test.state
		if !reflect.DeepEqual(got, want) {
			t.Errorf("NewState(%v) = %#v, want %#v", test.description, got, want)
		}
	}
}

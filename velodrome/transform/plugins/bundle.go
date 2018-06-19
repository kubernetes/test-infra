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
	"math"
	"sort"
	"time"
)

// ByDuration sorts a slice of time.Duration
type ByDuration []time.Duration

func (b ByDuration) Len() int           { return len(b) }
func (b ByDuration) Less(i, j int) bool { return b[i] < b[j] }
func (b ByDuration) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

// BundledStates saves the state of multiple issues/pull-requests. This
// will also allow us to then compute statistics on these states like:
// - Number of pull-requests in activated states (first return value of
// Total())
// - Sum of time since activated pull-requests are activated (second
// return value of Total())
// - Get a specific percentile time for activated pull-requests
// (Percentile())
type BundledStates struct {
	description string
	states      map[string]State
}

// NewBundledStates is the constructor for BundledStates
func NewBundledStates(description string) BundledStates {
	return BundledStates{
		description: description,
		states:      map[string]State{},
	}
}

// ReceiveEvent is called when something happens on an issue. The state
// for that issue is updated.
func (b BundledStates) ReceiveEvent(ID string, eventName, label string, t time.Time) bool {
	state, ok := b.states[ID]
	if !ok {
		state = NewState(b.description)
	}
	state, changed := state.ReceiveEvent(eventName, label, t)
	b.states[ID] = state
	return changed
}

// ages return the age of each active states
func (b BundledStates) ages(t time.Time) map[string]time.Duration {
	ages := map[string]time.Duration{}

	for id, state := range b.states {
		if !state.Active() {
			continue
		}
		ages[id] = state.Age(t)
	}
	return ages
}

// Total counts number of active state, and total age (in minutes, to compute average)
func (b BundledStates) Total(t time.Time) (count int, sum int64) {
	for _, age := range b.ages(t) {
		count++
		sum += int64(age / time.Minute)
	}
	return
}

// Percentile returns given percentile for age of all active states at time t
func (b BundledStates) Percentile(t time.Time, percentile int) time.Duration {
	if percentile > 100 || percentile <= 0 {
		panic(fmt.Errorf("percentile %d is out of scope", percentile))
	}

	ages := []time.Duration{}
	for _, age := range b.ages(t) {
		ages = append(ages, age)
	}

	if len(ages) == 0 {
		return 0
	}

	sort.Sort(ByDuration(ages))

	index := int(math.Ceil(float64(percentile)*float64(len(ages))/100) - 1)
	if index >= len(ages) {
		panic(fmt.Errorf("Index is out of range: %d/%d", index, len(ages)))
	}
	return ages[index]
}

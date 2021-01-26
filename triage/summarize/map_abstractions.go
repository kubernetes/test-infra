/*
Copyright 2020 The Kubernetes Authors.

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

/*
This file contains abstractions of certain complex map types that are used often throughout
summarize.go. They also have some often-used methods such as getting just the keys, or getting the
key-value pairs as a map.
*/

package summarize

import (
	"sort"
)

// map[string][]failure type alias

// failuresGroup maps strings to failure slices.
type failuresGroup map[string][]failure

// failuresGroupPair is a representation of a failuresGroup key-value mapping as a two-element
// struct.
type failuresGroupPair struct {
	Key      string    `json:"key"`
	Failures []failure `json:"failures"`
}

// keys provides the failuresGroup's keys as a string slice.
func (fg *failuresGroup) keys() []string {
	result := make([]string, 0, len(*fg))

	for key := range *fg {
		result = append(result, key)
	}

	return result
}

// asSlice returns the failuresGroup as a failuresGroupPair slice.
func (fg *failuresGroup) asSlice() []failuresGroupPair {
	result := make([]failuresGroupPair, 0, len(*fg))

	for str, failures := range *fg {
		result = append(result, failuresGroupPair{str, failures})
	}

	return result
}

// sortByMostFailures returns a failuresGroupPair slice sorted by the number of failures in each
// pair, descending. If the number of failures is the same for two pairs, they are sorted alphabetically
// by their keys.
func (fg *failuresGroup) sortByMostFailures() []failuresGroupPair {
	result := fg.asSlice()

	// Sort the slice.
	sort.Slice(result, func(i, j int) bool {
		iFailures := len(result[i].Failures)
		jFailures := len(result[j].Failures)

		if iFailures == jFailures {
			return result[i].Key < result[j].Key
		}

		return iFailures > jFailures
	})

	return result
}

// equal determines whether this failuresGroup is deeply equal to another failuresGroup.
func (a *failuresGroup) equal(b *failuresGroup) bool {
	// First check the length to deal with different-length maps
	if len(*a) != len(*b) {
		return false
	}

	for key, failuresA := range *a {
		// Make sure the other map contains the same keys
		if failuresB, ok := (*b)[key]; ok {
			// Check lengths
			if len(failuresA) != len(failuresB) {
				return false
			}
			// Compare the failures slices
			for i := range failuresA {
				if failuresA[i] != failuresB[i] {
					return false
				}
			}
		} else {
			// The other map is missing a key
			return false
		}
	}

	return true
}

// map[string]failuresGroup type alias, which is really a map[string]map[string][]failure type alias

// nestedFailuresGroups maps strings to failuresGroup instances.
type nestedFailuresGroups map[string]failuresGroup

// nestedFailuresGroupsPair is a representation of a nestedFailuresGroups key-value mapping as a
// two-element struct.
type nestedFailuresGroupsPair struct {
	Key   string        `json:"key"`
	Group failuresGroup `json:"group"`
}

// keys provides the nestedFailuresGroups's keys as a string slice.
func (nfg *nestedFailuresGroups) keys() []string {
	result := make([]string, len(*nfg))

	iter := 0
	for key := range *nfg {
		result[iter] = key
		iter++
	}

	return result
}

// asSlice returns the nestedFailuresGroups as a nestedFailuresGroupsPair slice.
func (nfg *nestedFailuresGroups) asSlice() []nestedFailuresGroupsPair {
	result := make([]nestedFailuresGroupsPair, len(*nfg))

	iter := 0
	for str, group := range *nfg {
		result[iter] = nestedFailuresGroupsPair{str, group}
		iter++
	}

	return result
}

// sortByMostAggregatedFailures returns a nestedFailuresGroupsPair slice sorted by the aggregate
// number of failures across all failure slices in each failuresGroup, descending. If the aggregate
// number of failures is the same for two pairs, they are sorted alphabetically by their keys.
func (nfg *nestedFailuresGroups) sortByMostAggregatedFailures() []nestedFailuresGroupsPair {
	result := nfg.asSlice()

	// Pre-compute the aggregate failures for each element of result so that the less
	// function doesn't have to compute it on every compare.
	// aggregates maps nestedFailuresGroups strings to number of aggregate failures across all of
	// their failure slices.
	aggregates := make(map[string]int, len(*nfg))
	for str, fg := range *nfg {
		aggregate := 0
		for _, group := range fg {
			aggregate += len(group)
		}
		aggregates[str] = aggregate
	}

	// Sort the slice.
	sort.Slice(result, func(i, j int) bool {
		if aggregates[result[i].Key] == aggregates[result[j].Key] {
			return result[i].Key < result[j].Key
		}

		return aggregates[result[i].Key] > aggregates[result[j].Key]
	})

	return result
}

// equal determines whether this nestedFailuresGroups object is deeply equal to another nestedFailuresGroups object.
func (a *nestedFailuresGroups) equal(b *nestedFailuresGroups) bool {
	// First check the length to deal with different-length maps
	if len(*a) != len(*b) {
		return false
	}

	for key, failuresGroupA := range *a {
		// Make sure the other map contains the same keys
		if failuresGroupB, ok := (*b)[key]; ok {
			if !failuresGroupA.equal(&failuresGroupB) {
				return false
			}
		} else {
			// The other map is missing a key
			return false
		}
	}

	return true
}

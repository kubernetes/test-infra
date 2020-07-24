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

import "sort"

// map[string][]failure type alias

// failuresGroup maps strings to failure slices.
type failuresGroup map[string][]failure

// failuresGroupPair is a representation of a failuresGroup key-value mapping as a two-element
// struct.
type failuresGroupPair struct {
	key      string
	failures []failure
}

// keys provides the failuresGroup's keys as a string slice.
func (fg *failuresGroup) keys() []string {
	result := make([]string, len(*fg))

	iter := 0
	for key := range *fg {
		result[iter] = key
		iter++
	}

	return result
}

// asSlice returns the failuresGroup as a failuresGroupPair slice.
func (fg *failuresGroup) asSlice() []failuresGroupPair {
	result := make([]failuresGroupPair, len(*fg))

	iter := 0
	for str, failures := range *fg {
		result[iter] = failuresGroupPair{str, failures}
		iter++
	}

	return result
}

// sortByNumberOfFailures returns a failuresGroupPair slice sorted by the number of failures in each
// pair, descending. If the number of failures is the same for two pairs, they are sorted alphabetically
// by their keys.
func (fg *failuresGroup) sortByNumberOfFailures() []failuresGroupPair {
	result := fg.asSlice()

	// Sort the slice.
	sort.Slice(result, func(i, j int) bool {
		iFailures := len(result[i].failures)
		jFailures := len(result[j].failures)

		if iFailures == jFailures {
			return result[i].key < result[j].key
		}

		// Use > instead of < so the largest values (i.e. clusters with the most failures) are first.
		return iFailures > jFailures
	})

	return result
}

// map[string]failuresGroup type alias, which is really a map[string]map[string][]failure type alias

// nestedFailuresGroups maps strings to failuresGroup instances.
type nestedFailuresGroups map[string]failuresGroup

// nestedFailuresGroupsPair is a representation of a nestedFailuresGroups key-value mapping as a
// two-element struct.
type nestedFailuresGroupsPair struct {
	key   string
	group failuresGroup
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

// sortByAggregateNumberOfFailures returns a nestedFailuresGroupsPair slice sorted by the aggregate
// number of failures across all failure slices in each failuresGroup, descending. If the aggregate
// number of failures is the same for two pairs, they are sorted alphabetically by their keys.
func (nfg *nestedFailuresGroups) sortByAggregateNumberOfFailures() []nestedFailuresGroupsPair {
	result := nfg.asSlice()

	// TODO(michaelkolber) Pre-compute the aggregate failures for each element of result so that the less
	// function doesn't have to compute it on every compare. This may require implementing sort.Interface.

	// Sort the slice.
	sort.Slice(result, func(i, j int) bool {
		iAggregateFailures := 0
		jAggregateFailures := 0

		iGroup := result[i].group
		for _, failures := range iGroup {
			iAggregateFailures += len(failures)
		}

		jGroup := result[j].group
		for _, failures := range jGroup {
			jAggregateFailures += len(failures)
		}

		if iAggregateFailures == jAggregateFailures {
			return result[i].key < result[j].key
		}

		// Use > instead of < so the largest values (i.e. largest number of failures across all
		// clusters) are first.
		return iAggregateFailures > jAggregateFailures
	})

	return result
}

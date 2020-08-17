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
Package utils is a small collection of utility functions helpful to Triage.
*/
package utils

import (
	"strings"
)

// Min takes any number of integers and returns the smallest. Panics if nothing is supplied.
func Min(nums ...int) int {
	smallest := nums[0]

	for _, num := range nums[1:] {
		if num < smallest {
			smallest = num
		}
	}

	return smallest
}

// Max takes any number of integers and returns the largest. Panics if nothing is supplied.
func Max(nums ...int) int {
	largest := nums[0]

	for _, num := range nums[1:] {
		if num > largest {
			largest = num
		}
	}

	return largest
}

// Abs returns the absolute value of an integer.
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// BtoI converts true to 1 and false to 0.
func BtoI(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ByteSliceInsert inserts an element into a slice at a given index.
func ByteSliceInsert(slc *[]byte, element byte, index int) {
	// Add a dummy element that will get overwritten to ensure capacity
	// for the new element
	*slc = append(*slc, 'a')

	// Effectively shift the elements to the right starting from the
	// desired index
	copy((*slc)[index+1:], (*slc)[index:])

	// Add in the new element
	(*slc)[index] = element
}

// RemoveDuplicateLines takes a "\n"-delimited string and removes duplicates.
// Strings consisting only of "\n" characters will return the empty string.
func RemoveDuplicateLines(text string) string {
	// First convert to a slice for easy traversal
	lines := strings.Split(text, "\n")

	// Holds the de-duplicated lines
	// Add the first line, which will always be included
	result := []string{lines[0]}

	// Traverse through the rest of the lines to find duplicates
	previousLine := lines[0]
	for _, currentLine := range lines[1:] {
		if currentLine == previousLine {
			continue
		}

		// Otherwise, store the result
		result = append(result, currentLine)
		// Update previousLine for the next iteration
		previousLine = currentLine
	}

	// Return the reassembled line
	return strings.Join(result, "\n")
}

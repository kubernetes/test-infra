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

package summarize

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	testCases := []struct {
		name     string
		argument string
		want     string
	}{
		{"Hex strings, letters, version number", "0x1234 a 123.13.45.43 b 2e24e003-9ffd-4e78-852c-9dcb6cbef493-123", "UNIQ1 a UNIQ2 b UNIQ3"},
		{"Date and time", "Mon, 12 January 2017 11:34:35 blah blah", "TIMEblah blah"},
		{"Version number, hex string", "123.45.68.12:345 abcd1234eeee", "UNIQ1 UNIQ2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalize(tc.argument)

			if got != tc.want {
				t.Errorf("normalize(%s) = %s, wanted %s", tc.argument, got, tc.want)
			}
		})
	}

	// Deal with long strings separately because it requires some setup
	t.Run("Incredibly large string", func(t *testing.T) {
		// Generate an incredibly long string
		var builder strings.Builder
		builder.Grow(10 * 500_000) // Allocate enough memory (10 characters in "foobarbaz ")

		for i := 0; i < 500_000; i++ {
			builder.WriteString("foobarbaz ")
		}

		generatedString := builder.String()
		// 10*500 = (number of characters in "foobarbaz ")*(500 repetitions)
		wantString := generatedString[:10*500] + "\n...[truncated]...\n" + generatedString[:10*500]

		got := normalize(generatedString)

		if got != wantString {
			t.Errorf("normalize(%s) = %s, wanted %s", generatedString, wantString, got)
		}
	})
}

func TestNgramEditDist(t *testing.T) {
	argument1 := "example text"
	argument2 := "exampl text"
	want := 1
	got := ngramEditDist(argument1, argument2)

	if got != want {
		t.Errorf("ngramEditDist(%#v, %#v) = %d, wanted %d", argument1, argument2, got, want)
	}
}

// Ensure stability of ngram count digest
func TestMakeNgramCountsDigest(t *testing.T) {
	want := "eddb950347d1eb05b5d7"
	got := makeNgramCountsDigest("some string")

	if got != want {
		t.Errorf("makeNgramCountsDigest(%#v) = %#v, wanted %#v", "some string", got, want)
	}
}

func TestCommonSpans(t *testing.T) {
	testCases := []struct {
		name     string
		argument []string
		want     []int
	}{
		{"Exact match", []string{"an exact match", "an exact match"}, []int{14}},
		{"Replaced word", []string{"some example string", "some other string"}, []int{5, 7, 7}},
		{"Deletion", []string{"a problem with a common set", "a common set"}, []int{2, 7, 1, 4, 13}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := commonSpans(tc.argument)

			// Check if the int slices are equal
			slicesAreEqual := true
			if len(tc.want) != len(got) {
				slicesAreEqual = false
			} else {
				for i := range tc.want {
					if tc.want[i] != got[i] {
						slicesAreEqual = false
						break
					}
				}
			}

			if !slicesAreEqual {
				t.Errorf("commonSpans(%#v) = %#v, wanted %#v", tc.argument, got, tc.want)
			}
		})
	}
}

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

package utils

import (
	"fmt"
	"testing"
)

func TestMin(t *testing.T) {
	testCases := []struct {
		name      string
		arguments []int
		want      int
	}{
		{"One value", []int{1}, 1},
		{"Two values", []int{1, 2}, 1},
		{"Three values", []int{1, 2, 3}, 1},
		{"Three values reordered", []int{2, 1, 3}, 1},
		{"Negative values", []int{-1, -2}, -2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Min(tc.arguments...)

			if got != tc.want {
				// Format the function arguments nicely
				formattedArgs := fmt.Sprintf("%d", tc.arguments[0])
				for _, arg := range tc.arguments[1:] {
					formattedArgs = fmt.Sprintf("%s, %d", formattedArgs, arg)
				}

				t.Errorf("Min(%s) = %d, wanted %d", formattedArgs, got, tc.want)
			}
		})
	}
}

func TestMax(t *testing.T) {
	testCases := []struct {
		name      string
		arguments []int
		want      int
	}{
		{"One value", []int{1}, 1},
		{"Two values", []int{1, 2}, 2},
		{"Three values", []int{1, 2, 3}, 3},
		{"Three values reordered", []int{3, 1, 2}, 3},
		{"Negative values", []int{-1, -2}, -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Max(tc.arguments...)

			if got != tc.want {
				// Format the function arguments nicely
				formattedArgs := fmt.Sprintf("%d", tc.arguments[0])
				for _, arg := range tc.arguments[1:] {
					formattedArgs = fmt.Sprintf("%s, %d", formattedArgs, arg)
				}

				t.Errorf("Max(%s) = %d, wanted %d", formattedArgs, got, tc.want)
			}
		})
	}
}

func TestAbs(t *testing.T) {
	testCases := []struct {
		name     string
		argument int
		want     int
	}{
		{"Negative", -1, 1},
		{"Positive", 1, 1},
		{"Zero", 0, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Abs(tc.argument)

			if got != tc.want {
				t.Errorf("Abs(%d) = %d; wanted %d", tc.argument, got, tc.want)
			}
		})
	}
}

func TestBtoI(t *testing.T) {
	testCases := []struct {
		name     string
		argument bool
		want     int
	}{
		{"True", true, 1},
		{"False", false, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := BtoI(tc.argument)

			if got != tc.want {
				t.Errorf("BtoI(%t) = %d, wanted %d", tc.argument, got, tc.want)
			}
		})
	}
}

func TestByteSliceInsert(t *testing.T) {
	t.Run("Normal slice", func(t *testing.T) {
		s := []byte{'1', '3', '4'}
		ByteSliceInsert(&s, '2', 1)

		for i, element := range s {
			// We want a slice of ['1' '2' '3' '4']
			if element != byte(48+i+1) { // U+0030 (48 decimal) is '0', add 1 to make it equal the current element
				t.Errorf("s[%d] = %q; wanted %d", i, element, i+1)
			}
		}
	})

	t.Run("Empty slice", func(t *testing.T) {
		s := make([]byte, 0)
		ByteSliceInsert(&s, '1', 0)

		if s[0] != '1' {
			t.Errorf("s[0] = %q; wanted %q", s[0], '1')
		}
	})
}

func TestRemoveDuplicateLines(t *testing.T) {
	testCases := []struct {
		name     string
		argument string
		want     string
	}{
		{"No duplicates", "this\nis\nmultiline\nstring", "this\nis\nmultiline\nstring"},
		{"Duplicates", "this\nis\nis\nstring", "this\nis\nstring"},
		{"\\n at beginning", "\nthis\nis\nmultiline\nstring", "\nthis\nis\nmultiline\nstring"},
		{"\\n at end", "this\nis\nmultiline\nstring\n", "this\nis\nmultiline\nstring\n"},
		{"No \\n", "this is multiline string", "this is multiline string"},
		{"Only one \\n", "\n", ""},
		{"Only two \\n", "\n\n", ""},
		{"Two \\n with space", "\n \n", "\n \n"},
		{"Empty string", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := RemoveDuplicateLines(tc.argument)

			if got != tc.want {
				t.Errorf("RemoveDuplicateLines(%#v) = %#v, wanted %#v", tc.argument, got, tc.want)
			}
		})
	}
}

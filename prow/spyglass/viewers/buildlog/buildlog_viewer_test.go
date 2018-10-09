/*
Copyright 2018 The Kubernetes Authors.

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

package buildlog

import (
	"testing"
)

func TestGroupLines(t *testing.T) {
	lorem := []string{
		"Lorem ipsum dolor sit amet",
		"consectetur adipiscing elit",
		"sed do eiusmod tempor incididunt ut labore et dolore magna aliqua",
		"Ut enim ad minim veniam",
		"quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat",
		"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur",
		"Excepteur sint occaecat cupidatat non proident",
		"sunt in culpa qui officia deserunt mollit anim id est laborum",
	}
	tests := []struct {
		name   string
		lines  []string
		groups []LineGroup
	}{
		{
			name:   "Test empty log",
			lines:  []string{},
			groups: []LineGroup{},
		},
		{
			name:  "Test error highlighting",
			lines: []string{"This is an ErRoR message"},
			groups: []LineGroup{
				{
					Start: 0,
					End:   1,
					Skip:  false,
				},
			},
		},
		{
			name:  "Test skip all",
			lines: lorem,
			groups: []LineGroup{
				{
					Start: 0,
					End:   8,
					Skip:  true,
				},
			},
		},
		{
			name: "Test skip none",
			lines: []string{
				"a", "b", "c", "d", "e",
				"Failed to immanentize the eschaton.",
				"a", "b", "c", "d", "e",
			},
			groups: []LineGroup{
				{
					Start: 0,
					End:   11,
					Skip:  false,
				},
			},
		},
		{
			name: "Test skip threshold",
			lines: []string{
				"a", "b", "c", "d", // skip threshold unmet
				"a", "b", "c", "d", "e", "Failed to immanentize the eschaton.", "a", "b", "c", "d", "e",
				"a", "b", "c", "d", "e", // skip threshold met
			},
			groups: []LineGroup{
				{
					Start: 0,
					End:   4,
					Skip:  false,
				},
				{
					Start: 4,
					End:   15,
					Skip:  false,
				},
				{
					Start: 15,
					End:   20,
					Skip:  true,
				},
			},
		},
		{
			name: "Test nearby errors",
			lines: []string{
				"a", "b", "c",
				"don't panic",
				"a", "b", "c",
				"don't panic",
				"a", "b", "c",
			},
			groups: []LineGroup{
				{
					Start: 0,
					End:   11,
					Skip:  false,
				},
			},
		},
		{
			name: "Test separated errors",
			lines: []string{
				"a", "b", "c",
				"don't panic",
				"a", "b", "c", "d", "e",
				"a", "b", "c",
				"a", "b", "c", "d", "e",
				"don't panic",
				"a", "b", "c",
			},
			groups: []LineGroup{
				{
					Start: 0,
					End:   9,
					Skip:  false,
				},
				{
					Start: 9,
					End:   12,
					Skip:  false,
				},
				{
					Start: 12,
					End:   21,
					Skip:  false,
				},
			},
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := groupLines(test.lines, i)
			if len(got) != len(test.groups) {
				t.Fatalf("Expected %d groups, got %d", len(test.groups), len(got))
			}
			for j, exp := range test.groups {
				if got[j].Start != exp.Start || got[j].End != exp.End {
					t.Fatalf("Group %d expected lines [%d, %d), got [%d, %d)", j, exp.Start, exp.End, got[j].Start, got[j].End)
				}
				if got[j].Skip != exp.Skip {
					t.Errorf("Lines [%d, %d) expected Skip = %t", exp.Start, exp.End, exp.Skip)
				}
			}
		})
	}
}

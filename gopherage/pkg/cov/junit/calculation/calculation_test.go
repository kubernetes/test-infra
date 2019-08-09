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

package calculation

import (
	"testing"

	"golang.org/x/tools/cover"
)

func TestCovList(t *testing.T) {
	profiles := []*cover.Profile{
		{FileName: "a", Mode: "count", Blocks: []cover.ProfileBlock{
			{NumStmt: 5, Count: 1},
			{NumStmt: 100, Count: 2},
			{NumStmt: 2018},
		},
		},
		{FileName: "b", Mode: "count", Blocks: []cover.ProfileBlock{
			{NumStmt: 59, Count: 1},
			{NumStmt: 1500, Count: 2},
			{NumStmt: 2000},
		},
		},
		{FileName: "a/c", Mode: "count", Blocks: []cover.ProfileBlock{}},
	}
	covList := ProduceCovList(profiles)

	if len(covList.Group) == 0 {
		t.Fatalf("covlist is empty\n")
	}

	testCases := []struct {
		covExpected Coverage
		covActual   Coverage
	}{
		{Coverage{Name: "a", NumCoveredStmts: 105, NumAllStmts: 2123},
			covList.Group[0]},
		{Coverage{Name: "b", NumCoveredStmts: 1559, NumAllStmts: 3559},
			covList.Group[1]},
		{Coverage{Name: "a/c"},
			covList.Group[2]},
	}

	for _, tc := range testCases {
		if tc.covExpected != tc.covActual {
			t.Fatalf("File level summarized coverage data does does match expectation: "+
				"expected = %v; actual = %v", tc.covExpected, tc.covActual)
		}
	}

	expected := float32(1664) / float32(5682)
	if expected != covList.Ratio() {
		t.Fatalf("Overall summarized coverage data does does match expectation: "+
			"expected = %v; actual = %v", expected, covList.Ratio())
	}
}

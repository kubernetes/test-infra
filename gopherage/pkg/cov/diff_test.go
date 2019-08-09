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

package cov_test

import (
	"golang.org/x/tools/cover"
	"k8s.io/test-infra/gopherage/pkg/cov"
	"reflect"
	"testing"
)

func TestDiffProfilesBasicDiff(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}
	b := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}

	result, err := cov.DiffProfiles(a, b)
	if err != nil {
		t.Fatalf("DiffProfiles failed: %v", err)
	}

	expected := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 4},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 0},
			},
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatal("diffed profile incorrect")
	}
}

func TestDiffProfilesWrongFileName(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}
	b := []*cover.Profile{
		{
			FileName: "b.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}
	if _, err := cov.DiffProfiles(a, b); err == nil {
		t.Fatal("expected DiffProfiles to fail when diffing mismatched files")
	}
}

func TestDiffProfilesWrongFileCount(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
		{
			FileName: "b.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}
	b := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}
	if _, err := cov.DiffProfiles(a, b); err == nil {
		t.Fatal("expected DiffProfiles to fail when diffing mismatched profiles")
	}
}

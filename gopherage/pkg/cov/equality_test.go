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

package cov

import (
	"testing"

	"golang.org/x/tools/cover"
)

func TestBlocksEqualValid(t *testing.T) {
	a := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     6,
	}
	b := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     7,
	}
	if !blocksEqual(a, b) {
		t.Error("equivalent blocks treated as mismatching")
	}
}

func TestBlocksEqualBadStartLine(t *testing.T) {
	a := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     6,
	}
	b := cover.ProfileBlock{
		StartLine: 8,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     7,
	}
	if blocksEqual(a, b) {
		t.Error("mismatching StartLine considered equivalent")
	}
}

func TestBlocksEqualBadStartCol(t *testing.T) {
	a := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     6,
	}
	b := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  8,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     7,
	}
	if blocksEqual(a, b) {
		t.Error("mismatching StartCol considered equivalent")
	}
}

func TestBlocksEqualBadEndLine(t *testing.T) {
	a := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     6,
	}
	b := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   8,
		EndCol:    4,
		NumStmt:   5,
		Count:     7,
	}
	if blocksEqual(a, b) {
		t.Error("mismatching EndLine considered equivalent")
	}
}

func TestBlocksEqualBadEndCol(t *testing.T) {
	a := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     6,
	}
	b := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    8,
		NumStmt:   5,
		Count:     7,
	}
	if blocksEqual(a, b) {
		t.Error("mismatching EndCol considered equivalent")
	}
}

func TestBlocksEqualBadNumStmt(t *testing.T) {
	a := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   5,
		Count:     6,
	}
	b := cover.ProfileBlock{
		StartLine: 1,
		StartCol:  2,
		EndLine:   3,
		EndCol:    4,
		NumStmt:   8,
		Count:     7,
	}
	if blocksEqual(a, b) {
		t.Error("mismatching NumStmt considered equivalent")
	}
}

func TestEnsureProfilesMatch(t *testing.T) {
	a := &cover.Profile{
		FileName: "a.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}
	b := &cover.Profile{
		FileName: "a.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}

	if err := ensureProfilesMatch(a, b); err != nil {
		t.Errorf("unexpected error comparing matching profiles: %v", err)
	}
}

func TestEnsureProfilesMatchBadName(t *testing.T) {
	a := &cover.Profile{
		FileName: "a.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}
	b := &cover.Profile{
		FileName: "b.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}

	if ensureProfilesMatch(a, b) == nil {
		t.Errorf("expected profiles with mismatching FileName to not match")
	}
}

func TestEnsureProfilesMatchBadMode(t *testing.T) {
	a := &cover.Profile{
		FileName: "a.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}
	b := &cover.Profile{
		FileName: "a.go",
		Mode:     "set",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}

	if ensureProfilesMatch(a, b) == nil {
		t.Errorf("expected profiles with mismatching Mode to not match")
	}
}

func TestEnsureProfilesMatchBadBlockCount(t *testing.T) {
	a := &cover.Profile{
		FileName: "a.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}
	b := &cover.Profile{
		FileName: "b.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
		},
	}

	if ensureProfilesMatch(a, b) == nil {
		t.Errorf("expected profiles with mismatching block count to not match")
	}
}

func TestEnsureProfilesMatchBadBlock(t *testing.T) {
	a := &cover.Profile{
		FileName: "a.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}
	b := &cover.Profile{
		FileName: "b.go",
		Mode:     "count",
		Blocks: []cover.ProfileBlock{
			{StartLine: 2, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
			{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
		},
	}

	if ensureProfilesMatch(a, b) == nil {
		t.Errorf("expected profiles with mismatching block content to not match")
	}
}

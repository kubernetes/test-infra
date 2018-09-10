package cov_test

import (
	"golang.org/x/tools/cover"
	"k8s.io/test-infra/gopherage/pkg/cov"
	"reflect"
	"testing"
)

func TestMergeProfilesSimilar(t *testing.T) {
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

	expected := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 10},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 4},
			},
		},
	}

	result, err := cov.MergeProfiles(a, b)
	if err != nil {
		t.Fatalf("error merging profiles: %v", err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatal("merged profile incorrect")
	}
}

func TestMergeProfilesDisjoint(t *testing.T) {
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
			},
		},
	}

	expected := []*cover.Profile{
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
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
			},
		},
	}

	result, err := cov.MergeProfiles(a, b)
	if err != nil {
		t.Fatalf("error merging profiles: %v", err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatal("merged profile incorrect")
	}
}

func TestMergeProfilesOverlapping(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "bar.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 17},
			},
		},
		{
			FileName: "foo.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}
	b := []*cover.Profile{
		{
			FileName: "bar.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 8},
			},
		},
		{
			FileName: "baz.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}

	expected := []*cover.Profile{
		{
			FileName: "bar.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 25},
			},
		},
		{
			FileName: "baz.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 7},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
		{
			FileName: "foo.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 2},
			},
		},
	}

	result, err := cov.MergeProfiles(a, b)
	if err != nil {
		t.Fatalf("error merging profiles: %v", err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatal("merged profile incorrect")
	}
}

func TestMergeMultipleProfiles(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "foo.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 3},
			},
		},
	}
	b := []*cover.Profile{
		{
			FileName: "foo.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 5},
			},
		},
	}
	c := []*cover.Profile{
		{
			FileName: "foo.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 8},
			},
		},
	}

	expected := []*cover.Profile{
		{
			FileName: "foo.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 16},
			},
		},
	}

	result, err := cov.MergeMultipleProfiles([][]*cover.Profile{a, b, c})
	if err != nil {
		t.Fatalf("error merging profiles: %v", err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatal("merged profile incorrect", result)
	}
}

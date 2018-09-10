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

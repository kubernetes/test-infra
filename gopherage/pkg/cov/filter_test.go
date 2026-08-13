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

func TestFilterProfilePathsInclude(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			},
		},
		{
			FileName: "b.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 2, StartCol: 15, EndLine: 6, EndCol: 14, NumStmt: 5, Count: 4},
			},
		},
		{
			FileName: "c.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 3, StartCol: 16, EndLine: 7, EndCol: 15, NumStmt: 6, Count: 5},
			},
		},
	}

	r, err := cov.FilterProfilePaths(a, []string{"a", "b"}, true)
	if err != nil {
		t.Fatalf("error filtering profile: %v", err)
	}

	expected := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			},
		},
		{
			FileName: "b.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 2, StartCol: 15, EndLine: 6, EndCol: 14, NumStmt: 5, Count: 4},
			},
		},
	}

	if !reflect.DeepEqual(r, expected) {
		t.Fatalf("filtered profile incorrect.")
	}
}

func TestFilterProfilePathsExclude(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			},
		},
		{
			FileName: "b.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 2, StartCol: 15, EndLine: 6, EndCol: 14, NumStmt: 5, Count: 4},
			},
		},
		{
			FileName: "c.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 3, StartCol: 16, EndLine: 7, EndCol: 15, NumStmt: 6, Count: 5},
			},
		},
	}

	r, err := cov.FilterProfilePaths(a, []string{"a", "b"}, false)
	if err != nil {
		t.Fatalf("error filtering profile: %v", err)
	}

	expected := []*cover.Profile{
		{
			FileName: "c.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 3, StartCol: 16, EndLine: 7, EndCol: 15, NumStmt: 6, Count: 5},
			},
		},
	}

	if !reflect.DeepEqual(r, expected) {
		t.Fatalf("filtered profile incorrect.")
	}
}

func TestFilterProfilePathsBadRegex(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
			},
		},
	}
	_, err := cov.FilterProfilePaths(a, []string{"("}, false)
	if err == nil {
		t.Fatalf("expected error when filtering profile, but didn't get one.")
	}
}

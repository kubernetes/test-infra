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
	"reflect"
	"testing"

	"golang.org/x/tools/cover"
	"k8s.io/test-infra/gopherage/pkg/cov"
)

func TestAggregateProfilesSingleProfile(t *testing.T) {
	p := [][]*cover.Profile{
		{
			{
				FileName: "a.go",
				Mode:     "count",
				Blocks: []cover.ProfileBlock{
					{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
					{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 0},
				},
			},
			{
				FileName: "b.go",
				Mode:     "count",
				Blocks: []cover.ProfileBlock{
					{StartLine: 3, StartCol: 2, EndLine: 7, EndCol: 2, NumStmt: 3, Count: 1},
				},
			},
		},
	}

	aggregate, err := cov.AggregateProfiles(p)
	if err != nil {
		t.Fatalf("AggregateProfiles failed: %v", err)
	}

	expected := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 1},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 0},
			},
		},
		{
			FileName: "b.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 3, StartCol: 2, EndLine: 7, EndCol: 2, NumStmt: 3, Count: 1},
			},
		},
	}

	if !reflect.DeepEqual(aggregate, expected) {
		t.Fatal("aggregate profile incorrect")
	}
}

func TestAggregateProfilesOverlapping(t *testing.T) {
	p := [][]*cover.Profile{
		{
			{
				FileName: "a.go",
				Mode:     "count",
				Blocks: []cover.ProfileBlock{
					{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 3},
					{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 0},
					{StartLine: 14, StartCol: 3, EndLine: 19, EndCol: 4, NumStmt: 6, Count: 0},
				},
			},
		},
		{
			{
				FileName: "a.go",
				Mode:     "count",
				Blocks: []cover.ProfileBlock{
					{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 0},
					{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 4},
					{StartLine: 14, StartCol: 3, EndLine: 19, EndCol: 4, NumStmt: 6, Count: 0},
				},
			},
		},
	}

	aggregate, err := cov.AggregateProfiles(p)
	if err != nil {
		t.Fatalf("AggregateProfiles failed: %v", err)
	}

	expected := []*cover.Profile{
		{
			FileName: "a.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 14, EndLine: 5, EndCol: 13, NumStmt: 4, Count: 1},
				{StartLine: 7, StartCol: 4, EndLine: 12, EndCol: 4, NumStmt: 3, Count: 1},
				{StartLine: 14, StartCol: 3, EndLine: 19, EndCol: 4, NumStmt: 6, Count: 0},
			},
		},
	}

	if !reflect.DeepEqual(aggregate, expected) {
		t.Fatal("aggregate profile incorrect")
	}
}

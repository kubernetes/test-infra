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
	"bytes"
	"golang.org/x/tools/cover"
	"k8s.io/test-infra/gopherage/pkg/cov"
	"testing"
)

func TestDumpProfileOneFile(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "foo.go",
			Mode:     "count",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 3},
				{StartLine: 22, StartCol: 5, EndLine: 28, EndCol: 2, NumStmt: 5, Count: 2},
			},
		},
	}

	expected := `mode: count
foo.go:1.3,20.1 10 3
foo.go:22.5,28.2 5 2
`

	var buffer bytes.Buffer
	if err := cov.DumpProfile(a, &buffer); err != nil {
		t.Fatalf("DumpProfile failed: %v", err)
	}

	if buffer.String() != expected {
		t.Errorf("bad result.\n\nexpected:\n%s\nactual:\n%s\n", expected, buffer.String())
	}
}

func TestDumpProfileMultipleFiles(t *testing.T) {
	a := []*cover.Profile{
		{
			FileName: "bar.go",
			Mode:     "atomic",
			Blocks: []cover.ProfileBlock{
				{StartLine: 5, StartCol: 1, EndLine: 16, EndCol: 7, NumStmt: 7, Count: 0},
			},
		},
		{
			FileName: "foo.go",
			Mode:     "atomic",
			Blocks: []cover.ProfileBlock{
				{StartLine: 1, StartCol: 3, EndLine: 20, EndCol: 1, NumStmt: 10, Count: 3},
				{StartLine: 22, StartCol: 5, EndLine: 28, EndCol: 2, NumStmt: 5, Count: 2},
			},
		},
	}

	expected := `mode: atomic
bar.go:5.1,16.7 7 0
foo.go:1.3,20.1 10 3
foo.go:22.5,28.2 5 2
`
	var buffer bytes.Buffer
	if err := cov.DumpProfile(a, &buffer); err != nil {
		t.Fatalf("DumpProfile failed: %v", err)
	}

	if buffer.String() != expected {
		t.Errorf("bad result.\n\nexpected:\n%s\nactual:\n%s\n", expected, buffer.String())
	}
}

func TestDumpProfileNoFiles(t *testing.T) {
	var buffer bytes.Buffer
	if err := cov.DumpProfile([]*cover.Profile{}, &buffer); err == nil {
		t.Error("expected dumping no files to fail")
	}
}

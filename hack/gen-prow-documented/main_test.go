/*
Copyright 2022 The Kubernetes Authors.

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

package main

import (
	"os"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGen(t *testing.T) {
	type FakeReporter struct {
		// Context is the name of the GitHub status context for the job.
		// Defaults: the same as the name of the job.
		Context string `json:"context,omitempty"`
		// SkipReport skips commenting and setting status on GitHub.
		SkipReport bool `json:"skip_report,omitempty"`
	}

	tests := []struct {
		name            string
		rawContents     []byte
		expectedRawYaml []byte
	}{
		{
			name: "also-read-raw",
			rawContents: []byte(`package main
type FakeReporter struct {
	// Context is the name of the GitHub status context for the job.
	// Defaults: the same as the name of the job.
	Context string ` + "`" + `json:"context,omitempty"` + "`" + `
	// SkipReport skips commenting and setting status on GitHub.
	SkipReport bool ` + "`" + `json:"skip_report,omitempty"` + "`" + `
}`),
			expectedRawYaml: []byte(`# Context is the name of the GitHub status context for the job.
# Defaults: the same as the name of the job.
context: ' '
# SkipReport skips commenting and setting status on GitHub.
skip_report: true
`),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			in, out := tc.name+".in", tc.name+".out"
			if err := os.WriteFile(path.Join(tmpDir, in), tc.rawContents, 0644); err != nil {
				t.Fatalf("Failed creating input: %v", err)
			}
			g := genConfig{
				in: []string{
					in,
				},
				format: &FakeReporter{},
				out:    out,
			}

			if err := g.gen(tmpDir); err != nil {
				t.Fatalf("Got unexpected error: %v", err)
			}

			got, err := os.ReadFile(path.Join(tmpDir, out))
			if err != nil {
				t.Fatalf("Failed reading out: %v", err)
			}
			if diff := cmp.Diff(string(tc.expectedRawYaml), string(got)); diff != "" {
				t.Fatalf("Mismatch:\n%s", diff)
			}
		})
	}
}

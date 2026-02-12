/*
Copyright 2026 The Kubernetes Authors.

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

import "testing"

func TestValidateOptions(t *testing.T) {
	t.Parallel()

	opts := options{
		releaseBranchDir:   "some/dir",
		outputDir:          "out/dir",
		testgridOutputPath: "testgrid.yaml",
	}

	err := validateOptions(opts)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateOptionsMissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts options
	}{
		{
			name: "missing release-branch-dir",
			opts: options{
				releaseBranchDir:   "",
				outputDir:          "out/dir",
				testgridOutputPath: "testgrid.yaml",
			},
		},
		{
			name: "missing output-dir",
			opts: options{
				releaseBranchDir:   "some/dir",
				outputDir:          "",
				testgridOutputPath: "testgrid.yaml",
			},
		},
		{
			name: "missing testgrid-output-path",
			opts: options{
				releaseBranchDir:   "some/dir",
				outputDir:          "out/dir",
				testgridOutputPath: "",
			},
		},
		{
			name: "all empty",
			opts: options{
				releaseBranchDir:   "",
				outputDir:          "",
				testgridOutputPath: "",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateOptions(tc.opts)
			if err == nil {
				t.Error("expected error for missing field")
			}
		})
	}
}

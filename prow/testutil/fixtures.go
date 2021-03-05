/*
Copyright 2021 The Kubernetes Authors.

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

package testutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// CompareWithFixtureDir will compare all files in a directory with a corresponding test fixture directory.
func CompareWithFixtureDir(t *testing.T, golden, output string) {
	if walkErr := filepath.Walk(golden, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(golden, path)
		if err != nil {
			// this should not happen
			t.Errorf("bug: could not compute relative path in fixture dir: %v", err)
		}
		CompareWithFixture(t, path, filepath.Join(output, relPath))
		return nil
	}); walkErr != nil {
		t.Errorf("failed to walk fixture tree for comparison: %v", walkErr)
	}
}

// CompareWithFixture will compare output files with a test fixture and allows to automatically update them
// by setting the UPDATE env var. The output and golden paths are relative to the test's directory.
func CompareWithFixture(t *testing.T, golden, output string) {
	actual, err := ioutil.ReadFile(output)
	if err != nil {
		t.Fatalf("failed to read testdata file: %v", err)
	}
	if os.Getenv("UPDATE") != "" {
		if err := os.MkdirAll(filepath.Dir(golden), 0755); err != nil {
			t.Fatalf("failed to create fixture directory: %v", err)
		}
		if err := ioutil.WriteFile(golden, actual, 0644); err != nil {
			t.Fatalf("failed to write updated fixture: %v", err)
		}
	}
	expected, err := ioutil.ReadFile(golden)
	if err != nil {
		t.Fatalf("failed to read testdata file: %v", err)
	}

	if diff := cmp.Diff(string(expected), string(actual)); diff != "" {
		t.Errorf("got diff between expected and actual result: \n%s\n\nIf this is expected, re-run the test with `UPDATE=true go test ./...` to update the fixtures.", diff)
	}
}

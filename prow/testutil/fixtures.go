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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"
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

func sanitizeFilename(s string) string {
	result := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r < 'z') || (r >= 'A' && r < 'Z') || r == '_' || r == '.' || (r >= '0' && r <= '9') {
			// The thing is documented as returning a nil error so lets just drop it
			_, _ = result.WriteRune(r)
			continue
		}
		if !strings.HasSuffix(result.String(), "_") {
			result.WriteRune('_')
		}
	}
	return "zz_fixture_" + result.String()
}

// CompareWithSerializedFixture compares an object that can be marshalled with a golden file containing the
// serialized version of the data.
func CompareWithSerializedFixture(t *testing.T, data interface{}) {
	t.Helper()
	tempFile, err := ioutil.TempFile("", "tmp-serialized")
	if err != nil {
		t.Fatalf("could not create temporary file to hold serialized data: %v", err)
	}
	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Errorf("could not remove temporary file: %v", err)
		}
	}()

	serialized, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to yaml marshal data of type %T: %v", data, err)
	}
	if _, err := tempFile.Write(serialized); err != nil {
		t.Fatalf("could not write serialized data: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Errorf("could not close temporary file: %v", err)
	}

	goldenFile, err := filepath.Abs(filepath.Join("testdata", sanitizeFilename(t.Name())+".yaml"))
	if err != nil {
		t.Fatalf("could not determine path to golden file: %v", err)
	}
	CompareWithFixture(t, goldenFile, tempFile.Name())
}

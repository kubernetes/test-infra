/*
Copyright 2020 The Kubernetes Authors.

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
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

const (
	testDir = "testdata"
)

func resolvePath(t *testing.T, filename string) string {
	name := strings.ToLower(filepath.Base(t.Name()))
	return filepath.Join(testDir, strings.ToLower(name), name+filename)
}

func TestGenjobs(t *testing.T) {
	tests := []struct {
		name   string
		output string
		args   []string
		equal  bool
	}{
		{
			name:  "single cluster",
			args:  []string{},
			equal: true,
		},
		{
			name:  "multiple clusters",
			args:  []string{},
			equal: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := resolvePath(t, "_in.yaml")
			outE := resolvePath(t, "_out.yaml")

			expected, err := ioutil.ReadFile(outE)
			if err != nil {
				t.Errorf("Failed reading expected output file %v: %v", outE, err)
			}

			tmpDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Errorf("Failed creating temp file: %v", err)
			}
			defer os.Remove(tmpDir)

			outA := filepath.Join(tmpDir, "out.yaml")

			os.Args = []string{"cm2kc"}
			pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
			os.Args = append(os.Args, test.args...)
			os.Args = append(os.Args, "--input="+in, "--output="+outA)
			main()

			actual, err := ioutil.ReadFile(outA)
			if err != nil {
				t.Errorf("Failed reading actual output file %v: %v", outA, err)
			}

			t.Logf("expected (%v):\n%v\n", test.name, string(expected))
			t.Logf("actual (%v):\n%v\n", test.name, string(actual))

			if os.Getenv("REFRESH_GOLDEN") == "true" {
				if err = ioutil.WriteFile(outE, actual, 0644); err != nil {
					t.Errorf("Failed writing expected output file %v: %v", outE, err)
				}
				expected = actual
			}

			equal := bytes.Equal(expected, actual)
			if equal != test.equal {
				t.Errorf("Expected output equality to be: %t.", test.equal)
			}
		})
	}
}

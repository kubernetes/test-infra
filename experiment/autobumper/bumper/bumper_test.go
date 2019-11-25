/*
Copyright 2019 The Kubernetes Authors.

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

package bumper

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/config/secret"
)

type fakeWriter struct {
	results []byte
}

func (w *fakeWriter) Write(content []byte) (n int, err error) {
	w.results = append(w.results, content...)
	return len(content), nil
}

func writeToFile(t *testing.T, path, content string) {
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Errorf("write file %s dir with error '%v'", path, err)
	}
}

func TestCallWithWriter(t *testing.T) {

	dir, err := ioutil.TempDir("", "TestCallWithWriter")
	if err != nil {
		t.Errorf("failed to create temp dir '%s': '%v'", dir, err)
	}
	defer os.RemoveAll(dir)

	file1 := filepath.Join(dir, "secret1")
	file2 := filepath.Join(dir, "secret2")

	writeToFile(t, file1, "abc")
	writeToFile(t, file2, "xyz")

	sa := &secret.Agent{}
	if err := sa.Start([]string{file1, file2}); err != nil {
		t.Errorf("failed to start secrets agent; %v", err)
	}

	fakeWriter1 := fakeWriter{}
	fakeWriter2 := fakeWriter{}

	stdout := HideSecretsWriter{Delegate: &fakeWriter1, Censor: sa}
	stderr := HideSecretsWriter{Delegate: &fakeWriter2, Censor: sa}

	testCases := []struct {
		description string
		command     string
		args        []string
		expected    []string
	}{
		{
			description: "no secret in stdout are working well",
			command:     "echo",
			args:        []string{"-n", "aaa: 123"},
			expected:    []string{"aaa: 123", ""},
		},
		{
			description: "secret in stdout are censored",
			command:     "echo",
			args:        []string{"-n", "abc: 123"},
			expected:    []string{"CENSORED: 123", ""},
		},
		{
			description: "secret in stderr are censored",
			command:     "ls",
			args:        []string{"/tmp/file-not-exist/abc/xyz/file-not-exist"},
			expected:    []string{"", "ls: cannot access '/tmp/file-not-exist/CENSORED/CENSORED/file-not-exist': No such file or directory\n"},
		},
		{
			description: "no secret in stderr are working well",
			command:     "ls",
			args:        []string{"/tmp/file-not-exist/aaa/file-not-exist"},
			expected:    []string{"", "ls: cannot access '/tmp/file-not-exist/aaa/file-not-exist': No such file or directory\n"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			fakeWriter1.results = []byte{}
			fakeWriter2.results = []byte{}
			_ = Call(stdout, stderr, tc.command, tc.args...)
			if strings.Compare(tc.expected[0], string(fakeWriter1.results)) != 0 {
				t.Errorf("Expected stdout '%s', got '%s'", tc.expected[0], string(fakeWriter1.results))
			}
			if strings.Compare(tc.expected[1], string(fakeWriter2.results)) != 0 {
				t.Errorf("Expected stderr '%s', got '%s'", tc.expected[1], string(fakeWriter2.results))
			}
		})
	}
}

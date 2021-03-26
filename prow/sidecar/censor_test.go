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

package sidecar

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
	"k8s.io/test-infra/prow/secretutil"
	"k8s.io/test-infra/prow/testutil"
)

func TestCensor(t *testing.T) {
	preamble := func() string {
		return `In my younger and more vulnerable years my father gave me some advice that I’ve been turning over in my mind ever since.`
	}

	var testCases = []struct {
		name          string
		input, output string
		secrets       []string
		bufferSize    int
	}{
		{
			name:       "input smaller than buffer size",
			input:      preamble()[:100],
			secrets:    []string{"younger", "my"},
			output:     "In ** ******* and more vulnerable years ** father gave me some advice that I’ve been turning over ",
			bufferSize: 200,
		},
		{
			name:       "input larger than buffer size, not a multiple",
			input:      preamble()[:100],
			secrets:    []string{"younger", "my"},
			output:     "In ** ******* and more vulnerable years ** father gave me some advice that I’ve been turning over ",
			bufferSize: 16,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			censorer := secretutil.NewCensorer()
			censorer.Refresh(testCase.secrets...)
			input := ioutil.NopCloser(bytes.NewBufferString(testCase.input))
			outputSink := &bytes.Buffer{}
			output := nopWriteCloser(outputSink)
			if err := censor(input, output, censorer, testCase.bufferSize); err != nil {
				t.Fatalf("expected no error from censor, got %v", err)
			}
			if diff := cmp.Diff(outputSink.String(), testCase.output); diff != "" {
				t.Fatalf("got incorrect output after censoring: %v", diff)
			}
		})
	}

}

func nopWriteCloser(w io.Writer) io.WriteCloser {
	return &nopCloser{Writer: w}
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

const inputDir = "testdata/input"

func copyTestData(t *testing.T) string {
	tempDir := t.TempDir()
	if err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		relpath, _ := filepath.Rel(inputDir, path) // this errors when it's not relative, but that's known here
		dest := filepath.Join(tempDir, relpath)
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			link, err := os.Readlink(path)
			if err != nil {
				t.Fatalf("failed to read input link: %v", err)
			}
			return os.Symlink(link, dest)
		}
		if info.Name() == "link" {
			link, err := ioutil.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read input link: %v", err)
			}
			return os.Symlink(string(link), dest)
		}
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer func() {
			if err := out.Close(); err != nil {
				t.Fatalf("could not close output file: %v", err)
			}
		}()
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			if err := in.Close(); err != nil {
				t.Fatalf("could not close input file: %v", err)
			}
		}()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("failed to copy input to temp dir: %v", err)
	}
	return tempDir
}

func TestCensorIntegration(t *testing.T) {
	// copy input to a temp dir so we don't touch the golden input files
	tempDir := copyTestData(t)
	// also, tar the input - it's not trivial to diff two tarballs while only caring about
	// file content, not metadata, so this test will tar up the archive from the input and
	// untar it after the fact for simple diffs and updates
	archiveDir := filepath.Join(tempDir, "artifacts/archive")
	archiveFile := filepath.Join(tempDir, "artifacts/archive.tar.gz")
	if err := archive(archiveDir, archiveFile); err != nil {
		t.Fatalf("failed to archive input: %v", err)
	}

	bufferSize := 1
	options := Options{
		GcsOptions: &gcsupload.Options{
			Items: []string{filepath.Join(tempDir, "artifacts")},
		},
		Entries: []wrapper.Options{
			{ProcessLog: filepath.Join(tempDir, "logs/one.log")},
			{ProcessLog: filepath.Join(tempDir, "logs/two.log")},
		},
		CensoringOptions: &CensoringOptions{
			SecretDirectories: []string{"testdata/secrets"},
			// this will be smaller than the size of a secret, so this tests our buffer calculation
			CensoringBufferSize: &bufferSize,
			ExcludeDirectories:  []string{"**/exclude"},
		},
	}
	if err := options.censor(); err != nil {
		t.Fatalf("got an error from censoring: %v", err)
	}

	if err := unarchive(archiveFile, archiveDir); err != nil {
		t.Fatalf("failed to unarchive input: %v", err)
	}
	if err := os.Remove(archiveFile); err != nil {
		t.Fatalf("failed to removce archive: %v", err)
	}

	testutil.CompareWithFixtureDir(t, "testdata/output", tempDir)
}

func TestArchiveMatchesTar(t *testing.T) {
	tempDir := t.TempDir()
	archiveOutput := filepath.Join(tempDir, "archive.tar.gz")
	archiveDir := "testdata/archives"
	archiveInputs := filepath.Join(archiveDir, "archive/")
	if err := archive(archiveInputs, archiveOutput); err != nil {
		t.Fatalf("failed to archive input: %v", err)
	}
	tarOutput := t.TempDir()
	cmd := exec.Command("tar", "-C", tarOutput, "-xzvf", archiveOutput)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("could not run tar: %v:\n %s", err, string(out))
	}
	testutil.CompareWithFixtureDir(t, tarOutput, archiveInputs)
}

func TestUnarchive(t *testing.T) {
	unarchiveOutput := t.TempDir()
	archiveDir := "testdata/archives"
	archiveInputs := filepath.Join(archiveDir, "archive/")
	archiveFile := filepath.Join(archiveDir, "archive.tar.gz")
	if err := unarchive(archiveFile, unarchiveOutput); err != nil {
		t.Fatalf("failed to unarchive input: %v", err)
	}
	testutil.CompareWithFixtureDir(t, archiveInputs, unarchiveOutput)
}

func TestUnarchiveMatchesTar(t *testing.T) {
	unarchiveOutput := t.TempDir()
	archiveDir := "testdata/archives"
	archiveFile := filepath.Join(archiveDir, "archive.tar.gz")
	if err := unarchive(archiveFile, unarchiveOutput); err != nil {
		t.Fatalf("failed to unarchive input: %v", err)
	}
	tarOutput := t.TempDir()
	cmd := exec.Command("tar", "-C", tarOutput, "-xzvf", archiveFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("could not run tar: %v:\n %s", err, string(out))
	}
	testutil.CompareWithFixtureDir(t, tarOutput, unarchiveOutput)
}

func TestRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	archiveOutput := filepath.Join(tempDir, "archive.tar.gz")
	unarchiveOutput := filepath.Join(tempDir, "archive/")
	archiveDir := "testdata/archives"
	archiveInputs := filepath.Join(archiveDir, "archive/")
	if err := archive(archiveInputs, archiveOutput); err != nil {
		t.Fatalf("failed to archive input: %v", err)
	}
	if err := unarchive(archiveOutput, unarchiveOutput); err != nil {
		t.Fatalf("failed to unarchive input: %v", err)
	}
	testutil.CompareWithFixtureDir(t, archiveInputs, unarchiveOutput)
}

func TestLoadDockerCredentials(t *testing.T) {
	expected := []string{"a", "b", "c", "d", "e", "f"}
	dockercfgraw := []byte(`{
	"registry": {
		"password": "a",
		"auth": "b"
	},
	"other": {
		"password": "c",
		"auth": "d"
	},
	"third": {
		"auth": "e"
	},
	"fourth": {
		"password": "f"
	}
}`)
	dockerconfigjsonraw := []byte(`{
	"auths": {
		"registry": {
			"password": "a",
			"auth": "b"
		},
		"other": {
			"password": "c",
			"auth": "d"
		},
		"third": {
			"auth": "e"
		},
		"fourth": {
			"password": "f"
		}
	}
}`)
	malformed := []byte(`notreallyjson`)

	if _, err := loadDockercfgAuths(malformed); err == nil {
		t.Error("dockercfg: expected loading malformed data to error, but it did not")
	}
	if _, err := loadDockerconfigJsonAuths(malformed); err == nil {
		t.Error("dockerconfigjson: expected loading malformed data to error, but it did not")
	}

	actual, err := loadDockercfgAuths(dockercfgraw)
	if err != nil {
		t.Errorf("dockercfg: expected loading data not to error, but it did: %v", err)
	}
	sort.Strings(actual)
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Errorf("dockercfg: got incorrect values: %s", err)
	}

	actual, err = loadDockerconfigJsonAuths(dockerconfigjsonraw)
	if err != nil {
		t.Errorf("dockerconfigjson: expected loading data not to error, but it did: %v", err)
	}
	sort.Strings(actual)
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Errorf("dockerconfigjson: got incorrect values: %s", err)
	}
}

func TestShouldCensor(t *testing.T) {
	var testCases = []struct {
		name     string
		path     string
		options  CensoringOptions
		expected bool
	}{
		{
			name:     "no options defaults to include",
			options:  CensoringOptions{},
			path:     "/usr/bin/bash",
			expected: true,
		},
		{
			name: "not matching include defaults to false",
			options: CensoringOptions{
				IncludeDirectories: []string{"/tmp/**/*"},
			},
			path:     "/usr/bin/bash",
			expected: false,
		},
		{
			name: "matching include censors",
			options: CensoringOptions{
				IncludeDirectories: []string{"/usr/**/*"},
			},
			path:     "/usr/bin/bash",
			expected: true,
		},
		{
			name: "matching include and exclude does not censor",
			options: CensoringOptions{
				IncludeDirectories: []string{"/usr/**/*"},
				ExcludeDirectories: []string{"/usr/bin/**/*"},
			},
			path:     "/usr/bin/bash",
			expected: false,
		},
		{
			name: "matching exclude does not censor",
			options: CensoringOptions{
				ExcludeDirectories: []string{"/usr/bin/**/*"},
			},
			path:     "/usr/bin/bash",
			expected: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			should, err := shouldCensor(testCase.options, testCase.path)
			if err != nil {
				t.Fatalf("%s: got an error from shouldCensor: %v", testCase.name, err)
			}
			if should != testCase.expected {
				t.Errorf("%s: expected %v, got %v", testCase.name, testCase.expected, should)
			}
		})
	}
}

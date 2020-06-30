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

func Test_validateOptions(t *testing.T) {
	emptyStr := ""
	whateverStr := "whatever"
	falseBool := false
	trueBool := true
	emptyArr := make([]string, 0)
	cases := []struct {
		name               string
		bumpProwImages     *bool
		bumpTestImages     *bool
		githubToken        *string
		githubOrg          *string
		githubRepo         *string
		skipPullRequest    *bool
		targetVersion      *string
		includeConfigPaths *[]string
		err                bool
	}{
		{
			name: "bumping up Prow and test images together works",
			err:  false,
		},
		{
			name:           "only bumping up Prow images works",
			bumpTestImages: &falseBool,
			err:            false,
		},
		{
			name:           "at least one type of bumps needs to be specified",
			bumpProwImages: &falseBool,
			bumpTestImages: &falseBool,
			err:            true,
		},
		{
			name:        "GitHubToken must not be empty when SkipPullRequest is false",
			githubToken: &emptyStr,
			err:         true,
		},
		{
			name:      "GitHubOrg cannot be empty when SkipPullRequest is false",
			githubOrg: &emptyStr,
			err:       true,
		},
		{
			name:       "GitHubRepo cannot be empty when SkipPullRequest is false",
			githubRepo: &emptyStr,
			err:        true,
		},
		{
			name:            "all GitHub related fields can be empty when SkipPullRequest is true",
			githubOrg:       &emptyStr,
			githubRepo:      &emptyStr,
			githubToken:     &emptyStr,
			skipPullRequest: &trueBool,
			err:             false,
		},
		{
			name:          "invalid TargetVersion is not allowed",
			targetVersion: &whateverStr,
			err:           true,
		},
		{
			name:               "must include at least one config path",
			includeConfigPaths: &emptyArr,
			err:                true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defaultOption := &Options{
				GitHubOrg:           "whatever-org",
				GitHubRepo:          "whatever-repo",
				GitHubLogin:         "whatever-login",
				GitHubToken:         "whatever-token",
				GitName:             "whatever-name",
				GitEmail:            "whatever-email",
				BumpProwImages:      true,
				BumpTestImages:      true,
				TargetVersion:       latestVersion,
				IncludedConfigPaths: []string{"whatever-config-path1", "whatever-config-path2"},
				SkipPullRequest:     false,
			}

			if tc.skipPullRequest != nil {
				defaultOption.SkipPullRequest = *tc.skipPullRequest
			}
			if tc.githubToken != nil {
				defaultOption.GitHubToken = *tc.githubToken
			}
			if tc.githubOrg != nil {
				defaultOption.GitHubOrg = *tc.githubOrg
			}
			if tc.githubRepo != nil {
				defaultOption.GitHubRepo = *tc.githubRepo
			}
			if tc.bumpProwImages != nil {
				defaultOption.BumpProwImages = *tc.bumpProwImages
			}
			if tc.bumpTestImages != nil {
				defaultOption.BumpTestImages = *tc.bumpTestImages
			}
			if tc.targetVersion != nil {
				defaultOption.TargetVersion = *tc.targetVersion
			}
			if tc.includeConfigPaths != nil {
				defaultOption.IncludedConfigPaths = *tc.includeConfigPaths
			}

			err := validateOptions(defaultOption)
			if err == nil && tc.err {
				t.Errorf("Expected to get an error for %#v but got nil", defaultOption)
			}
			if err != nil && !tc.err {
				t.Errorf("Expected to not get an error for %#v but got %v", defaultOption, err)
			}
		})
	}
}

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

	var sa secret.Agent
	if err := sa.Start([]string{file1, file2}); err != nil {
		t.Errorf("failed to start secrets agent; %v", err)
	}

	var fakeOut fakeWriter
	var fakeErr fakeWriter

	stdout := hideSecretsWriter{delegate: &fakeOut, censor: &sa}
	stderr := hideSecretsWriter{delegate: &fakeErr, censor: &sa}

	testCases := []struct {
		description string
		command     string
		args        []string
		expectedOut string
		expectedErr string
	}{
		{
			description: "no secret in stdout are working well",
			command:     "echo",
			args:        []string{"-n", "aaa: 123"},
			expectedOut: "aaa: 123",
		},
		{
			description: "secret in stdout are censored",
			command:     "echo",
			args:        []string{"-n", "abc: 123"},
			expectedOut: "CENSORED: 123",
		},
		{
			description: "secret in stderr are censored",
			command:     "ls",
			args:        []string{"/tmp/file-not-exist/abc/xyz/file-not-exist"},
			expectedErr: "/tmp/file-not-exist/CENSORED/CENSORED/file-not-exist",
		},
		{
			description: "no secret in stderr are working well",
			command:     "ls",
			args:        []string{"/tmp/file-not-exist/aaa/file-not-exist"},
			expectedErr: "/tmp/file-not-exist/aaa/file-not-exist",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			fakeOut.results = []byte{}
			fakeErr.results = []byte{}
			_ = call(stdout, stderr, tc.command, tc.args...)
			if full, want := string(fakeOut.results), tc.expectedOut; !strings.Contains(full, want) {
				t.Errorf("stdout does not contain %q, got %q", full, want)
			}
			if full, want := string(fakeErr.results), tc.expectedErr; !strings.Contains(full, want) {
				t.Errorf("stderr does not contain %q, got %q", full, want)
			}
		})
	}
}

func TestIsUnderPath(t *testing.T) {
	cases := []struct {
		description string
		paths       []string
		file        string
		expected    bool
	}{
		{
			description: "file is under the direct path",
			paths:       []string{"config/prow/"},
			file:        "config/prow/config.yaml",
			expected:    true,
		},
		{
			description: "file is under the indirect path",
			paths:       []string{"config/prow-staging/"},
			file:        "config/prow-staging/jobs/config.yaml",
			expected:    true,
		},
		{
			description: "file is under one path but not others",
			paths:       []string{"config/prow/", "config/prow-staging/"},
			file:        "config/prow-staging/jobs/whatever-repo/whatever-file",
			expected:    true,
		},
		{
			description: "file is not under the path but having the same prefix",
			paths:       []string{"config/prow/"},
			file:        "config/prow-staging/config.yaml",
			expected:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			actual := isUnderPath(tc.file, tc.paths)
			if actual != tc.expected {
				t.Errorf("expected to be %t but actual is %t", tc.expected, actual)
			}
		})
	}
}

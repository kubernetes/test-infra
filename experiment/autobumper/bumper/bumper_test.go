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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/config/secret"
)

func TestValidateOptions(t *testing.T) {
	emptyStr := ""
	whateverStr := "whatever"
	falseBool := false
	trueBool := true
	emptyArr := make([]string, 0)
	upstreamVersion := "upstream"
	cases := []struct {
		name               string
		bumpProwImages     *bool
		bumpBoskosImages   *bool
		bumpTestImages     *bool
		githubToken        *string
		githubOrg          *string
		githubRepo         *string
		remoteBranch       *string
		skipPullRequest    *bool
		targetVersion      *string
		includeConfigPaths *[]string
		err                bool
	}{
		{
			name: "bumping up Prow, Boskos and test images together works",
			err:  false,
		},
		{
			name:             "only bumping up Prow images works",
			bumpBoskosImages: &falseBool,
			bumpTestImages:   &falseBool,
			err:              false,
		},
		{
			name:             "at least one type of bumps needs to be specified",
			bumpProwImages:   &falseBool,
			bumpBoskosImages: &falseBool,
			bumpTestImages:   &falseBool,
			err:              true,
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
			name:         "RemoteBranch cannot be empty when SkipPullRequest is false",
			remoteBranch: &emptyStr,
			err:          true,
		},
		{
			name:            "all GitHub related fields can be empty when SkipPullRequest is true",
			githubOrg:       &emptyStr,
			githubRepo:      &emptyStr,
			githubToken:     &emptyStr,
			remoteBranch:    &emptyStr,
			skipPullRequest: &trueBool,
			err:             false,
		},
		{
			name:           "unformatted TargetVersion is also allowed",
			targetVersion:  &whateverStr,
			bumpTestImages: &falseBool,
			err:            false,
		},
		{
			name:             "only latest version can be used if both Prow and test images are bumped",
			bumpBoskosImages: &falseBool,
			targetVersion:    &upstreamVersion,
			err:              true,
		},
		{
			name:           "only latest version can be used if both Boskos and test images are bumped",
			bumpProwImages: &falseBool,
			targetVersion:  &upstreamVersion,
			err:            true,
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
				RemoteBranch:        "whatever-branch",
				BumpProwImages:      true,
				BumpBoskosImages:    true,
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
			if tc.remoteBranch != nil {
				defaultOption.RemoteBranch = *tc.remoteBranch
			}
			if tc.bumpProwImages != nil {
				defaultOption.BumpProwImages = *tc.bumpProwImages
			}
			if tc.bumpBoskosImages != nil {
				defaultOption.BumpBoskosImages = *tc.bumpBoskosImages
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
			t.Logf("err is: %v", err)
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

	stdout := HideSecretsWriter{Delegate: &fakeOut, Censor: &sa}
	stderr := HideSecretsWriter{Delegate: &fakeErr, Censor: &sa}

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
			_ = Call(stdout, stderr, tc.command, tc.args...)
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

func TestGetAssignment(t *testing.T) {
	cases := []struct {
		description          string
		oncallURL            string
		oncallServerResponse string
		expectResKeyword     string
	}{
		{
			description:          "empty oncall URL will return an empty string",
			oncallURL:            "",
			oncallServerResponse: "",
			expectResKeyword:     "",
		},
		{
			description:          "an invalid oncall URL will return an error message",
			oncallURL:            "whatever-url",
			oncallServerResponse: "",
			expectResKeyword:     "error",
		},
		{
			description:          "an invalid response will return an error message",
			oncallURL:            "auto",
			oncallServerResponse: "whatever-malformed-response",
			expectResKeyword:     "error",
		},
		{
			description:          "a valid response will return the oncaller",
			oncallURL:            "auto",
			oncallServerResponse: `{"Oncall":{"testinfra":"fake-oncall-name"}}`,
			expectResKeyword:     "fake-oncall-name",
		},
		{
			description:          "a valid response with empty oncall will return on oncall message",
			oncallURL:            "auto",
			oncallServerResponse: `{"Oncall":{"testinfra":""}}`,
			expectResKeyword:     "Nobody",
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			if tc.oncallURL == "auto" {
				// generate a test server so we can capture and inspect the request
				testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
					res.Write([]byte(tc.oncallServerResponse))
				}))
				defer func() { testServer.Close() }()
				tc.oncallURL = testServer.URL
			}

			res := getAssignment(tc.oncallURL)
			if !strings.Contains(res, tc.expectResKeyword) {
				t.Errorf("Expect the result %q contains keyword %q but it does not", res, tc.expectResKeyword)
			}
		})
	}
}

type fakeImageBumperCli struct {
	replacements map[string]string
}

func (c *fakeImageBumperCli) FindLatestTag(imageHost, imageName, currentTag string) (string, error) {
	return "fake-latest", nil
}

func (c *fakeImageBumperCli) UpdateFile(tagPicker func(imageHost, imageName, currentTag string) (string, error),
	path string, imageFilter *regexp.Regexp) error {
	targetTag, _ := tagPicker("", "", "")
	c.replacements[path] = targetTag
	return nil
}

func (c *fakeImageBumperCli) GetReplacements() map[string]string {
	return c.replacements
}

func TestUpdateReferences(t *testing.T) {
	cases := []struct {
		description        string
		targetVersion      string
		includeConfigPaths []string
		excludeConfigPaths []string
		extraFiles         []string
		expectedRes        map[string]string
		expectError        bool
	}{
		{
			description:        "update the images to the latest version",
			targetVersion:      latestVersion,
			includeConfigPaths: []string{"testdata/dir/subdir1", "testdata/dir/subdir2"},
			expectedRes: map[string]string{
				"testdata/dir/subdir1/test1-1.yaml": "fake-latest",
				"testdata/dir/subdir1/test1-2.yaml": "fake-latest",
				"testdata/dir/subdir2/test2-1.yaml": "fake-latest",
			},
			expectError: false,
		},
		{
			description:        "update the images to a specific version",
			targetVersion:      "v20200101-livebull",
			includeConfigPaths: []string{"testdata/dir/subdir2"},
			expectedRes: map[string]string{
				"testdata/dir/subdir2/test2-1.yaml": "v20200101-livebull",
			},
			expectError: false,
		},
		{
			description:        "by default only yaml files will be updated",
			targetVersion:      latestVersion,
			includeConfigPaths: []string{"testdata/dir/subdir3"},
			expectedRes: map[string]string{
				"testdata/dir/subdir3/test3-1.yaml": "fake-latest",
			},
			expectError: false,
		},
		{
			description:        "files under the excluded paths will not be updated",
			targetVersion:      latestVersion,
			includeConfigPaths: []string{"testdata/dir"},
			excludeConfigPaths: []string{"testdata/dir/subdir1", "testdata/dir/subdir2"},
			expectedRes: map[string]string{
				"testdata/dir/subdir3/test3-1.yaml": "fake-latest",
			},
			expectError: false,
		},
		{
			description:        "non YAML files could be configured by specifying extraFiles",
			targetVersion:      latestVersion,
			includeConfigPaths: []string{"testdata/dir/subdir3"},
			extraFiles:         []string{"testdata/dir/extra-file", "testdata/dir/subdir3/test3-2"},
			expectedRes: map[string]string{
				"testdata/dir/subdir3/test3-1.yaml": "fake-latest",
				"testdata/dir/extra-file":           "fake-latest",
				"testdata/dir/subdir3/test3-2":      "fake-latest",
			},
			expectError: false,
		},
		{
			description:        "updating non-existed files will return an error",
			targetVersion:      latestVersion,
			includeConfigPaths: []string{"testdata/dir/whatever-subdir"},
			expectError:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			cli := &fakeImageBumperCli{replacements: map[string]string{}}
			res, err := updateReferences(cli, nil, tc.targetVersion,
				tc.includeConfigPaths, tc.excludeConfigPaths, tc.extraFiles)
			if tc.expectError && err == nil {
				t.Errorf("Expected to get an error but the result is nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected to not get an error but got one: %v", err)
			}

			if !reflect.DeepEqual(res, tc.expectedRes) {
				t.Errorf("Expected to get the result map as %v but got %v", tc.expectedRes, res)
			}
		})
	}
}

func TestParseUpstreamImageVersion(t *testing.T) {
	cases := []struct {
		description            string
		upstreamURL            string
		upstreamServerResponse string
		expectedRes            string
		expectError            bool
	}{
		{
			description:            "empty upstream URL will return an error",
			upstreamURL:            "",
			upstreamServerResponse: "",
			expectedRes:            "",
			expectError:            true,
		},
		{
			description:            "an invalid upstream URL will return an error",
			upstreamURL:            "whatever-url",
			upstreamServerResponse: "",
			expectedRes:            "",
			expectError:            true,
		},
		{
			description:            "an invalid response will return an error",
			upstreamURL:            "auto",
			upstreamServerResponse: "whatever-response",
			expectedRes:            "",
			expectError:            true,
		},
		{
			description:            "a valid response will return the correct tag",
			upstreamURL:            "auto",
			upstreamServerResponse: "     image: gcr.io/k8s-prow/deck:v20200717-cf288082e1",
			expectedRes:            "v20200717-cf288082e1",
			expectError:            false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			if tc.upstreamURL == "auto" {
				// generate a test server so we can capture and inspect the request
				testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
					res.Write([]byte(tc.upstreamServerResponse))
				}))
				defer func() { testServer.Close() }()
				tc.upstreamURL = testServer.URL
			}

			res, err := parseUpstreamImageVersion(tc.upstreamURL)
			if res != tc.expectedRes {
				t.Errorf("The expected result %q != the actual result %q", tc.expectedRes, res)
			}
			if tc.expectError && err == nil {
				t.Errorf("Expected to get an error but the result is nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected to not get an error but got one: %v", err)
			}
		})
	}
}

func TestUpstreamImageVersionResolver(t *testing.T) {
	prowProdFakeVersion := "v-prow-prod-version"
	prowStagingFakeVersion := "v-prow-staging-version"
	boskosProdFakeVersion := "v-boskos-prod-version"
	boskosStagingFakeVersion := "v-boskos-staging-version"

	prowProdUpstreamAddress, boskosProdUpstreamAddress, _ := upstreamConfigFileAddresses(upstreamVersion)
	prowStagingUpstreamAddress, boskosStagingUpstreamAddress, _ := upstreamConfigFileAddresses(upstreamStagingVersion)

	fakeImageVersionParser := func(upstreamAddress string) (string, error) {
		switch upstreamAddress {
		case prowProdUpstreamAddress:
			return prowProdFakeVersion, nil
		case prowStagingUpstreamAddress:
			return prowStagingFakeVersion, nil
		case boskosProdUpstreamAddress:
			return boskosProdFakeVersion, nil
		case boskosStagingUpstreamAddress:
			return boskosStagingFakeVersion, nil
		default:
			return "", fmt.Errorf("unsupported upstream address %q for parsing the image version", upstreamAddress)
		}
	}

	cases := []struct {
		description         string
		upstreamVersionType string
		imageHost           string
		imageName           string
		currentTag          string
		expectedTargetTag   string
		expectError         bool
	}{
		{
			description:         "resolve image version with an invalid version type",
			upstreamVersionType: "whatever-version-type",
			expectError:         true,
		},
		{
			description:         "resolve production Prow image version",
			upstreamVersionType: upstreamVersion,
			imageHost:           prowPrefix,
			imageName:           "whatever-image-name",
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   prowProdFakeVersion,
			expectError:         false,
		},
		{
			description:         "resolve staging Prow image version",
			upstreamVersionType: upstreamStagingVersion,
			imageHost:           prowPrefix,
			imageName:           "whatever-image-name",
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   prowStagingFakeVersion,
			expectError:         false,
		},
		{
			description:         "resolve production Boskos image version",
			upstreamVersionType: upstreamVersion,
			imageHost:           boskosPrefix,
			imageName:           "whatever-image-name",
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   boskosProdFakeVersion,
			expectError:         false,
		},
		{
			description:         "resolve staging Boskos image version",
			upstreamVersionType: upstreamStagingVersion,
			imageHost:           boskosPrefix,
			imageName:           "whatever-image-name",
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   boskosStagingFakeVersion,
			expectError:         false,
		},
		{
			description:         "resolve random image version",
			upstreamVersionType: upstreamVersion,
			imageHost:           "whatever-image-host",
			imageName:           "whatever-image-name",
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   "whatever-current-tag",
			expectError:         false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			resolver, err := upstreamImageVersionResolver(tc.upstreamVersionType, fakeImageVersionParser)
			if tc.expectError && err == nil {
				t.Errorf("Expected to get an error but the result is nil")
				return
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected to not get an error but got one: %v", err)
				return
			}

			if err == nil && resolver == nil {
				t.Error("Expected to get an image resolver but got nil")
				return
			}

			if resolver != nil {
				res, _ := resolver(tc.imageHost, tc.imageName, tc.currentTag)
				if tc.expectedTargetTag != res {
					t.Errorf("Expected to get target tag %q but got %q", tc.expectedTargetTag, res)
				}
			}
		})
	}
}

func TestUpstreamConfigFileAddresses(t *testing.T) {
	prowProdUpstreamAddress := upstreamURLBase + "/" + prowRefConfigFile
	boskosProdUpstreamAddress := upstreamURLBase + "/" + boskosRefConfigFile
	prowStagingUpstreamAddress := upstreamURLBase + "/" + prowStagingRefConfigFile
	boskosStagingUpstreamAddress := upstreamURLBase + "/" + boskosStagingRefConfigFile

	cases := []struct {
		description                   string
		upstreamVersionType           string
		expectedProwUpstreamAddress   string
		expectedBoskosUpstreamAddress string
		expectError                   bool
	}{
		{
			description:                   "get config file addresses for prod",
			upstreamVersionType:           upstreamVersion,
			expectedProwUpstreamAddress:   prowProdUpstreamAddress,
			expectedBoskosUpstreamAddress: boskosProdUpstreamAddress,
			expectError:                   false,
		},
		{
			description:                   "get config file addresses for staging",
			upstreamVersionType:           upstreamStagingVersion,
			expectedProwUpstreamAddress:   prowStagingUpstreamAddress,
			expectedBoskosUpstreamAddress: boskosStagingUpstreamAddress,
			expectError:                   false,
		},
		{
			description:                   "get config file addresses for an invalid version type",
			upstreamVersionType:           "whatever-version-type",
			expectedProwUpstreamAddress:   "",
			expectedBoskosUpstreamAddress: "",
			expectError:                   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			prowUpstreamAddress, boskosUpstreamAddress, err := upstreamConfigFileAddresses(tc.upstreamVersionType)
			if tc.expectError && err == nil {
				t.Errorf("Expected to get an error but the result is nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected to not get an error but got one: %v", err)
			}
			if prowUpstreamAddress != tc.expectedProwUpstreamAddress {
				t.Errorf("Expected to get %q for the Prow upstream address but got %q",
					tc.expectedProwUpstreamAddress, prowUpstreamAddress)
			}
			if boskosUpstreamAddress != tc.expectedBoskosUpstreamAddress {
				t.Errorf("Expected to get %q for the Boskos upstream address but got %q",
					tc.expectedBoskosUpstreamAddress, boskosUpstreamAddress)
			}
		})
	}
}

func TestCDToRootDir(t *testing.T) {
	envName := "BUILD_WORKSPACE_DIRECTORY"

	cases := []struct {
		description       string
		buildWorkspaceDir string
		expectedResDir    string
		expectError       bool
	}{
		// This test case does not work when running with Bazel.
		{
			description:       "BUILD_WORKSPACE_DIRECTORY is a valid directory",
			buildWorkspaceDir: "./testdata/dir",
			expectedResDir:    "testdata/dir",
			expectError:       false,
		},
		{
			description:       "BUILD_WORKSPACE_DIRECTORY is an invalid directory",
			buildWorkspaceDir: "whatever-dir",
			expectedResDir:    "",
			expectError:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			curtDir, _ := os.Getwd()
			curtBuildWorkspaceDir := os.Getenv(envName)
			defer os.Chdir(curtDir)
			defer os.Setenv(envName, curtBuildWorkspaceDir)

			os.Setenv(envName, filepath.Join(curtDir, tc.buildWorkspaceDir))
			err := cdToRootDir()
			if tc.expectError && err == nil {
				t.Errorf("Expected to get an error but the result is nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected to not get an error but got one: %v", err)
			}

			if !tc.expectError {
				afterDir, _ := os.Getwd()
				if !strings.HasSuffix(afterDir, tc.expectedResDir) {
					t.Errorf("Expected to switch to %q but was switched to: %q", tc.expectedResDir, afterDir)
				}
			}
		})
	}
}

func TestGetNewProwVersion(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		images        map[string]string
		expectedTag   string
		expectedError string
	}{
		{
			name: "No images, no result, no error",
		},
		{
			name:        "One version is returned",
			images:      map[string]string{"gcr.io/k8s-prow/deck": "v20200914-1ac05b0ca2"},
			expectedTag: "v20200914-1ac05b0ca2",
		},
		{
			name:          "Multiple versions, error",
			images:        map[string]string{"gcr.io/k8s-prow/deck": "v20200914-1ac05b0ca2", "gcr.io/k8s-prow/hook": "v20200915-1ac05", "gcr.io/k8s-prow/tide": "v20200915-1ac05"},
			expectedError: `Expected a consistent version for all "gcr.io/k8s-prow/" images, but found multiple: map[v20200914-1ac05b0ca2:[gcr.io/k8s-prow/deck] v20200915-1ac05:[gcr.io/k8s-prow/hook gcr.io/k8s-prow/tide]]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var errMsg string
			actualTag, err := getNewProwVersion(tc.images)
			if err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.expectedError {
				t.Fatalf("got error %v, expected error %s", err, tc.expectedError)
			}
			if err != nil {
				return
			}
			if actualTag != tc.expectedTag {
				t.Errorf("expected tag %s, got tag %s", tc.expectedTag, actualTag)
			}
		})
	}
}

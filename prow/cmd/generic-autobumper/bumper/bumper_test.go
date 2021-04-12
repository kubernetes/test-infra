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

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/google/go-cmp/cmp"
	"k8s.io/test-infra/prow/config/secret"
)

func TestCommitToRef(t *testing.T) {
	cases := []struct {
		name     string
		commit   string
		expected string
	}{
		{
			name: "basically works",
		},
		{
			name:     "just tag works",
			commit:   "v0.0.30",
			expected: "v0.0.30",
		},
		{
			name:     "just commit works",
			commit:   "deadbeef",
			expected: "deadbeef",
		},
		{
			name:     "commits past tag works",
			commit:   "v0.0.30-14-gdeadbeef",
			expected: "deadbeef",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual, expected := commitToRef(tc.commit), tc.expected; actual != tc.expected {
				t.Errorf("commitToRef(%q) got %q want %q", tc.commit, actual, expected)
			}
		})
	}
}

func TestValidateOptions(t *testing.T) {
	emptyStr := ""
	whateverStr := "whatever"
	trueVar := true
	emptyArr := make([]string, 0)
	emptyPrefixes := make([]Prefix, 0)
	latestPrefixes := []Prefix{{
		Name:                 "test",
		Prefix:               "gcr.io/test/",
		RefConfigFile:        "",
		StagingRefConfigFile: "",
	}}
	upstreamPrefixes := []Prefix{{
		Name:                 "test",
		Prefix:               "gcr.io/test/",
		RefConfigFile:        "ref",
		StagingRefConfigFile: "stagingRef",
	}}
	upstreamVersion := "upstream"
	stagingVersion := "upstream-staging"
	cases := []struct {
		name                string
		githubToken         *string
		githubOrg           *string
		githubRepo          *string
		gerrit              *bool
		gerritAuthor        *string
		gerritPRIdentifier  *string
		gerritHostRepo      *string
		gerritCookieFile    *string
		remoteName          *string
		skipPullRequest     *bool
		targetVersion       *string
		includeConfigPaths  *[]string
		prefixes            *[]Prefix
		upstreamURLBase     *string
		err                 bool
		upstreamBaseChanged bool
	}{
		{
			name: "Everything correct",
			err:  false,
		},
		{
			name:        "GitHubToken must not be empty when SkipPullRequest is false",
			githubToken: &emptyStr,
			err:         true,
		},
		{
			name:       "remoteName must not be empty when SkipPullRequest is false",
			remoteName: &emptyStr,
			err:        true,
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
			name:         "gerritAuthor cannot be empty when SkipPullRequest is false and gerrit is true",
			gerrit:       &trueVar,
			gerritAuthor: &emptyStr,
			err:          true,
		},
		{
			name:           "gerritHostRepo cannot be empty when SkipPullRequest is false and gerrit is true",
			gerrit:         &trueVar,
			gerritHostRepo: &emptyStr,
			err:            true,
		},
		{
			name:             "gerritCookieFile cannot be empty when SkipPullRequest is false and gerrit is true",
			gerrit:           &trueVar,
			gerritCookieFile: &emptyStr,
			err:              true,
		},
		{
			name:               "gerritCommitId cannot be empty when SkipPullRequest is false and gerrit is true",
			gerrit:             &trueVar,
			gerritPRIdentifier: &emptyStr,
			err:                true,
		},

		{
			name:          "unformatted TargetVersion is also allowed",
			targetVersion: &whateverStr,
			err:           false,
		},
		{
			name:               "must include at least one config path",
			includeConfigPaths: &emptyArr,
			err:                true,
		},
		{
			name:                "must include upstreamURLBase if target version is upstream",
			upstreamURLBase:     &emptyStr,
			targetVersion:       &upstreamVersion,
			prefixes:            &upstreamPrefixes,
			err:                 false,
			upstreamBaseChanged: true,
		},
		{
			name:                "must include upstreamURLBase if target version is upstreamStaging",
			upstreamURLBase:     &emptyStr,
			targetVersion:       &stagingVersion,
			prefixes:            &upstreamPrefixes,
			err:                 false,
			upstreamBaseChanged: true,
		},
		{
			name:     "must include at least one prefix",
			prefixes: &emptyPrefixes,
			err:      true,
		},
		{
			name:          "must have ref files for upstream version",
			targetVersion: &upstreamVersion,
			prefixes:      &latestPrefixes,
			err:           true,
		},
		{
			name:          "must have stagingRef files for Stagingupstream version",
			targetVersion: &stagingVersion,
			prefixes:      &latestPrefixes,
			err:           true,
		},
		{
			name:                "don't use default upstreamURLbase if not needed for upstream",
			upstreamURLBase:     &whateverStr,
			targetVersion:       &upstreamVersion,
			prefixes:            &upstreamPrefixes,
			err:                 false,
			upstreamBaseChanged: false,
		},
		{
			name:                "don't use default upstreamURLbase if not neededfor upstreamStaging",
			upstreamURLBase:     &whateverStr,
			targetVersion:       &stagingVersion,
			prefixes:            &upstreamPrefixes,
			err:                 false,
			upstreamBaseChanged: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gerrit := &Gerrit{
				Author:               "whatever-author",
				CookieFile:           "whatever cookie file",
				AutobumpPRIdentifier: "whatever-commit-id",
				HostRepo:             "whatever-host-repo",
			}
			defaultOption := &Options{
				GitHubOrg:           "whatever-org",
				GitHubRepo:          "whatever-repo",
				GitHubLogin:         "whatever-login",
				GitHubToken:         "whatever-token",
				GitName:             "whatever-name",
				GitEmail:            "whatever-email",
				Gerrit:              nil,
				UpstreamURLBase:     "whatever-URLBase",
				RemoteName:          "whatever-name",
				Prefixes:            latestPrefixes,
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
			if tc.remoteName != nil {
				defaultOption.RemoteName = *tc.remoteName
			}
			if tc.githubOrg != nil {
				defaultOption.GitHubOrg = *tc.githubOrg
			}
			if tc.githubRepo != nil {
				defaultOption.GitHubRepo = *tc.githubRepo
			}
			if tc.targetVersion != nil {
				defaultOption.TargetVersion = *tc.targetVersion
			}
			if tc.includeConfigPaths != nil {
				defaultOption.IncludedConfigPaths = *tc.includeConfigPaths
			}
			if tc.prefixes != nil {
				defaultOption.Prefixes = *tc.prefixes
			}
			if tc.upstreamURLBase != nil {
				defaultOption.UpstreamURLBase = *tc.upstreamURLBase
			}
			if tc.gerrit != nil {
				defaultOption.Gerrit = gerrit
			}
			if tc.gerritAuthor != nil {
				defaultOption.Gerrit.Author = *tc.gerritAuthor
			}
			if tc.gerritPRIdentifier != nil {
				defaultOption.Gerrit.AutobumpPRIdentifier = *tc.gerritPRIdentifier
			}
			if tc.gerritCookieFile != nil {
				defaultOption.Gerrit.CookieFile = *tc.gerritCookieFile
			}
			if tc.gerritHostRepo != nil {
				defaultOption.Gerrit.HostRepo = *tc.gerritHostRepo
			}

			err := validateOptions(defaultOption)
			t.Logf("err is: %v", err)
			if err == nil && tc.err {
				t.Errorf("Expected to get an error for %#v but got nil", defaultOption)
			}
			if err != nil && !tc.err {
				t.Errorf("Expected to not get an error for %#v but got %v", defaultOption, err)
			}
			if tc.upstreamBaseChanged && defaultOption.UpstreamURLBase != defaultUpstreamURLBase {
				t.Errorf("UpstreamURLBase should have been changed to %q, but was %q", defaultOption.UpstreamURLBase, defaultUpstreamURLBase)
			}
			if !tc.upstreamBaseChanged && defaultOption.UpstreamURLBase == defaultUpstreamURLBase {
				t.Errorf("UpstreamURLBase should not have been changed to default, but was")
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
			expectedOut: "***: 123",
		},
		{
			description: "secret in stderr are censored",
			command:     "ls",
			args:        []string{"/tmp/file-not-exist/abc/xyz/file-not-exist"},
			expectedErr: "/tmp/file-not-exist/***/***/file-not-exist",
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
		oncallGroup          string
		oncallServerResponse string
		expectResKeyword     string
	}{
		{
			description:          "empty oncall URL will return an empty string",
			oncallURL:            "",
			oncallGroup:          defaultOncallGroup,
			oncallServerResponse: "",
			expectResKeyword:     "",
		},
		{
			description:          "an invalid oncall URL will return an error message",
			oncallURL:            "whatever-url",
			oncallGroup:          defaultOncallGroup,
			oncallServerResponse: "",
			expectResKeyword:     "error",
		},
		{
			description:          "an invalid response will return an error message",
			oncallURL:            "auto",
			oncallGroup:          defaultOncallGroup,
			oncallServerResponse: "whatever-malformed-response",
			expectResKeyword:     "error",
		},
		{
			description:          "a valid response will return the oncaller from default group",
			oncallURL:            "auto",
			oncallGroup:          defaultOncallGroup,
			oncallServerResponse: `{"Oncall":{"testinfra":"fake-oncall-name"}}`,
			expectResKeyword:     "fake-oncall-name",
		},
		{
			description:          "a valid response will return the oncaller from non-default group",
			oncallURL:            "auto",
			oncallGroup:          "another-group",
			oncallServerResponse: `{"Oncall":{"testinfra":"fake-oncall-name","another-group":"fake-oncall-name2"}}`,
			expectResKeyword:     "fake-oncall-name2",
		},
		{
			description:          "a valid response without expected oncall group",
			oncallURL:            "auto",
			oncallGroup:          "group-not-exist",
			oncallServerResponse: `{"Oncall":{"testinfra":"fake-oncall-name","another-group":"fake-oncall-name2"}}`,
			expectResKeyword:     "error",
		},
		{
			description:          "a valid response with empty oncall will return on oncall message",
			oncallURL:            "auto",
			oncallGroup:          defaultOncallGroup,
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

			res := getAssignment(tc.oncallURL, tc.oncallGroup)
			if !strings.Contains(res, tc.expectResKeyword) {
				t.Errorf("Expect the result %q contains keyword %q but it does not", res, tc.expectResKeyword)
			}
		})
	}
}

type fakeImageBumperCli struct {
	replacements map[string]string
	tagCache     map[string]string
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

func (c *fakeImageBumperCli) AddToCache(image, newTag string) {
	c.tagCache[image] = newTag
}

func (cli *fakeImageBumperCli) TagExists(imageHost, imageName, currentTag string) (bool, error) {
	if currentTag == "DNE" {
		return false, nil
	}
	return true, nil
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
			option := &Options{
				TargetVersion:       tc.targetVersion,
				IncludedConfigPaths: tc.includeConfigPaths,
				SkipPullRequest:     false,
				ExtraFiles:          tc.extraFiles,
				ExcludedConfigPaths: tc.excludeConfigPaths,
			}
			cli := &fakeImageBumperCli{replacements: map[string]string{}}
			res, err := updateReferences(cli, nil, option)
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
		prefix                 string
	}{
		{
			description:            "empty upstream URL will return an error",
			upstreamURL:            "",
			upstreamServerResponse: "",
			expectedRes:            "",
			expectError:            true,
			prefix:                 "gcr.io/k8s-prow/",
		},
		{
			description:            "an invalid upstream URL will return an error",
			upstreamURL:            "whatever-url",
			upstreamServerResponse: "",
			expectedRes:            "",
			expectError:            true,
			prefix:                 "gcr.io/k8s-prow/",
		},
		{
			description:            "an invalid response will return an error",
			upstreamURL:            "auto",
			upstreamServerResponse: "whatever-response",
			expectedRes:            "",
			expectError:            true,
			prefix:                 "gcr.io/k8s-prow/",
		},
		{
			description:            "a valid response will return the correct tag",
			upstreamURL:            "auto",
			upstreamServerResponse: "     image: gcr.io/k8s-prow/deck:v20200717-cf288082e1",
			expectedRes:            "v20200717-cf288082e1",
			expectError:            false,
			prefix:                 "gcr.io/k8s-prow/",
		},
		{
			description:            "a valid response will return the correct tag with other prefixes in the same file",
			upstreamURL:            "auto",
			upstreamServerResponse: "other random garbage\n image: gcr.io/k8s-other/deck:v22222222-cf288082e1\n image: gcr.io/k8s-prow/deck:v20200717-cf288082e1\n     image: gcr.io/k8s-another/deck:v11111111-cf288082e1",
			expectedRes:            "v20200717-cf288082e1",
			expectError:            false,
			prefix:                 "gcr.io/k8s-prow/",
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

			res, err := parseUpstreamImageVersion(tc.upstreamURL, tc.prefix)
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
	prowRefConfigFile := "prow-prod"
	boskosRefConfigFile := "boskos-prod"
	prowStagingRefConfigFile := "prow-staging"
	boskosStagingRefConfigFile := "boskos-staging"
	fakeUpstreamURLBase := "test.com"
	prowPrefix := "gcr.io/k8s-prow/"
	boskosPrefix := "gcr.io/k8s-boskos/"
	doesNotExistPrefix := "gcr.io/dne"
	doesNotExist := "DNE"

	prowPrefixStruct := Prefix{
		Prefix:               prowPrefix,
		RefConfigFile:        prowRefConfigFile,
		StagingRefConfigFile: prowStagingRefConfigFile,
	}
	boskosPrefixStruct := Prefix{
		Prefix:               boskosPrefix,
		RefConfigFile:        boskosRefConfigFile,
		StagingRefConfigFile: boskosStagingRefConfigFile,
	}
	//Prefix used to test when a tag does not exist. This is used to have parser return a tag that will make TagExists return false
	tagDoesNotExistPrefix := Prefix{
		Prefix:               doesNotExistPrefix,
		RefConfigFile:        doesNotExist,
		StagingRefConfigFile: doesNotExist,
	}

	fakeImageVersionParser := func(upstreamAddress, prefix string) (string, error) {
		switch upstreamAddress {
		case fakeUpstreamURLBase + "/" + doesNotExist:
			return doesNotExist, nil
		case fakeUpstreamURLBase + "/" + prowRefConfigFile:
			return prowProdFakeVersion, nil
		case fakeUpstreamURLBase + "/" + prowStagingRefConfigFile:
			return prowStagingFakeVersion, nil
		case fakeUpstreamURLBase + "/" + boskosRefConfigFile:
			return boskosProdFakeVersion, nil
		case fakeUpstreamURLBase + "/" + boskosStagingRefConfigFile:
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
		resolverError       bool
		prefixes            []Prefix
	}{
		{
			description:         "resolve image version with an invalid version type",
			upstreamVersionType: "whatever-version-type",
			expectError:         true,
			prefixes:            []Prefix{prowPrefixStruct, boskosPrefixStruct},
		},
		{
			description:         "resolve image with two prefixes possible and upstreamVersion",
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []Prefix{prowPrefixStruct, boskosPrefixStruct},
			imageHost:           prowPrefix,
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   prowProdFakeVersion,
		},
		{
			description:         "resolve image with two prefixes possible and staging version",
			upstreamVersionType: upstreamStagingVersion,
			expectError:         false,
			prefixes:            []Prefix{prowPrefixStruct, boskosPrefixStruct},
			imageHost:           boskosPrefix,
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   boskosStagingFakeVersion,
		},
		{
			description:         "resolve image when unknown prefix",
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []Prefix{boskosPrefixStruct},
			imageHost:           prowPrefix,
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   "whatever-current-tag",
		},
		{
			description:         "tag does not exist",
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []Prefix{tagDoesNotExistPrefix},
			imageHost:           doesNotExistPrefix,
			currentTag:          "doesNotExist",
			expectedTargetTag:   "",
			resolverError:       true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			option := &Options{
				UpstreamURLBase: fakeUpstreamURLBase,
				Prefixes:        tc.prefixes,
			}
			cli := &fakeImageBumperCli{replacements: map[string]string{}, tagCache: map[string]string{}}
			resolver, err := upstreamImageVersionResolver(option, tc.upstreamVersionType, fakeImageVersionParser, cli)
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
				res, resErr := resolver(tc.imageHost, tc.imageName, tc.currentTag)
				if !tc.resolverError && resErr != nil {
					t.Errorf("Expected resolver to return without error, but received error: %v", resErr)
				}
				if tc.resolverError && resErr == nil {
					t.Error("Expected resolver to return with error, but did not receive one")
				}
				if tc.expectedTargetTag != res {
					t.Errorf("Expected to get target tag %q but got %q", tc.expectedTargetTag, res)
				}

			}

		})
	}
}

func TestUpstreamConfigVersions(t *testing.T) {
	prowProdFakeVersion := "v-prow-prod-version"
	prowStagingFakeVersion := "v-prow-staging-version"
	boskosProdFakeVersion := "v-boskos-prod-version"
	boskosStagingFakeVersion := "v-boskos-staging-version"
	prowRefConfigFile := "prow-prod"
	boskosRefConfigFile := "boskos-prod"
	prowStagingRefConfigFile := "prow-staging"
	boskosStagingRefConfigFile := "boskos-staging"
	fakeUpstreamURLBase := "test.com"
	prowPrefix := "gcr.io/k8s-prow/"
	boskosPrefix := "gcr.io/k8s-boskos/"

	prowPrefixStruct := Prefix{
		Prefix:               prowPrefix,
		RefConfigFile:        prowRefConfigFile,
		StagingRefConfigFile: prowStagingRefConfigFile,
	}
	boskosPrefixStruct := Prefix{
		Prefix:               boskosPrefix,
		RefConfigFile:        boskosRefConfigFile,
		StagingRefConfigFile: boskosStagingRefConfigFile,
	}

	fakeImageVersionParser := func(upstreamAddress, prefix string) (string, error) {
		switch upstreamAddress {
		case fakeUpstreamURLBase + "/" + prowRefConfigFile:
			return prowProdFakeVersion, nil
		case fakeUpstreamURLBase + "/" + prowStagingRefConfigFile:
			return prowStagingFakeVersion, nil
		case fakeUpstreamURLBase + "/" + boskosRefConfigFile:
			return boskosProdFakeVersion, nil
		case fakeUpstreamURLBase + "/" + boskosStagingRefConfigFile:
			return boskosStagingFakeVersion, nil
		default:
			return "", fmt.Errorf("unsupported upstream address %q for parsing the image version", upstreamAddress)
		}
	}
	cases := []struct {
		description         string
		upstreamVersionType string
		expectedResult      map[string]string
		expectError         bool
		prefixes            []Prefix
	}{
		{
			description:         "resolve image version with an invalid version type",
			upstreamVersionType: "whatever-version-type",
			expectError:         true,
			prefixes:            []Prefix{prowPrefixStruct, boskosPrefixStruct},
		},
		{
			description:         "correct versions map for production",
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []Prefix{prowPrefixStruct, boskosPrefixStruct},
			expectedResult:      map[string]string{prowPrefix: prowProdFakeVersion, boskosPrefix: boskosProdFakeVersion},
		},
		{
			description:         "correct versions map for staging",
			upstreamVersionType: upstreamStagingVersion,
			expectError:         false,
			prefixes:            []Prefix{prowPrefixStruct, boskosPrefixStruct},
			expectedResult:      map[string]string{prowPrefix: prowStagingFakeVersion, boskosPrefix: boskosStagingFakeVersion},
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			option := &Options{
				UpstreamURLBase: fakeUpstreamURLBase,
				Prefixes:        tc.prefixes,
			}
			versions, err := upstreamConfigVersions(tc.upstreamVersionType, option, fakeImageVersionParser)
			if tc.expectError && err == nil {
				t.Errorf("Expected to get an error but the result is nil")
				return
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected to not get an error but got one: %v", err)
				return
			}
			if err == nil && versions == nil {
				t.Error("Expected to get an versions but did not")
				return
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

func TestGetVersionsAndCheckConsistency(t *testing.T) {
	prowPrefix := Prefix{Prefix: "gcr.io/k8s-prow/", ConsistentImages: true}
	boskosPrefix := Prefix{Prefix: "gcr.io/k8s-boskos/", ConsistentImages: true}
	inconsistentPrefix := Prefix{Prefix: "inconsistent/", ConsistentImages: false}
	testCases := []struct {
		name             string
		images           map[string]string
		prefixes         []Prefix
		expectedVersions map[string][]string
		err              bool
	}{
		{
			name:             "two prefixes being bumped with consistent tags",
			prefixes:         []Prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "newtag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag1"}},
		},
		{
			name:     "two prefixes being bumped with inconsistent tags",
			prefixes: []Prefix{prowPrefix, boskosPrefix},
			images:   map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "tag1"},
			err:      true,
		},
		{
			name:             "two prefixes being bumped with no bumps",
			prefixes:         []Prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{},
			err:              false,
			expectedVersions: map[string][]string{},
		},
		{
			name:             "Prefix being bumped with inconsistent tags",
			prefixes:         []Prefix{inconsistentPrefix},
			images:           map[string]string{"inconsistent/test:tag1": "newtag1", "inconsistent/test2:tag2": "newtag2"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"inconsistent/test:tag1"}, "newtag2": {"inconsistent/test2:tag2"}},
		},
		{
			name:             "One of the image types wasn't bumped. Do not include in versions.",
			prefixes:         []Prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "newtag1", "gcr.io/k8s-boskos/nobumped:tag1": "tag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag1"}},
		},
		{
			name:             "Two of the images in one type wasn't bumped. Do not include in versions. Do not error",
			prefixes:         []Prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "newtag1", "gcr.io/k8s-boskos/nobumped:tag1": "tag1", "gcr.io/k8s-boskos/nobumped2:tag1": "tag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag1"}},
		},
		{
			name:             "prefix was not consistent before bump and now is",
			prefixes:         []Prefix{prowPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag2": "newtag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag2"}},
		},
		{
			name:             "prefix was not consistent before bump one was bumped ahead manually",
			prefixes:         []Prefix{prowPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:newtag1": "newtag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1"}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			versions, err := getVersionsAndCheckConsistency(tc.prefixes, tc.images)
			if tc.err && err == nil {
				t.Errorf("expected error but did not get one")
			}
			if !tc.err && err != nil {
				t.Errorf("expected no error, but got one: %v", err)
			}
			if diff := cmp.Diff(tc.expectedVersions, versions, cmpopts.SortSlices(func(x, y string) bool { return strings.Compare(x, y) > 0 })); diff != "" {
				t.Errorf("versions returned unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMakeCommitSummary(t *testing.T) {
	prowPrefix := Prefix{Name: "Prow", Prefix: "gcr.io/k8s-prow/", ConsistentImages: true}
	boskosPrefix := Prefix{Name: "Boskos", Prefix: "gcr.io/k8s-boskos/", ConsistentImages: true}
	inconsistentPrefix := Prefix{Name: "Inconsistent", Prefix: "gcr.io/inconsistent/", ConsistentImages: false}
	testCases := []struct {
		name           string
		prefixes       []Prefix
		versions       map[string][]string
		consistency    bool
		expectedResult string
	}{
		{
			name:           "Two prefixes, but only one bumped",
			prefixes:       []Prefix{prowPrefix, boskosPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}},
			expectedResult: "Update Prow to tag1",
		},
		{
			name:           "Two prefixes, both bumped",
			prefixes:       []Prefix{prowPrefix, boskosPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2",
		},
		{
			name:           "Empty versions",
			prefixes:       []Prefix{prowPrefix, boskosPrefix},
			versions:       map[string][]string{},
			expectedResult: "Update Prow, Boskos images as necessary",
		},
		{
			name:           "One bumped inconsistently",
			prefixes:       []Prefix{prowPrefix, boskosPrefix, inconsistentPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}, "tag3": {"gcr.io/inconsistent/test:tag3"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2 and Inconsistent as needed",
		},
		{
			name:           "inconsistent tag was not bumped, do not include in result",
			prefixes:       []Prefix{prowPrefix, boskosPrefix, inconsistentPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2",
		},
		{
			name:           "Two images bumped to same version",
			prefixes:       []Prefix{prowPrefix, boskosPrefix, inconsistentPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/inconsistent/test:tag3"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2 and Inconsistent as needed",
		},
		{
			name:           "only bump inconsistent",
			prefixes:       []Prefix{inconsistentPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/inconsistent/test:tag3"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}},
			expectedResult: "Update Inconsistent as needed",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := makeCommitSummary(tc.prefixes, tc.versions)
			if res != tc.expectedResult {
				t.Errorf("expected commit string to be %q, but was %q", tc.expectedResult, res)
			}
		})
	}
}

func TestGenerateSummary(t *testing.T) {
	beforeCommit := "2b1234567"
	afterCommit := "3a1234567"
	beforeDate := "20210128"
	afterDate := "20210129"
	beforeCommit2 := "1c1234567"
	afterCommit2 := "4f1234567"
	beforeDate2 := "20210125"
	afterDate2 := "20210126"
	unsummarizedOutHeader := `Multiple distinct Test changes:

Commits | Dates | Images
--- | --- | ---`

	unsummarizedOutLine := "github.com/test/repo/compare/%s...%s | %s&nbsp;&#x2192;&nbsp;%s | %s"

	sampleImages := map[string]string{
		fmt.Sprintf("gcr.io/bumped/bumpName:v%s-%s", beforeDate, beforeCommit):      fmt.Sprintf("v%s-%s", afterDate, afterCommit),
		fmt.Sprintf("gcr.io/variant/name:v%s-%s-first", beforeDate, beforeCommit):   fmt.Sprintf("v%s-%s", afterDate, afterCommit),
		fmt.Sprintf("gcr.io/variant/name:v%s-%s-second", beforeDate, beforeCommit):  fmt.Sprintf("v%s-%s", afterDate, afterCommit),
		fmt.Sprintf("gcr.io/inconsistent/first:v%s-%s", beforeDate2, beforeCommit2): fmt.Sprintf("v%s-%s", afterDate2, afterCommit2),
		fmt.Sprintf("gcr.io/inconsistent/second:v%s-%s", beforeDate, beforeCommit):  fmt.Sprintf("v%s-%s", afterDate, afterCommit),
	}
	testCases := []struct {
		testName  string
		name      string
		repo      string
		prefix    string
		summarize bool
		images    map[string]string
		expected  string
	}{
		{
			testName:  "Image not bumped unsummarized",
			name:      "Test",
			repo:      "repo",
			prefix:    "gcr.io/none",
			summarize: true,
			images:    sampleImages,
			expected:  "No Test changes.",
		},
		{
			testName:  "Image not bumped summarized",
			name:      "Test",
			repo:      "repo",
			prefix:    "gcr.io/none",
			summarize: true,
			images:    sampleImages,
			expected:  "No Test changes.",
		},
		{
			testName:  "Image bumped: summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/bumped",
			summarize: true,
			images:    sampleImages,
			expected:  fmt.Sprintf("Test changes: github.com/test/repo/compare/%s...%s (%s → %s)", beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate)),
		},
		{
			testName:  "Image bumped: not summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/bumped",
			summarize: false,
			images:    sampleImages,
			expected:  fmt.Sprintf("%s\n%s\n", unsummarizedOutHeader, fmt.Sprintf(unsummarizedOutLine, beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate), "bumpName")),
		},
		{
			testName:  "Image bumped: not summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/variant",
			summarize: false,
			images:    sampleImages,
			expected:  fmt.Sprintf("%s\n%s\n", unsummarizedOutHeader, fmt.Sprintf(unsummarizedOutLine, beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate), "name(first), name(second)")),
		},
		{
			testName:  "Image bumped, inconsistent: not summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/inconsistent",
			summarize: false,
			images:    sampleImages,
			expected:  fmt.Sprintf("%s\n%s\n%s\n", unsummarizedOutHeader, fmt.Sprintf(unsummarizedOutLine, beforeCommit2, afterCommit2, formatTagDate(beforeDate2), formatTagDate(afterDate2), "first"), fmt.Sprintf(unsummarizedOutLine, beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate), "second")),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expected, generateSummary(tc.name, tc.repo, tc.prefix, tc.summarize, tc.images)); diff != "" {
				t.Errorf("generateSummary returned unexpected value (-want +got):\n%s", diff)
			}

		})

	}
}

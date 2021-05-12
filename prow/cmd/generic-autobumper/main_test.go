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

package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/google/go-cmp/cmp"
)

func TestValidateOptions(t *testing.T) {
	emptyStr := ""
	whateverStr := "whatever"
	emptyArr := make([]string, 0)
	emptyPrefixes := make([]prefix, 0)
	latestPrefixes := []prefix{{
		Name:                 "test",
		Prefix:               "gcr.io/test/",
		RefConfigFile:        "",
		StagingRefConfigFile: "",
	}}
	upstreamPrefixes := []prefix{{
		Name:                 "test",
		Prefix:               "gcr.io/test/",
		RefConfigFile:        "ref",
		StagingRefConfigFile: "stagingRef",
	}}
	upstreamVersion := "upstream"
	stagingVersion := "upstream-staging"
	cases := []struct {
		name                string
		targetVersion       *string
		includeConfigPaths  *[]string
		prefixes            *[]prefix
		upstreamURLBase     *string
		err                 bool
		upstreamBaseChanged bool
	}{
		{
			name: "Everything correct",
			err:  false,
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
			defaultOption := &options{
				UpstreamURLBase:     "whatever-URLBase",
				Prefixes:            latestPrefixes,
				TargetVersion:       latestVersion,
				IncludedConfigPaths: []string{"whatever-config-path1", "whatever-config-path2"},
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
	tmpDir, err := os.MkdirTemp(".", "test-update-references_")
	if err != nil {
		t.Fatalf("Failed created tmp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed cleanup tmp dir %q: %v", tmpDir, err)
		}
	})
	for dir, fps := range map[string][]string{
		"testdata/dir/subdir1": {"test1-1.yaml", "test1-2.yaml"},
		"testdata/dir/subdir2": {"test2-1.yaml"},
		"testdata/dir/subdir3": {"test3-1.yaml"},
		"testdata/dir":         {"extra-file"},
	} {
		if err := os.MkdirAll(path.Join(tmpDir, dir), 0755); err != nil {
			t.Fatalf("Faile creating dir %q: %v", dir, err)
		}
		for _, f := range fps {
			if _, err := os.Create(path.Join(tmpDir, dir, f)); err != nil {
				t.Fatalf("Faile creating file %q: %v", f, err)
			}
		}
	}

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
			description:   "update the images to the latest version",
			targetVersion: latestVersion,
			includeConfigPaths: []string{
				path.Join(tmpDir, "testdata/dir/subdir1"),
				path.Join(tmpDir, "testdata/dir/subdir2"),
			},
			expectedRes: map[string]string{
				path.Join(tmpDir, "testdata/dir/subdir1/test1-1.yaml"): "fake-latest",
				path.Join(tmpDir, "testdata/dir/subdir1/test1-2.yaml"): "fake-latest",
				path.Join(tmpDir, "testdata/dir/subdir2/test2-1.yaml"): "fake-latest",
			},
			expectError: false,
		},
		{
			description:   "update the images to a specific version",
			targetVersion: "v20200101-livebull",
			includeConfigPaths: []string{
				path.Join(tmpDir, "testdata/dir/subdir2"),
			},
			expectedRes: map[string]string{
				path.Join(tmpDir, "testdata/dir/subdir2/test2-1.yaml"): "v20200101-livebull",
			},
			expectError: false,
		},
		{
			description:   "by default only yaml files will be updated",
			targetVersion: latestVersion,
			includeConfigPaths: []string{
				path.Join(tmpDir, "testdata/dir/subdir3"),
			},
			expectedRes: map[string]string{
				path.Join(tmpDir, "testdata/dir/subdir3/test3-1.yaml"): "fake-latest",
			},
			expectError: false,
		},
		{
			description:   "files under the excluded paths will not be updated",
			targetVersion: latestVersion,
			includeConfigPaths: []string{
				path.Join(tmpDir, "testdata/dir"),
			},
			excludeConfigPaths: []string{
				path.Join(tmpDir, "testdata/dir/subdir1"),
				path.Join(tmpDir, "testdata/dir/subdir2"),
			},
			expectedRes: map[string]string{
				path.Join(tmpDir, "testdata/dir/subdir3/test3-1.yaml"): "fake-latest",
			},
			expectError: false,
		},
		{
			description:   "non YAML files could be configured by specifying extraFiles",
			targetVersion: latestVersion,
			includeConfigPaths: []string{
				path.Join(tmpDir, "testdata/dir/subdir3"),
			},
			extraFiles: []string{
				path.Join(tmpDir, "testdata/dir/extra-file"),
				path.Join(tmpDir, "testdata/dir/subdir3/test3-2"),
			},
			expectedRes: map[string]string{
				path.Join(tmpDir, "testdata/dir/subdir3/test3-1.yaml"): "fake-latest",
				path.Join(tmpDir, "testdata/dir/extra-file"):           "fake-latest",
				path.Join(tmpDir, "testdata/dir/subdir3/test3-2"):      "fake-latest",
			},
			expectError: false,
		},
		{
			description:   "updating non-existed files will return an error",
			targetVersion: latestVersion,
			includeConfigPaths: []string{
				path.Join(tmpDir, "testdata/dir/whatever-subdir"),
			},
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			option := &options{
				TargetVersion:       tc.targetVersion,
				IncludedConfigPaths: tc.includeConfigPaths,
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
	const (
		prowProdFakeVersion        = "v-prow-prod-version"
		prowStagingFakeVersion     = "v-prow-staging-version"
		boskosProdFakeVersion      = "v-boskos-prod-version"
		boskosStagingFakeVersion   = "v-boskos-staging-version"
		prowRefConfigFile          = "prow-prod"
		boskosRefConfigFile        = "boskos-prod"
		prowStagingRefConfigFile   = "prow-staging"
		boskosStagingRefConfigFile = "boskos-staging"
		fakeUpstreamURLBase        = "test.com"
		prowPrefix                 = "gcr.io/k8s-prow/"
		boskosPrefix               = "gcr.io/k8s-boskos/"
		doesNotExistPrefix         = "gcr.io/dne"
		doesNotExist               = "DNE"
	)
	prowPrefixStruct := prefix{
		Prefix:               prowPrefix,
		RefConfigFile:        prowRefConfigFile,
		StagingRefConfigFile: prowStagingRefConfigFile,
	}
	boskosPrefixStruct := prefix{
		Prefix:               boskosPrefix,
		RefConfigFile:        boskosRefConfigFile,
		StagingRefConfigFile: boskosStagingRefConfigFile,
	}
	// prefix used to test when a tag does not exist. This is used to have parser return a tag that will make TagExists return false
	tagDoesNotExistPrefix := prefix{
		Prefix:               doesNotExistPrefix,
		RefConfigFile:        doesNotExist,
		StagingRefConfigFile: doesNotExist,
	}

	cases := []struct {
		description         string
		parser              func(string, string) (string, error)
		upstreamVersionType string
		imageHost           string
		imageName           string
		currentTag          string
		expectedTargetTag   string
		expectError         bool
		resolverError       bool
		prefixes            []prefix
	}{
		{
			description: "resolve image version with an invalid version type",
			parser: func(upAddr, pref string) (string, error) {
				switch strings.TrimPrefix(upAddr, fakeUpstreamURLBase+"/") {
				case prowRefConfigFile:
					return prowProdFakeVersion, nil
				case boskosRefConfigFile:
					return boskosProdFakeVersion, nil
				default:
					return "", errors.New("not supported")
				}
			},
			upstreamVersionType: "whatever-version-type",
			expectError:         true,
			prefixes:            []prefix{prowPrefixStruct, boskosPrefixStruct},
		},
		{
			description: "resolve image with two prefixes possible and upstreamVersion",
			parser: func(upAddr, pref string) (string, error) {
				switch strings.TrimPrefix(upAddr, fakeUpstreamURLBase+"/") {
				case prowRefConfigFile:
					return prowProdFakeVersion, nil
				case boskosRefConfigFile:
					return boskosProdFakeVersion, nil
				default:
					return "", errors.New("not supported")
				}
			},
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []prefix{prowPrefixStruct, boskosPrefixStruct},
			imageHost:           prowPrefix,
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   prowProdFakeVersion,
		},
		{
			description: "resolve image with two prefixes possible and staging version",
			parser: func(upAddr, pref string) (string, error) {
				switch strings.TrimPrefix(upAddr, fakeUpstreamURLBase+"/") {
				case prowStagingRefConfigFile:
					return prowStagingFakeVersion, nil
				case boskosStagingRefConfigFile:
					return boskosStagingFakeVersion, nil
				default:
					return "", errors.New("not supported")
				}
			},
			upstreamVersionType: upstreamStagingVersion,
			expectError:         false,
			prefixes:            []prefix{prowPrefixStruct, boskosPrefixStruct},
			imageHost:           boskosPrefix,
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   boskosStagingFakeVersion,
		},
		{
			description: "resolve image when unknown prefix",
			parser: func(upAddr, pref string) (string, error) {
				switch strings.TrimPrefix(upAddr, fakeUpstreamURLBase+"/") {
				case boskosRefConfigFile:
					return boskosProdFakeVersion, nil
				default:
					return "", errors.New("not supported")
				}
			},
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []prefix{boskosPrefixStruct},
			imageHost:           prowPrefix,
			currentTag:          "whatever-current-tag",
			expectedTargetTag:   "whatever-current-tag",
		},
		{
			description: "tag does not exist",
			parser: func(upAddr, pref string) (string, error) {
				switch strings.TrimPrefix(upAddr, fakeUpstreamURLBase+"/") {
				case doesNotExist:
					return doesNotExist, nil
				default:
					return "", errors.New("not supported")
				}
			},
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []prefix{tagDoesNotExistPrefix},
			imageHost:           doesNotExistPrefix,
			currentTag:          "doesNotExist",
			expectedTargetTag:   "",
			resolverError:       true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			option := &options{
				UpstreamURLBase: fakeUpstreamURLBase,
				Prefixes:        tc.prefixes,
			}
			cli := &fakeImageBumperCli{replacements: map[string]string{}, tagCache: map[string]string{}}
			resolver, err := upstreamImageVersionResolver(option, tc.upstreamVersionType, tc.parser, cli)
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

	prowPrefixStruct := prefix{
		Prefix:               prowPrefix,
		RefConfigFile:        prowRefConfigFile,
		StagingRefConfigFile: prowStagingRefConfigFile,
	}
	boskosPrefixStruct := prefix{
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
		prefixes            []prefix
	}{
		{
			description:         "resolve image version with an invalid version type",
			upstreamVersionType: "whatever-version-type",
			expectError:         true,
			prefixes:            []prefix{prowPrefixStruct, boskosPrefixStruct},
		},
		{
			description:         "correct versions map for production",
			upstreamVersionType: upstreamVersion,
			expectError:         false,
			prefixes:            []prefix{prowPrefixStruct, boskosPrefixStruct},
			expectedResult:      map[string]string{prowPrefix: prowProdFakeVersion, boskosPrefix: boskosProdFakeVersion},
		},
		{
			description:         "correct versions map for staging",
			upstreamVersionType: upstreamStagingVersion,
			expectError:         false,
			prefixes:            []prefix{prowPrefixStruct, boskosPrefixStruct},
			expectedResult:      map[string]string{prowPrefix: prowStagingFakeVersion, boskosPrefix: boskosStagingFakeVersion},
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			option := &options{
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

func TestGetVersionsAndCheckConsistency(t *testing.T) {
	prowPrefix := prefix{Prefix: "gcr.io/k8s-prow/", ConsistentImages: true}
	boskosPrefix := prefix{Prefix: "gcr.io/k8s-boskos/", ConsistentImages: true}
	inconsistentPrefix := prefix{Prefix: "inconsistent/", ConsistentImages: false}
	testCases := []struct {
		name             string
		images           map[string]string
		prefixes         []prefix
		expectedVersions map[string][]string
		err              bool
	}{
		{
			name:             "two prefixes being bumped with consistent tags",
			prefixes:         []prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "newtag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag1"}},
		},
		{
			name:     "two prefixes being bumped with inconsistent tags",
			prefixes: []prefix{prowPrefix, boskosPrefix},
			images:   map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "tag1"},
			err:      true,
		},
		{
			name:             "two prefixes being bumped with no bumps",
			prefixes:         []prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{},
			err:              false,
			expectedVersions: map[string][]string{},
		},
		{
			name:             "Prefix being bumped with inconsistent tags",
			prefixes:         []prefix{inconsistentPrefix},
			images:           map[string]string{"inconsistent/test:tag1": "newtag1", "inconsistent/test2:tag2": "newtag2"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"inconsistent/test:tag1"}, "newtag2": {"inconsistent/test2:tag2"}},
		},
		{
			name:             "One of the image types wasn't bumped. Do not include in versions.",
			prefixes:         []prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "newtag1", "gcr.io/k8s-boskos/nobumped:tag1": "tag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag1"}},
		},
		{
			name:             "Two of the images in one type wasn't bumped. Do not include in versions. Do not error",
			prefixes:         []prefix{prowPrefix, boskosPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag1": "newtag1", "gcr.io/k8s-boskos/nobumped:tag1": "tag1", "gcr.io/k8s-boskos/nobumped2:tag1": "tag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag1"}},
		},
		{
			name:             "prefix was not consistent before bump and now is",
			prefixes:         []prefix{prowPrefix},
			images:           map[string]string{"gcr.io/k8s-prow/test:tag1": "newtag1", "gcr.io/k8s-prow/test2:tag2": "newtag1"},
			err:              false,
			expectedVersions: map[string][]string{"newtag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/k8s-prow/test2:tag2"}},
		},
		{
			name:             "prefix was not consistent before bump one was bumped ahead manually",
			prefixes:         []prefix{prowPrefix},
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
	prowPrefix := prefix{Name: "Prow", Prefix: "gcr.io/k8s-prow/", ConsistentImages: true}
	boskosPrefix := prefix{Name: "Boskos", Prefix: "gcr.io/k8s-boskos/", ConsistentImages: true}
	inconsistentPrefix := prefix{Name: "Inconsistent", Prefix: "gcr.io/inconsistent/", ConsistentImages: false}
	testCases := []struct {
		name           string
		prefixes       []prefix
		versions       map[string][]string
		consistency    bool
		expectedResult string
	}{
		{
			name:           "Two prefixes, but only one bumped",
			prefixes:       []prefix{prowPrefix, boskosPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}},
			expectedResult: "Update Prow to tag1",
		},
		{
			name:           "Two prefixes, both bumped",
			prefixes:       []prefix{prowPrefix, boskosPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2",
		},
		{
			name:           "Empty versions",
			prefixes:       []prefix{prowPrefix, boskosPrefix},
			versions:       map[string][]string{},
			expectedResult: "Update Prow, Boskos images as necessary",
		},
		{
			name:           "One bumped inconsistently",
			prefixes:       []prefix{prowPrefix, boskosPrefix, inconsistentPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}, "tag3": {"gcr.io/inconsistent/test:tag3"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2 and Inconsistent as needed",
		},
		{
			name:           "inconsistent tag was not bumped, do not include in result",
			prefixes:       []prefix{prowPrefix, boskosPrefix, inconsistentPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2",
		},
		{
			name:           "Two images bumped to same version",
			prefixes:       []prefix{prowPrefix, boskosPrefix, inconsistentPrefix},
			versions:       map[string][]string{"tag1": {"gcr.io/k8s-prow/test:tag1", "gcr.io/inconsistent/test:tag3"}, "tag2": {"gcr.io/k8s-boskos/test:tag2"}},
			expectedResult: "Update Prow to tag1, Boskos to tag2 and Inconsistent as needed",
		},
		{
			name:           "only bump inconsistent",
			prefixes:       []prefix{inconsistentPrefix},
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
	unsummarizedOutHeader := `Multiple distinct %s changes:

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
			expected:  "No gcr.io/none changes.",
		},
		{
			testName:  "Image not bumped summarized",
			name:      "Test",
			repo:      "repo",
			prefix:    "gcr.io/none",
			summarize: true,
			images:    sampleImages,
			expected:  "No gcr.io/none changes.",
		},
		{
			testName:  "Image bumped: summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/bumped",
			summarize: true,
			images:    sampleImages,
			expected:  fmt.Sprintf("gcr.io/bumped changes: github.com/test/repo/compare/%s...%s (%s → %s)", beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate)),
		},
		{
			testName:  "Image bumped: not summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/bumped",
			summarize: false,
			images:    sampleImages,
			expected:  fmt.Sprintf("%s\n%s\n", fmt.Sprintf(unsummarizedOutHeader, "gcr.io/bumped"), fmt.Sprintf(unsummarizedOutLine, beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate), "bumpName")),
		},
		{
			testName:  "Image bumped: not summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/variant",
			summarize: false,
			images:    sampleImages,
			expected:  fmt.Sprintf("%s\n%s\n", fmt.Sprintf(unsummarizedOutHeader, "gcr.io/variant"), fmt.Sprintf(unsummarizedOutLine, beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate), "name(first), name(second)")),
		},
		{
			testName:  "Image bumped, inconsistent: not summarized",
			name:      "Test",
			repo:      "github.com/test/repo",
			prefix:    "gcr.io/inconsistent",
			summarize: false,
			images:    sampleImages,
			expected:  fmt.Sprintf("%s\n%s\n%s\n", fmt.Sprintf(unsummarizedOutHeader, "gcr.io/inconsistent"), fmt.Sprintf(unsummarizedOutLine, beforeCommit2, afterCommit2, formatTagDate(beforeDate2), formatTagDate(afterDate2), "first"), fmt.Sprintf(unsummarizedOutLine, beforeCommit, afterCommit, formatTagDate(beforeDate), formatTagDate(afterDate), "second")),
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.testName, func(t *testing.T) {
			want, got := tc.expected, generateSummary(tc.name, tc.repo, tc.prefix, tc.summarize, tc.images)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("generateSummary returned unexpected value (-want +got):\n%s", diff)
			}
		})

	}
}

/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseGciExtractOption(t *testing.T) {
	cases := []struct {
		option          string
		expectFamily    string
		expectParamsMap map[string]string
	}{
		{
			option:       "gci-canary",
			expectFamily: "gci-canary",
			expectParamsMap: map[string]string{
				"project":        "container-vm-image-staging",
				"k8s-map-bucket": "container-vm-image-staging",
			},
		},
		{
			option:       "gci-canary?project=test-project",
			expectFamily: "gci-canary",
			expectParamsMap: map[string]string{
				"project":        "test-project",
				"k8s-map-bucket": "container-vm-image-staging",
			},
		},
		{
			option:       "gci-canary?k8s-map-bucket=test-bucket",
			expectFamily: "gci-canary",
			expectParamsMap: map[string]string{
				"project":        "container-vm-image-staging",
				"k8s-map-bucket": "test-bucket",
			},
		},
		{
			option:       "gci-canary?project=test-project:k8s-map-bucket=test-bucket",
			expectFamily: "gci-canary",
			expectParamsMap: map[string]string{
				"project":        "test-project",
				"k8s-map-bucket": "test-bucket",
			},
		},
	}

	var gotFamily string
	var gotParamsMap map[string]string

	for _, tc := range cases {
		gotFamily, gotParamsMap = parseGciExtractOption(tc.option)
		if gotFamily != tc.expectFamily || !reflect.DeepEqual(gotParamsMap, tc.expectParamsMap) {
			t.Errorf("got parseGciExtractOption(%q) = %q, %q; want %q, %q", tc.option, gotFamily, gotParamsMap, tc.expectFamily, tc.expectParamsMap)
		}
	}
}

func TestGetKube(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		success bool
	}{
		{
			name:    "can succeed",
			script:  "true",
			success: true,
		},
		{
			name:    "can fail",
			script:  "exit 1",
			success: false,
		},
		{
			name:    "can successfully retry",
			script:  "([[ -e ran ]] && true) || (touch ran && exit 1)",
			success: true,
		},
	}

	if !terminate.Stop() {
		<-terminate.C
	}
	if !interrupt.Stop() {
		<-interrupt.C
	}

	oldSleep := sleep
	defer func() { sleep = oldSleep }()
	sleep = func(d time.Duration) {}

	if o, err := os.Getwd(); err != nil {
		t.Fatal(err)
	} else {
		defer os.Chdir(o)
	}
	if d, err := ioutil.TempDir("", "extract"); err != nil {
		t.Fatal(err)
	} else if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		bytes := []byte(fmt.Sprintf("#!/bin/bash\necho hello\n%s\nmkdir -p ./kubernetes/cluster && touch ./kubernetes/cluster/get-kube-binaries.sh", tc.script))
		if err := ioutil.WriteFile("./get-kube.sh", bytes, 0700); err != nil {
			t.Fatal(err)
		}
		err := getKube("url", "version", false)
		if tc.success && err != nil {
			t.Errorf("%s did not succeed: %s", tc.name, err)
		}
		if !tc.success && err == nil {
			t.Errorf("%s unexpectedly succeeded", tc.name)
		}
	}
}

func TestExtractStrategies(t *testing.T) {
	cases := []struct {
		option        string
		expectURL     string
		expectVersion string
	}{
		{
			"bazel/v1.8.0-alpha.2.899+2c624e590f5670",
			"",
			"bazel/v1.8.0-alpha.2.899+2c624e590f5670",
		},
		{
			"bazel/49747/master:b341939d6d3666b119028400a4311cc66da9a542,49747:c4656c3d029e47d03b3d7d9915d79cab72a80852",
			"",
			"bazel/49747/master:b341939d6d3666b119028400a4311cc66da9a542,49747:c4656c3d029e47d03b3d7d9915d79cab72a80852",
		},
		{
			"gs://kubernetes-release-dev/bazel/v1.8.0-alpha.3.389+eab2f8f6c19fcb",
			"https://storage.googleapis.com/kubernetes-release-dev/bazel",
			"v1.8.0-alpha.3.389+eab2f8f6c19fcb",
		},
		{
			"v1.8.0-alpha.1",
			"https://storage.googleapis.com/k8s-release/release",
			"v1.8.0-alpha.1",
		},
		{
			"v1.8.0-alpha.2.899+2c624e590f5670",
			"https://storage.googleapis.com/k8s-release-dev/ci",
			"v1.8.0-alpha.2.899+2c624e590f5670",
		},
		{
			"v1.8.0-gke.0",
			"https://storage.googleapis.com/gke-release-staging/kubernetes/release",
			"v1.8.0-gke.0",
		},
		{
			"ci/latest",
			"https://storage.googleapis.com/k8s-release-dev/ci",
			"v1.2.3+abcde",
		},
		{
			"ci/latest-fast",
			"https://storage.googleapis.com/k8s-release-dev/ci/fast",
			"v1.2.3+abcde",
		},
		{
			"ci/gke-staging-latest",
			"https://storage.googleapis.com/gke-release-staging/kubernetes/release",
			"v1.2.3+abcde",
		},
		{
			"ci/gke-latest-1.13",
			"https://storage.googleapis.com/gke-release-staging/kubernetes/release",
			"v1.2.3+abcde",
		},
		{
			"ci/gke-latest-1.13.0",
			"https://storage.googleapis.com/gke-release-staging/kubernetes/release",
			"v1.2.3+abcde",
		},
		{
			"ci/gke-latest-1.13.0-gke",
			"https://storage.googleapis.com/gke-release-staging/kubernetes/release",
			"v1.2.3+abcde",
		},
		{
			"ci/gke-latest-1.13.10-gke",
			"https://storage.googleapis.com/gke-release-staging/kubernetes/release",
			"v1.2.3+abcde",
		},
		{
			"ci/gke-channel-rapid",
			"https://storage.googleapis.com/gke-release-staging/kubernetes/release",
			"v1.2.3+abcde",
		},
		{
			"gs://whatever-bucket/ci/latest.txt",
			"https://storage.googleapis.com/whatever-bucket/ci",
			"v1.2.3+abcde",
		},
	}

	var gotURL string
	var gotVersion string

	// getKube is tested independently, so mock it out here so we can test
	// that different extraction strategies call getKube with the correct
	// arguments.
	oldGetKube := getKube
	defer func() { getKube = oldGetKube }()
	getKube = func(url, version string, _ bool) error {
		gotURL = url
		gotVersion = version
		// This is needed or else Extract() will think that getKube failed.
		os.Mkdir("kubernetes", 0775)
		return nil
	}

	oldCat := gsutilCat
	defer func() { gsutilCat = oldCat }()
	gsutilCat = func(url string) ([]byte, error) {
		if path.Ext(url) != ".txt" {
			return []byte{}, fmt.Errorf("url %s must end with .txt", url)
		}
		if !strings.HasPrefix(path.Dir(url), "gs:/") {
			return []byte{}, fmt.Errorf("url %s must starts with gs:/", path.Dir(url))
		}

		return []byte("v1.2.3+abcde"), nil
	}

	oldHTTPCat := httpCat
	defer func() { httpCat = oldHTTPCat }()
	httpCat = func(url string) ([]byte, error) {
		if path.Ext(url) != ".txt" {
			return []byte{}, fmt.Errorf("url %s must end with .txt", url)
		}
		if !strings.HasPrefix(url, "https://") {
			return []byte{}, fmt.Errorf("url %s must starts with https://", url)
		}

		return []byte("v1.2.3+abcde"), nil
	}

	ciBucket := "k8s-release-dev"
	releaseBucket := "k8s-release"

	for _, tc := range cases {
		if d, err := ioutil.TempDir("", "extract"); err != nil {
			t.Fatal(err)
		} else if err := os.Chdir(d); err != nil {
			t.Fatal(err)
		}

		var es extractStrategies
		if err := es.Set(tc.option); err != nil {
			t.Errorf("extractStrategy.Set(%q) returned err: %q", tc.option, err)
		}
		if err := es.Extract("", "", "", ciBucket, releaseBucket, false); err != nil {
			t.Errorf("extractStrategy(%q).Extract() returned err: %q", tc.option, err)
		}

		if gotURL != tc.expectURL || gotVersion != tc.expectVersion {
			t.Errorf("extractStrategy(%q).Extract() wanted getKube(%q, %q), got getKube(%q, %q)", tc.option, tc.expectURL, tc.expectVersion, gotURL, gotVersion)
		}
	}
}

func TestGciExtractStrategy(t *testing.T) {
	cases := []struct {
		option                 string
		expectURL              string
		expectVersion          string
		expectFamily           string
		expectProject          string
		expectVersionMapBucket string
	}{
		{
			"gci/gci-canary",
			"https://storage.googleapis.com/k8s-release/release",
			"v1.2.3+abcde",
			"gci-canary",
			"container-vm-image-staging",
			"gs://container-vm-image-staging/k8s-version-map/test-image",
		},
		{
			"gci/gci-canary?project=test-project:k8s-map-bucket=test-bucket",
			"https://storage.googleapis.com/k8s-release/release",
			"v1.2.3+abcde",
			"gci-canary",
			"test-project",
			"gs://test-bucket/k8s-version-map/test-image",
		},
		{
			"gci/gci-canary?project=test-project/latest",
			"https://storage.googleapis.com/k8s-release-dev/ci",
			"1.2.3+abcde",
			"gci-canary",
			"test-project",
			"gs://k8s-release-dev/ci/latest.txt",
		},
	}

	var gotURL string
	var gotVersion string
	var gotFamily string
	var gotProject string
	var gotVersionMapBucket string

	// getKube is tested independently, so mock it out here so we can test
	// that different extraction strategies call getKube with the correct
	// arguments.
	oldGetKube := getKube
	defer func() { getKube = oldGetKube }()
	getKube = func(url, version string, _ bool) error {
		gotURL = url
		gotVersion = version
		// This is needed or else Extract() will think that getKube failed.
		os.Mkdir("kubernetes", 0775)
		return nil
	}

	oldCat := gsutilCat
	defer func() { gsutilCat = oldCat }()
	gsutilCat = func(url string) ([]byte, error) {
		if !strings.HasPrefix(path.Dir(url), "gs:/") {
			return []byte{}, fmt.Errorf("url %s must start with gs:/", path.Dir(url))
		}
		gotVersionMapBucket = url
		return []byte("1.2.3+abcde"), nil
	}

	oldGcloudGetImageName := gcloudGetImageName
	defer func() { gcloudGetImageName = oldGcloudGetImageName }()
	gcloudGetImageName = func(family string, project string) ([]byte, error) {
		gotFamily = family
		gotProject = project
		return []byte("test-image"), nil
	}

	ciBucket := "k8s-release-dev"
	releaseBucket := "k8s-release"

	for _, tc := range cases {
		if d, err := ioutil.TempDir("", "extract"); err != nil {
			t.Fatal(err)
		} else if err := os.Chdir(d); err != nil {
			t.Fatal(err)
		}

		var es extractStrategies
		if err := es.Set(tc.option); err != nil {
			t.Errorf("extractStrategy.Set(%q) returned err: %q", tc.option, err)
		}
		if err := es.Extract("", "", "", ciBucket, releaseBucket, false); err != nil {
			t.Errorf("extractStrategy(%q).Extract() returned err: %q", tc.option, err)
		}

		if gotFamily != tc.expectFamily || gotProject != tc.expectProject {
			t.Errorf("extractStrategies(%q).Extract() wanted setupGciVars(%q, %q), got setupGciVars(%q, %q)", tc.option, tc.expectFamily, tc.expectProject, gotFamily, gotProject)
		}
		if gotVersionMapBucket != tc.expectVersionMapBucket {
			t.Errorf("extractStrategies(%q).Extract() wanted gsutilCat(%q), got gsutilCat(%q)", tc.option, tc.expectVersionMapBucket, gotVersionMapBucket)
		}
		if gotURL != tc.expectURL || gotVersion != tc.expectVersion {
			t.Errorf("extractStrategy(%q).Extract() wanted getKube(%q, %q), got getKube(%q, %q)", tc.option, tc.expectURL, tc.expectVersion, gotURL, gotVersion)
		}
	}
}

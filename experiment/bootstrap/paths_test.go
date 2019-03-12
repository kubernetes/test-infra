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
	"path/filepath"
	"reflect"
	"testing"
)

func TestCIPaths(t *testing.T) {
	testCases := []struct {
		Name     string
		Base     string
		Job      string
		Build    string
		Expected *Paths
	}{
		{
			Name:  "normal",
			Base:  "/some/foo/base",
			Job:   "some-foo-job",
			Build: "1337",
			Expected: &Paths{
				Artifacts:   filepath.Join("/some/foo/base", "some-foo-job", "1337", "artifacts"),
				BuildLog:    filepath.Join("/some/foo/base", "some-foo-job", "1337", "build-log.txt"),
				Finished:    filepath.Join("/some/foo/base", "some-foo-job", "1337", "finished.json"),
				Latest:      filepath.Join("/some/foo/base", "some-foo-job", "latest-build.txt"),
				ResultCache: filepath.Join("/some/foo/base", "some-foo-job", "jobResultsCache.json"),
				Started:     filepath.Join("/some/foo/base", "some-foo-job", "1337", "started.json"),
			},
		},
	}
	for _, test := range testCases {
		res := CIPaths(test.Base, test.Job, test.Build)
		if !reflect.DeepEqual(res, test.Expected) {
			t.Errorf("Paths did not match expected for test: %#v", test.Name)
			t.Errorf("%#v", res)
			t.Errorf("%#v", test.Expected)
		}
	}
}

func TestPRPaths(t *testing.T) {
	// create some Repos values for use in the test cases below
	reposEmtpy := Repos{}
	reposK8sIO, err := ParseRepos([]string{"k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3,2001:03a564a5309ea84065fb203f628b50c382b65a50"})
	if err != nil {
		t.Errorf("got unexpected error parsing test repos: %v", err)
	}
	reposK8sIOTestInfra, err := ParseRepos([]string{"k8s.io/test-infra=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3"})
	if err != nil {
		t.Errorf("got unexpected error parsing test repos: %v", err)
	}
	reposKubernetes, err := ParseRepos([]string{"kubernetes/test-infra=master:0efa7e1b,2001:8b376c6c"})
	if err != nil {
		t.Errorf("got unexpected error parsing test repos: %v", err)
	}
	reposGithub, err := ParseRepos([]string{"github.com/foo/bar=master:0efa7e1b,2001:8b376c6c"})
	if err != nil {
		t.Errorf("got unexpected error parsing test repos: %v", err)
	}
	reposOther, err := ParseRepos([]string{"example.com/foo/bar=master:0efa7e1b,2001:8b376c6c"})
	if err != nil {
		t.Errorf("got unexpected error parsing test repos: %v", err)
	}
	// assert some known expected values and the expected failure for len(repos) == 0
	testCases := []struct {
		Name      string
		Base      string
		Repos     Repos
		Job       string
		Build     string
		Expected  *Paths
		ExpectErr bool
	}{
		{
			Name:  "normal-k8s.io/kubernetes",
			Base:  "/base",
			Job:   "some-job",
			Repos: reposK8sIO,
			Build: "1337",
			Expected: &Paths{
				Artifacts:     filepath.Join("/base", "pull", "batch", "some-job", "1337", "artifacts"),
				BuildLog:      filepath.Join("/base", "pull", "batch", "some-job", "1337", "build-log.txt"),
				PRPath:        filepath.Join("/base", "pull", "batch", "some-job", "1337"),
				PRBuildLink:   filepath.Join("/base", "directory", "some-job", "1337.txt"),
				PRLatest:      filepath.Join("/base", "pull", "batch", "some-job", "latest-build.txt"),
				PRResultCache: filepath.Join("/base", "pull", "batch", "some-job", "jobResultsCache.json"),
				ResultCache:   filepath.Join("/base", "directory", "some-job", "jobResultsCache.json"),
				Started:       filepath.Join("/base", "pull", "batch", "some-job", "1337", "started.json"),
				Finished:      filepath.Join("/base", "pull", "batch", "some-job", "1337", "finished.json"),
				Latest:        filepath.Join("/base", "directory", "some-job", "latest-build.txt"),
			},
			ExpectErr: false,
		},
		{
			Name:  "normal-k8s.io/test-infra",
			Base:  "/base",
			Job:   "some-job",
			Repos: reposK8sIOTestInfra,
			Build: "1337",
			Expected: &Paths{
				Artifacts:     filepath.Join("/base", "pull", "test-infra", "52057", "some-job", "1337", "artifacts"),
				BuildLog:      filepath.Join("/base", "pull", "test-infra", "52057", "some-job", "1337", "build-log.txt"),
				PRPath:        filepath.Join("/base", "pull", "test-infra", "52057", "some-job", "1337"),
				PRBuildLink:   filepath.Join("/base", "directory", "some-job", "1337.txt"),
				PRLatest:      filepath.Join("/base", "pull", "test-infra", "52057", "some-job", "latest-build.txt"),
				PRResultCache: filepath.Join("/base", "pull", "test-infra", "52057", "some-job", "jobResultsCache.json"),
				ResultCache:   filepath.Join("/base", "directory", "some-job", "jobResultsCache.json"),
				Started:       filepath.Join("/base", "pull", "test-infra", "52057", "some-job", "1337", "started.json"),
				Finished:      filepath.Join("/base", "pull", "test-infra", "52057", "some-job", "1337", "finished.json"),
				Latest:        filepath.Join("/base", "directory", "some-job", "latest-build.txt"),
			},
			ExpectErr: false,
		},
		{
			Name:  "normal-kubernetes/test-infra",
			Base:  "/base",
			Job:   "some-job",
			Repos: reposKubernetes,
			Build: "1337",
			Expected: &Paths{
				Artifacts:     filepath.Join("/base", "pull", "test-infra", "2001", "some-job", "1337", "artifacts"),
				BuildLog:      filepath.Join("/base", "pull", "test-infra", "2001", "some-job", "1337", "build-log.txt"),
				PRPath:        filepath.Join("/base", "pull", "test-infra", "2001", "some-job", "1337"),
				PRBuildLink:   filepath.Join("/base", "directory", "some-job", "1337.txt"),
				PRLatest:      filepath.Join("/base", "pull", "test-infra", "2001", "some-job", "latest-build.txt"),
				PRResultCache: filepath.Join("/base", "pull", "test-infra", "2001", "some-job", "jobResultsCache.json"),
				ResultCache:   filepath.Join("/base", "directory", "some-job", "jobResultsCache.json"),
				Started:       filepath.Join("/base", "pull", "test-infra", "2001", "some-job", "1337", "started.json"),
				Finished:      filepath.Join("/base", "pull", "test-infra", "2001", "some-job", "1337", "finished.json"),
				Latest:        filepath.Join("/base", "directory", "some-job", "latest-build.txt"),
			},
			ExpectErr: false,
		},
		{
			Name:  "normal-github",
			Base:  "/base",
			Job:   "some-job",
			Repos: reposGithub,
			Build: "1337",
			Expected: &Paths{
				Artifacts:     filepath.Join("/base", "pull", "foo_bar", "2001", "some-job", "1337", "artifacts"),
				BuildLog:      filepath.Join("/base", "pull", "foo_bar", "2001", "some-job", "1337", "build-log.txt"),
				PRPath:        filepath.Join("/base", "pull", "foo_bar", "2001", "some-job", "1337"),
				PRBuildLink:   filepath.Join("/base", "directory", "some-job", "1337.txt"),
				PRLatest:      filepath.Join("/base", "pull", "foo_bar", "2001", "some-job", "latest-build.txt"),
				PRResultCache: filepath.Join("/base", "pull", "foo_bar", "2001", "some-job", "jobResultsCache.json"),
				ResultCache:   filepath.Join("/base", "directory", "some-job", "jobResultsCache.json"),
				Started:       filepath.Join("/base", "pull", "foo_bar", "2001", "some-job", "1337", "started.json"),
				Finished:      filepath.Join("/base", "pull", "foo_bar", "2001", "some-job", "1337", "finished.json"),
				Latest:        filepath.Join("/base", "directory", "some-job", "latest-build.txt"),
			},
			ExpectErr: false,
		},
		{
			Name:  "normal-other",
			Base:  "/base",
			Job:   "some-job",
			Repos: reposOther,
			Build: "1337",
			Expected: &Paths{
				Artifacts:     filepath.Join("/base", "pull", "example.com_foo_bar", "2001", "some-job", "1337", "artifacts"),
				BuildLog:      filepath.Join("/base", "pull", "example.com_foo_bar", "2001", "some-job", "1337", "build-log.txt"),
				PRPath:        filepath.Join("/base", "pull", "example.com_foo_bar", "2001", "some-job", "1337"),
				PRBuildLink:   filepath.Join("/base", "directory", "some-job", "1337.txt"),
				PRLatest:      filepath.Join("/base", "pull", "example.com_foo_bar", "2001", "some-job", "latest-build.txt"),
				PRResultCache: filepath.Join("/base", "pull", "example.com_foo_bar", "2001", "some-job", "jobResultsCache.json"),
				ResultCache:   filepath.Join("/base", "/directory/some-job", "jobResultsCache.json"),
				Started:       filepath.Join("/base", "pull", "example.com_foo_bar", "2001", "some-job", "1337", "started.json"),
				Finished:      filepath.Join("/base", "pull", "example.com_foo_bar", "2001", "some-job", "1337", "finished.json"),
				Latest:        filepath.Join("/base", "/directory/some-job", "latest-build.txt"),
			},
			ExpectErr: false,
		},
		{
			Name:      "expect-failure (no repos)",
			Base:      "/some/foo/base",
			Job:       "some-foo-job",
			Repos:     reposEmtpy,
			Build:     "1337",
			Expected:  nil,
			ExpectErr: true,
		},
	}
	for _, test := range testCases {
		res, err := PRPaths(test.Base, test.Repos, test.Job, test.Build)
		if test.ExpectErr && err == nil {
			t.Errorf("err == nil and error expected for test %#v", test.Name)
		} else if err != nil && !test.ExpectErr {
			t.Errorf("Got error and did not expect one for test %#v, %v", test.Name, err)
		} else if !reflect.DeepEqual(res, test.Expected) {
			t.Errorf("Paths did not match expected for test: %#v", test.Name)
			t.Errorf("%#v", res)
			t.Errorf("%#v", test.Expected)
		}
	}
}

func TestGubernatorBuildURL(t *testing.T) {
	// test with and without gs://
	tests := []struct {
		Name     string
		Paths    *Paths
		Expected string
	}{
		{
			Name:     "with gs://",
			Paths:    CIPaths("gs://foo", "bar", "baz"),
			Expected: "https://gubernator.k8s.io/build/foo/bar/baz",
		},
		{
			Name:     "without gs://",
			Paths:    CIPaths("/foo", "bar", "baz"),
			Expected: "/foo/bar/baz",
		},
	}
	for _, test := range tests {
		res := GubernatorBuildURL(test.Paths)
		if res != test.Expected {
			t.Errorf("result did not match expected for test case %#v", test.Name)
			t.Errorf("%#v != %#v", res, test.Expected)
		}
	}
}

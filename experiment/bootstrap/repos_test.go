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
	"reflect"
	"testing"
)

func TestParseRepos(t *testing.T) {
	testCases := []struct {
		Name      string
		Repos     []string
		Expected  Repos
		ExpectErr bool
	}{
		{
			Name: "normal",
			Repos: []string{
				"k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"k8s.io/release",
			},
			Expected: []Repo{
				{
					Name:   "k8s.io/kubernetes",
					Branch: "",
					Pull:   "master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				},
				{
					Name:   "k8s.io/release",
					Branch: "master",
					Pull:   "",
				},
			},
		},
		{
			Name: "single-commit",
			Repos: []string{
				"k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871",
			},
			Expected: []Repo{
				{
					Name:   "k8s.io/kubernetes",
					Branch: "master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871",
					Pull:   "",
				},
			},
			ExpectErr: false,
		},
		{
			Name: "expect-to-fail (invalid repo)",
			Repos: []string{
				"k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f63https://github.com/googlecartographer/point_cloud_viewer9f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"k8s.io/release",
				"foobar=,=",
			},
			Expected:  nil,
			ExpectErr: true,
		},
	}
	for _, test := range testCases {
		res, err := ParseRepos(test.Repos)
		if test.ExpectErr && err == nil {
			t.Errorf("err == nil and error expected for test %#v", test.Name)
		} else if err != nil && !test.ExpectErr {
			t.Errorf("Got error and did not expect one for test %#v: %v", test.Name, err)
		} else if !reflect.DeepEqual(res, test.Expected) {
			t.Errorf("Repos did not match expected for test: %#v", test.Name)
			t.Errorf("%#v", res)
			t.Errorf("%#v", test.Expected)
			// assert that currently Repos.Main() == Repos[0]
		} else if len(test.Expected) > 0 && res.Main() != &res[0] {
			t.Errorf("Expected repos.Main() to be &res[0] for all tests (test: %#v)", test.Name)
		}
	}
}

func TestRepoGitBasePath(t *testing.T) {
	// TODO(bentheelder): use ParseRepos instead of "hand-written" Repo{}s?
	// these are based on expected Repos from TestParseRepos
	testCases := []struct {
		Name     string
		Repo     Repo
		SSH      bool
		Expected string
	}{
		{
			Name: "k8s.io",
			Repo: Repo{
				Name:   "k8s.io/kubernetes",
				Branch: "master",
				Pull:   "",
			},
			SSH:      false,
			Expected: "https://github.com/kubernetes/kubernetes",
		},
		{
			Name: "k8s.io,ssh",
			Repo: Repo{
				Name:   "k8s.io/kubernetes",
				Branch: "master",
				Pull:   "",
			},
			SSH:      true,
			Expected: "git@github.com:kubernetes/kubernetes",
		},
		{
			Name: "kubernetes/test-infra",
			Repo: Repo{
				Name:   "github.com/kubernetes/test-infra",
				Branch: "master",
				Pull:   "",
			},
			SSH:      false,
			Expected: "https://github.com/kubernetes/test-infra",
		},
		{
			Name: "kubernetes/test-infra,ssh",
			Repo: Repo{
				Name:   "github.com/kubernetes/test-infra",
				Branch: "master",
				Pull:   "",
			},
			SSH:      true,
			Expected: "git@github.com:kubernetes/test-infra",
		},
	}
	for _, test := range testCases {
		res := test.Repo.GitBasePath(test.SSH)
		if res != test.Expected {
			t.Errorf("result did not match expected for test case %#v", test.Name)
			t.Errorf("%#v != %#v", res, test.Expected)
		}
	}
}

func TestRepoPullNumbers(t *testing.T) {
	// TODO(bentheelder): use ParseRepos instead of "hand-written" Repo{}s?
	// these are based on expected Repos from TestParseRepos
	testCases := []struct {
		Name     string
		Repo     Repo
		Expected []string
	}{
		{
			Name: "",
			Repo: Repo{
				Name:   "k8s.io/kubernetes",
				Branch: "",
				Pull:   "master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
			},
			Expected: []string{"52057"},
		},
	}
	for _, test := range testCases {
		res := test.Repo.PullNumbers()
		if !reflect.DeepEqual(res, test.Expected) {
			t.Errorf("result did not match expected for test case %#v", test.Name)
			t.Errorf("%#v", res)
			t.Errorf("%#v", test.Expected)
		}
	}
}

func TestRepos(t *testing.T) {
	// assert that Repos.Main() is nil for empty Repos
	emptyRepos := Repos{}
	if emptyRepos.Main() != nil {
		t.Errorf("Expected emptyRepos.Main() == nil")
	}
	// assert that currently Repos.Main() == Repos[0]
	repos, err := ParseRepos([]string{
		"k8s.io/release",
	})
	if err != nil {
		t.Errorf("Expected err to be nil")
	}
	if repos.Main() != &repos[0] {
		t.Errorf("Expected repos.Main() to be &repos[0]")
	}
}

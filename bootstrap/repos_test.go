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
	tests := []struct {
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
				"k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"k8s.io/release",
				"foobar=,=",
			},
			Expected:  nil,
			ExpectErr: true,
		},
	}
	for _, test := range tests {
		res, err := ParseRepos(test.Repos)
		if test.ExpectErr && err == nil {
			t.Errorf("err == nil and error expected for test %s", test.Name)
		} else if err != nil && !test.ExpectErr {
			t.Errorf("Got error and did not expect one for test %s, %v", test.Name, err)
		} else if !reflect.DeepEqual(res, test.Expected) {
			t.Errorf("Repos did not match expected for test: %s", test.Name)
			t.Errorf("%#v", res)
			t.Errorf("%#v", test.Expected)
			// assert that currently Repos.Main() == Repos[0]
		} else if len(test.Expected) > 0 && res.Main() != &res[0] {
			t.Errorf("Expected repos.Main() to be &res[0] for all tests (test: %s)", test.Name)
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

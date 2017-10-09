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

func TestParseArgs(t *testing.T) {
	tests := []struct {
		Name      string
		Arguments []string
		Expected  *Args
		ExpectErr bool
	}{
		{
			Name: "normal",
			Arguments: []string{
				"--repo=k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"--repo=foobar",
				"--job=fake",
			},
			Expected: &Args{
				Root: ".",
				Job:  "fake",
				Repo: []string{
					"k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
					"foobar",
				},
			},
			ExpectErr: false,
		},
		{
			Name: "single-commit",
			Arguments: []string{
				"--repo=k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871",
				"--job=fake",
			},
			Expected: &Args{
				Root: ".",
				Job:  "fake",
				Repo: []string{
					"k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871",
				},
			},
			ExpectErr: false,
		},
		{
			Name: "expect-to-fail (no --job)",
			Arguments: []string{
				"--repo=k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"--repo=foobar",
			},
			Expected:  nil,
			ExpectErr: true,
		},
		{
			Name: "expect-to-fail (--job=\"\")",
			Arguments: []string{
				"--repo=k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"--repo=foobar",
				"--job=",
			},
			Expected:  nil,
			ExpectErr: true,
		},
		{
			Name: "expect-to-fail (bad flags)",
			Arguments: []string{
				"--repo=k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"---repo=foobar",
				"--job=fake",
			},
			Expected:  nil,
			ExpectErr: true,
		},
		{
			Name: "expect-to-fail (--repo and --bare)",
			Arguments: []string{
				"--repo=k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3",
				"--bare",
				"--job=fake",
			},
			Expected:  nil,
			ExpectErr: true,
		},
	}
	for _, test := range tests {
		res, err := ParseArgs(test.Arguments)
		if test.ExpectErr && err == nil {
			t.Errorf("err == nil and error expected for test %s", test.Name)
		} else if err != nil && !test.ExpectErr {
			t.Errorf("Got error and did not expect one for test %s, %v", test.Name, err)
		} else if !reflect.DeepEqual(res, test.Expected) {
			t.Errorf("Args did not match expected for test: %s", test.Name)
			t.Errorf("%+v", res)
			t.Errorf("%+v", test.Expected)
		}
	}
}

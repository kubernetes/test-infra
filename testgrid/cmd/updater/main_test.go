/*
Copyright 2018 The Kubernetes Authors.

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
	"testing"

	"k8s.io/test-infra/testgrid/state"
)

func Test_ExtractRows(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		expected map[string]state.Row_Result
		err      bool
	}{
		{
			name: "basic testsuite",
			content: `
			  <testsuite>
			    <testcase name="good"/>
			    <testcase name="bad"><failure/></testcase>
			    <testcase name="skip"><skipped/></testcase>
			  </testsuite>`,
			expected: map[string]state.Row_Result{
				"good": state.Row_PASS,
				"bad":  state.Row_FAIL,
			},
		},
		{
			name: "basic testsuites",
			content: `
			  <testsuites>
			  <testsuite>
			    <testcase name="good"/>
			  </testsuite>
			  <testsuite>
			    <testcase name="bad"><failure/></testcase>
			    <testcase name="skip"><skipped/></testcase>
			  </testsuite>
			  </testsuites>`,
			expected: map[string]state.Row_Result{
				"good": state.Row_PASS,
				"bad":  state.Row_FAIL,
			},
		},
	}

	for _, tc := range cases {
		actual := map[string]state.Row_Result{}
		err := extractRows([]byte(tc.content), actual)
		switch {
		case err == nil && tc.err:
			t.Errorf("%s: failed to raise an error", tc.name)
		case err != nil && !tc.err:
			t.Errorf("%s: unexpected err: %v", tc.name, err)
		case len(actual) > len(tc.expected):
			t.Errorf("%s: extra keys: actual %#v != expected %#v", tc.name, actual, tc.expected)
		default:
			for target, er := range tc.expected {
				if ar, ok := actual[target]; !ok {
					t.Errorf("%s: missing key: %s", tc.name, target)
				} else if ar != er {
					t.Errorf("%s: actual %s != expected %s", tc.name, ar, er)
				}
			}
		}
	}
}

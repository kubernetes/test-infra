/*
Copyright 2016 The Kubernetes Authors.

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

package github

import "testing"

func TestParseLinks(t *testing.T) {
	var testcases = []struct {
		data     string
		expected map[string]string
	}{
		{
			``,
			map[string]string{},
		},
		{
			`<u>; rel="r"`,
			map[string]string{"r": "u"},
		},
		{
			`<u>; rel="r", <u2>; rel="r2"`,
			map[string]string{"r": "u", "r2": "u2"},
		},
		{
			`<u>;rel="r"`,
			map[string]string{"r": "u"},
		},
		{
			`<u>; rel="r"; other="o"`,
			map[string]string{"r": "u"},
		},
		{
			`<u>; rel="r",,`,
			map[string]string{"r": "u"},
		},
	}
	for _, tc := range testcases {
		actual := parseLinks(tc.data)
		if len(actual) != len(tc.expected) {
			t.Errorf("lengths mismatch: %v and %v", actual, tc.expected)
		}
		for k, v := range tc.expected {
			if av, ok := actual[k]; !ok {
				t.Errorf("link not found: %s", k)
			} else if av != v {
				t.Errorf("link %s was %s but should have been %s", k, av, v)
			}
		}
	}
}

/*
Copyright 2020 The Kubernetes Authors.

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

package flagutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStringToStringSlice(t *testing.T) {
	testCases := []struct {
		name        string
		value       string
		expectedErr bool
		expected    *StringToStringSlice
	}{
		{
			value: "a=1,2,3,4;b=5,2,3,7;c=5,8,9,7;z=1,3,2,1",
			expected: NewStringToStringSlice(map[string][]string{
				"a": {"1", "2", "3", "4"},
				"b": {"5", "2", "3", "7"},
				"c": {"5", "8", "9", "7"},
				"z": {"1", "3", "2", "1"},
			}),
		},
		{
			value: "cla: no=cla: yes,automerge;approved=automerge,lgtm",
			expected: NewStringToStringSlice(map[string][]string{
				"cla: no":  {"cla: yes", "automerge"},
				"approved": {"automerge", "lgtm"},
			}),
		},
		{
			value: "key=Value",
			expected: NewStringToStringSlice(map[string][]string{
				"key": {"Value"},
			}),
		},
		{
			value:       "a;b=c",
			expected:    NewStringToStringSlice(map[string][]string{}),
			expectedErr: true,
		},
		{
			value:       "a=b=c=d",
			expected:    NewStringToStringSlice(map[string][]string{}),
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		actual := NewStringToStringSlice(map[string][]string{})
		err := actual.Set(tc.value)

		if err == nil && tc.expectedErr {
			t.Fatal("error expected")
		}
		if err != nil && !tc.expectedErr {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cmp.Equal(tc.expected.Get(), actual.Get(), cmp.AllowUnexported(StringToStringSlice{})) {
			t.Fatalf("expected and actual are not equal: %v", cmp.Diff(tc.expected.Get(), actual.Get()))
		}
		if !cmp.Equal(tc.expected, actual, cmp.AllowUnexported(StringToStringSlice{})) {
			t.Fatalf("expected and actual string representations are not equal: %v", cmp.Diff(tc.expected, actual))
		}
	}
}

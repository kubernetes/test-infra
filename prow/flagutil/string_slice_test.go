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

func TestStringSlice(t *testing.T) {
	testCases := []struct {
		name        string
		value       string
		expectedErr bool
		expected    *StringSlice
	}{
		{
			value:    "a,b,c,z",
			expected: NewStringSlice([]string{"a", "b", "c", "z"}),
		},
		{
			value:    "cla: yes,approved",
			expected: NewStringSlice([]string{"cla: yes", "approved"}),
		},
		{
			value:    "key",
			expected: NewStringSlice([]string{"key"}),
		},
	}

	for _, tc := range testCases {
		actual := NewStringSlice([]string{})
		err := actual.Set(tc.value)

		if err == nil && tc.expectedErr {
			t.Fatal("error expected")
		}
		if err != nil && !tc.expectedErr {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cmp.Equal(tc.expected.Get(), actual.Get(), cmp.AllowUnexported(StringSlice{})) {
			t.Fatalf("expected and actual are not equal: %v", cmp.Diff(tc.expected.Get(), actual.Get()))
		}
		if !cmp.Equal(tc.expected, actual, cmp.AllowUnexported(StringSlice{})) {
			t.Fatalf("expected and actual string representations are not equal: %v", cmp.Diff(tc.expected, actual))
		}
	}
}

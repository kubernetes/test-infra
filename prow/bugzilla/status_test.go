/*
Copyright 2019 The Kubernetes Authors.

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

package bugzilla

import "testing"

func TestPrettyStatus(t *testing.T) {
	testCases := []struct {
		name       string
		status     string
		resolution string
		expected   string
	}{
		{
			name:     "both empty",
			expected: "",
		},
		{
			name:     "only status",
			status:   "CLOSED",
			expected: "CLOSED",
		},
		{
			name:       "only resolution",
			resolution: "NOTABUG",
			expected:   "any status with resolution NOTABUG",
		},
		{
			name:       "status and resolution",
			status:     "CLOSED",
			resolution: "NOTABUG",
			expected:   "CLOSED (NOTABUG)",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := PrettyStatus(tc.status, tc.resolution)
			if actual != tc.expected {
				t.Errorf("%s: expected %q, got %q", tc.name, tc.expected, actual)
			}
		})
	}
}

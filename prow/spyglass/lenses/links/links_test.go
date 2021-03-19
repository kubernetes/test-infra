/*
Copyright 2021 The Kubernetes Authors.

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

package links

import "testing"

func TestHumanReadableName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "master-and-node-logs.txt",
			want:  "Master and node logs",
		},
		{
			input: "dashboard.link.txt",
			want:  "Dashboard",
		},
		{
			input: "no-suffix",
			want:  "No suffix",
		},

		// Malformed inputs
		{
			input: ".link.txt",
			want:  "",
		},
		{
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := humanReadableName(tc.input); got != tc.want {
				t.Errorf("humanReadableName(%v)=%q, want: %v", tc.input, got, tc.want)
			}
		})
	}
}

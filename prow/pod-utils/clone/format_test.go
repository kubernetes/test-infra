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

package clone

import (
	"strings"
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestFormatRecord(t *testing.T) {
	cases := []struct {
		name    string
		r       Record
		require []string
		deny    []string
	}{
		{
			name: "basically works",
		},
		{
			name:    "no repo creates an environment setup record",
			require: []string{"Environment setup"},
			deny:    []string{"Cloning"},
		},
		{
			name: "setting repo creates a cloning record",
			r: Record{
				Refs: prowapi.Refs{
					Org:     "foo",
					Repo:    "bar",
					BaseRef: "deadbeef",
				},
			},
			require: []string{"Cloning foo/bar at deadbeef"},
			deny:    []string{"Environment setup"},
		},
		{
			name: "include base sha when set",
			r: Record{
				Refs: prowapi.Refs{
					Repo:    "bar",
					BaseSHA: "abcdef",
				},
			},
			require: []string{"abcdef"},
		},
		{
			name: "include passing commands",
			r: Record{
				Commands: []Command{
					{
						Command: "cat spam",
						Output:  "eggs",
					},
					{
						Command: "more",
						Output:  "fun",
					},
				},
			},
			require: []string{"cat spam", "eggs", "more", "fun"},
			deny:    []string{"Error:"},
		},
		{
			name: "include failing command",
			r: Record{
				Commands: []Command{
					{
						Command: "rm /something",
						Output:  "barf",
						Error:   "command failed",
					},
				},
			},
			require: []string{"rm /something", "barf", "# Error: command failed"},
		},
		{
			name: "skip pulls when missing",
			deny: []string{"Checking out pulls"},
		},
		{
			name: "include pulls when present",
			r: Record{
				Refs: prowapi.Refs{
					Pulls: []prowapi.Pull{
						{
							Number: 42,
							SHA:    "food",
						},
						{
							Number: 13,
						},
					},
				},
			},
			require: []string{"42", "food", "13"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := FormatRecord(tc.r)
			for _, r := range tc.require {
				if !strings.Contains(actual, r) {
					t.Errorf("%q missing %q", actual, r)
				}
			}
			for _, d := range tc.deny {
				if strings.Contains(actual, d) {
					t.Errorf("%q should not contain %q", actual, d)
				}
			}
		})
	}

}

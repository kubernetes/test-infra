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

package branchprotection

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

func TestMakePolicy(t *testing.T) {
	yes := true
	cases := []struct {
		name       string
		input      Policy
		expected   *Policy
		deprecated bool
		err        bool
	}{
		{
			name: "default returns nil",
		},
		{
			name: "modern returns false",
			input: Policy{
				Protect: &yes,
			},
			expected: &Policy{
				Protect: &yes,
			},
		},
		{
			name: "deprecated returns true",
			input: Policy{
				deprecatedPolicy: deprecatedPolicy{
					DeprecatedProtect: &yes,
				},
			},
			expected: &Policy{
				Protect: &yes,
			},
			deprecated: true,
		},
		{
			name: "mixed errors",
			input: Policy{
				Protect: &yes,
				deprecatedPolicy: deprecatedPolicy{
					DeprecatedProtect: &yes,
				},
			},
			err:        true,
			deprecated: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, dep, err := tc.input.MakePolicy()
			switch {
			case err != nil && !tc.err:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.err:
				t.Errorf("failed to receive an error")
			case dep && !tc.deprecated:
				t.Errorf("unexpected deprecation")
			case !dep && tc.deprecated:
				t.Errorf("failed to detect deprecation")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("bad policy:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
			}
		})
	}
}

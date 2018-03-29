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

package clone

import (
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestPathForRefs(t *testing.T) {
	var testCases = []struct {
		name     string
		refs     *kube.Refs
		expected string
	}{
		{
			name: "literal override",
			refs: &kube.Refs{
				PathAlias: "alias",
			},
			expected: "base/src/alias",
		},
		{
			name: "default generated",
			refs: &kube.Refs{
				Org:  "org",
				Repo: "repo",
			},
			expected: "base/src/github.com/org/repo",
		},
	}

	for _, testCase := range testCases {
		if actual, expected := PathForRefs("base", testCase.refs), testCase.expected; actual != expected {
			t.Errorf("%s: expected path %q, got %q", testCase.name, expected, actual)
		}
	}
}

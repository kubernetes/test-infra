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

package sidecar

import (
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestGetRevisionFromRef(t *testing.T) {
	var tests = []struct {
		name     string
		refs     *kube.Refs
		expected string
	}{
		{
			name: "Refs with Pull",
			refs: &kube.Refs{
				BaseRef: "master",
				BaseSHA: "deadbeef",
				Pulls: []kube.Pull{
					{
						Number: 123,
						SHA:    "abcd1234",
					},
				},
			},
			expected: "abcd1234",
		},
		{
			name: "Refs with BaseSHA",
			refs: &kube.Refs{
				BaseRef: "master",
				BaseSHA: "deadbeef",
			},
			expected: "deadbeef",
		},
		{
			name: "Refs with BaseRef",
			refs: &kube.Refs{
				BaseRef: "master",
			},
			expected: "master",
		},
	}

	for _, test := range tests {
		if actual, expected := getRevisionFromRef(test.refs), test.expected; actual != expected {
			t.Errorf("%s: got revision:%s but expected: %s", test.name, actual, expected)
		}
	}
}

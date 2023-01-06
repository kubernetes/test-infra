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

package kube

import (
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestRefs(t *testing.T) {
	testcases := []struct {
		ref      prowapi.Refs
		expected string
	}{
		{
			ref: prowapi.Refs{
				BaseRef: "master",
				BaseSHA: "123abc",
			},
			expected: "master:123abc",
		},
		{
			ref: prowapi.Refs{
				BaseRef: "release-1.6",
				BaseSHA: "123",
				Pulls: []prowapi.Pull{
					{
						Number: 5,
						SHA:    "qwe",
					},
				},
			},
			expected: "release-1.6:123,5:qwe",
		},
		{
			ref: prowapi.Refs{
				BaseRef: "foo",
				BaseSHA: "123",
				Pulls: []prowapi.Pull{
					{
						Number: 1,
						SHA:    "qwe",
					},
					{
						Number: 2,
						SHA:    "asd",
					},
				},
			},
			expected: "foo:123,1:qwe,2:asd",
		},
		{
			ref: prowapi.Refs{
				BaseRef: "foo",
				BaseSHA: "123",
				Pulls: []prowapi.Pull{
					{
						Number: 1,
						SHA:    "qwe",
						Ref:    "refs/changes/00/1/1",
					},
					{
						Number: 2,
						SHA:    "asd",
						Ref:    "refs/changes/00/1/2",
					},
				},
			},
			expected: "foo:123,1:qwe:refs/changes/00/1/1,2:asd:refs/changes/00/1/2",
		},
	}
	for _, tc := range testcases {
		actual := tc.ref.String()
		if actual != tc.expected {
			t.Errorf("Ref %+v, got %s, expected, %s", tc.ref, actual, tc.expected)
		}
	}
}

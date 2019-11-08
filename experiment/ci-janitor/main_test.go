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

// ci-janitor cleans up dedicated projects in k8s prowjob configs
package main

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"

	"k8s.io/test-infra/prow/config"
)

func TestContainers(t *testing.T) {
	cases := []struct {
		name     string
		jb       config.JobBase
		expected []v1.Container
	}{
		{
			name: "only podspec",
			jb: config.JobBase{
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "hello",
						},
						{
							Image: "there",
						},
					},
				},
			},
			expected: []v1.Container{
				{
					Name: "hello",
				},
				{
					Image: "there",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := containers(tc.jb)
			if !equality.Semantic.DeepEqual(actual, tc.expected) {
				t.Errorf("containers do not match:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
			}
		})
	}
}

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

package spyglasstests

import (
	"testing"

	"k8s.io/test-infra/prow/spyglass"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// Tests reading last N Lines from files in GCS
func TestGCSReadLastNLines(t *testing.T) {
	buildLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(buildLogName), "", fakeGCSJobSource.JobPath())
	//longLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(longLogName), "", fakeGCSJobSource.JobPath())
	testCases := []struct {
		name     string
		n        int64
		a        *spyglass.GCSArtifact
		expected []string
	}{
		{
			name:     "Read last 2 lines of a 4-line file",
			n:        2,
			a:        buildLogArtifact,
			expected: []string{"this is", "crazy"},
		},
		{
			name:     "Read last 5 lines of a 4-line file",
			n:        5,
			a:        buildLogArtifact,
			expected: []string{"Oh wow", "logs", "this is", "crazy"},
		},
		//{
		//	name:     "Read last 100 lines of a long log file",
		//	n:        100,
		//	a:        longLogArtifact,
		//	expected: longLogLines[len(longLogLines)-100:],
		//},
	}
	for _, tc := range testCases {
		actual, err := viewers.LastNLines(tc.a, tc.n)
		if err != nil {
			t.Fatalf("Test %s failed with error: %s", tc.name, err)
		}
		if len(actual) != len(tc.expected) {
			t.Fatalf("Test %s failed.\nExpected length:\n%d\nActual length:\n%d", tc.name, len(tc.expected), len(actual))
		}
		for ix, line := range tc.expected {
			if line != actual[ix] {
				t.Errorf("Test %s failed.\nExpected:\n%s\nActual:\n%s", tc.name, line, actual[ix])
			}
		}
		for ix, line := range actual {
			if line != tc.expected[ix] {
				t.Errorf("Test %s failed.\nExpected:\n%s\nActual:\n%s", tc.name, tc.expected[ix], line)
			}
		}
	}
}

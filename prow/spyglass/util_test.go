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

package spyglass

import (
	"testing"
)

// Tests reading last N Lines from files in GCS
func TestGCSReadLastNLines(t *testing.T) {
	buildLogArtifact := NewGCSArtifact(fakeGCSBucket.Object(buildLogName), "", fakeGCSJobSource.JobPath())
	testCases := []struct {
		name     string
		n        int64
		a        *GCSArtifact
		expected string
	}{
		{
			name:     "Read last 2 lines of a 4-line file",
			n:        2,
			a:        buildLogArtifact,
			expected: "this is\ncrazy",
		},
		{
			name:     "Read last 5 lines of a 4-line file",
			n:        5,
			a:        buildLogArtifact,
			expected: "Oh wow\nlogs\nthis is\ncrazy",
		},
	}
	for _, tc := range testCases {
		actual := LastNLines(tc.a, tc.n)
		if tc.expected != actual {
			t.Errorf("Test %s failed.\nExpected:\n%s\nActual:\n%s", tc.name, tc.expected, actual)
		}
	}
}

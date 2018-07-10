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
	"bytes"
	"testing"

	"k8s.io/test-infra/prow/spyglass"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// Tests reading at most n bytes of data from files in GCS
func TestGCSReadAtMost(t *testing.T) {
	buildLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(buildLogKey), "", buildLogName)
	testCases := []struct {
		name     string
		a        viewers.Artifact
		n        int64
		expected []byte
	}{
		{
			name:     "ReadN example build log",
			n:        4,
			a:        buildLogArtifact,
			expected: []byte("Oh w"),
		},
	}
	for _, tc := range testCases {
		actualBytes, err := tc.a.ReadAtMost(tc.n)
		if err != nil {
			t.Errorf("Test %s failed with err:\n%s", tc.name, err)
		}
		if !bytes.Equal(actualBytes, tc.expected) {
			t.Errorf("Test %s failed.\nExpected: %s\nActual: %s", tc.name, tc.expected, actualBytes)
		}
	}
}

// Tests reading all data from files in GCS
func TestGCSReadAll(t *testing.T) {
	buildLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(buildLogKey), "", buildLogName)
	testCases := []struct {
		name     string
		a        viewers.Artifact
		expected []byte
	}{
		{
			name:     "ReadAll example build log",
			a:        buildLogArtifact,
			expected: []byte("Oh wow\nlogs\nthis is\ncrazy"),
		},
	}
	for _, tc := range testCases {
		actualBytes, err := tc.a.ReadAll()
		if err != nil {
			t.Errorf("Test %s failed with err:\n%s", tc.name, err)
		}
		if !bytes.Equal(actualBytes, tc.expected) {
			t.Errorf("Test %s failed.\nExpected: %s\nActual: %s", tc.name, tc.expected, actualBytes)
		}
	}
}

func TestGCSSize(t *testing.T) {
	startedArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(startedKey), "", startedName)
	testCases := []struct {
		name     string
		a        viewers.Artifact
		expected int64
	}{
		{
			name: "Started size",
			a:    startedArtifact,
			expected: int64(len([]byte(`{
						  "node": "gke-prow-default-pool-3c8994a8-qfhg", 
						  "repo-version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "timestamp": 1528742858, 
						  "repos": {
						    "k8s.io/kubernetes": "master", 
						    "k8s.io/release": "master"
						  }, 
						  "version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "metadata": {
						    "pod": "cbc53d8e-6da7-11e8-a4ff-0a580a6c0269"
						  }
						}`))),
		},
	}
	for _, tc := range testCases {
		actual := tc.a.Size()
		if tc.expected != actual {
			t.Errorf("Test %s failed.\nExpected:\n%d\nActual:\n%d", tc.name, tc.expected, actual)
		}
	}
}

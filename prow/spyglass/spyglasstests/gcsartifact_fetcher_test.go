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
)

// Tests listing objects associated with the current job in GCS
// TODO: fake-gcs-server does not support the GCS XML API, may use a fake S3 server >.<
//func TestGCSListArtifacts(t *testing.T) {
//	blgArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(buildLogName), "", buildLogName)
//	srtArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(startedName), "", startedName)
//	finArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(finishedName), "", finishedName)
//	junitArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(junitName), "", junitName)
//	longLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(longLogName), "", longLogName)
//	testCases := []struct {
//		name              string
//		gcsJobSource      *spyglass.GCSJobSource
//		expectedArtifacts []string
//	}{
//		{
//			name:         "Fetch Example CI Run #403 Artifacts",
//			gcsJobSource: fakeGCSJobSource,
//			expectedArtifacts: []string{
//				blgArtifact.JobPath(),
//				srtArtifact.JobPath(),
//				junitArtifact.JobPath(),
//				finArtifact.JobPath(),
//				longLogArtifact.JobPath(),
//			},
//		},
//	}
//
//	for _, tc := range testCases {
//		actualArtifacts := testAf.Artifacts(tc.gcsJobSource)
//		for _, ea := range tc.expectedArtifacts {
//			found := false
//			for _, aa := range actualArtifacts {
//				if ea == aa {
//					found = true
//					break
//				}
//			}
//			if !found {
//				t.Errorf("Case %s failed to retrieve the following artifact: %s\nRetrieved: %s.", tc.name, ea, actualArtifacts)
//			}
//
//		}
//		if len(tc.expectedArtifacts) != len(actualArtifacts) {
//			t.Errorf("Case %s produced more artifacts than expected. Expected: %s\nActual: %s.", tc.name, tc.expectedArtifacts, actualArtifacts)
//		}
//	}
//}

// Tests getting handles to objects associated with the current job in GCS
func TestGCSFetchArtifacts(t *testing.T) {
	maxSize := int64(500e6)
	testCases := []struct {
		name            string
		artifactName    string
		gcsJobSource    *spyglass.GCSJobSource
		expectedSize    int64
		expectedJobPath string
	}{
		{
			name:            "Fetch build-log.txt from example CI Run #403 Artifacts",
			artifactName:    "build-log.txt",
			gcsJobSource:    fakeGCSJobSource,
			expectedSize:    25,
			expectedJobPath: "build-log.txt",
		},
	}

	for _, tc := range testCases {
		artifact := testAf.Artifact(tc.gcsJobSource, tc.artifactName, maxSize)
		if artifact.JobPath() != tc.expectedJobPath {
			t.Errorf("%s expected artifact with job path %s but got %s", tc.name, tc.expectedJobPath, artifact.JobPath())
		}
		if artifact.Size() != tc.expectedSize {
			t.Errorf("%s expected artifact with size %d but got %d", tc.name, tc.expectedSize, artifact.Size())
		}
	}
}

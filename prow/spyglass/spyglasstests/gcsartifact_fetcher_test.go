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

// Tests getting handles to objects associated with the current job in GCS
func TestGCSFetchArtifacts(t *testing.T) {
	blgArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(buildLogName), "", buildLogName)
	srtArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(startedName), "", startedName)
	finArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(finishedName), "", finishedName)
	junitArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(junitName), "", junitName)
	longLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(longLogName), "", longLogName)
	testCases := []struct {
		name              string
		gcsJobSource      *spyglass.GCSJobSource
		expectedArtifacts []string
	}{
		{
			name:         "Fetch Example CI Run #403 Artifacts",
			gcsJobSource: fakeGCSJobSource,
			expectedArtifacts: []string{
				blgArtifact.JobPath(),
				srtArtifact.JobPath(),
				junitArtifact.JobPath(),
				finArtifact.JobPath(),
				longLogArtifact.JobPath(),
			},
		},
	}

	for _, tc := range testCases {
		actualArtifacts := testAf.Artifacts(tc.gcsJobSource)
		for _, ea := range tc.expectedArtifacts {
			found := false
			for _, aa := range actualArtifacts {
				if ea == aa {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Case %s failed to retrieve the following artifact: %s\nRetrieved: %s.", tc.name, ea, actualArtifacts)
			}

		}
		if len(tc.expectedArtifacts) != len(actualArtifacts) {
			t.Errorf("Case %s produced more artifacts than expected. Expected: %s\nActual: %s.", tc.name, tc.expectedArtifacts, actualArtifacts)
		}
	}
}

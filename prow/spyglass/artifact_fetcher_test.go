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

// Tests getting handles to objects associated with the current job in GCS
func TestGCSFetchArtifacts(t *testing.T) {
	testCases := []struct {
		name              string
		gcsJobSource      *GCSJobSource
		expectedArtifacts []Artifact
	}{
		{
			name:         "Fetch Example CI Run #403 Artifacts",
			gcsJobSource: fakeGCSJobSource,
			expectedArtifacts: []Artifact{
				NewGCSArtifact(fakeGCSBucket.Object(buildLogName), fakeGCSJobSource.JobPath()),
				NewGCSArtifact(fakeGCSBucket.Object(startedName), fakeGCSJobSource.JobPath()),
				NewGCSArtifact(fakeGCSBucket.Object(finishedName), fakeGCSJobSource.JobPath()),
			},
		},
	}

	for _, tc := range testCases {
		actualArtifacts := testAf.Artifacts(tc.gcsJobSource)
		for _, ea := range tc.expectedArtifacts {
			found := false
			for _, aa := range actualArtifacts {
				if ea.CanonicalLink() == aa.CanonicalLink() {
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

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
	"context"
	"testing"
)

// Tests getting handles to objects associated with the current Prow job
func TestFetchArtifacts_Prow(t *testing.T) {
	goodFetcher := NewPodLogArtifactFetcher(&fakePodLogJAgent{})
	maxSize := int64(500e6)
	testCases := []struct {
		name      string
		job       string
		buildID   string
		expectErr bool
	}{
		{
			name:    "Fetch build-log.txt from valid src",
			job:     "BFG",
			buildID: "435",
		},
		{
			name:      "Fetch log from empty src",
			job:       "",
			buildID:   "",
			expectErr: true,
		},
		{
			name:      "Fetch log from incomplete src",
			job:       "BFG",
			buildID:   "",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		artifact, err := goodFetcher.Artifact(context.Background(), tc.job, tc.buildID, maxSize)
		if err != nil && !tc.expectErr {
			t.Errorf("%s: failed unexpectedly for artifact %s, err: %v", tc.name, artifact.JobPath(), err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s: expected error, got no error", tc.name)
		}
	}
}

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
	"bytes"
	"context"
	"fmt"
	"testing"

	"k8s.io/test-infra/prow/kube"
)

// Tests getting handles to objects associated with the current Prow job
func TestFetchArtifacts_Prow(t *testing.T) {
	goodFetcher := NewPodLogArtifactFetcher(&fakePodLogJAgent{})
	maxSize := int64(500e6)
	testCases := []struct {
		name         string
		key          string
		artifact     string
		expectedPath string
		expectedLink string
		expected     []byte
		expectErr    bool
	}{
		{
			name:         "Fetch build-log.txt from valid src",
			key:          "BFG/435",
			artifact:     singleLogName,
			expectedLink: fmt.Sprintf("/log?container=%s&id=435&job=BFG", kube.TestContainerName),
			expected:     []byte("frobscottle"),
		},
		{
			name:      "Fetch log from empty src",
			key:       "",
			artifact:  singleLogName,
			expectErr: true,
		},
		{
			name:      "Fetch log from incomplete src",
			key:       "BFG",
			artifact:  singleLogName,
			expectErr: true,
		},
		{
			name:      "Fetch log with no artifact name",
			key:       "BFG/435",
			artifact:  "",
			expectErr: true,
		},
		{
			name:         "Fetch log with custom artifact name",
			key:          "BFG/435",
			artifact:     fmt.Sprintf("%s-%s", customContainerName, singleLogName),
			expectedLink: fmt.Sprintf("/log?container=%s&id=435&job=BFG", customContainerName),
			expected:     []byte("snozzcumber"),
		},
	}

	for _, tc := range testCases {
		artifact, err := goodFetcher.Artifact(context.Background(), tc.key, tc.artifact, maxSize)
		if err != nil && !tc.expectErr {
			t.Errorf("%s: failed unexpectedly for artifact %s, err: %v", tc.name, artifact.JobPath(), err)
			continue
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s: expected error, got no error", tc.name)
			continue
		}

		if artifact != nil {
			if artifact.JobPath() != tc.artifact {
				t.Errorf("Unexpected job path, expected %s, got %q", artifact.JobPath(), tc.artifact)
			}
			link := artifact.CanonicalLink()
			if link != tc.expectedLink {
				t.Errorf("Unexpected link, expected %s, got %q", tc.expectedLink, link)
			}
			res, err := artifact.ReadAll()
			if err != nil {
				t.Fatalf("%s failed reading bytes of log. got err: %v", tc.name, err)
				continue
			}
			if !bytes.Equal(tc.expected, res) {
				t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
			}
		}

	}
}

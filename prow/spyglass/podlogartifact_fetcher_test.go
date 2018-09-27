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
	"net/url"
	"testing"

	"k8s.io/test-infra/prow/deck/jobs"
)

// Tests getting handles to objects associated with the current Prow job
func TestFetchArtifacts_Prow(t *testing.T) {
	badFetcher := NewPodLogArtifactFetcher((*jobs.JobAgent)(nil))
	goodFetcher := NewPodLogArtifactFetcher(&fakePodLogJAgent{})
	maxSize := int64(500e6)
	testCases := []struct {
		name         string
		artifactName string
		rawSrc       string
		expectErr    bool
	}{
		{
			name:         "Fetch build-log.txt from valid url",
			artifactName: "build-log.txt",
			rawSrc:       "https://prow.k8s.io/prowjob?job=example-ci-run&id=403",
		},
		{
			name:         "Fetch log with missing job id",
			artifactName: "build-log.txt",
			rawSrc:       "https://prow.k8s.io/prowjob?job=example-ci-run",
			expectErr:    true,
		},
		{
			name:         "Fetch log with empty job id",
			artifactName: "build-log.txt",
			rawSrc:       "https://prow.k8s.io/prowjob?job=example-ci-run&id=",
			expectErr:    true,
		},
		{
			name:         "Fetch log with missing build name",
			artifactName: "build-log.txt",
			rawSrc:       "https://prow.k8s.io/prowjob?id=403",
			expectErr:    true,
		},
		{
			name:         "Fetch log with empty build name",
			artifactName: "build-log.txt",
			rawSrc:       "https://prow.k8s.io/prowjob?job=&id=1234",
			expectErr:    true,
		},
	}

	for _, tc := range testCases {
		src, err := url.Parse(tc.rawSrc)
		if err != nil {
			t.Fatalf("Unexpected error: invalid url %s in test %s", tc.rawSrc, tc.name)
		}
		_, err = badFetcher.artifact(*src, tc.artifactName, maxSize)
		if err == nil {
			t.Errorf("%s: expected nil job agent to produce error, got no error", tc.name)
		}
		artifact, err := goodFetcher.artifact(*src, tc.artifactName, maxSize)
		if err != nil && !tc.expectErr {
			t.Fatalf("%s: failed unexpectedly for artifact %s, err: %v", tc.name, artifact.JobPath(), err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s: expected error, got no error", tc.name)
		}
	}
}

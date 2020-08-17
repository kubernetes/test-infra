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
	"fmt"

	"k8s.io/test-infra/prow/spyglass/api"
)

// PodLogArtifactFetcher is used to fetch artifacts from k8s apiserver
type PodLogArtifactFetcher struct {
	jobAgent
}

// NewPodLogArtifactFetcher returns a PodLogArtifactFetcher using the given job agent as storage
func NewPodLogArtifactFetcher(ja jobAgent) *PodLogArtifactFetcher {
	return &PodLogArtifactFetcher{jobAgent: ja}
}

// artifact constructs an artifact handle for the given job build
func (af *PodLogArtifactFetcher) Artifact(_ context.Context, jobName, buildID string, sizeLimit int64) (api.Artifact, error) {

	podLog, err := NewPodLogArtifact(jobName, buildID, sizeLimit, af.jobAgent)
	if err != nil {
		return nil, fmt.Errorf("error accessing pod log from given source: %w", err)
	}
	return podLog, nil
}

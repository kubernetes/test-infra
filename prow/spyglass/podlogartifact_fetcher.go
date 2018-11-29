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
	"fmt"
	"strings"

	"k8s.io/test-infra/prow/spyglass/lenses"
)

// PodLogArtifactFetcher is used to fetch artifacts from k8s apiserver
type PodLogArtifactFetcher struct {
	jobAgent
}

// NewPodLogArtifactFetcher returns a PodLogArtifactFetcher using the given job agent as storage
func NewPodLogArtifactFetcher(ja jobAgent) *PodLogArtifactFetcher {
	return &PodLogArtifactFetcher{jobAgent: ja}
}

// artifact constructs an artifact handle from the given key
func (af *PodLogArtifactFetcher) artifact(key string, sizeLimit int64) (lenses.Artifact, error) {
	parsed := strings.Split(key, "/")
	if len(parsed) != 2 {
		return nil, fmt.Errorf("Could not fetch artifact: key %q incorrectly formatted", key)
	}
	jobName := parsed[0]
	buildID := parsed[1]

	podLog, err := NewPodLogArtifact(jobName, buildID, sizeLimit, af.jobAgent)
	if err != nil {
		return nil, fmt.Errorf("Error accessing pod log from given source: %v", err)
	}
	return podLog, nil
}

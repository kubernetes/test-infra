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

	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// PodLogArtifactFetcher is used to fetch artifacts from prow storage
type PodLogArtifactFetcher struct {
	jobAgent podLogJobAgent
}

// NewPodLogArtifactFetcher returns a PodLogArtifactFetcher using the given job agent as storage
func NewPodLogArtifactFetcher(ja podLogJobAgent) *PodLogArtifactFetcher {
	return &PodLogArtifactFetcher{jobAgent: ja}
}

func (af *PodLogArtifactFetcher) artifact(src string, sizeLimit int64) (viewers.Artifact, error) {
	split := strings.SplitN(src, "/", 2)
	if len(split) < 2 {
		return nil, fmt.Errorf("Invalid prow src: %s", src)
	}
	id := split[1]

	if af.jobAgent == nil || af.jobAgent == (*jobs.JobAgent)(nil) {
		return nil, fmt.Errorf("Prow job agent doesn't exist (are you running locally?)")
	}
	podLog, err := NewPodLogArtifact(id, sizeLimit, af.jobAgent)
	if err != nil {
		return nil, fmt.Errorf("Error accessing pod log from given source: %v", err)
	}
	return podLog, nil
}

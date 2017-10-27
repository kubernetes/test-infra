/*
Copyright 2017 The Kubernetes Authors.

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

package git

import (
	"strings"

	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

const (
	KubernetesCommitPrefix         = "Kubernetes-commit: "
	ancientSyncCommitSubjectPrefix = "sync(k8s.io/kubernetes)"
)

// kubeHash extracts kube commit from commit message
func KubeHash(c *object.Commit) plumbing.Hash {
	lines := strings.Split(c.Message, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, KubernetesCommitPrefix) {
			return plumbing.NewHash(strings.TrimSpace(line[len(KubernetesCommitPrefix):]))
		}
	}

	if strings.HasPrefix(lines[0], ancientSyncCommitSubjectPrefix) {
		return plumbing.NewHash(strings.TrimSpace(lines[0][len(ancientSyncCommitSubjectPrefix):]))
	}

	return plumbing.ZeroHash
}

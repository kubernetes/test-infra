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
	"fmt"

	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// KubeCommitsToDstCommits returns a mapping from all kube mainline commits
// to the corresponding dst commits after collapsing using "git filter-branch --sub-directory-filter":
//
// dst upstream
//  |    |
//  F'<--F
//  z    |
//  y    |
//  E'<--E
//  x   ,D
//  |  / |
//  C'<--C
//  w    |
//  v    |
//       B
//       A - initial commit
//
func KubeCommitsToDstCommits(r *gogit.Repository, dstFirstParents, kFirstParents []*object.Commit) (map[plumbing.Hash]plumbing.Hash, error) {
	// compute merge point table
	kubeMergePoints, err := MergePoints(r, kFirstParents)
	if err != nil {
		return nil, fmt.Errorf("failed to build merge point table: %v", err)
	}

	// convert dstFirstParents to HashesWithKubeHashes
	directKubeHashToDstMainLineHash := map[plumbing.Hash]plumbing.Hash{}
	for _, c := range dstFirstParents {
		// kh might be a non-mainline-merge (because we had used branch commits as kube hashes long ago)
		kh := KubeHash(c)
		if kh == plumbing.ZeroHash {
			continue
		}
		merge := kubeMergePoints[kh]
		if merge == nil {
			continue
		}
		// do not override, because we might have seen the actual merge before
		if _, found := directKubeHashToDstMainLineHash[merge.Hash]; !found {
			directKubeHashToDstMainLineHash[merge.Hash] = c.Hash
		}
	}

	// fill up mainlineKubeHashes in dstMainlineCommits with collapsed kube commits
	dst := plumbing.ZeroHash
	kubeHashToDstMainLineHash := map[plumbing.Hash]plumbing.Hash{}
	for i := len(kFirstParents) - 1; i >= 0; i-- {
		kc := kFirstParents[i]
		if dh, found := directKubeHashToDstMainLineHash[kc.Hash]; found {
			dst = dh
		}
		if dst != plumbing.ZeroHash {
			kubeHashToDstMainLineHash[kc.Hash] = dst
		}
	}
	if dst == plumbing.ZeroHash {
		return nil, fmt.Errorf("no upstream mainline commit found on branch")
	}

	return kubeHashToDstMainLineHash, nil
}

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

	"k8s.io/test-infra/mungegithub/publisher/pkg/cache"
)

// FirstParent returns the first parent of a commit. For a merge commit this
// is the parent which is usually depicted on the left.
func FirstParent(r *gogit.Repository, c *object.Commit) (*object.Commit, error) {
	if c == nil {
		return nil, nil
	}
	if len(c.ParentHashes) == 0 {
		return nil, nil
	}
	p, err := cache.CommitObject(r, c.ParentHashes[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get %v: %v", c.ParentHashes[0], err)
	}
	return p, nil
}

// FirstParentList visits the ancestors of c using the FirstParent func. It returns the list
// of visited commits.
func FirstParentList(r *gogit.Repository, c *object.Commit) ([]*object.Commit, error) {
	l := []*object.Commit{}
	for {
		if c == nil {
			break
		}

		l = append(l, c)

		// continue with first parent if there is one
		next, err := FirstParent(r, c)
		if err != nil {
			return nil, fmt.Errorf("failed to get first parent of %s: %v", c.Hash, err)
		}
		c = next
	}
	return l, nil
}

// MergePoints creates a look-up table from feature branch commit hashes to their merge commits
// onto the given mainline.
func MergePoints(r *gogit.Repository, mainLine []*object.Commit) (map[plumbing.Hash]*object.Commit, error) {
	// create lookup table for the position in mainLine
	mainLinePos := map[plumbing.Hash]int{}
	for i, c := range mainLine {
		mainLinePos[c.Hash] = i
	}

	bestMergePoints := map[plumbing.Hash]int{}
	seen := map[plumbing.Hash]*object.Commit{}

	// pos is the position of the current mainline commit, h
	var visit func(pos int, h plumbing.Hash) error
	visit = func(pos int, h plumbing.Hash) error {
		// stop if we reached the mainline
		if _, isOnMainLine := mainLinePos[h]; isOnMainLine {
			return nil
		}

		// was h seen before as descendent of a mainline commit? It must have had
		// a better position as we saw it earlier.
		if _, seenBefore := bestMergePoints[h]; seenBefore {
			return nil
		}

		bestMergePoints[h] = pos

		// resolve hash
		c := seen[h]
		if c == nil {
			var err error
			c, err = cache.CommitObject(r, h)
			if err != nil {
				return err
			}
			seen[h] = c
		}

		// recurse into parents
		for _, ph := range c.ParentHashes {
			err := visit(pos, ph)
			if err != nil {
				return err
			}
		}

		return nil
	}

	// recursively enumerate all reachable commits
	for pos, c := range mainLine {
		bestMergePoints[c.Hash] = pos
		seen[c.Hash] = c
		for _, ph := range c.ParentHashes {
			err := visit(pos, ph)
			if err != nil {
				return nil, err
			}
		}
	}

	// map commit hash to best merge point on mainline
	result := map[plumbing.Hash]*object.Commit{}
	for _, c := range seen {
		result[c.Hash] = mainLine[bestMergePoints[c.Hash]]
	}

	return result, nil
}

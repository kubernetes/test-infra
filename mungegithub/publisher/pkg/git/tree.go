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
	"path"

	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"gopkg.in/src-d/go-git.v4/plumbing/filemode"
)

// SetTreeEntryAt returns a Tree object with treePath replaced by the given replacement. It is stored in the
// given repository. treePath's last segment must match the name of the replacement if the later is non-nil.
// A nil replacement will delete the path.
func SetTreeEntryAt(r *gogit.Repository, t *object.Tree, pathPrefix string, treePath []string, replacement *object.TreeEntry) (*object.Tree, error) {
	if len(treePath) == 0 {
		return nil, fmt.Errorf("replacement path cannot be empty")
	}
	if replacement != nil && treePath[len(treePath)-1] != replacement.Name {
		return nil, fmt.Errorf("replacement name %q does not match path %q base name", replacement.Name, path.Join(treePath...))
	}

	i := findEntry(t.Entries, treePath[0])
	if i != -1 {
		var newT *object.Tree
		if replacement == nil {
			newT = &object.Tree{
				Entries: append(t.Entries[:max(i-1, 0)], t.Entries[i+1:]...),
			}
		} else {
			newEntry := replacement
			if len(treePath) > 1 {
				if t.Entries[i].Mode&filemode.Dir == 0 {
					return nil, fmt.Errorf("expected %q to be a directory", path.Join(pathPrefix, treePath[0]))
				}
				oldSubT, err := object.GetTree(r.Storer, t.Entries[i].Hash)
				subT, err := SetTreeEntryAt(r, oldSubT, path.Join(pathPrefix, treePath[0]), treePath[1:], replacement)
				if err != nil {
					return nil, err
				}
				if oldSubT.Hash.String() == subT.Hash.String() {
					return t, nil
				}
				newEntry = &object.TreeEntry{
					Name: treePath[0],
					Mode: filemode.Dir,
					Hash: subT.Hash,
				}
			}
			newT = &object.Tree{
				Entries: append(append(t.Entries[:max(i-1, 0)], *newEntry), t.Entries[i+1:]...),
			}
		}
		encoded := r.Storer.NewEncodedObject()
		err := newT.Encode(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to create tree for %v", path.Join(pathPrefix, path.Join(treePath...)))
		}
		hash, err := r.Storer.SetEncodedObject(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to store tree for %v", path.Join(pathPrefix, path.Join(treePath...)))
		}
		return object.GetTree(r.Storer, hash)
	} else if replacement == nil {
		return t, nil
	}

	// sub-directory not found. Create new entries.
	newEntry := replacement
	for {
		treePath = treePath[:len(treePath)-1]
		if len(treePath) == 0 {
			break
		}
		newT := &object.Tree{
			Entries: []object.TreeEntry{*newEntry},
		}
		encoded := r.Storer.NewEncodedObject()
		err := newT.Encode(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to create tree for %v", path.Join(pathPrefix, path.Join(treePath...)))
		}
		hash, err := r.Storer.SetEncodedObject(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to store tree for %v", path.Join(pathPrefix, path.Join(treePath...)))
		}
		newEntry = &object.TreeEntry{
			Name: treePath[len(treePath)-1],
			Mode: filemode.Dir,
			Hash: hash,
		}
	}
	newT := &object.Tree{
		Entries: append(t.Entries, *newEntry),
	}
	encoded := r.Storer.NewEncodedObject()
	err := newT.Encode(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to create tree for %v", path.Join(pathPrefix, path.Join(treePath...)))
	}
	hash, err := r.Storer.SetEncodedObject(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to store tree for %v", path.Join(pathPrefix, path.Join(treePath...)))
	}
	return object.GetTree(r.Storer, hash)
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func findEntry(entries []object.TreeEntry, path string) int {
	for i := range entries {
		if entries[i].Name == path {
			return i
		}
	}
	return -1
}

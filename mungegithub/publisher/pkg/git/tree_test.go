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
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	"strings"

	"fmt"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-billy.v3/osfs"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

func TestReplacePath(t *testing.T) {
	// create repo fixture
	tmpDir := os.TempDir()
	defer os.RemoveAll(tmpDir)
	wt := osfs.New(tmpDir)
	r, err := gogit.Init(memory.NewStorage(), wt)
	assert.NoError(t, err)

	// create vendor tree entry fixture
	testTree := func(files map[string]string) *object.Tree {
		ret, err := createTreeFixture(r, files)
		if err != nil {
			t.Fatalf("Failed to create tree: %v", err)
		}
		return ret
	}
	vendorFiles := map[string]string{
		"vendor/foo.go":                  "foo",
		"vendor/bar.go":                  "bar",
		"vendor/k8s.io/client-go/foo.go": "foo",
	}
	vendorTree := testTree(vendorFiles)
	vendorTreeEntry := func(path string) *object.TreeEntry {
		e, err := vendorTree.FindEntry(path)
		if err != nil {
			t.Fatalf("Failed to find vendor tree path %q: %v", path, err)
		}
		return e
	}

	type args struct {
		files       map[string]string
		path        string
		replacement *object.TreeEntry
	}
	tests := []struct {
		name      string
		args      args
		wantFiles map[string]string
		wantErr   bool
	}{

		{"empty tree, nil replacement",
			args{map[string]string{}, "vendor", nil},
			map[string]string{},
			false,
		},
		{"nil replacement",
			args{map[string]string{"vendor/old.go": "old"}, "vendor", nil},
			map[string]string{},
			false,
		},
		{"non-nil replacement",
			args{map[string]string{"vendor/old.go": "old"}, "vendor", vendorTreeEntry("vendor")},
			vendorFiles,
			false,
		},
		{"vendor files in tree, non-nil replacement",
			args{map[string]string{"vendor/old.go": "old", "main.go": "main"}, "vendor", vendorTreeEntry("vendor")},
			mergeFiles(map[string]string{"main.go": "main"}, vendorFiles),
			false,
		},

		{"vendor files in tree, non-nil replacement, replace deep inside",
			args{map[string]string{"vendor/old.go": "old", "main.go": "main"}, "vendor/vendor", vendorTreeEntry("vendor")},
			mergeFiles(map[string]string{"main.go": "main", "vendor/old.go": "old"}, prefixFiles(vendorFiles, "vendor")),
			false,
		},
		{"empty tree, non-nil replacement, replace deep inside",
			args{map[string]string{}, "dir1/dir2/dir3/vendor", vendorTreeEntry("vendor")},
			prefixFiles(vendorFiles, "dir1/dir2/dir3"),
			false,
		},
	}

	for _, tt := range tests {
		treePath := strings.Split(tt.args.path, "/")
		got, err := SetTreeEntryAt(r, testTree(tt.args.files), "", treePath, tt.args.replacement)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: SetTreeEntryAt() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			return
		}
		if got == nil {
			t.Errorf("%s: SetTreeEntryAt() = %v, want non-nil", tt.name, got)
		}
		gotFiles := treeFiles(got)
		if !reflect.DeepEqual(gotFiles, tt.wantFiles) {
			t.Errorf("%s: SetTreeEntryAt() = %v, want %v", tt.name, gotFiles, tt.wantFiles)
		}
	}
}

func mergeFiles(a, b map[string]string) map[string]string {
	ret := map[string]string{}
	for k, v := range a {
		ret[k] = v
	}
	for k, v := range b {
		ret[k] = v
	}
	return ret
}

func prefixFiles(a map[string]string, prefix string) map[string]string {
	ret := map[string]string{}
	for k, v := range a {
		ret[path.Join(prefix, k)] = v
	}
	return ret
}

func treeFiles(t *object.Tree) map[string]string {
	ret := map[string]string{}
	t.Files().ForEach(func(f *object.File) error {
		c, _ := f.Contents()
		ret[f.Name] = c
		return nil
	})
	return ret
}

var fixtureAuthor = object.Signature{
	Name:  "Fixture",
	Email: "fixture@fixture.com",
	When:  time.Now(),
}

func createTreeFixture(r *gogit.Repository, pathContents map[string]string) (*object.Tree, error) {
	// create local working tree
	tmpDir := os.TempDir()
	defer os.RemoveAll(tmpDir)
	fs := osfs.New(tmpDir)
	tmpR, err := gogit.Open(r.Storer, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize temporary repo: %v", err)
	}
	wt, err := tmpR.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary worktree: %v", err)
	}

	// empty working tree
	empty, err := emptyCommit(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create empty commit: %v", err)
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: empty, Force: true}); err != nil {
		return nil, fmt.Errorf("failed to checkout empty commit: %v", err)
	}

	for p, c := range pathContents {
		fs.MkdirAll(path.Dir(p), 0755)
		f, err := fs.Create(p)
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary worktree file %q: %v", p, err)
		}
		f.Write([]byte(c))
		f.Close()

		wt.Add(p)
	}

	hash, err := wt.Commit("fixture-", &gogit.CommitOptions{
		All:    true,
		Author: &fixtureAuthor,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary commit: %v", err)
	}
	c, err := r.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get temporary commit %v: %v", hash, err)
	}
	return c.Tree()
}

func emptyCommit(r *gogit.Repository) (plumbing.Hash, error) {
	obj := r.Storer.NewEncodedObject()
	emptyTree := object.Tree{}
	err := emptyTree.Encode(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to encode empty tree: %v", err)
	}
	emptyTree.Hash, err = r.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to store empty tree: %v", err)
	}
	empty := object.Commit{Message: "Empty fixture", Author: fixtureAuthor, TreeHash: emptyTree.Hash}
	obj = r.Storer.NewEncodedObject()
	if err := empty.Encode(obj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to encode empty commit: %v", err)
	}
	empty.Hash, err = r.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to store empty commit: %v", err)
	}
	return empty.Hash, nil
}

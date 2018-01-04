/*
Copyright 2016 The Kubernetes Authors.

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

package genfiles

import (
	"bytes"
	"testing"
)

func TestGroupLoad(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		repoPaths []string
		err       error

		want Group
	}{
		{
			name: "k8s-config",
			src: `# Files that should be ignored by tools which do not want to consider generated
# code.
#
# eg: https://git.k8s.io/test-infra/prow/plugins/size/size.go
#
# This file is a series of lines, each of the form:
#     <type> <name>
#
# Type can be:
#    path - an exact path to a single file
#    file-name - an exact leaf filename, regardless of path
#    path-prefix - a prefix match on the file path
#    file-prefix - a prefix match of the leaf filename (no path)
#    paths-from-repo - read a file from the repo and load file paths
#

file-prefix zz_generated.

file-name   BUILD
file-name   types.generated.go
file-name   generated.pb.go
file-name   generated.proto
file-name   types_swagger_doc_generated.go

path-prefix Godeps/
path-prefix vendor/
path-prefix api/swagger-spec/
path-prefix pkg/generated/

paths-from-repo docs/.generated_docs`,
			repoPaths: []string{"docs/.generated_docs"},
			want: Group{
				FileNames: map[string]bool{
					"BUILD":                          true,
					"types.generated.go":             true,
					"generated.pb.go":                true,
					"generated.proto":                true,
					"types_swagger_doc_generated.go": true,
				},
				PathPrefixes: map[string]bool{
					"Godeps/":           true,
					"vendor/":           true,
					"api/swagger-spec/": true,
					"pkg/generated/":    true,
				},
				FilePrefixes: map[string]bool{
					"zz_generated.": true,
				},
			},
		},
		{
			name: "malformed config",
			src: `# This is an invalid .generated_files

what is this line anyway?`,
			err:  &ParseError{line: "what is this line anyway?"},
			want: Group{},
		},
		{
			name: "partially valid config",
			src: `# This file contains some valid lines, and then som bad lines.

# Good lines

file-prefix     myprefix
file-name mypath

paths-from-repo myrepo

# Bad lines

badline

invalid command`,
			repoPaths: []string{"myrepo"},
			err:       &ParseError{line: "badline"},
			want: Group{
				FileNames: map[string]bool{
					"mypath": true,
				},
				FilePrefixes: map[string]bool{
					"myprefix": true,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := &Group{
				Paths:        make(map[string]bool),
				FileNames:    make(map[string]bool),
				PathPrefixes: make(map[string]bool),
				FilePrefixes: make(map[string]bool),
			}

			rps, err := g.load(bytes.NewBufferString(c.src))

			// Check repoPaths
			if got, want := len(rps), len(c.repoPaths); got != want {
				t.Logf("g.load, repoPaths: got %v, want %v", rps, c.repoPaths)
				t.Fatalf("len(repoPaths) mismatch: got %d, want %d", got, want)
			}

			for i, p := range rps {
				if got, want := p, c.repoPaths[i]; got != want {
					t.Fatalf("repoPaths mismatch at index %d: got %s, want %s", i, got, want)
				}
			}

			// Check err
			if err != nil && c.err == nil {
				t.Fatalf("load error: %v", err)
			}

			if err == nil && c.err != nil {
				t.Fatalf("load wanted error %v, got nil", err)
			}

			if got, want := err, c.err; got != nil && got.Error() != want.Error() {
				t.Fatalf("load errors mismatch: got %v, want %v", got, want)
			}

			// Check g.Paths
			if got, want := len(g.Paths), len(c.want.Paths); got != want {
				t.Logf("g.Paths: got %v, want %v", g.Paths, c.want.Paths)
				t.Fatalf("len(g.Paths) mismatch: got %d, want %d", got, want)
			}

			for k, v := range g.Paths {
				if got, want := v, c.want.Paths[k]; got != want {
					t.Fatalf("g.Paths mismatch at key %q: got %t, want %t", k, got, want)
				}
			}

			// Check g.FileNames
			if got, want := len(g.FileNames), len(c.want.FileNames); got != want {
				t.Logf("g.FileNames: got %v, want %v", g.FileNames, c.want.FileNames)
				t.Fatalf("len(g.FileNames) mismatch: got %d, want %d", got, want)
			}

			for k, v := range g.FileNames {
				if got, want := v, c.want.FileNames[k]; got != want {
					t.Fatalf("g.FileNames mismatch at key %q: got %t, want %t", k, got, want)
				}
			}

			// Check g.PathPrefixes
			if got, want := len(g.PathPrefixes), len(c.want.PathPrefixes); got != want {
				t.Logf("g.PathPrefixes: got %v, want %v", g.PathPrefixes, c.want.PathPrefixes)
				t.Fatalf("len(g.PathPrefixes) mismatch: got %d, want %d", got, want)
			}

			for k, v := range g.PathPrefixes {
				if got, want := v, c.want.PathPrefixes[k]; got != want {
					t.Fatalf("g.PathPrefixes mismatch at key %q: got %t, want %t", k, got, want)
				}
			}

			// Check g.FilePrefixes
			if got, want := len(g.FilePrefixes), len(c.want.FilePrefixes); got != want {
				t.Logf("g.FilePrefixes: got %v, want %v", g.FilePrefixes, c.want.FilePrefixes)
				t.Fatalf("len(g.FilePrefixes) mismatch: got %d, want %d", got, want)
			}

			for k, v := range g.FilePrefixes {
				if got, want := v, c.want.FilePrefixes[k]; got != want {
					t.Fatalf("g.FilePrefixes mismatch at key %q: got %t, want %t", k, got, want)
				}
			}
		})
	}
}

func TestGroupLoadPaths(t *testing.T) {
	cases := []struct {
		name string
		src  string
		err  error

		// Assume that the Group started empty.
		want Group
	}{
		{
			name: "k8s/docs",
			src: `# first 3 lines of kubernetes/docs/.generated_docs
docs/.generated_docs
docs/admin/cloud-controller-manager.md
docs/admin/federation-apiserver.md
# ...`,
			err: nil,

			want: Group{
				Paths: map[string]bool{
					"docs/.generated_docs":                   true,
					"docs/admin/cloud-controller-manager.md": true,
					"docs/admin/federation-apiserver.md":     true,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := &Group{
				Paths:        make(map[string]bool),
				FileNames:    make(map[string]bool),
				PathPrefixes: make(map[string]bool),
				FilePrefixes: make(map[string]bool),
			}

			if got, want := g.loadPaths(bytes.NewBufferString(c.src)), c.err; got != nil && want != nil && got.Error() != want.Error() {
				t.Fatalf("g.loadPaths: got %s, want %s", got, want)
			}

			// Check g.Paths
			if got, want := len(g.Paths), len(c.want.Paths); got != want {
				t.Logf("g.Paths: got %v, want %v", g.Paths, c.want.Paths)
				t.Fatalf("len(g.Paths) mismatch: got %d, want %d", got, want)
			}

			for k, v := range g.Paths {
				if got, want := v, c.want.Paths[k]; got != want {
					t.Fatalf("g.Paths mismatch at key %q: got %t, want %t", k, got, want)
				}
			}
		})
	}
}

func TestGroupMatch(t *testing.T) {
	group := &Group{
		Paths: map[string]bool{
			"foo":     true,
			"bar/":    true,
			"foo/bar": true,
		},
		PathPrefixes: map[string]bool{
			"kubernetes/generated": true,
		},
		FileNames: map[string]bool{
			"generated.txt": true,
		},
		FilePrefixes: map[string]bool{
			"mygen": true,
		},
	}

	cases := []struct {
		path  string
		match bool
	}{
		// Intend to test group.Paths
		{},
		{path: "foo", match: true},
		{path: "foo/bar", match: true},
		{path: "foo/baz", match: false},

		// Intend to test group.PathPrefixes
		{path: "kubernetes/generated/one.txt", match: true},
		{path: "kubernetes/generated/two.txt", match: true},
		{path: "kubernetes/not_generated.txt", match: false},

		// Intend to test group.FileNames
		{path: "kubernetes/generated.txt", match: true},
		{path: "generated.txt", match: true},
		{path: "/any/old/path/generated.txt", match: true},
		{path: "/everywhere/generated.txt", match: true},
		{path: "/and/nowhere/generated.txt", match: true},
		{path: "/and/nowhere/NOT_generated.txt", match: false},

		// Intend to test group.FilePrefixes
		{path: "mygen.proto", match: true},
		{path: "/any/path/mygen.go", match: true},
		{path: "/any/mygenWithAName.go", match: true},
		{path: "notmygen.go", match: false},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got, want := group.Match(c.path), c.match; got != want {
				t.Fatalf("group.Match: got %t, want %t", got, want)
			}
		})
	}
}

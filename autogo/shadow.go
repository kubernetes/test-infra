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

// autogo creates a shadow copy of the go packages at origin in a destination.
//
// In other words this program will walk the directory tree at origin
// and for each:
// * directory - create a directory with the same name in destination
// * go-related-file - link the file into the matching destination directory
//
// The effect is similar to:
//   rsync -zarv --include="*/" --include="*.sh" --exclude="*" "$from" "$to"
// TODO(fejta): investigate just using rsync?
//
// The intended use case of this program is with the autogo_generate in //autogo:def.bzl. This rule will clone your primary workspace into an autogo workspace, and then run gazelle to generate rules for go packages.
//
// Usage:
//   autogo -- <ORIGIN_DIR> <DEST_DIR>
//
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// main calls shadowClone and exits non-zero on failure
func main() {
	if n := len(os.Args); n != 3 {
		log.Printf("Expected 2 args, not %d", n)
		log.Fatalf("Usage: %s <ORIGIN> <DEST>", filepath.Base(os.Args[0]))
	}
	if err := shadowClone(os.Args[1], os.Args[2]); err != nil {
		log.Fatalf("Failed to clone %s to %s: %v", os.Args[1], os.Args[2], err)
	}
}

// shadowClone walks origin to clone the directory structure and link files at the destination.
func shadowClone(origin, dest string) error {
	v := visitor{
		origin: origin,
		dest:   dest,
	}
	return filepath.Walk(v.origin, v.visit)
}

// action does something with the current file (mkdir or link)
type action func(origin string, info os.FileInfo, dest string) error

// visitor stores the origin we're cloning from and destination we clone to.
type visitor struct {
	// origin is the path we read to determine what to clone
	origin string
	// dest is where we clone
	dest string
}

// visit chooses and then performs the right action for the current path
func (v visitor) visit(path string, info os.FileInfo, verr error) error {
	act, dest, err := v.choose(path, info, verr)
	if err != nil {
		return err
	}
	if act == nil {
		return nil
	}
	return act(path, info, dest)
}

// choose looks at the current path and returns the appropriate action:
// - testdata directory => skip it and everything under it
// - directory => create the directory
// - go file (.go, .s) => create it at dest
// - other files => do nothing
// TODO(fejta): consider including .proto in the mix
func (v visitor) choose(path string, info os.FileInfo, verr error) (action, string, error) {
	if verr != nil {
		return nil, "", fmt.Errorf("failed to walk to %s: %v", path, verr)
	}

	// If origin is /foo/bar and we receive /foo/bar/something/special
	// Change this to something/special
	r, err := filepath.Rel(v.origin, path)
	// First ensure the path is a child of origin
	if err != nil {
		return nil, "", fmt.Errorf("%q is not relative to %q: %v", path, v.origin, err)
	}
	if r == ".." || strings.HasPrefix(r, "../") {
		return nil, "", fmt.Errorf("%q is not a child of %q", path, v.origin)
	}

	// TODO(fejta): handle symlink loops better
	// Many repos (dep is one) have testdata folders which are
	// either symlink loops back to some other part of the repo
	// or else intentionally do not compile.
	//
	// The gazelle tool is not robust to packages containing files that fail to compile.
	//
	// For now just ignore these testdata folders (usually in vendored packages). This makes bazel build work at the cost of breaking bazel test
	if info.IsDir() && strings.HasSuffix(path, "/testdata") {
		log.Printf("Skipping %s...", path)
		return nil, "", filepath.SkipDir
	}

	d := filepath.Join(v.dest, r)
	// Create dirs
	if info.IsDir() {
		return mkdir, d, nil
	}

	if strings.Contains(path, "/testdata/") {
		return nil, "", fmt.Errorf("%s is within a testdata dir, which should not happen", path)
	}

	// Ignore files irrelevant to go
	if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".s") {
		// Assume .go and .s are the only relevant files to a build
		// TODO(fejta): consider adding BUILD and BUILD.bazel files, or at least the go rules within them
		return nil, "", nil
	}

	// Link in golang files
	return link, d, nil
}

// mkdir creates directories for dest
func mkdir(_ string, info os.FileInfo, dest string) error {
	if err := os.MkdirAll(dest, info.Mode()); err != nil {
		return fmt.Errorf("failed to create %q: %v", dest, err)
	}
	return nil
}

// link clones source at dest
func link(source string, _ os.FileInfo, dest string) error {
	// First try a hard link
	if err := os.Link(source, dest); err == nil {
		return nil
	}

	// If that fails require a symlink
	return os.Symlink(source, dest)
}

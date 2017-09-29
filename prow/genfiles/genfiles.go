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

// Package genfiles understands the .generated_files config file.
// The ".generated_files" config lives in the repo's root.
//
// The config is a series of newline-delimited statements. Statements which
// begin with a `#` are ignored. A statement is a white-space delimited
// key-value tuple.
//
//		statement = key val
//
// where whitespace is ignored, and:
//
//		key = "path" | "file-name" | "path-prefix" |
//		"file-prefix" | "paths-from-repo"
//
// For example:
//
//		# Simple generated files config
//		file-prefix	zz_generated.
//		file-name	generated.pb.go
//
// The statement's `key` specifies the type of the corresponding value:
//  - "path": exact path to a single file
//  - "file-name": exact leaf file name, regardless of path
//  - "path-prefix": prefix match on the file path
//  - "file-prefix": prefix match of the leaf filename (no path)
//  - "paths-from-repo": load file paths from a file in repo
package genfiles

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"k8s.io/test-infra/prow/github"
)

const genConfigFile = ".generated_files"

// ghFileClient scopes to the only relevant functionality we require of a github client.
type ghFileClient interface {
	GetFile(org, repo, filepath, commit string) ([]byte, error)
}

// Group is a logical collection of files. Check for a file's
// inclusion in the group using the Match method.
type Group struct {
	Paths, FileNames, PathPrefixes, FilePrefixes map[string]bool
}

// NewGroup reads the .generated_files file in the root of the repository
// and any referenced path files (from "path-from-repo" commands).
func NewGroup(gc ghFileClient, owner, repo, sha string) (*Group, error) {
	g := &Group{
		Paths:        make(map[string]bool),
		FileNames:    make(map[string]bool),
		PathPrefixes: make(map[string]bool),
		FilePrefixes: make(map[string]bool),
	}

	bs, err := gc.GetFile(owner, repo, genConfigFile, sha)
	if err != nil {
		switch err.(type) {
		case *github.FileNotFound:
			return g, nil
		default:
			return nil, fmt.Errorf("could not get .generated_files: %v", err)
		}
	}

	repoFiles, err := g.load(bytes.NewBuffer(bs))
	if err != nil {
		return nil, err
	}
	for _, f := range repoFiles {
		bs, err = gc.GetFile(owner, repo, f, sha)
		if err != nil {
			return nil, err
		}
		if err = g.loadPaths(bytes.NewBuffer(bs)); err != nil {
			return nil, err
		}
	}

	return g, nil
}

// Use load to read a generated files config file, and populate g with the commands.
// "paths-from-repo" commands are aggregated into repoPaths. It is the caller's
// responsibility to fetch these and load them via g.loadPaths.
func (g *Group) load(r io.Reader) ([]string, error) {
	var repoPaths []string
	s := bufio.NewScanner(r)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || l[0] == '#' {
			// Ignore comments and empty lines.
			continue
		}

		fs := strings.Fields(l)
		if len(fs) != 2 {
			return repoPaths, &ParseError{line: l}
		}

		switch fs[0] {
		case "prefix", "path-prefix":
			g.PathPrefixes[fs[1]] = true
		case "file-prefix":
			g.FilePrefixes[fs[1]] = true
		case "file-name":
			g.FileNames[fs[1]] = true
		case "path":
			g.FileNames[fs[1]] = true
		case "paths-from-repo":
			// Despite the name, this command actually requires a file
			// of paths from the _same_ repo in which the .generated_files
			// config lives.
			repoPaths = append(repoPaths, fs[1])
		default:
			return repoPaths, &ParseError{line: l}
		}
	}

	if err := s.Err(); err != nil {
		return repoPaths, err
	}

	return repoPaths, nil
}

// Use loadPaths to load a file of new-line delimited paths, such as
// resolving file data referenced in a "paths-from-repo" command.
func (g *Group) loadPaths(r io.Reader) error {
	s := bufio.NewScanner(r)

	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || l[0] == '#' {
			// Ignore comments and empty lines.
			continue
		}

		g.Paths[l] = true
	}

	if err := s.Err(); err != nil {
		return fmt.Errorf("scan error: %v", err)
	}

	return nil
}

// Match determines whether a file, given here by its full path
// is included in the generated files group.
func (g *Group) Match(path string) bool {
	if g.Paths[path] {
		return true
	}

	for prefix := range g.PathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	base := filepath.Base(path)

	if g.FileNames[base] {
		return true
	}

	for prefix := range g.FilePrefixes {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}

	return false
}

// ParseError is an invalid line in a .generated_files config.
type ParseError struct {
	line string
}

func (pe *ParseError) Error() string {
	return fmt.Sprintf("invalid config line: %q", pe.line)
}

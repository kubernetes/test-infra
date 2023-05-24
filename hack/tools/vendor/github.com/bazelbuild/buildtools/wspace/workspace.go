/*
Copyright 2016 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package wspace provides a method to find the root of the bazel tree.
package wspace

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/buildtools/build"
)

const workspaceFile = "WORKSPACE"
const buildFile = "BUILD"

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeType == 0
}

func isExecutable(fi os.FileInfo) bool {
	return isFile(fi) && fi.Mode()&0100 == 0100
}

var repoRootFiles = map[string]func(os.FileInfo) bool{
	workspaceFile:            isFile,
	workspaceFile + ".bazel": isFile,
	".buckconfig":            isFile,
	"pants":                  isExecutable,
}

var packageRootFiles = map[string]func(os.FileInfo) bool{
	buildFile:            isFile,
	buildFile + ".bazel": isFile,
}

// findContextPath finds the context path inside of a WORKSPACE-rooted source tree.
func findContextPath(rootDir string) (string, error) {
	if rootDir == "" {
		return os.Getwd()
	}
	return rootDir, nil
}

// FindWorkspaceRoot splits the current code context (the rootDir if present,
// the working directory if not.) It returns the path of the directory
// containing the WORKSPACE file, and the rest.
func FindWorkspaceRoot(rootDir string) (root string, rest string) {
	wd, err := findContextPath(rootDir)
	if err != nil {
		return "", ""
	}
	if root, err = find(wd, repoRootFiles); err != nil {
		return "", ""
	}
	if len(wd) == len(root) {
		return root, ""
	}
	return root, wd[len(root)+1:]
}

// find searches from the given dir and up for the file that satisfies a condition of `rootFiles`
// returning the directory containing it, or an error if none found in the tree.
func find(dir string, rootFiles map[string]func(os.FileInfo) bool) (string, error) {
	if dir == "" || dir == "/" || dir == "." || (len(dir) == 3 && strings.HasSuffix(dir, ":\\")) {
		return "", os.ErrNotExist
	}
	for repoRootFile, fiFunc := range rootFiles {
		if fi, err := os.Stat(filepath.Join(dir, repoRootFile)); err == nil && fiFunc(fi) {
			return dir, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return find(filepath.Dir(dir), rootFiles)
}

// FindRepoBuildFiles parses the WORKSPACE to find BUILD files for non-Bazel
// external repositories, specifically those defined by one of these rules:
//   new_local_repository(), new_git_repository(), new_http_archive()
func FindRepoBuildFiles(root string) (map[string]string, error) {
	ws := filepath.Join(root, workspaceFile)
	kinds := []string{
		"new_local_repository",
		"new_git_repository",
		"new_http_archive",
	}
	data, err := ioutil.ReadFile(ws)
	if err != nil {
		return nil, err
	}
	ast, err := build.Parse(ws, data)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string)
	for _, kind := range kinds {
		for _, r := range ast.Rules(kind) {
			buildFile := r.AttrString("build_file")
			if buildFile == "" {
				continue
			}
			buildFile = strings.Replace(buildFile, ":", "/", -1)
			files[r.Name()] = filepath.Join(root, buildFile)
		}
	}
	return files, nil
}

// relPath returns a path for `target` relative to `base`, but an empty string
// instead of "." if the directories are equivalent, and with forward slashes.
func relPath(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", nil
	}
	return strings.ReplaceAll(rel, string(os.PathSeparator), "/"), nil
}

// SplitFilePath splits a file path into the workspace root, package name and label.
// Workspace root is determined as the last directory in the file path that
// contains a WORKSPACE (or WORKSPACE.bazel) file.
// Package and label are always separated with forward slashes.
// Returns empty strings if no WORKSPACE file is found.
func SplitFilePath(filename string) (workspaceRoot, pkg, label string) {
	dir := filepath.Dir(filename)
	workspaceRoot, err := find(dir, repoRootFiles)
	if err != nil {
		return "", "", ""
	}
	packageRoot, err := find(dir, packageRootFiles)
	if err != nil || !strings.HasPrefix(packageRoot, workspaceRoot) {
		// No BUILD file or it's outside of the workspace. Shouldn't happen,
		// but assume it's in the workspace root.
		packageRoot = workspaceRoot
	}
	pkg, err = relPath(workspaceRoot, packageRoot)
	if err != nil {
		return "", "", ""
	}
	label, err = relPath(packageRoot, filename)
	if err != nil {
		return "", "", ""
	}
	return workspaceRoot, pkg, label
}

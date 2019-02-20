/*
Copyright 2019 The Kubernetes Authors.

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

package shim

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// FindDeployer locates the binary implementing the named deployer
// TODO(bentheelder): move this to another package?
func FindDeployer(name string) (path string, err error) {
	binary := fmt.Sprintf("%s-%s", BinaryName, name)
	path, err = exec.LookPath(binary)
	if err != nil {
		return "", errors.Errorf("%#v not found in PATH, could not locate %#v deployer", binary, name)
	}
	return path, err
}

// FindDeployers looks for all deployers in PATH, returning a map of the
// deployer name to the first matching binary found in path
func FindDeployers() map[string]string {
	nameToPath := make(map[string]string)
	prefix := fmt.Sprintf("%s-", BinaryName)

	// search every directory in PATH for kubetest2-* binaries
	searchPaths := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range searchPaths {
		// mimic LookPath() for nicer results
		if dir == "" {
			// Unix shell semantics: path element "" means "."
			dir = "."
		}

		// list all files in the directory
		files, err := ioutil.ReadDir(dir)

		// ignore bad directories in PATH
		if os.IsNotExist(err) {
			continue
		}

		// check every file in the directory against the prefix
		for _, f := range files {
			// ignore directories
			if f.IsDir() {
				continue
			}
			// ensure the prefix matches
			fileName := f.Name()
			if !strings.HasPrefix(fileName, prefix) {
				continue
			}
			// convert the file name to a deployer name
			// TODO(bentheelder): handle PATHEXT on windows
			name := strings.TrimPrefix(fileName, prefix)
			// only keep the first result
			if _, foundAlready := nameToPath[name]; foundAlready {
				continue
			}
			// use FindDeployer / LookPath to ensure consistency
			path, err := FindDeployer(name)
			if err != nil {
				continue
			}
			nameToPath[name] = path
		}
	}
	return nameToPath
}

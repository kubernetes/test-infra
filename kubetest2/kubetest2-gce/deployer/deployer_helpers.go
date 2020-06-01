/*
Copyright 2020 The Kubernetes Authors.

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

package deployer

import (
	"fmt"
	"os"
)

func (d *deployer) setRepoPathIfNotSet() error {
	if d.repoRootPath != "" {
		return nil
	}

	path, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Failed to get current working directory for setting Kubernetes root path: %s", err)
	}
	d.repoRootPath = path

	return nil
}

// verifyBuildFlags only checks flags that are needed for Build
func (d *deployer) verifyBuildFlags() error {
	return d.setRepoPathIfNotSet()
}

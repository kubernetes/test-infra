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

	"k8s.io/klog"
	"k8s.io/test-infra/kubetest2/pkg/exec"
)

func (d *deployer) Build() error {
	klog.Info("GCE deployer starting Build()")

	if err := d.verifyBuildFlags(); err != nil {
		return fmt.Errorf("Build() failed to verify build-specific flags: %s", err)
	}

	// this code path supports the kubernetes/cloud-provider-gcp build
	// TODO: update in future patch to support legacy (k/k) build
	cmd := exec.Command("bazel", "build", "//release:release-tars")
	cmd.SetDir(d.RepoRoot)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error during make step of build: %s", err)
	}

	// no untarring, uploading, etc is necessary because
	// kube-up/down use find-release-tars and upload-tars
	// which know how to find the tars, assuming KUBE_ROOT
	// is set

	return nil
}

func (d *deployer) setRepoPathIfNotSet() error {
	if d.RepoRoot != "" {
		return nil
	}

	path, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Failed to get current working directory for setting Kubernetes root path: %s", err)
	}
	klog.Infof("defaulting repo root to the current directory: %s", path)
	d.RepoRoot = path

	return nil
}

// verifyBuildFlags only checks flags that are needed for Build
func (d *deployer) verifyBuildFlags() error {
	return d.setRepoPathIfNotSet()
}

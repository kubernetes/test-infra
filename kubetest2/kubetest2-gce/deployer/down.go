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
	"path/filepath"

	"k8s.io/klog"
	"k8s.io/test-infra/kubetest2/pkg/exec"
)

func (d *deployer) Down() error {
	klog.Info("GCE deployer starting Down()")

	if err := d.init(); err != nil {
		return fmt.Errorf("down failed to init: %s", err)
	}

	env := d.buildEnv()
	script := filepath.Join(d.RepoRoot, "cluster", "kube-down.sh")
	klog.Infof("About to run script at: %s", script)

	cmd := exec.Command(script)
	cmd.SetEnv(env...)
	exec.InheritOutput(cmd)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error encountered during %s: %s", script, err)
	}

	if d.boskos != nil {
		if err := d.releaseBoskosProject(); err != nil {
			return fmt.Errorf("down failed to release boskos project: %s", err)
		}
	}

	return nil
}

func (d *deployer) releaseBoskosProject() error {
	if err := d.boskos.Release(d.boskosProject.Name, "free"); err != nil {
		return fmt.Errorf("failed to release %s: %s", d.boskosProject.Name, err)
	}
	return nil
}

func (d *deployer) verifyDownFlags() error {
	if err := d.setRepoPathIfNotSet(); err != nil {
		return err
	}

	d.kubectl = filepath.Join(d.RepoRoot, "cluster", "kubectl.sh")

	if d.GCPProject == "" {
		return fmt.Errorf("gcp project must be set")
	}

	return nil
}

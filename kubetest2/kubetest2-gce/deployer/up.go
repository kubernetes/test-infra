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
	"path/filepath"

	"k8s.io/klog"
	"k8s.io/test-infra/kubetest2/pkg/exec"
)

func (d *deployer) Up() error {
	klog.Info("GCE deployer starting Up()")

	if err := d.verifyFlags(); err != nil {
		return fmt.Errorf("up failed to verify flags: %s", err)
	}

	if err := enableComputeAPI(d.GCPProject); err != nil {
		return fmt.Errorf("up couldn't enable compute API: %s", err)
	}

	defer func() {
		if err := d.DumpClusterLogs(); err != nil {
			klog.Warningf("Dumping cluster logs at the end of Up() failed: %s", err)
		}
	}()

	env := d.buildEnv()
	script := filepath.Join(d.RepoRoot, "cluster", "kube-up.sh")
	klog.Infof("About to run script at: %s", script)

	cmd := exec.Command(script)
	cmd.SetEnv(env...)
	exec.InheritOutput(cmd)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error encountered during %s: %s", script, err)
	}

	isUp, err := d.IsUp()
	if err != nil {
		klog.Warningf("failed to check if cluster is up: %s", err)
	} else if isUp {
		klog.Infof("cluster reported as up")
	} else {
		klog.Errorf("cluster reported as down")
	}

	return nil
}

func (d *deployer) buildEnv() []string {
	// The base env currently does not inherit the current os env (except for PATH)
	// because (for now) it doesn't have to. In future, this may have to change when
	// support is added for k/k's kube-up.sh and kube-down.sh which support a wide
	// variety of environment variables. Before doing so, it is worth investigating
	// inheriting the os env vs. adding flags to this deployer on a case-by-case
	// basis to support individual environment configurations.
	var env []string

	// path is necessary for scripts to find gsutil, gcloud, etc
	// can be removed if env is inherited from the os
	env = append(env, fmt.Sprintf("PATH=%s", os.Getenv("PATH")))

	// used by config-test.sh to set $NETWORK in the default case
	// if unset, bash's set -u gets angry and kills the log dump script
	// can be removed if env is inherited from the os
	env = append(env, fmt.Sprintf("USER=%s", os.Getenv("USER")))

	// kube-up.sh, kube-down.sh etc. use PROJECT as a parameter for all gcloud commands
	env = append(env, fmt.Sprintf("PROJECT=%s", d.GCPProject))

	// kubeconfig is set to tell kube-up.sh where to generate the kubeconfig
	// we don't want this to be the default because this kubeconfig "belongs" to
	// the run of kubetest2 and so should be placed in the artifacts directory
	env = append(env, fmt.Sprintf("KUBECONFIG=%s", d.kubeconfigPath))

	// kube-up and kube-down get this as a default ("kubernetes") but log-dump
	// does not. opted to set it manually here for maximum consistency
	env = append(env, "KUBE_GCE_INSTANCE_PREFIX=kubetest2")
	return env
}

func (d *deployer) verifyFlags() error {
	if err := d.setRepoPathIfNotSet(); err != nil {
		return err
	}

	d.kubectl = filepath.Join(d.RepoRoot, "cluster", "kubectl.sh")

	if d.GCPProject == "" {
		return fmt.Errorf("gcp project must be set")
	}

	return nil
}

func enableComputeAPI(project string) error {
	// In freshly created GCP projects, the compute API is
	// not enabled. We need it. Enabling it after it has
	// already been enabled is a relatively fast no-op,
	// so this can be called without consequence.

	env := os.Environ()
	cmd := exec.Command(
		"gcloud",
		"services",
		"enable",
		"compute.googleapis.com",
		"--project="+project,
	)
	cmd.SetEnv(env...)
	exec.InheritOutput(cmd)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to enable compute API: %s", err)
	}

	return nil
}

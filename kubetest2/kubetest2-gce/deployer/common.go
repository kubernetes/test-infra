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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/klog"
	"sigs.k8s.io/boskos/client"
	boskosCommon "sigs.k8s.io/boskos/common"
)

func (d *deployer) init() error {
	if d.commonOptions.ShouldBuild() {
		if err := d.verifyBuildFlags(); err != nil {
			return fmt.Errorf("init failed to check build flags: %s", err)
		}
	}

	if d.commonOptions.ShouldUp() {
		if d.GCPProject == "" {
			klog.Info("No GCP project provided, acquiring from Boskos")

			if err := d.getProjectFromBoskos(); err != nil {
				return fmt.Errorf("init failed to get project from boskos: %s", err)
			}
			klog.Infof("Got project %s from boskos", d.GCPProject)
		}

		if err := d.verifyFlags(); err != nil {
			return fmt.Errorf("init failed to verify flags for up: %s", err)
		}
	}

	if d.commonOptions.ShouldDown() {
		if err := d.verifyFlags(); err != nil {
			return fmt.Errorf("init failed to verify flags for up: %s", err)
		}
	}

	return nil
}

// getProjectFromBoskos creates a boskos client, acquires a gcp project
// and starts a heartbeat goroutine to keep the project reserved
func (d *deployer) getProjectFromBoskos() error {
	// TODO
	// boskosLocation := "http://boskos.test-pods.svc.cluster.local."
	boskosLocation := "http://localhost:8080"
	boskos, err := client.NewClient(
		os.Getenv("JOB_NAME")+"-kubetest2",
		boskosLocation,
		"",
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to create boskos client: %s", err)
	}

	resourceType := "gce-project"
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	boskosProject, err := boskos.AcquireWait(ctx, resourceType, "free", "busy")
	if err != nil {
		return fmt.Errorf("failed to get a %s from boskos: %s", resourceType, err)
	}
	if boskosProject == nil {
		return fmt.Errorf("boksos had no %s available", resourceType)
	}

	go func(c *client.Client, resource *boskosCommon.Resource) {
		for range time.Tick(time.Minute * 5) {
			if err := c.UpdateOne(resource.Name, "busy", nil); err != nil {
				klog.Warningf("[Boskos] Update of %s failed with %v", resource.Name, err)
			}
		}
	}(boskos, boskosProject)

	d.boskos = boskos
	d.boskosProject = boskosProject
	d.GCPProject = boskosProject.Name

	return nil
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

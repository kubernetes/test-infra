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
	"k8s.io/test-infra/kubetest2/pkg/exec"
	"sigs.k8s.io/boskos/client"
	boskosCommon "sigs.k8s.io/boskos/common"
)

func (d *deployer) Up() error {
	klog.Info("GCE deployer starting Up()")

	if err := d.init(); err != nil {
		return fmt.Errorf("up failed to init: %s", err)
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

// getProjectFromBoskos creates a boskos client, acquires a gcp project
// and starts a heartbeat goroutine to keep the project reserved
func (d *deployer) getProjectFromBoskos() error {
	boskos, err := client.NewClient(
		os.Getenv("JOB_NAME")+"-kubetest2",
		d.BoskosLocation,
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
		return fmt.Errorf("failed to get a %q from boskos: %s", resourceType, err)
	}
	if boskosProject == nil {
		return fmt.Errorf("boskos had no %s available", resourceType)
	}

	startBoskosHeartbeat(
		boskos,
		boskosProject,
		time.Duration(d.BoskosAcquireTimeoutSeconds)*time.Second,
		d.boskosHeartbeatClose,
	)

	d.boskos = boskos
	d.boskosProject = boskosProject
	d.GCPProject = boskosProject.Name

	return nil
}

// startBoskosHeartbeat starts a goroutine that sends periodic updates to boskos
// about the provided resource until the channel is closed. This prevents
// boskos from taking the resource from the deployer while it is still in use.
func startBoskosHeartbeat(c *client.Client, resource *boskosCommon.Resource, interval time.Duration, close chan struct{}) {
	go func(c *client.Client, resource *boskosCommon.Resource) {
		for {
			select {
			case <-close:
				klog.Info("Boskos heartbeat func received signal to close")
				return
			case <-time.Tick(interval):
				klog.Info("Sending heartbeat to Boskos")
				if err := c.UpdateOne(resource.Name, "busy", nil); err != nil {
					klog.Warningf("[Boskos] Update of %s failed with %v", resource.Name, err)
				}
			}
		}
	}(c, resource)
}

func (d *deployer) verifyUpFlags() error {
	if err := d.setRepoPathIfNotSet(); err != nil {
		return err
	}

	d.kubectlPath = filepath.Join(d.RepoRoot, "cluster", "kubectl.sh")

	// verifyUpFlags does not check for a gcp project because it is
	// assumed that one will be acquired from boskos if it is not set

	return nil
}

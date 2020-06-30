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
	"time"

	"k8s.io/klog"
	"sigs.k8s.io/boskos/client"
	boskosCommon "sigs.k8s.io/boskos/common"
)

// const (for the run) owner string for consistency between up and down
var boskosOwner = os.Getenv("JOB_NAME") + "-kubetest2"

func makeBoskosClient(boskosLocation string) (*client.Client, error) {
	boskos, err := client.NewClient(
		boskosOwner,
		boskosLocation,
		"",
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create boskos client: %s", err)
	}

	return boskos, nil
}

// getProjectFromBoskos creates a boskos client, acquires a gcp project
// and starts a heartbeat goroutine to keep the project reserved
func getProjectFromBoskos(boskosClient *client.Client, timeout time.Duration, heartbeatClose chan struct{}) (string, error) {
	resourceType := "gce-project"
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	boskosProject, err := boskosClient.AcquireWait(ctx, resourceType, "free", "busy")
	if err != nil {
		return "", fmt.Errorf("failed to get a %q from boskos: %s", resourceType, err)
	}
	if boskosProject == nil {
		return "", fmt.Errorf("boskos had no %s available", resourceType)
	}

	startBoskosHeartbeat(
		boskosClient,
		boskosProject,
		5*time.Minute,
		heartbeatClose,
	)

	return boskosProject.Name, nil
}

// startBoskosHeartbeat starts a goroutine that sends periodic updates to boskos
// about the provided resource until the channel is closed. This prevents
// reaper from taking the resource from the deployer while it is still in use.
func startBoskosHeartbeat(boskosClient *client.Client, resource *boskosCommon.Resource, interval time.Duration, close chan struct{}) {
	go func(c *client.Client, resource *boskosCommon.Resource) {
		klog.V(2).Info("boskos hearbeat starting")

		for {
			select {
			case <-close:
				klog.V(2).Info("Boskos heartbeat func received signal to close")
				return
			case <-time.Tick(interval):
				klog.V(2).Info("Sending heartbeat to Boskos")
				if err := c.UpdateOne(resource.Name, "busy", nil); err != nil {
					klog.Warningf("[Boskos] Update of %s failed with %v", resource.Name, err)
				}
			}
		}
	}(boskosClient, resource)
}

func releaseBoskosProject(client *client.Client, projectName string, heartbeatClose chan struct{}) error {
	if err := client.Release(projectName, "free"); err != nil {
		return fmt.Errorf("failed to release %s: %s", projectName, err)
	}

	close(heartbeatClose)

	return nil
}

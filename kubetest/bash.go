/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"os"
	"os/exec"
	"strconv"
)

type bashDeployer struct {
	clusterIPRange *string
}

var _ deployer = &bashDeployer{}

func newBash(clusterIPRange *string) *bashDeployer {
	b := &bashDeployer{clusterIPRange}
	return b
}

func (b *bashDeployer) Up() error {
	if b.clusterIPRange == nil || *b.clusterIPRange == "" {
		if numNodes, err := strconv.Atoi(os.Getenv("NUM_NODES")); err == nil {
			*b.clusterIPRange = getClusterIPRange(numNodes)
		}
	}
	if *b.clusterIPRange != "" {
		pop, err := pushEnv("CLUSTER_IP_RANGE", *b.clusterIPRange)
		if err != nil {
			return err
		}
		defer pop()
	}
	return finishRunning(exec.Command("./hack/e2e-internal/e2e-up.sh"))
}

func (b *bashDeployer) IsUp() error {
	return finishRunning(exec.Command("./hack/e2e-internal/e2e-status.sh"))
}

func (b *bashDeployer) DumpClusterLogs(localPath, gcsPath string) error {
	return defaultDumpClusterLogs(localPath, gcsPath)
}

func (b *bashDeployer) TestSetup() error {
	return nil
}

func (b *bashDeployer) Down() error {
	return finishRunning(exec.Command("./hack/e2e-internal/e2e-down.sh"))
}

// Calculates the cluster IP range based on the no. of nodes in the cluster.
// Note: This mimics the function get-cluster-ip-range used by kube-up script.
func getClusterIPRange(numNodes int) string {
	suggestedRange := "10.160.0.0/14"
	if numNodes > 1000 {
		suggestedRange = "10.160.0.0/13"
	}
	if numNodes > 2000 {
		suggestedRange = "10.160.0.0/12"
	}
	if numNodes > 4000 {
		suggestedRange = "10.160.0.0/11"
	}
	return suggestedRange
}

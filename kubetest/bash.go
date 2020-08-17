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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type bashDeployer struct {
	clusterIPRange          string
	gcpProject              string
	gcpZone                 string
	gcpSSHProxyInstanceName string
	provider                string
	nodeLoggingEnabled      bool
}

var _ deployer = &bashDeployer{}

var (
	bashNodeLogging = flag.Bool("bash-node-logging", false, "(bash only) enable node logging to gcp")
)

func newBash(clusterIPRange *string, gcpProject, gcpZone, gcpSSHProxyInstanceName, provider string) *bashDeployer {
	if *clusterIPRange == "" {
		if numNodes, err := strconv.Atoi(os.Getenv("NUM_NODES")); err == nil {
			*clusterIPRange = getClusterIPRange(numNodes)
		}
	}
	b := &bashDeployer{*clusterIPRange, gcpProject, gcpZone, gcpSSHProxyInstanceName, provider, *bashNodeLogging}
	return b
}

func (b *bashDeployer) Up() error {
	script := "./hack/e2e-internal/e2e-up.sh"
	cmd := exec.Command(script)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("CLUSTER_IP_RANGE=%s", b.clusterIPRange))
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBE_ENABLE_NODE_LOGGING=%t", b.nodeLoggingEnabled))
	return control.FinishRunning(cmd)
}

func (b *bashDeployer) IsUp() error {
	return control.FinishRunning(exec.Command("./hack/e2e-internal/e2e-status.sh"))
}

func (b *bashDeployer) DumpClusterLogs(localPath, gcsPath string) error {
	return defaultDumpClusterLogs(localPath, gcsPath)
}

func (b *bashDeployer) TestSetup() error {
	if b.provider == "gce" && b.gcpSSHProxyInstanceName != "" {
		if err := setKubeShhBastionEnv(b.gcpProject, b.gcpZone, b.gcpSSHProxyInstanceName); err != nil {
			return err
		}
	}
	return nil
}

func (b *bashDeployer) Down() error {
	return control.FinishRunning(exec.Command("./hack/e2e-internal/e2e-down.sh"))
}

func (b *bashDeployer) GetClusterCreated(gcpProject string) (time.Time, error) {
	res, err := control.Output(exec.Command(
		"gcloud",
		"compute",
		"instance-groups",
		"list",
		"--project="+gcpProject,
		"--format=json(name,creationTimestamp)"))
	if err != nil {
		return time.Time{}, fmt.Errorf("list instance-group failed : %v", err)
	}

	created, err := getLatestClusterUpTime(string(res))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time failed : got gcloud res %s, err %v", string(res), err)
	}
	return created, nil
}

func (b *bashDeployer) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

// Calculates the cluster IP range based on the no. of nodes in the cluster.
// Note: This mimics the function get-cluster-ip-range used by kube-up script.
func getClusterIPRange(numNodes int) string {
	suggestedRange := "10.64.0.0/14"
	if numNodes > 1000 {
		suggestedRange = "10.64.0.0/13"
	}
	if numNodes > 2000 {
		suggestedRange = "10.64.0.0/12"
	}
	if numNodes > 4000 {
		suggestedRange = "10.64.0.0/11"
	}
	return suggestedRange
}

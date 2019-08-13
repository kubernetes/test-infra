/*
Copyright 2018 The Kubernetes Authors.

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

// Package main / eks.go implements kubetest deployer interface for EKS.
// It uses 'aws-k8s-tester' and 'kubectl' binaries, rather than importing internal packages.
// All underlying implementation and external dependencies are compiled into one binary.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aws/aws-k8s-tester/eksconfig"
	"github.com/aws/aws-k8s-tester/ekstester"
	"k8s.io/test-infra/kubetest/process"
	"k8s.io/test-infra/kubetest/util"
)

var (
	eksKubectlPath      = flag.String("eks-kubectl-path", "/tmp/aws-k8s-tester/kubectl", "(eks only) Path to the kubectl binary to use.")
	eksKubecfgPath      = flag.String("eks-kubeconfig-path", "/tmp/aws-k8s-tester/kubeconfig", "(eks only) Path to the kubeconfig file to use.")
	eksNodes            = flag.String("eks-nodes", "1", "(eks only) Number of nodes in the EKS cluster.")
	eksNodeInstanceType = flag.String("eks-node-instance-type", "m3.xlarge", "(eks only) Instance type to use for nodes.")
)

func migrateEKSOptions() error {
	// Prevent ginkgo-e2e.sh from using the cluster/eks functions.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return err
	}
	if err := os.Setenv("KUBERNETES_CONFORMANCE_PROVIDER", "eks"); err != nil {
		return err
	}
	return util.MigrateOptions([]util.MigratedOption{
		// Env vars required by upstream ginkgo-e2e.sh.
		{
			Env:    "KUBECTL",
			Option: eksKubectlPath,
			Name:   "--eks-kubectl-path",
		},
		{
			Env:    "KUBECONFIG",
			Option: eksKubecfgPath,
			Name:   "--eks-kubeconfig-path",
		},
		// Env vars specific to aws-k8s-tester.
		{
			Env:    "AWS_K8S_TESTER_EKS_WORKER_NODE_ASG_MIN",
			Option: eksNodes,
			Name:   "--eks-nodes",
		},
		{
			Env:    "AWS_K8S_TESTER_EKS_WORKER_NODE_ASG_MAX",
			Option: eksNodes,
			Name:   "--eks-nodes",
		},
		{
			Env:    "AWS_K8S_TESTER_EKS_WORKER_NODE_INSTANCE_TYPE",
			Option: eksNodeInstanceType,
			Name:   "--eks-node-instance-type",
		},
	})
}

// eksDeployer implements EKS deployer interface using "aws-k8s-tester" binary.
// Satisfies "k8s.io/test-infra/kubetest/main.go" 'deployer' and 'publisher" interfaces.
// Reference https://github.com/kubernetes/test-infra/blob/master/kubetest/main.go.
type eksDeployer struct {
	stopc chan struct{}
	cfg   *eksconfig.Config
	ctrl  *process.Control
}

// newEKS creates a new EKS deployer.
func newEKS(timeout time.Duration, verbose bool) (ekstester.Deployer, error) {
	err := migrateEKSOptions()
	if err != nil {
		return nil, err
	}
	cfg := eksconfig.NewDefault()
	err = cfg.UpdateFromEnvs()
	if err != nil {
		return nil, err
	}
	var f *os.File
	f, err = ioutil.TempFile(os.TempDir(), "aws-k8s-tester-config")
	if err != nil {
		return nil, err
	}
	cfg.ConfigPath = f.Name()
	if err = f.Close(); err != nil {
		return nil, fmt.Errorf("failed to close aws-k8s-tester-config file %v", err)
	}
	if err = cfg.Sync(); err != nil {
		return nil, err
	}

	dp := &eksDeployer{
		stopc: make(chan struct{}),
		cfg:   cfg,
		ctrl: process.NewControl(
			timeout,
			time.NewTimer(timeout),
			time.NewTimer(timeout),
			verbose,
		),
	}
	if err := dp.fetchAWSK8sTester(); err != nil {
		return nil, fmt.Errorf("failed to fetch aws-k8s-tester: %v", err)
	}
	return dp, nil
}

// Up creates a new EKS cluster.
func (dp *eksDeployer) Up() (err error) {
	// "create cluster" command outputs cluster information
	// in the configuration file (e.g. VPC ID, ALB DNS names, etc.)
	// this needs be reloaded for other deployer method calls
	createCmd := osexec.Command(
		dp.cfg.AWSK8sTesterPath,
		"eks",
		"--path="+dp.cfg.ConfigPath,
		"create",
		"cluster",
	)
	errc := make(chan error)
	go func() {
		_, oerr := dp.ctrl.Output(createCmd)
		errc <- oerr
	}()
	select {
	case <-dp.stopc:
		fmt.Fprintln(os.Stderr, "received stop signal, interrupting 'create cluster' command...")
		ierr := createCmd.Process.Signal(syscall.SIGINT)
		err = fmt.Errorf("'create cluster' command interrupted (interrupt error %v)", ierr)
	case err = <-errc:
	}
	return err
}

// Down tears down the existing EKS cluster.
func (dp *eksDeployer) Down() (err error) {
	// reload configuration from disk to read the latest configuration
	if _, err = dp.LoadConfig(); err != nil {
		return err
	}
	_, err = dp.ctrl.Output(osexec.Command(
		dp.cfg.AWSK8sTesterPath,
		"eks",
		"--path="+dp.cfg.ConfigPath,
		"delete",
		"cluster",
	))
	return err
}

// IsUp returns an error if the cluster is not up and running.
func (dp *eksDeployer) IsUp() (err error) {
	// reload configuration from disk to read the latest configuration
	if _, err = dp.LoadConfig(); err != nil {
		return err
	}
	_, err = dp.ctrl.Output(osexec.Command(
		dp.cfg.AWSK8sTesterPath,
		"eks",
		"--path="+dp.cfg.ConfigPath,
		"check",
		"cluster",
	))
	if err != nil {
		return err
	}
	if _, err = dp.LoadConfig(); err != nil {
		return err
	}
	if dp.cfg.ClusterState.Status != "ACTIVE" {
		return fmt.Errorf("cluster %q status is %q",
			dp.cfg.ClusterName,
			dp.cfg.ClusterState.Status,
		)
	}
	return nil
}

// TestSetup checks if EKS testing cluster has been set up or not.
func (dp *eksDeployer) TestSetup() error {
	return dp.IsUp()
}

// GetClusterCreated returns EKS cluster creation time and error (if any).
func (dp *eksDeployer) GetClusterCreated(v string) (time.Time, error) {
	err := dp.IsUp()
	if err != nil {
		return time.Time{}, err
	}
	return dp.cfg.ClusterState.Created, nil
}

func (dp *eksDeployer) GetWorkerNodeLogs() (err error) {
	// reload configuration from disk to read the latest configuration
	if _, err = dp.LoadConfig(); err != nil {
		return err
	}
	_, err = dp.ctrl.Output(osexec.Command(
		dp.cfg.AWSK8sTesterPath,
		"eks",
		"--path="+dp.cfg.ConfigPath,
		"test", "get-worker-node-logs",
	))
	return err
}

// DumpClusterLogs dumps all logs to artifact directory.
// Let default kubetest log dumper handle all artifact uploads.
// See https://github.com/kubernetes/test-infra/pull/9811/files#r225776067.
func (dp *eksDeployer) DumpClusterLogs(artifactDir, _ string) (err error) {
	// reload configuration from disk to read the latest configuration
	if _, err = dp.LoadConfig(); err != nil {
		return err
	}
	_, err = dp.ctrl.Output(osexec.Command(
		dp.cfg.AWSK8sTesterPath,
		"eks",
		"--path="+dp.cfg.ConfigPath,
		"test", "get-worker-node-logs",
	))
	if err != nil {
		return err
	}
	_, err = dp.ctrl.Output(osexec.Command(
		dp.cfg.AWSK8sTesterPath,
		"eks",
		"--path="+dp.cfg.ConfigPath,
		"test", "dump-cluster-logs",
		artifactDir,
	))
	return err
}

// KubectlCommand returns "kubectl" command object for API reachability tests.
func (dp *eksDeployer) KubectlCommand() (*osexec.Cmd, error) {
	// reload configuration from disk to read the latest configuration
	if _, err := dp.LoadConfig(); err != nil {
		return nil, err
	}
	return osexec.Command(dp.cfg.KubectlPath, "--kubeconfig="+dp.cfg.KubeConfigPath), nil
}

// Stop stops ongoing operations.
// This is useful for local development.
// For example, one may run "Up" but have to cancel ongoing "Up"
// operation. Then, it can just send syscall.SIGINT to trigger "Stop".
func (dp *eksDeployer) Stop() {
	close(dp.stopc)
}

// LoadConfig reloads configuration from disk to read the latest
// cluster configuration and its states.
func (dp *eksDeployer) LoadConfig() (eksconfig.Config, error) {
	var err error
	dp.cfg, err = eksconfig.Load(dp.cfg.ConfigPath)
	return *dp.cfg, err
}

func getLatestAWSK8sTesterURL() (string, error) {
	resp, err := http.Get("https://github.com/aws/aws-k8s-tester/releases/latest")
	if err != nil {
		return "", err
	}
	redirectURL := resp.Request.URL.String()
	basepath, version := filepath.Split(redirectURL)
	if basepath == "" {
		return "", fmt.Errorf("Couldn't extract version from redirect URL")
	}
	return fmt.Sprintf("https://github.com/aws/aws-k8s-tester/releases/download/%s/aws-k8s-tester-%s-linux-amd64", version, version), nil
}

func (dp *eksDeployer) fetchAWSK8sTester() error {
	if err := os.RemoveAll(dp.cfg.AWSK8sTesterPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dp.cfg.AWSK8sTesterPath), 0700); err != nil {
		return err
	}
	f, err := os.Create(dp.cfg.AWSK8sTesterPath)
	if err != nil {
		return fmt.Errorf("failed to create %q (%v)", dp.cfg.AWSK8sTesterPath, err)
	}
	dp.cfg.AWSK8sTesterPath = f.Name()
	var awsK8sTesterDownloadURL string
	awsK8sTesterDownloadURL, err = getLatestAWSK8sTesterURL()
	if err != nil {
		return err
	}
	if err = httpRead(awsK8sTesterDownloadURL, f); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("failed to close aws-k8s-tester file %v", err)
	}
	if err = util.EnsureExecutable(dp.cfg.AWSK8sTesterPath); err != nil {
		return err
	}
	return nil
}

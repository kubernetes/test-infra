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

// Package conformance implements conformance test kubetest code.
package conformance

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/test-infra/kubetest/e2e"
	"k8s.io/test-infra/kubetest/process"
)

// Tester runs conformance tests against a given cluster.
type Tester struct {
	kubecfg   string
	ginkgo    string
	e2etest   string
	reportdir string
	testArgs  *string
	control   *process.Control
}

// BuildTester returns an object that knows how to test the cluster it deployed.
func (d *Deployer) BuildTester(o *e2e.BuildTesterOptions) (e2e.Tester, error) {
	reportdir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if o.FocusRegex == "" {
		o.FocusRegex = "\".*\""
	}
	if o.SkipRegex == "" {
		o.SkipRegex = "\".*(Feature)|(NFS)|(StatefulSet).*\""
	}

	t := e2e.NewGinkgoTester(o)

	t.Seed = 1436380640
	t.GinkgoParallel = 10
	t.Kubeconfig = d.kubecfg
	t.FlakeAttempts = 2
	t.NumNodes = 4
	t.SystemdServices = []string{"docker", "kubelet"}
	t.ReportDir = reportdir

	return t, nil
}

// Deployer returns a deployer stub that expects a cluster to already exist.
type Deployer struct {
	kubecfg   string
	apiserver *kubernetes.Clientset
}

// Deployer implements e2e.TestBuilder, overriding testing
var _ e2e.TestBuilder = &Deployer{}

// NewDeployer returns a new Deployer.
func NewDeployer(kubecfg string) (*Deployer, error) {
	// The easiest thing to do is just load the altereted kubecfg from the file we wrote.
	config, err := clientcmd.BuildConfigFromFlags("", kubecfg)
	if err != nil {
		return nil, err
	}
	apiserver, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Deployer{
		kubecfg:   kubecfg,
		apiserver: apiserver,
	}, nil
}

// Up synchronously starts a cluster, or times out.
func (d *Deployer) Up() error {
	return fmt.Errorf("cannot up a conformance cluster")
}

// IsUp returns nil if the apiserver is running, or the error received while checking.
func (d *Deployer) IsUp() error {
	_, err := d.isAPIServerUp()
	return err
}

func (d *Deployer) isAPIServerUp() (*v1.ComponentStatusList, error) {
	if d.apiserver == nil {
		return nil, fmt.Errorf("no apiserver client available")
	}
	//TODO(Q-Lee): check that relevant components have started. May consider checking addons.
	return d.apiserver.CoreV1().ComponentStatuses().List(context.TODO(), metav1.ListOptions{})
}

// DumpClusterLogs is a no-op.
func (d *Deployer) DumpClusterLogs(localPath, gcsPath string) error {
	return nil
}

// TestSetup is a no-op.
func (d *Deployer) TestSetup() error {
	return nil
}

// Down stops and removes the cluster container.
func (d *Deployer) Down() error {
	return fmt.Errorf("cannot down a conformance cluster")
}

// GetClusterCreated returns the start time of the cluster container. If the container doesn't exist, has no start time, or has a malformed start time, then an error is returned.
func (d *Deployer) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, fmt.Errorf("cannot get cluster create time for conformance cluster")
}

func (d *Deployer) KubectlCommand() (*exec.Cmd, error) {
	log.Print("Noop - Conformance KubectlCommand()")
	return nil, nil
}

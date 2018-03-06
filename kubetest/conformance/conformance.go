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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/test-infra/kubetest/process"
	"k8s.io/test-infra/kubetest/util"
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

// NewTester returns an object that knows how to test the cluster it deployed.
func NewTester(e2etest, ginkgo, kubecfg, reportdir string, testArgs *string, control *process.Control) (*Tester, error) {
	// Find the ginkgo and e2e.test artifacts we need. We'll cheat for now, and pull them from a known path.
	if e2etest == "" {
		e2etest = util.K8s("kubernetes", "bazel-bin", "test", "e2e", "e2e.test")
	}
	if ginkgo == "" {
		ginkgo = util.K8s("kubernetes", "bazel-bin", "vendor", "github.com", "onsi", "ginkgo", "ginkgo", "linux_amd64_stripped", "ginkgo")
	}

	if reportdir == "" {
		var err error
		reportdir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	// Check that our files and folders exist.
	if _, err := os.Stat(e2etest); err != nil {
		return nil, fmt.Errorf("e2e.test not found at %s, build before tests (--build=e2e): %v", e2etest, err)
	}
	if _, err := os.Stat(ginkgo); err != nil {
		return nil, fmt.Errorf("ginkgo not found at %s, build before tests (--build=e2e): %v", ginkgo, err)
	}
	if _, err := os.Stat(reportdir); err != nil {
		return nil, fmt.Errorf("reportdir %s must exist before tests are run: %v", reportdir, err)
	}

	return &Tester{
		e2etest:   e2etest,
		ginkgo:    ginkgo,
		kubecfg:   kubecfg,
		reportdir: reportdir,
		testArgs:  testArgs,
		control:   control,
	}, nil
}

// Test just execs ginkgo. This will take more parameters in the future.
func (t *Tester) Test(focus, skip string) error {
	// Overwrite the conformance focus and skip args if specified.
	focusRegex := "\".*\""
	skipRegex := "\".*(Feature)|(NFS)|(StatefulSet).*\""
	if focus == "" {
		focusRegex = focus
	}
	if skip == "" {
		skipRegex = skip
	}
	focusArg := fmt.Sprintf("--focus=%s", focusRegex)
	skipArg := fmt.Sprintf("--skip=%s", skipRegex)

	// Execute ginkgo, which in turn executes e2e.test.
	args := []string{"--seed=1436380640", "--nodes=10", focusArg, skipArg, t.e2etest,
		"--", "--kubeconfig", t.kubecfg, "--ginkgo.flakeAttempts=2", "--num-nodes=4", "--systemd-services=docker,kubelet",
		"--report-dir", t.reportdir}
	args = append(args, strings.Fields(*t.testArgs)...)
	cmd := exec.Command(t.ginkgo, args...)
	return t.control.FinishRunning(cmd)
}

// Deployer returns a deployer stub that expects a cluster to already exist.
type Deployer struct {
	kubecfg   string
	testArgs  *string
	control   *process.Control
	apiserver *kubernetes.Clientset
}

// NewDeployer returns a new Deployer.
func NewDeployer(kubecfg string, testArgs *string, control *process.Control) (*Deployer, error) {
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
		control:   control,
		testArgs:  testArgs,
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
	return d.apiserver.CoreV1().ComponentStatuses().List(metav1.ListOptions{})
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

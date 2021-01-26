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

package e2e

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/process"
)

// GinkgoTester runs e2e tests directly (by calling ginkgo)
type GinkgoTester struct {
	// Required
	Kubeconfig string
	Provider   string
	KubeRoot   string

	GinkgoParallel int

	// Other options defined in hack/ginkgo.sh
	KubeMasterURL         string
	FlakeAttempts         int
	GCEProject            string
	GCEZone               string
	GCERegion             string
	GCEMultizone          bool
	GKECluster            string
	KubeMaster            string
	ClusterID             string
	CloudConfig           string
	NodeInstanceGroup     string
	KubeGCEInstancePrefix string
	Network               string
	NodeTag               string
	MasterTag             string
	ClusterMonitoringMode string
	KubeContainerRuntime  string
	MasterOSDistribution  string
	NodeOSDistribution    string
	NumNodes              int
	ReportDir             string
	ReportPrefix          string
	StorageTestDriver     string

	// Other ginkgo options
	FocusRegex      string
	SkipRegex       string
	Seed            int
	SystemdServices []string
}

// NewGinkgoTester returns a new instance of GinkgoTester
func NewGinkgoTester(o *BuildTesterOptions) *GinkgoTester {
	t := &GinkgoTester{
		FlakeAttempts: 1,
	}

	t.GinkgoParallel = o.Parallelism
	t.FocusRegex = o.FocusRegex
	t.SkipRegex = o.SkipRegex

	return t
}

// args is a list of arguments, defining some helper functions
type args struct {
	values []string
}

func (a *args) addIfNonEmpty(flagName, value string) {
	if value != "" {
		a.values = append(a.values, fmt.Sprintf("--%s=%s", flagName, value))
	}
}

func (a *args) addBool(flagName string, value bool) {
	a.values = append(a.values, fmt.Sprintf("--%s=%t", flagName, value))
}

func (a *args) addInt(flagName string, value int) {
	a.values = append(a.values, fmt.Sprintf("--%s=%d", flagName, value))
}

// validate checks that fields are set sanely
func (t *GinkgoTester) validate() error {
	if t.Kubeconfig == "" {
		return errors.New("Kubeconfig cannot be empty")
	}

	if t.Provider == "" {
		return errors.New("Provider cannot be empty")
	}

	if t.KubeRoot == "" {
		return errors.New("Kuberoot cannot be empty")
	}

	if t.GinkgoParallel <= 0 {
		return errors.New("GinkgoParallel must be at least 1")
	}

	// Check that our files and folders exist.
	if t.ReportDir != "" {
		if _, err := os.Stat(t.ReportDir); err != nil {
			return fmt.Errorf("ReportDir %s must exist before tests are run: %v", t.ReportDir, err)
		}
	}

	return nil
}

// Run executes the test (calling ginkgo)
func (t *GinkgoTester) Run(control *process.Control, extraArgs []string) error {
	if err := t.validate(); err != nil {
		return fmt.Errorf("configuration error in GinkgoTester: %v", err)
	}

	ginkgoPath, err := t.findBinary("ginkgo")
	if err != nil {
		return err
	}
	e2eTest, err := t.findBinary("e2e.test")
	if err != nil {
		return err
	}

	a := &args{}

	if t.Seed != 0 {
		a.addInt("seed", t.Seed)
	}

	a.addIfNonEmpty("focus", t.FocusRegex)
	a.addIfNonEmpty("skip", t.SkipRegex)

	a.addInt("nodes", t.GinkgoParallel)

	a.values = append(a.values, []string{
		e2eTest,
		"--",
	}...)

	a.addIfNonEmpty("kubeconfig", t.Kubeconfig)
	a.addInt("ginkgo.flakeAttempts", t.FlakeAttempts)
	a.addIfNonEmpty("provider", t.Provider)
	a.addIfNonEmpty("gce-project", t.GCEProject)
	a.addIfNonEmpty("gce-zone", t.GCEZone)
	a.addIfNonEmpty("gce-region", t.GCERegion)
	a.addBool("gce-multizone", t.GCEMultizone)

	a.addIfNonEmpty("gke-cluster", t.GKECluster)
	a.addIfNonEmpty("host", t.KubeMasterURL)
	a.addIfNonEmpty("kube-master", t.KubeMaster)
	a.addIfNonEmpty("cluster-tag", t.ClusterID)
	a.addIfNonEmpty("cloud-config-file", t.CloudConfig)
	a.addIfNonEmpty("repo-root", t.KubeRoot)
	a.addIfNonEmpty("node-instance-group", t.NodeInstanceGroup)
	a.addIfNonEmpty("prefix", t.KubeGCEInstancePrefix)
	a.addIfNonEmpty("network", t.Network)
	a.addIfNonEmpty("node-tag", t.NodeTag)
	a.addIfNonEmpty("master-tag", t.MasterTag)
	a.addIfNonEmpty("cluster-monitoring-mode", t.ClusterMonitoringMode)

	a.addIfNonEmpty("container-runtime", t.KubeContainerRuntime)
	a.addIfNonEmpty("master-os-distro", t.MasterOSDistribution)
	a.addIfNonEmpty("node-os-distro", t.NodeOSDistribution)
	a.addInt("num-nodes", t.NumNodes)
	a.addIfNonEmpty("report-dir", t.ReportDir)
	a.addIfNonEmpty("report-prefix", t.ReportPrefix)

	a.addIfNonEmpty("systemd-services", strings.Join(t.SystemdServices, ","))
	a.addIfNonEmpty("storage.testdriver", t.StorageTestDriver)

	ginkgoArgs := append(a.values, extraArgs...)

	cmd := exec.Command(ginkgoPath, ginkgoArgs...)

	log.Printf("running ginkgo: %s %s", cmd.Path, strings.Join(cmd.Args, " "))

	return control.FinishRunning(cmd)
}

// findBinary finds a file by name, from a list of well-known output locations
// When multiple matches are found, the most recent will be returned
// Based on kube::util::find-binary from kubernetes/kubernetes
func (t *GinkgoTester) findBinary(name string) (string, error) {
	kubeRoot := t.KubeRoot

	locations := []string{
		filepath.Join(kubeRoot, "_output", "bin", name),
		filepath.Join(kubeRoot, "_output", "dockerized", "bin", name),
		filepath.Join(kubeRoot, "_output", "local", "bin", name),
		filepath.Join(kubeRoot, "platforms", runtime.GOOS, runtime.GOARCH, name),
	}

	bazelBin := filepath.Join(kubeRoot, "bazel-bin")
	bazelBinExists := true
	if _, err := os.Stat(bazelBin); os.IsNotExist(err) {
		bazelBinExists = false
		log.Printf("bazel-bin not found at %s", bazelBin)
	}

	if bazelBinExists {
		err := filepath.Walk(bazelBin, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("error from walk: %v", err)
			}
			if info.Name() != name {
				return nil
			}
			if !strings.Contains(path, runtime.GOOS+"_"+runtime.GOARCH) {
				return nil
			}
			locations = append(locations, path)
			return nil
		})
		if err != nil {
			return "", err
		}
	}

	newestLocation := ""
	var newestModTime time.Time
	for _, loc := range locations {
		stat, err := os.Stat(loc)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("error from stat %s: %v", loc, err)
		}
		if newestLocation == "" || stat.ModTime().After(newestModTime) {
			newestModTime = stat.ModTime()
			newestLocation = loc
		}
	}

	if newestLocation == "" {
		log.Printf("could not find %s, looked in %s", name, locations)
		return "", fmt.Errorf("could not find %s", name)
	}

	return newestLocation, nil
}

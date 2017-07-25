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

// Package main / gke.go provides the Google Container Engine (GKE)
// kubetest deployer via newGKE().
//
// TODO(zmerlynn): Pull this out to a separate package?
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultPool = "default"
	e2eAllow    = "tcp:22,tcp:80,tcp:8080,tcp:30000-32767,udp:30000-32767"
)

var (
	gkeAdditionalZones = flag.String("gke-additional-zones", "", "(gke only) List of additional Google Compute Engine zones to use. Clusters are created symmetrically across zones by default, see --gke-shape for details.")
	gkeShape           = flag.String("gke-shape", `{"default":{"Nodes":3,"MachineType":"n1-standard-2"}}`, `(gke only) A JSON description of node pools to create. The node pool 'default' is required and used for initial cluster creation. All node pools are symmetric across zones, so the cluster total node count is {total nodes in --gke-shape} * {1 + (length of --gke-additional-zones)}. Example: '{"default":{"Nodes":999,"MachineType:":"n1-standard-1"},"heapster":{"Nodes":1, "MachineType":"n1-standard-8"}}`)

	// poolRe matches instance group URLs of the form `https://www.googleapis.com/compute/v1/projects/some-project/zones/a-zone/instanceGroupManagers/gke-some-cluster-some-pool-90fcb815-grp`. Match meaning:
	// m[0]: path starting with zones/
	// m[1]: zone
	// m[2]: pool name (passed to e2es)
	// m[3]: unique hash (used as nonce for firewall rules)
	poolRe = regexp.MustCompile(`zones/([^/]+)/instanceGroupManagers/(gke-.*-([0-9a-f]{8})-grp)$`)
)

type gkeNodePool struct {
	Nodes       int
	MachineType string
}

type gkeDeployer struct {
	project         string
	zone            string
	additionalZones string
	cluster         string
	shape           map[string]gkeNodePool
	network         string

	setup          bool
	kubecfg        string
	instanceGroups []*ig
}

type ig struct {
	path string
	zone string
	name string
	uniq string
}

var _ deployer = &gkeDeployer{}

func newGKE(provider, project, zone, network, cluster string) (*gkeDeployer, error) {
	if provider != "gke" {
		return nil, fmt.Errorf("--provider must be 'gke' for GKE deployment, found %q", provider)
	}
	g := &gkeDeployer{}

	if cluster == "" {
		return nil, fmt.Errorf("--cluster must be set for GKE deployment")
	}
	g.cluster = cluster

	if project == "" {
		return nil, fmt.Errorf("--gcp-project must be set for GKE deployment")
	}
	g.project = project

	if zone == "" {
		return nil, fmt.Errorf("--gcp-zone must be set for GKE deployment")
	}
	g.zone = zone

	if network == "" {
		return nil, fmt.Errorf("--gcp-network must be set for GKE deployment")
	}
	g.network = network

	g.additionalZones = *gkeAdditionalZones

	err := json.Unmarshal([]byte(*gkeShape), &g.shape)
	if err != nil {
		return nil, fmt.Errorf("--gke-shape must be valid JSON, unmarshal error: %v, JSON: %q", err, *gkeShape)
	}
	if _, ok := g.shape[defaultPool]; !ok {
		return nil, fmt.Errorf("--gke-shape must include a node pool named 'default', found %q", *gkeShape)
	}

	// Override kubecfg to a temporary file rather than trashing the user's.
	f, err := ioutil.TempFile("", "gke-kubecfg")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	kubecfg := f.Name()
	if err := f.Chmod(0600); err != nil {
		return nil, err
	}
	g.kubecfg = kubecfg

	// We want no KUBERNETES_PROVIDER, but to set
	// KUBERNETES_CONFORMANCE_PROVIDER and
	// KUBERNETES_CONFORMANCE_TEST. This prevents ginkgo-e2e.sh from
	// using the cluster/gke functions.
	//
	// We do this in the deployer constructor so that
	// cluster/gce/list-resources.sh outputs the same provider for the
	// extent of the binary. (It seems like it belongs in TestSetup,
	// but that way leads to madness.)
	//
	// TODO(zmerlynn): This is gross.
	if err := os.Unsetenv("KUBERNETES_PROVIDER"); err != nil {
		return nil, err
	}
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}
	if err := os.Setenv("KUBERNETES_CONFORMANCE_PROVIDER", "gke"); err != nil {
		return nil, err
	}

	// TODO(zmerlynn): Another snafu of cluster/gke/list-resources.sh:
	// Set KUBE_GCE_INSTANCE_PREFIX so that we don't accidentally pick
	// up CLUSTER_NAME later.
	if err := os.Setenv("KUBE_GCE_INSTANCE_PREFIX", "gke-"+g.cluster); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *gkeDeployer) Up() error {
	// Create network if it doesn't exist.
	if err := finishRunning(exec.Command("gcloud", "compute", "networks", "describe", g.network,
		"--project="+g.project,
		"--format=value(name)")); err != nil {
		// Assume error implies non-existent.
		if err := finishRunning(exec.Command("gcloud", "compute", "networks", "create", g.network,
			"--project="+g.project,
			"--mode=auto")); err != nil {
			return err
		}
	}

	def := g.shape[defaultPool]
	args := []string{"container", "clusters", "create", g.cluster,
		"--project=" + g.project,
		"--zone=" + g.zone,
		"--machine-type=" + def.MachineType,
		"--num-nodes=" + strconv.Itoa(def.Nodes),
		"--network=" + g.network,
	}
	if g.additionalZones != "" {
		args = append(args, "--additional-zones="+g.additionalZones)
	}
	// TODO(zmerlynn): The version should be plumbed through Extract
	// or a separate flag rather than magic env variables.
	if v := os.Getenv("CLUSTER_API_VERSION"); v != "" {
		args = append(args, "--cluster-version="+v)
	}
	if err := finishRunning(exec.Command("gcloud", args...)); err != nil {
		return fmt.Errorf("error creating cluster: %v", err)
	}
	for poolName, pool := range g.shape {
		if poolName == defaultPool {
			continue
		}
		if err := finishRunning(exec.Command("gcloud", "container", "node-pools", "create", poolName,
			"--cluster="+g.cluster,
			"--project="+g.project,
			"--zone="+g.zone,
			"--machine-type="+pool.MachineType,
			"--num-nodes="+strconv.Itoa(pool.Nodes))); err != nil {
			return fmt.Errorf("error creating node pool %q: %v", poolName, err)
		}
	}
	return nil
}

func (g *gkeDeployer) IsUp() error {
	return isUp(g)
}

func (g *gkeDeployer) TestSetup() error {
	if g.setup {
		// Ensure setup is a singleton.
		return nil
	}
	if err := g.getKubeConfig(); err != nil {
		return err
	}
	if err := g.getInstanceGroups(); err != nil {
		return err
	}
	if err := g.ensureFirewall(); err != nil {
		return err
	}
	if err := g.setupEnv(); err != nil {
		return err
	}
	g.setup = true
	return nil
}

func (g *gkeDeployer) getKubeConfig() error {
	info, err := os.Stat(g.kubecfg)
	if err != nil {
		return err
	}
	if info.Size() > 0 {
		// Assume that if we already have it, it's good.
		return nil
	}
	if err := os.Setenv("KUBECONFIG", g.kubecfg); err != nil {
		return err
	}
	if err := finishRunning(exec.Command("gcloud", "container", "clusters", "get-credentials", g.cluster,
		"--project="+g.project,
		"--zone="+g.zone)); err != nil {
		return fmt.Errorf("error executing get-credentials: %v", err)
	}
	return nil
}

// setupEnv is to appease ginkgo-e2e.sh and other pieces of the e2e infrastructure. It
// would be nice to handle this elsewhere, and not with env
// variables. c.f. kubernetes/test-infra#3330.
func (g *gkeDeployer) setupEnv() error {
	// Set NODE_INSTANCE_GROUP to the names of the instance groups in
	// the cluster's primary zone. (e2e expects this).
	var filt []string
	for _, ig := range g.instanceGroups {
		if ig.zone == g.zone {
			filt = append(filt, ig.name)
		}
	}
	if err := os.Setenv("NODE_INSTANCE_GROUP", strings.Join(filt, ",")); err != nil {
		return fmt.Errorf("error setting NODE_INSTANCE_GROUP: %v", err)
	}
	return nil
}

func (g *gkeDeployer) ensureFirewall() error {
	firewall, err := g.getClusterFirewall()
	if err != nil {
		return fmt.Errorf("error getting unique firewall: %v", err)
	}
	if finishRunning(exec.Command("gcloud", "compute", "firewall-rules", "describe", firewall,
		"--project="+g.project,
		"--format=value(name)")) == nil {
		// Assume that if this unique firewall exists, it's good to go.
		return nil
	}

	tagOut, err := exec.Command("gcloud", "compute", "instances", "list",
		"--project="+g.project,
		"--filter=metadata.created-by:*"+g.instanceGroups[0].path,
		"--limit=1",
		"--format=get(tags.items)").Output()
	if err != nil {
		return fmt.Errorf("instances list failed: %s", execError(err))
	}
	tag := strings.TrimSpace(string(tagOut))
	if tag == "" {
		return fmt.Errorf("instances list returned no instances (or instance has no tags)")
	}

	if err := finishRunning(exec.Command("gcloud", "compute", "firewall-rules", "create", firewall,
		"--project="+g.project,
		"--network="+g.network,
		"--allow="+e2eAllow,
		"--target-tags="+tag)); err != nil {
		return fmt.Errorf("error creating e2e firewall: %v", err)
	}
	return nil
}

func (g *gkeDeployer) getInstanceGroups() error {
	if len(g.instanceGroups) > 0 {
		return nil
	}
	igs, err := exec.Command("gcloud", "container", "clusters", "describe", g.cluster,
		"--format=value(instanceGroupUrls)",
		"--project="+g.project,
		"--zone="+g.zone).Output()
	if err != nil {
		return fmt.Errorf("instance group URL fetch failed: %s", execError(err))
	}
	igURLs := strings.Split(strings.TrimSpace(string(igs)), ";")
	if len(igURLs) == 0 {
		return fmt.Errorf("no instance group URLs returned by gcloud, output %q", string(igs))
	}
	sort.Strings(igURLs)
	for _, igURL := range igURLs {
		m := poolRe.FindStringSubmatch(igURL)
		if len(m) == 0 {
			return fmt.Errorf("instanceGroupUrl %q did not match regex %v", igURL, poolRe)
		}
		g.instanceGroups = append(g.instanceGroups, &ig{path: m[0], zone: m[1], name: m[2], uniq: m[3]})
	}
	return nil
}

func (g *gkeDeployer) getClusterFirewall() (string, error) {
	if err := g.getInstanceGroups(); err != nil {
		return "", err
	}
	// We want to ensure that there's an e2e-ports-* firewall rule
	// that maps to the cluster nodes, but the target tag for the
	// nodes can be slow to get. Use the hash from the lexically first
	// node pool instead.
	return "e2e-ports-" + g.instanceGroups[0].uniq, nil
}

func (g *gkeDeployer) Down() error {
	firewall, err := g.getClusterFirewall()
	if err != nil {
		// This is expected if the cluster doesn't exist.
		return nil
	}

	errCluster := finishRunning(exec.Command(
		"gcloud", "container", "clusters", "delete", "-q", g.cluster,
		"--project="+g.project,
		"--zone="+g.zone))
	errFirewall := finishRunning(exec.Command("gcloud", "compute", "firewall-rules", "delete", firewall,
		"--project="+g.project))
	if errCluster != nil {
		return fmt.Errorf("error deleting cluster: %v", errCluster)
	}
	if errFirewall != nil {
		return fmt.Errorf("error deleting firewall: %v", errFirewall)
	}
	return nil
}

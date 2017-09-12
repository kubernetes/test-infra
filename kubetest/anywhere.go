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
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
	"time"
)

var (
	// kubernetes-anywhere specific flags.
	kubernetesAnywherePath = flag.String("kubernetes-anywhere-path", "",
		"(kubernetes-anywhere only) Path to the kubernetes-anywhere directory. Must be set for kubernetes-anywhere.")
	kubernetesAnywherePhase2Provider = flag.String("kubernetes-anywhere-phase2-provider", "ignition",
		"(kubernetes-anywhere only) Provider for phase2 bootstrapping. (Defaults to ignition).")
	kubernetesAnywhereKubeadmVersion = flag.String("kubernetes-anywhere-kubeadm-version", "stable",
		"(kubernetes-anywhere only) Version of kubeadm to use, if phase2-provider is kubeadm. May be \"stable\" or a gs:// link to a custom build.")
	kubernetesAnywhereKubernetesVersion = flag.String("kubernetes-anywhere-kubernetes-version", "",
		"(kubernetes-anywhere only) Version of Kubernetes to use (e.g. latest, stable, latest-1.6, 1.6.3, etc).")
	kubernetesAnywhereCluster = flag.String("kubernetes-anywhere-cluster", "",
		"(kubernetes-anywhere only) Cluster name. Must be set for kubernetes-anywhere.")
	kubernetesAnywhereUpTimeout = flag.Duration("kubernetes-anywhere-up-timeout", 20*time.Minute,
		"(kubernetes-anywhere only) Time limit between starting a cluster and making a successful call to the Kubernetes API.")
	kubernetesAnywhereNumNodes = flag.Int("kubernetes-anywhere-num-nodes", 4,
		"(kubernetes-anywhere only) Number of nodes to be deployed in the cluster.")
)

const kubernetesAnywhereConfigTemplate = `
.phase1.num_nodes={{.NumNodes}}
.phase1.cluster_name="{{.Cluster}}"
.phase1.ssh_user=""
.phase1.cloud_provider="gce"

.phase1.gce.os_image="ubuntu-1604-xenial-v20160420c"
.phase1.gce.instance_type="n1-standard-1"
.phase1.gce.project="{{.Project}}"
.phase1.gce.region="{{.Region}}"
.phase1.gce.zone="{{.Zone}}"
.phase1.gce.network="default"

.phase2.installer_container="docker.io/colemickens/k8s-ignition:latest"
.phase2.docker_registry="gcr.io/google-containers"
.phase2.kubernetes_version="{{.KubernetesVersion}}"
.phase2.provider="{{.Phase2Provider}}"
.phase2.kubeadm.version="{{.KubeadmVersion}}"
.phase2.kube_context_name="{{.KubeContext}}"

.phase3.run_addons=y
.phase3.weave_net={{if eq .Phase2Provider "kubeadm" -}} y {{- else -}} n {{- end}}
.phase3.kube_proxy=n
.phase3.dashboard=n
.phase3.heapster=n
.phase3.kube_dns=n
`

type kubernetesAnywhere struct {
	path string
	// These are exported only because their use in the config template requires it.
	Phase2Provider    string
	KubeadmVersion    string
	KubernetesVersion string
	NumNodes          int
	Project           string
	Cluster           string
	Zone              string
	Region            string
	KubeContext       string
}

func newKubernetesAnywhere(project, zone string) (deployer, error) {
	if *kubernetesAnywherePath == "" {
		return nil, fmt.Errorf("--kubernetes-anywhere-path is required")
	}

	if *kubernetesAnywhereCluster == "" {
		return nil, fmt.Errorf("--kubernetes-anywhere-cluster is required")
	}

	if project == "" {
		return nil, fmt.Errorf("--provider=kubernetes-anywhere requires --gcp-project")
	}

	if zone == "" {
		zone = "us-central1-c"
	}

	// Set KUBERNETES_CONFORMANCE_TEST so the auth info is picked up
	// from kubectl instead of bash inference.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}

	k := &kubernetesAnywhere{
		path:              *kubernetesAnywherePath,
		Phase2Provider:    *kubernetesAnywherePhase2Provider,
		KubeadmVersion:    *kubernetesAnywhereKubeadmVersion,
		KubernetesVersion: *kubernetesAnywhereKubernetesVersion,
		NumNodes:          *kubernetesAnywhereNumNodes,
		Project:           project,
		Cluster:           *kubernetesAnywhereCluster,
		Zone:              zone,
		Region:            regexp.MustCompile(`-[^-]+$`).ReplaceAllString(zone, ""),
	}

	if err := k.writeConfig(); err != nil {
		return nil, err
	}
	return k, nil
}

func (k *kubernetesAnywhere) getConfig() ([]byte, error) {
	// As needed, plumb through more CLI options to replace these defaults
	tmpl, err := template.New("kubernetes-anywhere-config").Parse(kubernetesAnywhereConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("Error creating template for KubernetesAnywhere config: %v", err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, k); err != nil {
		return nil, fmt.Errorf("Error executing template for KubernetesAnywhere config: %v", err)
	}

	return buf.Bytes(), nil
}

func (k *kubernetesAnywhere) writeConfig() error {
	config, err := k.getConfig()
	if err != nil {
		return fmt.Errorf("Could not generate config: %v", err)
	}
	return ioutil.WriteFile(k.path+"/.config", config, 0644)
}

func (k *kubernetesAnywhere) Up() error {
	cmd := exec.Command("make", "-C", k.path, "WAIT_FOR_KUBECONFIG=y", "deploy")
	if err := finishRunning(cmd); err != nil {
		return err
	}

	nodes := k.NumNodes
	return waitForNodes(k, nodes+1, *kubernetesAnywhereUpTimeout)
}

func (k *kubernetesAnywhere) IsUp() error {
	return isUp(k)
}

func (k *kubernetesAnywhere) DumpClusterLogs(localPath, gcsPath string) error {
	// TODO(pipejakob): the default implementation (log-dump.sh) doesn't work for
	// kubernetes-anywhere yet, so just skip attempting to dump logs.
	// https://github.com/kubernetes/kubeadm/issues/256
	log.Print("DumpClusterLogs is a no-op for kubernetes-anywhere deployments. Not doing anything.")
	log.Print("If you care about enabling this feature, follow this issue for progress:")
	log.Print("    https://github.com/kubernetes/kubeadm/issues/256")
	return nil
}

func (k *kubernetesAnywhere) TestSetup() error {
	o, err := output(exec.Command("make", "--silent", "-C", k.path, "kubeconfig-path"))
	if err != nil {
		return fmt.Errorf("Could not get kubeconfig-path: %v", err)
	}
	kubecfg := strings.TrimSuffix(string(o), "\n")

	if err = os.Setenv("KUBECONFIG", kubecfg); err != nil {
		return err
	}
	return nil
}

func (k *kubernetesAnywhere) Down() error {
	err := finishRunning(exec.Command("make", "-C", k.path, "kubeconfig-path"))
	if err != nil {
		// This is expected if the cluster doesn't exist.
		return nil
	}
	return finishRunning(exec.Command("make", "-C", k.path, "FORCE_DESTROY=y", "destroy"))
}

const defaultConfigFile = ".config"

type kubernetesAnywhereMultiCluster struct {
	*kubernetesAnywhere
	multiClusters multiClusterDeployment
	configFile    map[string]string
}

// newKubernetesAnywhereMultiCluster returns the deployer based on kubernetes-anywhere
// which can be used to deploy multiple clusters simultaneously.
func newKubernetesAnywhereMultiCluster(project, zone string, multiClusters multiClusterDeployment) (deployer, error) {
	if len(multiClusters.clusters) < 1 {
		return nil, fmt.Errorf("invalid --multi-clusters flag passed")
	}
	k, err := newKubernetesAnywhere(project, zone)
	if err != nil {
		return nil, err
	}
	mk := &kubernetesAnywhereMultiCluster{k.(*kubernetesAnywhere), multiClusters, make(map[string]string)}

	for _, cluster := range mk.multiClusters.clusters {
		specificZone, specified := mk.multiClusters.zones[cluster]
		if specified {
			mk.Zone = specificZone
		}
		mk.Cluster = cluster
		mk.KubeContext = mk.Zone + "-" + mk.Cluster
		mk.configFile[cluster] = defaultConfigFile + "-" + mk.Cluster
		if err := mk.writeConfig(); err != nil {
			return nil, err
		}
	}
	return mk, nil
}

// writeConfig writes the kubernetes-anywhere config file to file system after
// rendering the template file with configuration in deployer.
func (k *kubernetesAnywhereMultiCluster) writeConfig() error {
	config, err := k.getConfig()
	if err != nil {
		return fmt.Errorf("could not generate config: %v", err)
	}

	return ioutil.WriteFile(k.path+"/"+k.configFile[k.Cluster], config, 0644)
}

// Up brings up multiple k8s clusters in parallel.
func (k *kubernetesAnywhereMultiCluster) Up() error {
	var cmds []*exec.Cmd
	for _, cluster := range k.multiClusters.clusters {
		cmd := exec.Command("make", "-C", k.path, "CONFIG_FILE="+k.configFile[cluster], "deploy")
		cmds = append(cmds, cmd)
	}

	if err := finishRunningParallel(cmds...); err != nil {
		return err
	}

	return k.TestSetup()
}

// TestSetup sets up test environment by merging kubeconfig of multiple deployments.
func (k *kubernetesAnywhereMultiCluster) TestSetup() error {
	var kubecfg string
	for _, cluster := range k.multiClusters.clusters {
		o, err := output(exec.Command("make", "--silent", "-C", k.path, "CONFIG_FILE="+k.configFile[cluster], "kubeconfig-path"))
		if err != nil {
			return fmt.Errorf("could not get kubeconfig-path: %v", err)
		}
		if len(kubecfg) != 0 {
			kubecfg += ":"
		}
		kubecfg += strings.TrimSuffix(string(o), "\n")
	}

	if err := os.Setenv("KUBECONFIG", kubecfg); err != nil {
		return err
	}
	return nil
}

// IsUp checks if all the clusters in the deployer are up.
func (k *kubernetesAnywhereMultiCluster) IsUp() error {
	if err := k.TestSetup(); err != nil {
		return err
	}

	for _, cluster := range k.multiClusters.clusters {
		zone := k.multiClusters.zones[cluster]
		kubeContext := zone + "-" + cluster
		o, err := output(exec.Command("kubectl", "--context="+kubeContext, "get", "nodes", "--no-headers"))
		if err != nil {
			log.Printf("kubectl get nodes failed for cluster %s: %s\n%s", cluster, wrapError(err).Error(), string(o))
			return err
		}
		stdout := strings.TrimSpace(string(o))
		log.Printf("Cluster nodes of cluster %s:\n%s", cluster, stdout)

		n := len(strings.Split(stdout, "\n"))
		if n < k.NumNodes {
			return fmt.Errorf("cluster %s found, but %d nodes reported", cluster, n)
		}
	}
	return nil
}

// Down brings down multiple k8s clusters in parallel.
func (k *kubernetesAnywhereMultiCluster) Down() error {
	if err := k.TestSetup(); err != nil {
		// This is expected if the clusters doesn't exist.
		return nil
	}

	var cmds []*exec.Cmd
	for _, cluster := range k.multiClusters.clusters {
		cmd := exec.Command("make", "-C", k.path, "CONFIG_FILE="+k.configFile[cluster], "FORCE_DESTROY=y", "destroy")
		cmds = append(cmds, cmd)
	}
	return finishRunningParallel(cmds...)
}

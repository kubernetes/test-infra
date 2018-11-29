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
	"errors"
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

const defaultKubeadmCNI = "weave"

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
	kubernetesAnywhereKubeletVersion = flag.String("kubernetes-anywhere-kubelet-version", "stable",
		"(kubernetes-anywhere only) Version of Kubelet to use, if phase2-provider is kubeadm. May be \"stable\" or a gs:// link to a custom build.")
	kubernetesAnywhereKubeletCIVersion = flag.String("kubernetes-anywhere-kubelet-ci-version", "",
		"(kubernetes-anywhere only) If specified, the ci version for the kubelet to use. Overrides kubernetes-anywhere-kubelet-version.")
	kubernetesAnywhereCluster = flag.String("kubernetes-anywhere-cluster", "",
		"(kubernetes-anywhere only) Cluster name. Must be set for kubernetes-anywhere.")
	kubernetesAnywhereProxyMode = flag.String("kubernetes-anywhere-proxy-mode", "",
		"(kubernetes-anywhere only) Chose kube-proxy mode.")
	kubernetesAnywhereUpTimeout = flag.Duration("kubernetes-anywhere-up-timeout", 20*time.Minute,
		"(kubernetes-anywhere only) Time limit between starting a cluster and making a successful call to the Kubernetes API.")
	kubernetesAnywhereNumNodes = flag.Int("kubernetes-anywhere-num-nodes", 4,
		"(kubernetes-anywhere only) Number of nodes to be deployed in the cluster.")
	kubernetesAnywhereUpgradeMethod = flag.String("kubernetes-anywhere-upgrade-method", "upgrade",
		"(kubernetes-anywhere only) Indicates whether to do the control plane upgrade with kubeadm method \"init\" or \"upgrade\"")
	kubernetesAnywhereCNI = flag.String("kubernetes-anywhere-cni", "",
		"(kubernetes-anywhere only) The name of the CNI plugin used for the cluster's SDN.")
	kubernetesAnywhereDumpClusterLogs = flag.Bool("kubernetes-anywhere-dump-cluster-logs", true,
		"(kubernetes-anywhere only) Whether to dump cluster logs.")
	kubernetesAnywhereOSImage = flag.String("kubernetes-anywhere-os-image", "ubuntu-1604-xenial-v20171212",
		"(kubernetes-anywhere only) The name of the os_image to use for nodes")
	kubernetesAnywhereKubeadmFeatureGates = flag.String("kubernetes-anywhere-kubeadm-feature-gates", "",
		"(kubernetes-anywhere only) A set of key=value pairs that describes feature gates for kubeadm features. If specified, this flag will pass on to kubeadm.")
)

const kubernetesAnywhereConfigTemplate = `
.phase1.num_nodes={{.NumNodes}}
.phase1.cluster_name="{{.Cluster}}"
.phase1.ssh_user=""
.phase1.cloud_provider="gce"

.phase1.gce.os_image="{{.OSImage}}"
.phase1.gce.instance_type="n1-standard-1"
.phase1.gce.project="{{.Project}}"
.phase1.gce.region="{{.Region}}"
.phase1.gce.zone="{{.Zone}}"
.phase1.gce.network="default"

.phase2.installer_container="docker.io/colemickens/k8s-ignition:latest"
.phase2.docker_registry="k8s.gcr.io"
.phase2.kubernetes_version="{{.KubernetesVersion}}"
.phase2.provider="{{.Phase2Provider}}"
.phase2.kubelet_version="{{.KubeletVersion}}"
.phase2.kubeadm.version="{{.KubeadmVersion}}"
.phase2.kube_context_name="{{.KubeContext}}"
.phase2.proxy_mode="{{.KubeproxyMode}}"
.phase2.kubeadm.master_upgrade.method="{{.UpgradeMethod}}"
.phase2.kubeadm.feature_gates="{{.KubeadmFeatureGates}}"

.phase3.run_addons=y
.phase3.kube_proxy=n
.phase3.dashboard=n
.phase3.heapster=n
.phase3.kube_dns=n
.phase3.cni="{{.CNI}}"
`

const kubernetesAnywhereMultiClusterConfigTemplate = kubernetesAnywhereConfigTemplate + `
.phase2.enable_cloud_provider=y
.phase3.gce_storage_class=y
`

type kubernetesAnywhere struct {
	path string
	// These are exported only because their use in the config template requires it.
	Phase2Provider      string
	KubeadmVersion      string
	KubeletVersion      string
	UpgradeMethod       string
	KubernetesVersion   string
	NumNodes            int
	Project             string
	Cluster             string
	Zone                string
	Region              string
	KubeContext         string
	CNI                 string
	KubeproxyMode       string
	OSImage             string
	KubeadmFeatureGates string
}

func initializeKubernetesAnywhere(project, zone string) (*kubernetesAnywhere, error) {
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

	kubeletVersion := *kubernetesAnywhereKubeletVersion
	if *kubernetesAnywhereKubeletCIVersion != "" {
		// resolvedVersion is EG v1.11.0-alpha.0.1031+d37460147ec956-bazel
		resolvedVersion, err := resolveCIVersion(*kubernetesAnywhereKubeletCIVersion)
		if err != nil {
			return nil, err
		}
		kubeletVersion = fmt.Sprintf("gs://kubernetes-release-dev/ci/%v/bin/linux/amd64/", resolvedVersion)
	}

	// preserve backwards compatibility for e2e tests which never provided cni name
	if *kubernetesAnywhereCNI == "" && *kubernetesAnywherePhase2Provider == "kubeadm" {
		*kubernetesAnywhereCNI = defaultKubeadmCNI
	}

	k := &kubernetesAnywhere{
		path:                *kubernetesAnywherePath,
		Phase2Provider:      *kubernetesAnywherePhase2Provider,
		KubeadmVersion:      *kubernetesAnywhereKubeadmVersion,
		KubeletVersion:      kubeletVersion,
		UpgradeMethod:       *kubernetesAnywhereUpgradeMethod,
		KubernetesVersion:   *kubernetesAnywhereKubernetesVersion,
		NumNodes:            *kubernetesAnywhereNumNodes,
		Project:             project,
		Cluster:             *kubernetesAnywhereCluster,
		Zone:                zone,
		Region:              regexp.MustCompile(`-[^-]+$`).ReplaceAllString(zone, ""),
		CNI:                 *kubernetesAnywhereCNI,
		KubeproxyMode:       *kubernetesAnywhereProxyMode,
		OSImage:             *kubernetesAnywhereOSImage,
		KubeadmFeatureGates: *kubernetesAnywhereKubeadmFeatureGates,
	}

	return k, nil
}

func newKubernetesAnywhere(project, zone string) (deployer, error) {
	k, err := initializeKubernetesAnywhere(project, zone)
	if err != nil {
		return nil, err
	}

	// Set KUBERNETES_CONFORMANCE_TEST so the auth info is picked up
	// from kubectl instead of bash inference.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}

	// Set KUBERNETES_CONFORMANCE_PROVIDER since KUBERNETES_CONFORMANCE_TEST is set
	// to ensure the right provider is passed onto the test.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_PROVIDER", "kubernetes-anywhere"); err != nil {
		return nil, err
	}

	if err := k.writeConfig(kubernetesAnywhereConfigTemplate); err != nil {
		return nil, err
	}
	return k, nil
}

func resolveCIVersion(version string) (string, error) {
	file := fmt.Sprintf("gs://kubernetes-release-dev/ci/%v.txt", version)
	return readGSFile(file)
}

// Implemented as a function var for testing.
var readGSFile = readGSFileImpl

func readGSFileImpl(filepath string) (string, error) {
	contents, err := control.Output(exec.Command("gsutil", "cat", filepath))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(contents)), nil
}

func (k *kubernetesAnywhere) getConfig(configTemplate string) ([]byte, error) {
	// As needed, plumb through more CLI options to replace these defaults
	tmpl, err := template.New("kubernetes-anywhere-config").Parse(configTemplate)
	if err != nil {
		return nil, fmt.Errorf("Error creating template for KubernetesAnywhere config: %v", err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, k); err != nil {
		return nil, fmt.Errorf("Error executing template for KubernetesAnywhere config: %v", err)
	}

	return buf.Bytes(), nil
}

func (k *kubernetesAnywhere) writeConfig(configTemplate string) error {
	config, err := k.getConfig(configTemplate)
	if err != nil {
		return fmt.Errorf("Could not generate config: %v", err)
	}
	return ioutil.WriteFile(k.path+"/.config", config, 0644)
}

func (k *kubernetesAnywhere) Up() error {
	cmd := exec.Command("make", "-C", k.path, "WAIT_FOR_KUBECONFIG=y", "deploy")
	if err := control.FinishRunning(cmd); err != nil {
		return err
	}

	if err := k.TestSetup(); err != nil {
		return err
	}

	return waitForReadyNodes(k.NumNodes+1, *kubernetesAnywhereUpTimeout, 1)
}

func (k *kubernetesAnywhere) IsUp() error {
	return isUp(k)
}

func (k *kubernetesAnywhere) DumpClusterLogs(localPath, gcsPath string) error {
	if !*kubernetesAnywhereDumpClusterLogs {
		log.Printf("Cluster log dumping disabled for Kubernetes Anywhere.")
		return nil
	}
	return defaultDumpClusterLogs(localPath, gcsPath)
}

func (k *kubernetesAnywhere) TestSetup() error {
	o, err := control.Output(exec.Command("make", "--silent", "-C", k.path, "kubeconfig-path"))
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
	err := control.FinishRunning(exec.Command("make", "-C", k.path, "kubeconfig-path"))
	if err != nil {
		// This is expected if the cluster doesn't exist.
		return nil
	}
	return control.FinishRunning(exec.Command("make", "-C", k.path, "FORCE_DESTROY=y", "destroy"))
}

func (k *kubernetesAnywhere) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

func (_ *kubernetesAnywhere) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

const defaultConfigFile = ".config"

type kubernetesAnywhereMultiCluster struct {
	*kubernetesAnywhere
	multiClusters  multiClusterDeployment
	configFile     map[string]string
	kubeContextMap map[string]string
}

// newKubernetesAnywhereMultiCluster returns the deployer based on kubernetes-anywhere
// which can be used to deploy multiple clusters simultaneously.
func newKubernetesAnywhereMultiCluster(project, zone string, multiClusters multiClusterDeployment) (deployer, error) {
	if len(multiClusters.clusters) < 1 {
		return nil, fmt.Errorf("invalid --multi-clusters flag passed")
	}
	k, err := initializeKubernetesAnywhere(project, zone)
	if err != nil {
		return nil, err
	}
	mk := &kubernetesAnywhereMultiCluster{k, multiClusters, make(map[string]string), make(map[string]string)}

	for _, cluster := range mk.multiClusters.clusters {
		specificZone, specified := mk.multiClusters.zones[cluster]
		if specified {
			mk.Zone = specificZone
		}
		mk.Cluster = cluster
		// TODO: revisit the naming of kubecontexts. Currently the federation CI jobs require that the
		// cluster contexts be prefixed with `federation-` and with particular pattern.
		mk.KubeContext = "federation-e2e-gce-" + mk.Zone
		mk.kubeContextMap[cluster] = mk.KubeContext
		mk.configFile[cluster] = defaultConfigFile + "-" + mk.Cluster
		if err := mk.writeConfig(kubernetesAnywhereMultiClusterConfigTemplate); err != nil {
			return nil, err
		}
	}
	return mk, nil
}

// writeConfig writes the kubernetes-anywhere config file to file system after
// rendering the template file with configuration in deployer.
func (k *kubernetesAnywhereMultiCluster) writeConfig(configTemplate string) error {
	config, err := k.getConfig(configTemplate)
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

	if err := control.FinishRunningParallel(cmds...); err != nil {
		return err
	}

	return k.TestSetup()
}

// TestSetup sets up test environment by merging kubeconfig of multiple deployments.
func (k *kubernetesAnywhereMultiCluster) TestSetup() error {
	mergedKubeconfigPath := k.path + "/kubeconfig.json"
	var kubecfg string
	for _, cluster := range k.multiClusters.clusters {
		o, err := control.Output(exec.Command("make", "--silent", "-C", k.path, "CONFIG_FILE="+k.configFile[cluster], "kubeconfig-path"))
		if err != nil {
			return fmt.Errorf("could not get kubeconfig-path: %v", err)
		}
		if len(kubecfg) != 0 {
			kubecfg += ":"
		}
		kubecfg += strings.TrimSuffix(string(o), "\n")
	}
	if len(kubecfg) != 0 {
		kubecfg += ":" + mergedKubeconfigPath
	}

	if err := os.Setenv("KUBECONFIG", kubecfg); err != nil {
		return err
	}

	o, err := control.Output(exec.Command("kubectl", "config", "view", "--flatten=true", "--raw=true"))
	if err != nil {
		return fmt.Errorf("could not get kubeconfig-path: %v", err)
	}

	err = ioutil.WriteFile(mergedKubeconfigPath, o, 0644)
	if err != nil {
		return err
	}

	return os.Setenv("KUBECONFIG", mergedKubeconfigPath)
}

// IsUp checks if all the clusters in the deployer are up.
func (k *kubernetesAnywhereMultiCluster) IsUp() error {
	if err := k.TestSetup(); err != nil {
		return err
	}

	for _, cluster := range k.multiClusters.clusters {
		kubeContext := k.kubeContextMap[cluster]
		o, err := control.Output(exec.Command("kubectl", "--context="+kubeContext, "get", "nodes", "--no-headers"))
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
	return control.FinishRunningParallel(cmds...)
}

func (_ *kubernetesAnywhereMultiCluster) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

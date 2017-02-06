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
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

var (
	// kubernetes-anywhere specific flags.
	kubernetesAnywherePath           = flag.String("kubernetes-anywhere-path", "", "(kubernetes-anywhere only) Path to the kubernetes-anywhere directory. Must be set for kubernetes-anywhere.")
	kubernetesAnywherePhase2Provider = flag.String("kubernetes-anywhere-phase2-provider", "ignition", "(kubernetes-anywhere only) Provider for phase2 bootstrapping. (Defaults to ignition).")
	kubernetesAnywhereCluster        = flag.String("kubernetes-anywhere-cluster", "", "(kubernetes-anywhere only) Cluster name. Must be set for kubernetes-anywhere.")
	kubernetesAnywhereUpTimeout      = flag.Duration("kubernetes-anywhere-up-timeout", 20*time.Minute, "(kubernetes-anywhere only) Time limit between starting a cluster and making a successful call to the Kubernetes API.")
)

const kubernetesAnywhereConfigTemplate = `
.phase1.num_nodes=4
.phase1.cluster_name="{{.Cluster}}"
.phase1.cloud_provider="gce"

.phase1.gce.os_image="ubuntu-1604-xenial-v20160420c"
.phase1.gce.instance_type="n1-standard-2"
.phase1.gce.project="{{.Project}}"
.phase1.gce.region="us-central1"
.phase1.gce.zone="us-central1-b"
.phase1.gce.network="default"

.phase2.installer_container="docker.io/colemickens/k8s-ignition:latest"
.phase2.docker_registry="gcr.io/google-containers"
.phase2.kubernetes_version="v1.4.1"
.phase2.provider="{{.Phase2Provider}}"

.phase3.run_addons=y
.phase3.kube_proxy=y
.phase3.dashboard=y
.phase3.heapster=y
.phase3.kube_dns=y
`

type kubernetesAnywhere struct {
	path string
	// These are exported only because their use in the config template requires it.
	Phase2Provider string
	Project        string
	Cluster        string
}

func NewKubernetesAnywhere() (*kubernetesAnywhere, error) {
	if *kubernetesAnywherePath == "" {
		return nil, fmt.Errorf("--kubernetes-anywhere-path is required")
	}

	if *kubernetesAnywhereCluster == "" {
		return nil, fmt.Errorf("--kubernetes-anywhere-cluster is required")
	}

	project, ok := os.LookupEnv("PROJECT")
	if !ok {
		return nil, fmt.Errorf("The PROJECT environment variable is required to be set for kubernetes-anywhere")
	}

	// Set KUBERNETES_CONFORMANCE_TEST so the auth info is picked up
	// from kubectl instead of bash inference.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}

	k := &kubernetesAnywhere{
		path:           *kubernetesAnywherePath,
		Phase2Provider: *kubernetesAnywherePhase2Provider,
		Project:        project,
		Cluster:        *kubernetesAnywhereCluster,
	}

	if err := k.writeConfig(); err != nil {
		return nil, err
	}
	return k, nil
}

func (k kubernetesAnywhere) getConfig() (string, error) {
	// As needed, plumb through more CLI options to replace these defaults
	tmpl, err := template.New("kubernetes-anywhere-config").Parse(kubernetesAnywhereConfigTemplate)

	if err != nil {
		return "", fmt.Errorf("Error creating template for KubernetesAnywhere config: %v", err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, k); err != nil {
		return "", fmt.Errorf("Error executing template for KubernetesAnywhere config: %v", err)
	}

	return buf.String(), nil
}

func (k kubernetesAnywhere) writeConfig() error {
	config, err := k.getConfig()
	if err != nil {
		return fmt.Errorf("Could not generate config: %v", err)
	}

	f, err := os.Create(k.path + "/.config")
	if err != nil {
		return fmt.Errorf("Could not create file: %v", err)
	}
	defer f.Close()

	fmt.Fprint(f, config)
	return nil
}

func (k kubernetesAnywhere) Up() error {
	cmd := exec.Command("make", "-C", k.path, "WAIT_FOR_KUBECONFIG=y", "deploy-cluster")
	if err := finishRunning(cmd); err != nil {
		return err
	}

	nodes := 4 // For now, this is hardcoded in the config
	return waitForNodes(k, nodes+1, *kubernetesAnywhereUpTimeout)
}

func (k kubernetesAnywhere) IsUp() error {
	return isUp(k)
}

func (k kubernetesAnywhere) SetupKubecfg() error {
	output, err := exec.Command("make", "--silent", "-C", k.path, "kubeconfig-path").Output()
	if err != nil {
		return fmt.Errorf("Could not get kubeconfig-path: %v", err)
	}
	kubecfg := strings.TrimSuffix(string(output), "\n")

	if err = os.Setenv("KUBECONFIG", kubecfg); err != nil {
		return err
	}
	return nil
}

func (k kubernetesAnywhere) Down() error {
	err := finishRunning(exec.Command("make", "-C", k.path, "kubeconfig-path"))
	if err != nil {
		// This is expected if the cluster doesn't exist.
		return nil
	}
	return finishRunning(exec.Command("make", "-C", k.path, "FORCE_DESTROY=y", "destroy-cluster"))
}

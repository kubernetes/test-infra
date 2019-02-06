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
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestNewKubernetesAnywhere(t *testing.T) {
	cases := []struct {
		name                string
		phase2              string
		kubeadmVersion      string
		kubeadmUpgrade      string
		kubeletCIVersion    string
		kubeletVersion      string
		kubernetesVersion   string
		cni                 string
		expectConfigLines   []string
		kubeproxyMode       string
		osImage             string
		KubeadmFeatureGates string
	}{
		{
			name:   "kubeadm defaults",
			phase2: "kubeadm",
			cni:    "weave",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"\"",
				".phase2.kubeadm.master_upgrade.method=\"\"",
				".phase2.kubernetes_version=\"\"",
				".phase2.kubelet_version=\"\"",
				".phase3.cni=\"weave\"",
			},
		},
		{
			name:   "ignition defaults",
			phase2: "ignition",
			expectConfigLines: []string{
				".phase2.provider=\"ignition\"",
				".phase2.kubernetes_version=\"\"",
			},
		},
		{
			name:   "flannel on kubeadm",
			phase2: "kubeadm",
			cni:    "flannel",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase3.cni=\"flannel\"",
			},
		},
		{
			name:              "kubeadm with specific versions",
			phase2:            "kubeadm",
			kubeadmVersion:    "unstable",
			kubeadmUpgrade:    "init",
			kubeletVersion:    "foo",
			kubernetesVersion: "latest-1.6",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"unstable\"",
				".phase2.kubeadm.master_upgrade.method=\"init\"",
				".phase2.kubernetes_version=\"latest-1.6\"",
				".phase2.kubelet_version=\"foo\"",
				".phase3.cni=\"weave\"",
			},
		},
		{
			name:              "kubeadm with ci kubelet version",
			phase2:            "kubeadm",
			kubeadmVersion:    "unstable",
			kubeletCIVersion:  "latest-bazel",
			kubernetesVersion: "latest-bazel",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"unstable\"",
				".phase2.kubernetes_version=\"latest-bazel\"",
				".phase2.kubelet_version=\"gs://kubernetes-release-dev/ci/v1.11.0-alpha.0.1031+d37460147ec956-bazel/bin/linux/amd64/\"",
				".phase3.cni=\"weave\"",
			},
		},
		{
			name:              "kubeadm with 1.9 ci kubelet version",
			phase2:            "kubeadm",
			kubeadmVersion:    "unstable",
			kubeletCIVersion:  "latest-bazel-1.9",
			kubernetesVersion: "latest-bazel-1.9",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"unstable\"",
				".phase2.kubernetes_version=\"latest-bazel-1.9\"",
				".phase2.kubelet_version=\"gs://kubernetes-release-dev/ci/v1.9.4-beta.0.53+326c7c181909a8-bazel/bin/linux/amd64/\"",
				".phase3.cni=\"weave\"",
			},
		},
		{
			name:          "kubeadm with kube-proxy in ipvs mode",
			phase2:        "kubeadm",
			kubeproxyMode: "ipvs",

			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"\"",
				".phase2.kubeadm.master_upgrade.method=\"\"",
				".phase2.kubernetes_version=\"\"",
				".phase2.kubelet_version=\"\"",
				".phase2.proxy_mode=\"ipvs\"",
				".phase3.cni=\"weave\"",
			},
		},
		{
			name:   "kubeadm with default os_image",
			phase2: "kubeadm",

			expectConfigLines: []string{
				".phase1.gce.os_image=\"ubuntu-1604-xenial-v20171212\"",
				".phase2.provider=\"kubeadm\"",
			},
		},
		{
			name:    "kubeadm with specific os_image",
			phase2:  "kubeadm",
			osImage: "my-awesome-os-image",

			expectConfigLines: []string{
				".phase1.gce.os_image=\"my-awesome-os-image\"",
				".phase2.provider=\"kubeadm\"",
			},
		},
		{
			name:                "kubeadm with SelfHosting feature enabled",
			phase2:              "kubeadm",
			KubeadmFeatureGates: "SelfHosting=true",

			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"\"",
				".phase2.kubeadm.master_upgrade.method=\"\"",
				".phase2.kubernetes_version=\"\"",
				".phase2.kubelet_version=\"\"",
				".phase2.kubeadm.feature_gates=\"SelfHosting=true\"",
				".phase3.cni=\"weave\"",
			},
		},
	}

	mockGSFiles := map[string]string{
		"gs://kubernetes-release-dev/ci/latest-bazel.txt":     "v1.11.0-alpha.0.1031+d37460147ec956-bazel",
		"gs://kubernetes-release-dev/ci/latest-bazel-1.9.txt": "v1.9.4-beta.0.53+326c7c181909a8-bazel",
	}

	originalReadGSFile := readGSFile
	defer func() { readGSFile = originalReadGSFile }()

	readGSFile = func(location string) (string, error) {
		if val, ok := mockGSFiles[location]; ok {
			return val, nil
		}
		return "vbar", nil
	}

	for _, tc := range cases {
		tmpdir, err := ioutil.TempDir("", "kubernetes-anywhere-test")
		if err != nil {
			t.Errorf("couldn't create tempdir: %v", err)
			continue
		}

		defer os.Remove(tmpdir)

		*kubernetesAnywherePath = tmpdir
		*kubernetesAnywhereCluster = "test-cluster"
		*kubernetesAnywherePhase2Provider = tc.phase2
		*kubernetesAnywhereKubeadmVersion = tc.kubeadmVersion
		*kubernetesAnywhereKubernetesVersion = tc.kubernetesVersion
		*kubernetesAnywhereKubeletVersion = tc.kubeletVersion
		*kubernetesAnywhereKubeletCIVersion = tc.kubeletCIVersion
		*kubernetesAnywhereUpgradeMethod = tc.kubeadmUpgrade
		*kubernetesAnywhereCNI = tc.cni
		*kubernetesAnywhereProxyMode = tc.kubeproxyMode
		if tc.osImage != "" {
			*kubernetesAnywhereOSImage = tc.osImage
		}
		*kubernetesAnywhereKubeadmFeatureGates = tc.KubeadmFeatureGates

		_, err = newKubernetesAnywhere("fake-project", "fake-zone")
		if err != nil {
			t.Errorf("newKubernetesAnywhere(%s) failed: %v", tc.name, err)
			continue
		}

		config, err := ioutil.ReadFile(tmpdir + "/.config")
		if err != nil {
			t.Errorf("newKubernetesAnywhere(%s) failed to create readable config file: %v", tc.name, err)
			continue
		}

		configLines := strings.Split(string(config), "\n")

		if !containsLine(configLines, ".phase1.cloud_provider=\"gce\"") {
			t.Errorf("newKubernetesAnywhere(%s) config got %q, wanted line: .cloud_provider=\"gce\"", tc.name, config)
		}

		for _, line := range tc.expectConfigLines {
			if !containsLine(configLines, line) {
				t.Errorf("newKubernetesAnywhere(%s) config got %q, wanted line: %v", tc.name, config, line)
			}
		}
	}
}

func containsLine(haystack []string, needle string) bool {
	for _, line := range haystack {
		if line == needle {
			return true
		}
	}
	return false
}

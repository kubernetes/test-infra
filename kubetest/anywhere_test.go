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
		name              string
		phase2            string
		kubeadmVersion    string
		kubernetesVersion string
		expectConfigLines []string
	}{
		{
			name:   "kubeadm defaults",
			phase2: "kubeadm",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"\"",
				".phase2.kubernetes_version=\"\"",
				".phase3.weave_net=y",
			},
		},
		{
			name:   "ignition defaults",
			phase2: "ignition",
			expectConfigLines: []string{
				".phase2.provider=\"ignition\"",
				".phase2.kubernetes_version=\"\"",
				".phase3.weave_net=n",
			},
		},
		{
			name:              "kubeadm with specific versions",
			phase2:            "kubeadm",
			kubeadmVersion:    "unstable",
			kubernetesVersion: "latest-1.6",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase2.kubeadm.version=\"unstable\"",
				".phase2.kubernetes_version=\"latest-1.6\"",
				".phase3.weave_net=y",
			},
		},
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

func TestNewKubernetesAnywhereMultiCluster(t *testing.T) {
	tests := map[string]struct {
		mcFlag      string
		expectError bool
	}{
		"ZeroCluster": {
			mcFlag:      "",
			expectError: true,
		},
		"SingleCluster": {
			mcFlag:      "c1",
			expectError: false,
		},
		"MultiClusters": {
			mcFlag:      "c1,c2,c3",
			expectError: false,
		},
		"MultiClustersWithZonesSpecifiedForAll": {
			mcFlag:      "z1:c1,z2:c2,z3:c3",
			expectError: false,
		},
		"MultiClustersWithZonesSpecifiedForSome": {
			mcFlag:      "c1,z2:c2,c3",
			expectError: false,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			tmpdir, err := ioutil.TempDir("", "kubernetes-anywhere-multi-cluster-test")
			if err != nil {
				t.Errorf("couldn't create tempdir: %v", err)
			}
			defer os.Remove(tmpdir)

			*kubernetesAnywherePath = tmpdir
			*kubernetesAnywhereCluster = "test-cluster"
			*kubernetesAnywherePhase2Provider = "kubeadm"
			*kubernetesAnywhereKubeadmVersion = "stable"
			*kubernetesAnywhereKubernetesVersion = ""

			multiClusters := multiClusterDeployment{}
			multiClusters.Set(test.mcFlag)
			zone := "fake-zone-a"

			_, err = newKubernetesAnywhereMultiCluster("fake-project", zone, multiClusters)
			if test.expectError {
				if err == nil {
					t.Errorf("expected err but newKubernetesAnywhereMultiCluster(%s) suceeded.", testName)
				}
			} else {
				if err != nil {
					t.Errorf("newKubernetesAnywhereMultiCluster(%s) failed: %v", testName, err)
				}
			}

			for _, cluster := range multiClusters.clusters {
				config, err := ioutil.ReadFile(tmpdir + "/.config-" + cluster)
				if err != nil {
					t.Errorf("newKubernetesAnywhereMultiCluster(%s) failed to create readable config file: %v", testName, err)
				}

				specificZone, specified := multiClusters.zones[cluster]
				if specified {
					zone = specificZone
				}
				kubeContext := zone + "-" + cluster
				expectConfigLines := []string{
					".phase1.cloud_provider=\"gce\"",
					".phase2.provider=\"kubeadm\"",
					".phase2.kubeadm.version=\"stable\"",
					".phase2.kubernetes_version=\"\"",
					".phase3.weave_net=y",
					".phase1.cluster_name=\"" + cluster + "\"",
					".phase1.gce.zone=\"" + zone + "\"",
					".phase2.kube_context_name=\"" + kubeContext + "\"",
				}

				configLines := strings.Split(string(config), "\n")

				for _, line := range expectConfigLines {
					if !containsLine(configLines, line) {
						t.Errorf("newKubernetesAnywhereMultiCluster(%s) config got %q, wanted line: %v", testName, config, line)
					}
				}
			}
		})
	}
}

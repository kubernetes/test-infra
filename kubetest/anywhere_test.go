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
	if err := os.Setenv("PROJECT", "test-project"); err != nil {
		t.Fatalf("couldn't set PROJECT environment: %v", err)
	}

	cases := []struct {
		phase2            string
		expectConfigLines []string
	}{
		{
			phase2: "kubeadm",
			expectConfigLines: []string{
				".phase2.provider=\"kubeadm\"",
				".phase3.weave_net=y",
			},
		},
		{
			phase2: "ignition",
			expectConfigLines: []string{
				".phase2.provider=\"ignition\"",
				".phase3.weave_net=n",
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

		_, err = NewKubernetesAnywhere()
		if err != nil {
			t.Errorf("NewKubernetesAnywhere(%s) failed: %v", tc.phase2, err)
			continue
		}

		config, err := ioutil.ReadFile(tmpdir + "/.config")
		if err != nil {
			t.Errorf("NewKubernetesAnywhere(%s) failed to create readable config file: %v", tc.phase2, err)
			continue
		}

		configLines := strings.Split(string(config), "\n")

		if !containsLine(configLines, ".phase1.cloud_provider=\"gce\"") {
			t.Errorf("NewKubernetesAnywhere(%s) config got %q, wanted line: .cloud_provider=\"gce\"", tc.phase2, config)
		}

		for _, line := range tc.expectConfigLines {
			if !containsLine(configLines, line) {
				t.Errorf("NewKubernetesAnywhere(%s) config got %q, wanted line: %v", tc.phase2, config, line)
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

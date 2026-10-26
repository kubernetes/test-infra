/*
Copyright 2026 The Kubernetes Authors.

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

package tests

import (
	"strings"
	"testing"
)

func TestDRANodePresubmitRunIfChanged(t *testing.T) {
	tests := []struct {
		name    string
		changes []string
		want    bool
	}{
		{name: "node_test", changes: []string{"test/e2e_node/dra_test.go"}, want: true},
		{name: "node_helper", changes: []string{"test/e2e_node/dra_helpers.go"}, want: true},
		{name: "resource_api", changes: []string{"pkg/apis/resource/types.go"}, want: true},
		{name: "resource_api_yaml", changes: []string{"pkg/apis/resource/testdata/claim.yaml"}, want: true},
		{name: "kubelet_dra", changes: []string{"pkg/kubelet/cm/dra/manager.go"}, want: true},
		{name: "staging_api", changes: []string{"staging/src/k8s.io/api/resource/v1/types.go"}, want: true},
		{name: "dynamicresources", changes: []string{"staging/src/k8s.io/dynamic-resource-allocation/kubeletplugin/draplugin.go"}, want: true},
		{name: "e2e_yaml", changes: []string{"test/e2e/dra/test-driver.yaml"}, want: true},
		{name: "nested_resource_api", changes: []string{"nested/pkg/apis/resource/types.go"}, want: true},
		{name: "nested_node_test", changes: []string{"nested/test/e2e_node/dra_test.go"}, want: true},
		{name: "unrelated_api", changes: []string{"pkg/apis/core/types.go"}},
		{name: "unrelated_node_test", changes: []string{"test/e2e_node/cpu_manager_test.go"}},
		{name: "directory_prefix", changes: []string{"notpkg/apis/resource/types.go"}},
		{name: "node_directory_prefix", changes: []string{"othertest/e2e_node/dra_test.go"}},
		{name: "directory_suffix", changes: []string{"pkg/apis/resources/types.go"}},
		{name: "filename_prefix", changes: []string{"test/e2e_node/drain_test.go"}},
		{name: "documentation", changes: []string{"pkg/kubelet/cm/dra/README.md"}},
		{name: "no_changes"},
		{name: "mixed_changes", changes: []string{"README.md", "pkg/apis/resource/types.go"}, want: true},
	}

	for _, suffix := range []string{"ubuntu-dra", "ubuntu-dra-alpha-beta-features"} {
		t.Run(suffix, func(t *testing.T) {
			matched := false
			for _, job := range c.GetPresubmitsStatic("kubernetes/kubernetes") {
				if !strings.HasPrefix(job.Name, "pull-kubernetes-node-e2e-containerd-") ||
					!strings.HasSuffix(job.Name, "-"+suffix) || !job.CouldRun("master") {
					continue
				}
				matched = true
				t.Run(job.Name, func(t *testing.T) {
					for _, test := range tests {
						t.Run(test.name, func(t *testing.T) {
							got, err := job.ShouldRun("master", func() ([]string, error) {
								return test.changes, nil
							}, false, false)
							if err != nil {
								t.Fatal(err)
							}
							if got != test.want {
								t.Errorf("ShouldRun for %v = %t, want %t", test.changes, got, test.want)
							}
						})
					}
				})
			}
			if !matched {
				t.Fatalf("no master containerd node presubmit found for %q", suffix)
			}
		})
	}
}

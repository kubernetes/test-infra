/*
Copyright 2019 The Kubernetes Authors.

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

package flagutil

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/test-infra/pkg/flagutil"
)

func TestExperimentalKubernetesOptions_Validate(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "some-file")
	if err := os.WriteFile(configFile, []byte("a"), 0644); err != nil {
		t.Fatalf("failed to write kubeconfig file %q: %v", configFile, err)
	}
	configDir := filepath.Join(dir, "some-dir")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create a directory %q: %v", configDir, err)
	}
	defer t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("failed to remove files in %s: %v", dir, err)
		}
	})

	var testCases = []struct {
		name        string
		dryRun      bool
		kubernetes  flagutil.OptionGroup
		expectedErr bool
	}{
		{
			name:        "all ok without dry-run",
			dryRun:      false,
			kubernetes:  &KubernetesOptions{},
			expectedErr: false,
		},
		{
			name:        "all ok with dry-run",
			dryRun:      true,
			kubernetes:  &KubernetesOptions{},
			expectedErr: false,
		},
		{
			name: "kubeconfig can be set alone",
			kubernetes: &KubernetesOptions{
				kubeconfig: configFile,
			},
		},
		{
			name: "kubeconfigDir can be set alone",
			kubernetes: &KubernetesOptions{
				kubeconfigDir: configDir,
			},
		},
		{
			name: "kubeconfig and kubeconfigDir can be set together",
			kubernetes: &KubernetesOptions{
				kubeconfig:    configFile,
				kubeconfigDir: configDir,
			},
		},
		{
			name: "kubeconfigSuffix can be set",
			kubernetes: &KubernetesOptions{
				kubeconfigDir:    configDir,
				kubeconfigSuffix: "suffix",
			},
		},
		{
			name: "kubeconfigSuffix must be used with kubeconfigDir",
			kubernetes: &KubernetesOptions{
				kubeconfigSuffix: "suffix",
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.kubernetes.Validate(testCase.dryRun)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
		})
	}
}

func TestResolveSetsConfigsButDoesntSetClients(t *testing.T) {
	config := `apiVersion: v1
clusters:
- cluster:
    server: https://build
  name: build
- cluster:
    server: https://kubernetes.default
  name: incluster
contexts:
- context:
    cluster: build
    user: build
  name: build
- context:
    cluster: incluster
    user: incluster
  name: incluster
kind: Config
# We consider current-context to be in-cluster context to be prowjob context ¯\_(ツ)_/¯
# https://github.com/kubernetes/test-infra/blob/1f02f841b7ffdbe78f14d20cabba4531e34feb20/prow/kube/config.go#L126-L127
current-context: incluster
users:
- name: build
  user:
    token: abc
- name: incluster
  user:
    token: cde
`

	tmpFile, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatalf("failed to get tempfile: %v", err)
	}
	kubeconfig := tmpFile.Name()
	defer func() {
		if err := os.Remove(kubeconfig); err != nil {
			t.Errorf("failed to remove tempfile: %v", err)
		}
	}()
	if _, err := tmpFile.Write([]byte(config)); err != nil {
		t.Errorf("failed to write config to tempfile: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Errorf("failed to close tempfile: %v", err)
	}

	o := &KubernetesOptions{kubeconfig: kubeconfig}
	if err := o.resolve(true); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(o.clusterConfigs) == 0 {
		t.Errorf("expected clsuterconfig to be set, was %v", o.clusterConfigs)
	}
	if o.infrastructureClusterConfig == nil {
		t.Error("expected infrastructureClusterConfig to be set, was nil")
	}
	if len(o.kubernetesClientsByContext) != 0 {
		t.Errorf("expected kubernetes clients to be nil, was %v", o.kubernetesClientsByContext)
	}
	if o.prowJobClientset != nil {
		t.Errorf("expected prowJobClientset to be nil, was %v", o.prowJobClientset)
	}
}

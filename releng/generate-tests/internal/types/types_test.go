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

package types_test

import (
	"strings"
	"testing"

	"k8s.io/test-infra/releng/generate-tests/internal/types"
	"sigs.k8s.io/yaml"
)

func sampleProwJob() types.ProwJob {
	return types.ProwJob{
		Name:     "test-job",
		Interval: "1h",
		Decorate: true,
		Tags:     []string{"generated"},
		Labels:   map[string]string{"foo": "bar"},
		DecorationConfig: map[string]string{
			"timeout": "100m",
		},
		Cluster: "test-cluster",
		Spec: types.ProwSpec{
			Containers: []types.ProwContainer{
				{
					Command: []string{"cmd"},
					Args:    []string{"--arg"},
					Image:   "image:latest",
					Resources: types.ProwResources{
						Requests: types.ProwResourceValues{CPU: "1000m", Memory: "3Gi"},
						Limits:   types.ProwResourceValues{CPU: "1000m", Memory: "3Gi"},
					},
				},
			},
		},
		ExtraRefs: []types.ProwExtraRef{
			{
				Org:       "org",
				Repo:      "repo",
				BaseRef:   "main",
				PathAlias: "path/alias",
			},
		},
		Annotations: map[string]string{"key": "value"},
	}
}

func TestProwJobYAMLSnakeCaseFields(t *testing.T) {
	t.Parallel()

	prowJob := sampleProwJob()

	data, err := yaml.Marshal(prowJob)
	if err != nil {
		t.Fatalf("failed to marshal ProwJob: %v", err)
	}

	yamlStr := string(data)

	expectedFields := []string{
		"tags:", "interval:", "labels:", "decorate:", "decoration_config:",
		"name:", "spec:", "cluster:", "extra_refs:", "annotations:",
		"containers:", "command:", "args:", "image:", "resources:",
		"requests:", "limits:", "cpu:", "memory:",
		"org:", "repo:", "base_ref:", "path_alias:",
	}
	for _, field := range expectedFields {
		if !strings.Contains(yamlStr, field) {
			t.Errorf("expected field %q in YAML output, got:\n%s", field, yamlStr)
		}
	}
}

func TestProwJobYAMLNoPascalCase(t *testing.T) {
	t.Parallel()

	prowJob := sampleProwJob()

	data, err := yaml.Marshal(prowJob)
	if err != nil {
		t.Fatalf("failed to marshal ProwJob: %v", err)
	}

	yamlStr := string(data)

	forbiddenFields := []string{
		"Tags:", "Interval:", "Labels:", "Decorate:", "DecorationConfig:",
		"Name:", "Spec:", "Cluster:", "ExtraRefs:", "Annotations:",
		"Containers:", "Command:", "Args:", "Image:", "Resources:",
		"Requests:", "Limits:", "CPU:", "Memory:",
		"Org:", "Repo:", "BaseRef:", "PathAlias:",
	}
	for _, field := range forbiddenFields {
		if strings.Contains(yamlStr, field) {
			t.Errorf("unexpected PascalCase field %q in YAML output, got:\n%s", field, yamlStr)
		}
	}
}

func TestTestGroupYAMLFieldNames(t *testing.T) {
	t.Parallel()

	testGroup := types.TestGroup{
		Name:      "test-group",
		GCSPrefix: "bucket/logs/test-group",
		ColumnHeader: []types.ColumnHeader{
			{ConfigurationValue: "node_os_image"},
		},
	}

	data, err := yaml.Marshal(testGroup)
	if err != nil {
		t.Fatalf("failed to marshal TestGroup: %v", err)
	}

	yamlStr := string(data)

	expectedFields := []string{
		"name:", "gcs_prefix:", "column_header:", "configuration_value:",
	}
	for _, field := range expectedFields {
		if !strings.Contains(yamlStr, field) {
			t.Errorf("expected field %q in YAML output, got:\n%s", field, yamlStr)
		}
	}
}

func TestProwConfigYAMLFieldNames(t *testing.T) {
	t.Parallel()

	prowConfig := types.ProwConfig{
		Periodics: []types.ProwJob{
			{
				Name:             "test",
				Interval:         "",
				Tags:             nil,
				Labels:           nil,
				Decorate:         false,
				DecorationConfig: nil,
				Spec:             types.ProwSpec{Containers: nil},
				Cluster:          "",
				ExtraRefs:        nil,
				Annotations:      nil,
			},
		},
	}

	data, err := yaml.Marshal(prowConfig)
	if err != nil {
		t.Fatalf("failed to marshal ProwConfig: %v", err)
	}

	if !strings.Contains(string(data), "periodics:") {
		t.Errorf("expected 'periodics:' in YAML output, got:\n%s", string(data))
	}
}

func TestTestGridConfigYAMLFieldNames(t *testing.T) {
	t.Parallel()

	tgc := types.TestGridConfig{
		TestGroups: []types.TestGroup{
			{
				Name:         "test",
				GCSPrefix:    "",
				ColumnHeader: nil,
			},
		},
	}

	data, err := yaml.Marshal(tgc)
	if err != nil {
		t.Fatalf("failed to marshal TestGridConfig: %v", err)
	}

	if !strings.Contains(string(data), "test_groups:") {
		t.Errorf("expected 'test_groups:' in YAML output, got:\n%s", string(data))
	}
}

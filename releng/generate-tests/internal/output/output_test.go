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

package output_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/test-infra/releng/generate-tests/internal/config"
	"k8s.io/test-infra/releng/generate-tests/internal/output"
	"k8s.io/test-infra/releng/generate-tests/internal/types"
	"sigs.k8s.io/yaml"
)

func sampleConfigs() (types.ProwConfig, types.TestGridConfig) {
	prowConfig := types.ProwConfig{
		Periodics: []types.ProwJob{
			{
				Name:             "test-job-1",
				Interval:         "1h",
				Tags:             []string{"generated"},
				Labels:           map[string]string{"preset-service-account": "true"},
				Decorate:         true,
				DecorationConfig: map[string]string{"timeout": "140m"},
				Cluster:          config.Cluster,
				Spec: types.ProwSpec{
					Containers: []types.ProwContainer{
						{
							Command: []string{"runner.sh"},
							Args:    []string{"--arg1"},
							Image:   config.DefaultImage,
							Resources: types.ProwResources{
								Requests: types.ProwResourceValues{CPU: "1000m", Memory: "3Gi"},
								Limits:   types.ProwResourceValues{CPU: "1000m", Memory: "3Gi"},
							},
						},
					},
				},
				ExtraRefs: []types.ProwExtraRef{
					{
						Org:       "kubernetes",
						Repo:      "kubernetes",
						BaseRef:   "release-1.35",
						PathAlias: "k8s.io/kubernetes",
					},
				},
				Annotations: map[string]string{"testgrid-dashboards": "test"},
			},
		},
	}

	tgc := types.TestGridConfig{
		TestGroups: []types.TestGroup{
			{
				Name:      "test-job-1",
				GCSPrefix: config.GCSLogPrefix + "test-job-1",
				ColumnHeader: []types.ColumnHeader{
					{ConfigurationValue: "node_os_image"},
				},
			},
		},
	}

	return prowConfig, tgc
}

func TestWriteOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tgPath := filepath.Join(dir, "testgrid.yaml")
	pc, tgc := sampleConfigs()

	if err := output.Write(pc, tgc, dir, tgPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify Prow config file exists and is valid YAML.
	prowPath := filepath.Join(dir, "generated.yaml")

	prowData, err := os.ReadFile(prowPath)
	if err != nil {
		t.Fatalf("failed to read generated.yaml: %v", err)
	}

	var parsedPC types.ProwConfig
	if err := yaml.Unmarshal(prowData, &parsedPC); err != nil {
		t.Fatalf("failed to parse generated.yaml: %v", err)
	}

	if len(parsedPC.Periodics) != 1 {
		t.Errorf("expected 1 periodic, got %d", len(parsedPC.Periodics))
	}

	if parsedPC.Periodics[0].Name != "test-job-1" {
		t.Errorf("expected job name test-job-1, got %q", parsedPC.Periodics[0].Name)
	}

	// Verify TestGrid config file exists and is valid YAML.
	tgData, err := os.ReadFile(tgPath)
	if err != nil {
		t.Fatalf("failed to read testgrid.yaml: %v", err)
	}

	var parsedTGC types.TestGridConfig
	if err := yaml.Unmarshal(tgData, &parsedTGC); err != nil {
		t.Fatalf("failed to parse testgrid.yaml: %v", err)
	}

	if len(parsedTGC.TestGroups) != 1 {
		t.Errorf("expected 1 test group, got %d", len(parsedTGC.TestGroups))
	}
}

func TestWriteOutputTestGridComment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tgPath := filepath.Join(dir, "testgrid.yaml")
	pc, tgc := sampleConfigs()

	if err := output.Write(pc, tgc, dir, tgPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	tgData, err := os.ReadFile(tgPath)
	if err != nil {
		t.Fatalf("failed to read testgrid.yaml: %v", err)
	}

	if !strings.HasPrefix(string(tgData), "# "+config.Comment) {
		t.Errorf("testgrid output should start with comment, got:\n%s", string(tgData)[:100])
	}
}

func TestWriteOutputCreatesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tgPath := filepath.Join(dir, "subdir", "testgrid.yaml")
	pc, tgc := sampleConfigs()

	// Writing to a non-existent subdirectory should fail.
	err := output.Write(pc, tgc, dir, tgPath)
	if err == nil {
		t.Error("expected error writing to nonexistent subdirectory")
	}
}

func TestWriteOutputInvalidDir(t *testing.T) {
	t.Parallel()

	pc, tgc := sampleConfigs()

	err := output.Write(pc, tgc, "/nonexistent/dir", "/tmp/tg.yaml")
	if err == nil {
		t.Error("expected error for nonexistent output dir")
	}
}

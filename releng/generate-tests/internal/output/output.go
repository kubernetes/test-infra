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

// Package output writes generated Prow and TestGrid configurations to YAML files.
package output

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	gyaml "go.yaml.in/yaml/v2"
	"k8s.io/test-infra/releng/generate-tests/internal/config"
	"k8s.io/test-infra/releng/generate-tests/internal/types"
	"sigs.k8s.io/yaml"
)

func init() {
	gyaml.FutureLineWrap()
}

// Write writes the Prow config and TestGrid config to the specified paths.
func Write(
	prowConfig types.ProwConfig,
	tgc types.TestGridConfig,
	outputDir, testgridOutputPath string,
) error {

	prowData, err := yaml.Marshal(prowConfig)
	if err != nil {
		return fmt.Errorf("marshaling Prow config: %w", err)
	}

	outputFile := filepath.Join(outputDir, "generated.yaml")
	log.Printf("writing prow configuration to: %s", outputFile)

	if err := os.WriteFile(outputFile, prowData, config.FilePermissions); err != nil {
		return fmt.Errorf("writing Prow config: %w", err)
	}

	tgData, err := yaml.Marshal(tgc)
	if err != nil {
		return fmt.Errorf("marshaling TestGrid config: %w", err)
	}

	tgOutput := []byte("# " + config.Comment + "\n\n")
	tgOutput = append(tgOutput, tgData...)

	log.Printf("writing testgrid configuration to: %s", testgridOutputPath)

	if err := os.WriteFile(testgridOutputPath, tgOutput, config.FilePermissions); err != nil {
		return fmt.Errorf("writing TestGrid config: %w", err)
	}

	return nil
}

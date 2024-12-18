/*
Copyright 2018 The Kubernetes Authors.

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

package policy

// This file validates Kubernetes's jobs configs against policies.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/prow/pkg/config"
	"sigs.k8s.io/yaml"
)

func TestYaml(t *testing.T) {
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		t.Run(path, func(t *testing.T) {
			var content struct {
				Presets    []config.Preset               `json:"presets,omitempty"`
				Templates  any                           `json:"templates,omitempty"`
				Periodics  []config.Periodic             `json:"periodics,omitempty"`
				PreSubmits map[string][]config.Presubmit `json:"presubmits,omitempty"`
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("error reading file: %v", err)
			}
			// Because of https://github.com/kubernetes-sigs/yaml/issues/46,
			// strict parsing cannot be used as it would complain about
			// repeated keys in maps, which is intentional and (in YAML)
			// valid when using aliases.
			if err := yaml.Unmarshal(data, &content, func(d *json.Decoder) *json.Decoder {
				d.DisallowUnknownFields()
				return d
			}); err != nil {
				t.Fatalf("error unmarshaling data: %v", err)
			}

			// The Templates are just helpers, what matters are the jobs.
			content.Templates = nil

			data, err = yaml.Marshal(&content)
			if err != nil {
				t.Fatalf("error re-marshaling content: %v", err)
			}
			t.Logf("\n%s", string(data))
		})
		return nil
	}); err != nil {
		t.Errorf("Error looking for YAML files: %v", err)
	}
}

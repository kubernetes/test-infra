/*
Copyright The Kubernetes Authors.

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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"
)

const (
	supplementalPluginConfigSuffix = "_pluginconfig.yaml"
	// supplementalPluginConfigOrg must match the config_updater rule for
	// supplemental plugin configs in config/prow/plugins.yaml.
	supplementalPluginConfigOrg = "kubernetes-sigs"
)

// Test_SupplementalPluginConfigs ensures that supplemental plugin config files
// only let subproject maintainers own their branch-to-milestone mappings.
// Plugin enablement and other plugin configuration stay in
// config/prow/plugins.yaml under the existing ownership model, so these files
// may only contain milestone_applier entries for the repository they live in.
func Test_SupplementalPluginConfigs(t *testing.T) {
	for _, err := range validateSupplementalPluginConfigs(filepath.Join(configPath, "prow", "plugins")) {
		t.Error(err)
	}
}

func Test_ValidateSupplementalPluginConfigs(t *testing.T) {
	const valid = "milestone_applier:\n  kubernetes-sigs/kueue:\n    main: v0.21\n    release-0.20: v0.20\n"
	testCases := map[string]struct {
		files   map[string]string
		links   map[string]string
		wantErr []string
	}{
		"milestone mappings for the matching repo are allowed": {
			files: map[string]string{"OWNERS": "", "kubernetes-sigs/kueue/OWNERS": "", "kubernetes-sigs/kueue/kueue_pluginconfig.yaml": valid},
		},
		"other plugin fields are rejected": {
			files:   map[string]string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml": valid + "plugins:\n  kubernetes-sigs/kueue:\n    plugins:\n    - milestone\n"},
			wantErr: []string{`kubernetes-sigs/kueue/kueue_pluginconfig.yaml: field "plugins" is not allowed, only milestone_applier may be configured here`},
		},
		"milestone mappings for another repo are rejected": {
			files:   map[string]string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml": valid + "  kubernetes-sigs/jobset:\n    main: v0.11\n"},
			wantErr: []string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml: milestone_applier may only contain kubernetes-sigs/kueue, but contains kubernetes-sigs/jobset"},
		},
		"org-level milestone mappings are rejected": {
			files:   map[string]string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml": "milestone_applier:\n  kubernetes-sigs:\n    main: v1.0\n"},
			wantErr: []string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml: milestone_applier may only contain kubernetes-sigs/kueue, but contains kubernetes-sigs"},
		},
		"malformed milestone mappings are rejected": {
			files:   map[string]string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml": "milestone_applier:\n  kubernetes-sigs/kueue: v0.21\n"},
			wantErr: []string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml: milestone_applier must map kubernetes-sigs/kueue to branch-to-milestone mappings"},
		},
		"duplicate keys are rejected": {
			files:   map[string]string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml": valid + "    main: v0.22\n"},
			wantErr: []string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml: failed to parse: error converting YAML to JSON: yaml: unmarshal errors:\n  line 5: key \"main\" already set in map"},
		},
		"orgs other than kubernetes-sigs are rejected": {
			files:   map[string]string{"kubernetes/kubernetes/kubernetes_pluginconfig.yaml": "milestone_applier:\n  kubernetes/kubernetes:\n    master: v1.38\n"},
			wantErr: []string{"kubernetes/kubernetes/kubernetes_pluginconfig.yaml: supplemental plugin configs are only supported for the kubernetes-sigs org"},
		},
		"files not named after their repo are rejected": {
			files:   map[string]string{"kubernetes-sigs/kueue/jobset_pluginconfig.yaml": valid},
			wantErr: []string{"kubernetes-sigs/kueue/jobset_pluginconfig.yaml: supplemental plugin configs must be located at <org>/<repo>/<repo>_pluginconfig.yaml"},
		},
		"files outside <org>/<repo> are rejected": {
			files: map[string]string{"kubernetes-sigs/kueue_pluginconfig.yaml": valid, "kubernetes-sigs/kueue/sub/sub_pluginconfig.yaml": valid},
			wantErr: []string{
				"kubernetes-sigs/kueue/sub/sub_pluginconfig.yaml: supplemental plugin configs must be located at <org>/<repo>/<repo>_pluginconfig.yaml",
				"kubernetes-sigs/kueue_pluginconfig.yaml: supplemental plugin configs must be located at <org>/<repo>/<repo>_pluginconfig.yaml",
			},
		},
		"symlinks are rejected and do not hide other errors": {
			files: map[string]string{"kubernetes-sigs/kueue/kueue_pluginconfig.yaml": valid + "plugins: {}\n"},
			links: map[string]string{"kubernetes-sigs/aaa/aaa_pluginconfig.yaml": "missing"},
			wantErr: []string{
				"kubernetes-sigs/aaa/aaa_pluginconfig.yaml: only regular files are allowed",
				`kubernetes-sigs/kueue/kueue_pluginconfig.yaml: field "plugins" is not allowed, only milestone_applier may be configured here`,
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write := func(rel string) string {
				path := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				return path
			}
			for rel, content := range tc.files {
				if err := os.WriteFile(write(rel), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for rel, target := range tc.links {
				if err := os.Symlink(target, write(rel)); err != nil {
					t.Fatal(err)
				}
			}

			var gotErr []string
			for _, err := range validateSupplementalPluginConfigs(root) {
				gotErr = append(gotErr, strings.TrimPrefix(err.Error(), root+string(filepath.Separator)))
			}
			if diff := cmp.Diff(tc.wantErr, gotErr); diff != "" {
				t.Errorf("unexpected errors (-want +got):\n%s", diff)
			}
		})
	}
}

func validateSupplementalPluginConfigs(root string) []error {
	var errs []error
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			errs = append(errs, fmt.Errorf("%s: only regular files are allowed", path))
			return nil
		}
		if info.Name() == "OWNERS" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if err := validateSupplementalPluginConfig(path, filepath.ToSlash(rel)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
		return nil
	}); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func validateSupplementalPluginConfig(path, rel string) error {
	orgRepo, err := validateSupplementalPluginConfigPath(rel)
	if err != nil {
		return err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	if err := yaml.UnmarshalStrict(b, &cfg); err != nil {
		return fmt.Errorf("failed to parse: %w", err)
	}
	for field := range cfg {
		if field != "milestone_applier" {
			return fmt.Errorf("field %q is not allowed, only milestone_applier may be configured here", field)
		}
	}

	var parsed struct {
		MilestoneApplier map[string]map[string]string `json:"milestone_applier"`
	}
	if err := yaml.Unmarshal(b, &parsed); err != nil {
		return fmt.Errorf("milestone_applier must map %s to branch-to-milestone mappings", orgRepo)
	}
	for orgOrRepo := range parsed.MilestoneApplier {
		if orgOrRepo != orgRepo {
			return fmt.Errorf("milestone_applier may only contain %s, but contains %s", orgRepo, orgOrRepo)
		}
	}
	return nil
}

// validateSupplementalPluginConfigPath checks that rel is located at
// <org>/<repo>/<repo>_pluginconfig.yaml for a supported org and returns
// the <org>/<repo> the file configures.
func validateSupplementalPluginConfigPath(rel string) (string, error) {
	// config_updater uses the file name as the configmap key, so the file
	// is named after its repo to keep keys unique across repos.
	errLayout := fmt.Errorf("supplemental plugin configs must be located at <org>/<repo>/<repo>%s", supplementalPluginConfigSuffix)
	parts := strings.Split(rel, "/")
	if len(parts) != 3 {
		return "", errLayout
	}
	org, repo, file := parts[0], parts[1], parts[2]
	if file != repo+supplementalPluginConfigSuffix {
		return "", errLayout
	}
	if org != supplementalPluginConfigOrg {
		return "", fmt.Errorf("supplemental plugin configs are only supported for the %s org", supplementalPluginConfigOrg)
	}
	return org + "/" + repo, nil
}

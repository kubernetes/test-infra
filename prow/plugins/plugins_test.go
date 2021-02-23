/*
Copyright 2016 The Kubernetes Authors.

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

package plugins

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/pkg/genyaml"
)

func TestHasSelfApproval(t *testing.T) {
	cases := []struct {
		name     string
		cfg      string
		expected bool
	}{
		{
			name:     "self approval by default",
			expected: true,
		},
		{
			name:     "reject approval when require_self_approval set",
			cfg:      `{"require_self_approval": true}`,
			expected: false,
		},
		{
			name:     "has approval when require_self_approval set to false",
			cfg:      `{"require_self_approval": false}`,
			expected: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a Approve
			if err := yaml.Unmarshal([]byte(tc.cfg), &a); err != nil {
				t.Fatalf("failed to unmarshal cfg: %v", err)
			}
			if actual := a.HasSelfApproval(); actual != tc.expected {
				t.Errorf("%t != expected %t", actual, tc.expected)
			}
		})
	}
}

func TestConsiderReviewState(t *testing.T) {
	cases := []struct {
		name     string
		cfg      string
		expected bool
	}{
		{
			name:     "consider by default",
			expected: true,
		},
		{
			name: "do not consider when irs = true",
			cfg:  `{"ignore_review_state": true}`,
		},
		{
			name:     "consider when irs = false",
			cfg:      `{"ignore_review_state": false}`,
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a Approve
			if err := yaml.Unmarshal([]byte(tc.cfg), &a); err != nil {
				t.Fatalf("failed to unmarshal cfg: %v", err)
			}
			if actual := a.ConsiderReviewState(); actual != tc.expected {
				t.Errorf("%t != expected %t", actual, tc.expected)
			}
		})
	}
}

func TestGetPluginsLegacy(t *testing.T) {
	var testcases = []struct {
		name            string
		pluginMap       map[string][]string // this is read from the plugins.yaml file typically.
		owner           string
		repo            string
		expectedPlugins []string
	}{
		{
			name: "All plugins enabled for org should be returned for any org/repo query",
			pluginMap: map[string][]string{
				"org1": {"plugin1", "plugin2"},
			},
			owner:           "org1",
			repo:            "repo",
			expectedPlugins: []string{"plugin1", "plugin2"},
		},
		{
			name: "All plugins enabled for org/repo should be returned for a org/repo query",
			pluginMap: map[string][]string{
				"org1":      {"plugin1", "plugin2"},
				"org1/repo": {"plugin3"},
			},
			owner:           "org1",
			repo:            "repo",
			expectedPlugins: []string{"plugin1", "plugin2", "plugin3"},
		},
		{
			name: "Plugins for org1/repo should not be returned for org2/repo query",
			pluginMap: map[string][]string{
				"org1":      {"plugin1", "plugin2"},
				"org1/repo": {"plugin3"},
			},
			owner:           "org2",
			repo:            "repo",
			expectedPlugins: nil,
		},
		{
			name: "Plugins for org1 should not be returned for org2/repo query",
			pluginMap: map[string][]string{
				"org1":      {"plugin1", "plugin2"},
				"org2/repo": {"plugin3"},
			},
			owner:           "org2",
			repo:            "repo",
			expectedPlugins: []string{"plugin3"},
		},
	}
	for _, tc := range testcases {
		pa := ConfigAgent{configuration: &Configuration{Plugins: OldToNewPlugins(tc.pluginMap)}}

		plugins := pa.getPlugins(tc.owner, tc.repo)
		if diff := cmp.Diff(plugins, tc.expectedPlugins); diff != "" {
			t.Errorf("Actual plugins differ from expected: %s", diff)
		}
	}
}

func TestGetPlugins(t *testing.T) {
	var testcases = []struct {
		name            string
		pluginMap       Plugins // this is read from the plugins.yaml file typically.
		owner           string
		repo            string
		expectedPlugins []string
	}{
		{
			name: "All plugins enabled for org should be returned for any org/repo query",
			pluginMap: Plugins{
				"org1": {Plugins: []string{"plugin1", "plugin2"}},
			},
			owner:           "org1",
			repo:            "repo",
			expectedPlugins: []string{"plugin1", "plugin2"},
		},
		{
			name: "All plugins enabled for org/repo should be returned for a org/repo query",
			pluginMap: Plugins{
				"org1":      {Plugins: []string{"plugin1", "plugin2"}},
				"org1/repo": {Plugins: []string{"plugin3"}},
			},
			owner:           "org1",
			repo:            "repo",
			expectedPlugins: []string{"plugin1", "plugin2", "plugin3"},
		},
		{
			name: "Excluded plugins for repo enabled for org/repo should not be returned for a org/repo query",
			pluginMap: Plugins{
				"org1":      {Plugins: []string{"plugin1", "plugin2", "plugin3"}, ExcludedRepos: []string{"repo"}},
				"org1/repo": {Plugins: []string{"plugin3"}},
			},
			owner:           "org1",
			repo:            "repo",
			expectedPlugins: []string{"plugin3"},
		},
		{
			name: "Plugins for org1/repo should not be returned for org2/repo query",
			pluginMap: Plugins{
				"org1":      {Plugins: []string{"plugin1", "plugin2"}},
				"org1/repo": {Plugins: []string{"plugin3"}},
			},
			owner:           "org2",
			repo:            "repo",
			expectedPlugins: nil,
		},
		{
			name: "Plugins for org1 should not be returned for org2/repo query",
			pluginMap: Plugins{
				"org1":      {Plugins: []string{"plugin1", "plugin2"}},
				"org2/repo": {Plugins: []string{"plugin3"}},
			},
			owner:           "org2",
			repo:            "repo",
			expectedPlugins: []string{"plugin3"},
		},
	}
	for _, tc := range testcases {
		pa := ConfigAgent{configuration: &Configuration{Plugins: tc.pluginMap}}

		plugins := pa.getPlugins(tc.owner, tc.repo)
		if diff := cmp.Diff(plugins, tc.expectedPlugins); diff != "" {
			t.Errorf("Actual plugins differ from expected: %s", diff)
		}
	}
}

func TestGenYamlDocs(t *testing.T) {
	const fixtureName = "./plugin-config-documented.yaml"
	inputFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("filepath.Glob: %v", err)
	}

	commentMap, err := genyaml.NewCommentMap(inputFiles...)
	if err != nil {
		t.Fatalf("failed to construct commentMap: %v", err)
	}
	actualYaml, err := commentMap.GenYaml(genyaml.PopulateStruct(&Configuration{}))
	if err != nil {
		t.Fatalf("genyaml errored: %v", err)
	}
	if os.Getenv("UPDATE") != "" {
		if err := ioutil.WriteFile(fixtureName, []byte(actualYaml), 0644); err != nil {
			t.Fatalf("failed to write fixture: %v", err)
		}
	}
	expectedYaml, err := ioutil.ReadFile(fixtureName)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if diff := cmp.Diff(actualYaml, string(expectedYaml)); diff != "" {
		t.Errorf("Actual result differs from expected: %s. If this is expected, re-run the tests with the UPDATE env var set to update the fixture: UPDATE=true go test ./...", diff)
	}
}

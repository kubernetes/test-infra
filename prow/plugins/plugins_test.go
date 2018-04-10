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
	"errors"
	"reflect"
	"testing"
)

func TestGetPlugins(t *testing.T) {
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
		pa := PluginAgent{configuration: &Configuration{Plugins: tc.pluginMap}}

		plugins := pa.getPlugins(tc.owner, tc.repo)
		if len(plugins) != len(tc.expectedPlugins) {
			t.Errorf("Different number of plugins for case \"%s\". Got %v, expected %v", tc.name, plugins, tc.expectedPlugins)
		} else {
			for i := range plugins {
				if plugins[i] != tc.expectedPlugins[i] {
					t.Errorf("Different plugin for case \"%s\": Got %v expected %v", tc.name, plugins, tc.expectedPlugins)
				}
			}
		}
	}
}

func TestValidateExternalPlugins(t *testing.T) {
	tests := []struct {
		name        string
		plugins     map[string][]ExternalPlugin
		expectedErr error
	}{
		{
			name: "valid config",
			plugins: map[string][]ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name: "cherrypick",
					},
					{
						Name: "configupdater",
					},
					{
						Name: "tetris",
					},
				},
				"kubernetes": {
					{
						Name: "coffeemachine",
					},
					{
						Name: "blender",
					},
				},
			},
			expectedErr: nil,
		},
		{
			name: "invalid config",
			plugins: map[string][]ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name: "cherrypick",
					},
					{
						Name: "configupdater",
					},
					{
						Name: "tetris",
					},
				},
				"kubernetes": {
					{
						Name: "coffeemachine",
					},
					{
						Name: "tetris",
					},
				},
			},
			expectedErr: errors.New("invalid plugin configuration:\n\texternal plugins [tetris] are duplicated for kubernetes/test-infra and kubernetes"),
		},
	}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)

		err := validateExternalPlugins(test.plugins)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("unexpected error: %v, expected: %v", err, test.expectedErr)
		}
	}
}

func TestSetDefault_Maps(t *testing.T) {
	cases := []struct {
		name     string
		config   ConfigUpdater
		expected map[string]ConfigMapSpec
	}{
		{
			name: "nothing",
			expected: map[string]ConfigMapSpec{
				"prow/config.yaml":  {Name: "config"},
				"prow/plugins.yaml": {Name: "plugins"},
			},
		},
		{
			name: "basic",
			config: ConfigUpdater{
				Maps: map[string]ConfigMapSpec{
					"hello.yaml": {Name: "my-cm"},
					"world.yaml": {Name: "you-cm"},
				},
			},
			expected: map[string]ConfigMapSpec{
				"hello.yaml": {Name: "my-cm"},
				"world.yaml": {Name: "you-cm"},
			},
		},
		{
			name: "deprecated config",
			config: ConfigUpdater{
				ConfigFile: "foo.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"foo.yaml":          {Name: "config"},
				"prow/plugins.yaml": {Name: "plugins"},
			},
		},
		{
			name: "deprecated plugins",
			config: ConfigUpdater{
				PluginFile: "bar.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"bar.yaml":         {Name: "plugins"},
				"prow/config.yaml": {Name: "config"},
			},
		},
		{
			name: "deprecated both",
			config: ConfigUpdater{
				ConfigFile: "foo.yaml",
				PluginFile: "bar.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"foo.yaml": {Name: "config"},
				"bar.yaml": {Name: "plugins"},
			},
		},
		{
			name: "both current and deprecated",
			config: ConfigUpdater{
				Maps: map[string]ConfigMapSpec{
					"config.yaml":        {Name: "overwrite-config"},
					"plugins.yaml":       {Name: "overwrite-plugins"},
					"unconflicting.yaml": {Name: "ignored"},
				},
				ConfigFile: "config.yaml",
				PluginFile: "plugins.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"config.yaml":        {Name: "overwrite-config"},
				"plugins.yaml":       {Name: "overwrite-plugins"},
				"unconflicting.yaml": {Name: "ignored"},
			},
		},
	}
	for _, tc := range cases {
		cfg := Configuration{
			ConfigUpdater: tc.config,
		}
		cfg.setDefaults()
		actual := cfg.ConfigUpdater.Maps
		if len(actual) != len(tc.expected) {
			t.Errorf("%s: actual and expected have different keys: %v %v", tc.name, actual, tc.expected)
			continue
		}
		for k, n := range tc.expected {
			if an := actual[k]; an != n {
				t.Errorf("%s - %s: expected %s != actual %s", tc.name, k, n, an)
			}
		}
	}
}

func TestSetDefaults(t *testing.T) {
	tests := []struct {
		name string

		trustedOrg string
		joinOrgURL string

		expectedTrustedOrg string
		expectedJoinOrgURL string
	}{
		{
			name: "url defaults to org",

			trustedOrg: "kubernetes",
			joinOrgURL: "",

			expectedTrustedOrg: "kubernetes",
			expectedJoinOrgURL: "https://github.com/orgs/kubernetes/people",
		},
		{
			name: "both org and url are set",

			trustedOrg: "kubernetes",
			joinOrgURL: "https://git.k8s.io/community/community-membership.md#member",

			expectedTrustedOrg: "kubernetes",
			expectedJoinOrgURL: "https://git.k8s.io/community/community-membership.md#member",
		},
		{
			name: "only url is set",

			trustedOrg: "",
			joinOrgURL: "https://git.k8s.io/community/community-membership.md#member",

			expectedTrustedOrg: "",
			expectedJoinOrgURL: "https://git.k8s.io/community/community-membership.md#member",
		},
		{
			name: "nothing is set",

			trustedOrg: "",
			joinOrgURL: "",

			expectedTrustedOrg: "",
			expectedJoinOrgURL: "",
		},
	}

	for _, test := range tests {
		c := &Configuration{
			Triggers: []Trigger{
				{
					TrustedOrg: test.trustedOrg,
					JoinOrgURL: test.joinOrgURL,
				},
			},
		}

		c.setDefaults()

		if c.Triggers[0].TrustedOrg != test.expectedTrustedOrg {
			t.Errorf("unexpected trusted_org: %s, expected: %s", c.Triggers[0].TrustedOrg, test.expectedTrustedOrg)
		}
		if c.Triggers[0].JoinOrgURL != test.expectedJoinOrgURL {
			t.Errorf("unexpected join_org_url: %s, expected: %s", c.Triggers[0].JoinOrgURL, test.expectedJoinOrgURL)
		}
	}
}

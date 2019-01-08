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

package plugins

import (
	"errors"
	"reflect"
	"testing"
)

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
				"prow/config.yaml":  {Name: "config", Namespaces: []string{""}},
				"prow/plugins.yaml": {Name: "plugins", Namespaces: []string{""}},
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
				"hello.yaml": {Name: "my-cm", Namespaces: []string{""}},
				"world.yaml": {Name: "you-cm", Namespaces: []string{""}},
			},
		},
		{
			name: "deprecated config",
			config: ConfigUpdater{
				ConfigFile: "foo.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"foo.yaml":          {Name: "config", Namespaces: []string{""}},
				"prow/plugins.yaml": {Name: "plugins", Namespaces: []string{""}},
			},
		},
		{
			name: "deprecated plugins",
			config: ConfigUpdater{
				PluginFile: "bar.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"bar.yaml":         {Name: "plugins", Namespaces: []string{""}},
				"prow/config.yaml": {Name: "config", Namespaces: []string{""}},
			},
		},
		{
			name: "deprecated both",
			config: ConfigUpdater{
				ConfigFile: "foo.yaml",
				PluginFile: "bar.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"foo.yaml": {Name: "config", Namespaces: []string{""}},
				"bar.yaml": {Name: "plugins", Namespaces: []string{""}},
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
				"config.yaml":        {Name: "overwrite-config", Namespaces: []string{""}},
				"plugins.yaml":       {Name: "overwrite-plugins", Namespaces: []string{""}},
				"unconflicting.yaml": {Name: "ignored", Namespaces: []string{""}},
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
			if an := actual[k]; !reflect.DeepEqual(an, n) {
				t.Errorf("%s - %s: expected %s != actual %s", tc.name, k, n, an)
			}
		}
	}
}

func TestSetTriggerDefaults(t *testing.T) {
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

func TestSetCherryPickUnapprovedDefaults(t *testing.T) {
	defaultBranchRegexp := `^release-.*$`
	defaultComment := `This PR is not for the master branch but does not have the ` + "`cherry-pick-approved`" + `  label. Adding the ` + "`do-not-merge/cherry-pick-not-approved`" + `  label.

To approve the cherry-pick, please assign the patch release manager for the release branch by writing ` + "`/assign @username`" + ` in a comment when ready.

The list of patch release managers for each release can be found [here](https://git.k8s.io/sig-release/release-managers.md).`

	testcases := []struct {
		name string

		branchRegexp string
		comment      string

		expectedBranchRegexp string
		expectedComment      string
	}{
		{
			name:                 "none of branchRegexp and comment are set",
			branchRegexp:         "",
			comment:              "",
			expectedBranchRegexp: defaultBranchRegexp,
			expectedComment:      defaultComment,
		},
		{
			name:                 "only branchRegexp is set",
			branchRegexp:         `release-1.1.*$`,
			comment:              "",
			expectedBranchRegexp: `release-1.1.*$`,
			expectedComment:      defaultComment,
		},
		{
			name:                 "only comment is set",
			branchRegexp:         "",
			comment:              "custom comment",
			expectedBranchRegexp: defaultBranchRegexp,
			expectedComment:      "custom comment",
		},
		{
			name:                 "both branchRegexp and comment are set",
			branchRegexp:         `release-1.1.*$`,
			comment:              "custom comment",
			expectedBranchRegexp: `release-1.1.*$`,
			expectedComment:      "custom comment",
		},
	}

	for _, tc := range testcases {
		c := &Configuration{
			CherryPickUnapproved: CherryPickUnapproved{
				BranchRegexp: tc.branchRegexp,
				Comment:      tc.comment,
			},
		}

		c.setDefaults()

		if c.CherryPickUnapproved.BranchRegexp != tc.expectedBranchRegexp {
			t.Errorf("unexpected branchRegexp: %s, expected: %s", c.CherryPickUnapproved.BranchRegexp, tc.expectedBranchRegexp)
		}
		if c.CherryPickUnapproved.Comment != tc.expectedComment {
			t.Errorf("unexpected comment: %s, expected: %s", c.CherryPickUnapproved.Comment, tc.expectedComment)
		}
	}
}

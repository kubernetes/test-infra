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

package main

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/plugins"
	"sigs.k8s.io/yaml"
)

func TestEnsureValidConfiguration(t *testing.T) {
	var testCases = []struct {
		name                                    string
		tideSubSet, tideSuperSet, pluginsSubSet *orgRepoConfig
		expectedErr                             bool
	}{
		{
			name:          "nothing enabled makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, nil),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on org without tide makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo without tide makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, nil),
			pluginsSubSet: newOrgRepoConfig(nil, sets.NewString("org/repo")),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo with tide on repo makes no error",
			tideSubSet:    newOrgRepoConfig(nil, sets.NewString("org/repo")),
			tideSuperSet:  newOrgRepoConfig(nil, sets.NewString("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, sets.NewString("org/repo")),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo with tide on org makes error",
			tideSubSet:    newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, sets.NewString("org/repo")),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on org with tide on repo makes no error",
			tideSubSet:    newOrgRepoConfig(nil, sets.NewString("org/repo")),
			tideSuperSet:  newOrgRepoConfig(nil, sets.NewString("org/repo")),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on org with tide on org makes no error",
			tideSubSet:    newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			expectedErr:   false,
		},
		{
			name:          "tide enabled on org without plugin makes error",
			tideSubSet:    newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   true,
		},
		{
			name:          "tide enabled on repo without plugin makes error",
			tideSubSet:    newOrgRepoConfig(nil, sets.NewString("org/repo")),
			tideSuperSet:  newOrgRepoConfig(nil, sets.NewString("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on org with any tide record but no specific tide requirement makes error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on repo with any tide record but no specific tide requirement makes error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, sets.NewString("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, sets.NewString("org/repo")),
			expectedErr:   true,
		},
		{
			name:          "any tide org record but no specific tide requirement or plugin makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   false,
		},
		{
			name:          "any tide repo record but no specific tide requirement or plugin makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, sets.NewString("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   false,
		},
		{
			name:          "irrelevant repo exception in tide superset doesn't stop missing req error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			expectedErr:   true,
		},
		{
			name:          "repo exception in tide superset (no missing req error)",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, sets.NewString("org/repo")),
			expectedErr:   false,
		},
		{
			name:          "repo exception in tide subset (new missing req error)",
			tideSubSet:    newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.String{"org": nil}, nil),
			expectedErr:   true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ensureValidConfiguration("plugin", "label", "verb", testCase.tideSubSet, testCase.tideSuperSet, testCase.pluginsSubSet)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
		})
	}
}

func TestOrgRepoDifference(t *testing.T) {
	testCases := []struct {
		name           string
		a, b, expected *orgRepoConfig
	}{
		{
			name:     "subtract nil",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{}, sets.NewString()),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "no overlap",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "subtract self",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString()),
		},
		{
			name:     "subtract superset",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo"), "org2": nil}, sets.NewString("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString()),
		},
		{
			name:     "remove org with org",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/foo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/foo"), "2": nil}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "shrink org with org",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/foo"), "2": nil}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString("org/foo", "4/1", "4/2")),
		},
		{
			name:     "shrink org with repo",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil}, sets.NewString("org/foo", "3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/foo")}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "remove repo with org",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil, "4": sets.NewString("4/2")}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/2", "5/1")),
		},
		{
			name:     "remove repo with repo",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil}, sets.NewString("3/1", "4/2", "4/3")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "5/1")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.difference(tc.b)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("expected config: %#v, but got config: %#v", tc.expected, got)
			}
		})
	}
}

func TestOrgRepoIntersection(t *testing.T) {
	testCases := []struct {
		name           string
		a, b, expected *orgRepoConfig
	}{
		{
			name:     "intersect empty",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{}, sets.NewString()),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString()),
		},
		{
			name:     "no overlap",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString()),
		},
		{
			name:     "intersect self",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "intersect superset",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo"), "org2": nil}, sets.NewString("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "remove org",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/foo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org2": sets.NewString("org2/repo1")}, sets.NewString("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "shrink org with org",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/bar")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/foo"), "2": nil}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/foo", "org/bar")}, sets.NewString()),
		},
		{
			name:     "shrink org with repo",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil}, sets.NewString("org/repo", "org/foo", "3/1", "4/1")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString("org/foo", "4/1")),
		},
		{
			name:     "remove repo with org",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil, "4": sets.NewString("4/2")}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString("4/1", "4/3")),
		},
		{
			name:     "remove repo with repo",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": nil}, sets.NewString("3/1", "4/2", "4/3")),
			expected: newOrgRepoConfig(map[string]sets.String{}, sets.NewString("4/2", "4/3")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.intersection(tc.b)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("expected config: %#v, but got config: %#v", tc.expected, got)
			}
		})
	}
}

func TestOrgRepoUnion(t *testing.T) {
	testCases := []struct {
		name           string
		a, b, expected *orgRepoConfig
	}{
		{
			name:     "second set empty, get first set back",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{}, sets.NewString()),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "no overlap, simple union",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"2": sets.NewString()}, sets.NewString("3/1")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo"), "2": sets.NewString()}, sets.NewString("4/1", "4/2", "3/1")),
		},
		{
			name:     "union self, get self back",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
		},
		{
			name:     "union superset, get superset back",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo"), "org2": sets.NewString()}, sets.NewString("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo"), "org2": sets.NewString()}, sets.NewString("4/1", "4/2", "5/1")),
		},
		{
			name:     "keep only common blacklist items for an org",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/bar")}, sets.NewString()),
			b:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo", "org/foo")}, sets.NewString()),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString()),
		},
		{
			name:     "remove items from an org blacklist if they're in a repo whitelist",
			a:        newOrgRepoConfig(map[string]sets.String{"org": sets.NewString("org/repo")}, sets.NewString()),
			b:        newOrgRepoConfig(map[string]sets.String{}, sets.NewString("org/repo")),
			expected: newOrgRepoConfig(map[string]sets.String{"org": sets.NewString()}, sets.NewString()),
		},
		{
			name:     "remove repos when they're covered by an org whitelist",
			a:        newOrgRepoConfig(map[string]sets.String{}, sets.NewString("4/1", "4/2", "4/3")),
			b:        newOrgRepoConfig(map[string]sets.String{"4": sets.NewString("4/2")}, sets.NewString()),
			expected: newOrgRepoConfig(map[string]sets.String{"4": sets.NewString()}, sets.NewString()),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.union(tc.b)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("%s: did not get expected config:\n%v", tc.name, diff.ObjectGoPrintDiff(tc.expected, got))
			}
		})
	}
}

func TestValidateUnknownFields(t *testing.T) {
	testCases := []struct {
		name, filename string
		cfg            interface{}
		configBytes    []byte
		config         interface{}
		expectedErr    error
	}{
		{
			name:     "valid config",
			filename: "valid-conf.yaml",
			cfg:      &plugins.Configuration{},
			configBytes: []byte(`plugins:
  kube/kube:
  - size
  - config-updater
config_updater:
  maps:
    # Update the plugins configmap whenever plugins.yaml changes
    kube/plugins.yaml:
      name: plugins
size:
  s: 1`),
			expectedErr: nil,
		},
		{
			name:     "invalid top-level property",
			filename: "toplvl.yaml",
			cfg:      &plugins.Configuration{},
			configBytes: []byte(`plugins:
  kube/kube:
  - size
  - config-updater
notconfig_updater:
  maps:
    # Update the plugins configmap whenever plugins.yaml changes
    kube/plugins.yaml:
      name: plugins
size:
  s: 1`),
			expectedErr: fmt.Errorf("unknown fields present in toplvl.yaml: notconfig_updater"),
		},
		{
			name:     "invalid second-level property",
			filename: "seclvl.yaml",
			cfg:      &plugins.Configuration{},
			configBytes: []byte(`plugins:
  kube/kube:
  - size
  - config-updater
size:
  xs: 1
  s: 5`),
			expectedErr: fmt.Errorf("unknown fields present in seclvl.yaml: size.xs"),
		},
		{
			name:     "invalid array element",
			filename: "home/array.yaml",
			cfg:      &plugins.Configuration{},
			configBytes: []byte(`plugins:
  kube/kube:
  - size
  - trigger
triggers:
- repos:
  - kube/kube
- repoz:
  - kube/kubez`),
			expectedErr: fmt.Errorf("unknown fields present in home/array.yaml: triggers[1].repoz"),
		},
		{
			name:     "invalid map entry",
			filename: "map.yaml",
			cfg:      &plugins.Configuration{},
			configBytes: []byte(`plugins:
  kube/kube:
  - size
  - config-updater
config_updater:
  maps:
    # Update the plugins configmap whenever plugins.yaml changes
    kube/plugins.yaml:
      name: plugins
    kube/config.yaml:
      validation: config
size:
  s: 1`),
			expectedErr: fmt.Errorf("unknown fields present in map.yaml: " +
				"config_updater.maps.kube/config.yaml.validation"),
		},
		{
			name:     "multiple invalid elements",
			filename: "multiple.yaml",
			cfg:      &plugins.Configuration{},
			configBytes: []byte(`plugins:
  kube/kube:
  - size
  - trigger
triggers:
- repoz:
  - kube/kubez
- repos:
  - kube/kube
size:
  s: 1
  xs: 1`),
			expectedErr: fmt.Errorf("unknown fields present in multiple.yaml: " +
				"size.xs, triggers[0].repoz"),
		},
		{
			name:     "embedded structs",
			filename: "embedded.yaml",
			cfg:      &config.Config{},
			configBytes: []byte(`presubmits:
  kube/kube:
  - name: test-presubmit
    decorate: true
    always_run: true
    never_run: false
    skip_report: true
    spec:
      containers:
      - image: alpine
        command: ["/bin/printenv"]
tide:
  squash_label: sq
  not-a-property: true
size:
  s: 1
  xs: 1`),
			expectedErr: fmt.Errorf("unknown fields present in embedded.yaml: " +
				"presubmits.kube/kube[0].never_run, size, tide.not-a-property"),
		},
		{
			name:     "pointer to a slice",
			filename: "pointer.yaml",
			cfg:      &plugins.Configuration{},
			configBytes: []byte(`bugzilla:
  default:
    '*':
      statuses:
      - foobar
      extra: oops`),
			expectedErr: fmt.Errorf("unknown fields present in pointer.yaml: " +
				"bugzilla.default.*.extra"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := yaml.Unmarshal(tc.configBytes, tc.cfg); err != nil {
				t.Fatalf("Unable to unmarhsal yaml: %v", err)
			}
			got := validateUnknownFields(tc.cfg, tc.configBytes, tc.filename)
			if !reflect.DeepEqual(got, tc.expectedErr) {
				t.Errorf("%s: did not get expected validation error:\n%v", tc.name,
					diff.ObjectGoPrintDiff(tc.expectedErr, got))
			}
		})
	}
}

func TestValidateStrictBranches(t *testing.T) {
	trueVal := true
	falseVal := false
	testcases := []struct {
		name   string
		config config.ProwConfig

		errItems []string
		okItems  []string
	}{
		{
			name: "no conflict: no strict config",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Orgs: []string{"kubernetes"},
						},
					},
				},
			},
			errItems: []string{},
			okItems:  []string{"kubernetes"},
		},
		{
			name: "no conflict: no tide config",
			config: config.ProwConfig{
				BranchProtection: config.BranchProtection{
					Orgs: map[string]config.Org{
						"kubernetes": {
							Policy: config.Policy{
								Protect: &trueVal,
								RequiredStatusChecks: &config.ContextPolicy{
									Strict: &trueVal,
								},
							},
						},
					},
				},
			},
			errItems: []string{},
			okItems:  []string{"kubernetes"},
		},
		{
			name: "no conflict: tide repo exclusion",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Orgs:          []string{"kubernetes"},
							ExcludedRepos: []string{"kubernetes/test-infra"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Orgs: map[string]config.Org{
						"kubernetes": {
							Policy: config.Policy{
								Protect: &falseVal,
							},
							Repos: map[string]config.Repo{
								"test-infra": {
									Policy: config.Policy{
										Protect: &trueVal,
										RequiredStatusChecks: &config.ContextPolicy{
											Strict: &trueVal,
										},
									},
								},
							},
						},
					},
				},
			},
			errItems: []string{},
			okItems:  []string{"kubernetes", "kubernetes/test-infra"},
		},
		{
			name: "no conflict: protection repo exclusion",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Repos: []string{"kubernetes/test-infra"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Orgs: map[string]config.Org{
						"kubernetes": {
							Policy: config.Policy{
								Protect: &trueVal,
								RequiredStatusChecks: &config.ContextPolicy{
									Strict: &trueVal,
								},
							},
							Repos: map[string]config.Repo{
								"test-infra": {
									Policy: config.Policy{
										Protect: &falseVal,
									},
								},
							},
						},
					},
				},
			},
			errItems: []string{},
			okItems:  []string{"kubernetes", "kubernetes/test-infra"},
		},
		{
			name: "conflict: tide more general",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Orgs: []string{"kubernetes"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Policy: config.Policy{
						Protect: &trueVal,
					},
					Orgs: map[string]config.Org{
						"kubernetes": {
							Repos: map[string]config.Repo{
								"test-infra": {
									Policy: config.Policy{
										Protect: &trueVal,
										RequiredStatusChecks: &config.ContextPolicy{
											Strict: &trueVal,
										},
									},
								},
							},
						},
					},
				},
			},
			errItems: []string{"kubernetes/test-infra"},
			okItems:  []string{"kubernetes"},
		},
		{
			name: "conflict: tide more specific",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Repos: []string{"kubernetes/test-infra"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Policy: config.Policy{
						Protect: &trueVal,
					},
					Orgs: map[string]config.Org{
						"kubernetes": {
							Policy: config.Policy{
								RequiredStatusChecks: &config.ContextPolicy{
									Strict: &trueVal,
								},
							},
						},
					},
				},
			},
			errItems: []string{"kubernetes/test-infra"},
			okItems:  []string{"kubernetes"},
		},
		{
			name: "conflict: org level",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Orgs: []string{"kubernetes", "k8s"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Policy: config.Policy{
						Protect: &trueVal,
					},
					Orgs: map[string]config.Org{
						"kubernetes": {
							Policy: config.Policy{
								RequiredStatusChecks: &config.ContextPolicy{
									Strict: &trueVal,
								},
							},
						},
					},
				},
			},
			errItems: []string{"kubernetes"},
			okItems:  []string{"k8s"},
		},
		{
			name: "conflict: repo level",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Repos: []string{"kubernetes/kubernetes"},
						},
						{
							Repos: []string{"kubernetes/test-infra"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Policy: config.Policy{
						Protect: &trueVal,
					},
					Orgs: map[string]config.Org{
						"kubernetes": {
							Repos: map[string]config.Repo{
								"kubernetes": {
									Policy: config.Policy{
										RequiredStatusChecks: &config.ContextPolicy{
											Strict: &trueVal,
										},
									},
								},
							},
						},
					},
				},
			},
			errItems: []string{"kubernetes/kubernetes"},
			okItems:  []string{"kubernetes", "kubernetes/test-infra"},
		},
		{
			name: "conflict: branch level",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Repos:            []string{"kubernetes/test-infra"},
							IncludedBranches: []string{"master"},
						},
						{
							Repos: []string{"kubernetes/kubernetes"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Policy: config.Policy{
						Protect: &trueVal,
					},
					Orgs: map[string]config.Org{
						"kubernetes": {
							Repos: map[string]config.Repo{
								"test-infra": {
									Branches: map[string]config.Branch{
										"master": {
											Policy: config.Policy{
												RequiredStatusChecks: &config.ContextPolicy{
													Strict: &trueVal,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			errItems: []string{"kubernetes/test-infra"},
			okItems:  []string{"kubernetes", "kubernetes/kubernetes"},
		},
		{
			name: "conflict: global strict",
			config: config.ProwConfig{
				Tide: config.Tide{
					Queries: []config.TideQuery{
						{
							Repos: []string{"kubernetes/test-infra"},
						},
					},
				},
				BranchProtection: config.BranchProtection{
					Policy: config.Policy{
						Protect: &trueVal,
						RequiredStatusChecks: &config.ContextPolicy{
							Strict: &trueVal,
						},
					},
				},
			},
			errItems: []string{"global"},
			okItems:  []string{},
		},
		{
			name: "no conflict: global strict, Tide disabled",
			config: config.ProwConfig{
				BranchProtection: config.BranchProtection{
					Policy: config.Policy{
						Protect: &trueVal,
						RequiredStatusChecks: &config.ContextPolicy{
							Strict: &trueVal,
						},
					},
				},
			},
			errItems: []string{},
			okItems:  []string{"global"},
		},
	}
	for i := range testcases {
		t.Run(testcases[i].name, func(t *testing.T) {
			tc := testcases[i]
			t.Parallel()
			err := validateStrictBranches(tc.config)
			if err == nil && len(tc.errItems) > 0 {
				t.Errorf("Expected errors for the following items, but didn't see an error: %v.", tc.errItems)
			} else if err != nil && len(tc.errItems) == 0 {
				t.Errorf("Unexpected error: %v.", err)
			}
			if err == nil {
				return
			}
			errText := err.Error()
			for _, errItem := range tc.errItems {
				// Search for the token while explicitly forbidding neighboring slashes
				// so that orgs don't match member repos.
				re, err := regexp.Compile(fmt.Sprintf("[^/]%s[^/]", errItem))
				if err != nil {
					t.Fatalf("Unexpected error compiling regexp: %v.", err)
				}
				if !re.MatchString(errText) {
					t.Errorf("Error did not reference expected error item %q: %q.", errItem, errText)
				}
			}
			for _, okItem := range tc.okItems {
				re, err := regexp.Compile(fmt.Sprintf("[^/]%s[^/]", okItem))
				if err != nil {
					t.Fatalf("Unexpected error compiling regexp: %v.", err)
				}
				if re.MatchString(errText) {
					t.Errorf("Error unexpectedly included ok item %q: %q.", okItem, errText)
				}
			}
		})
	}
}

func TestWarningEnabled(t *testing.T) {
	var testCases = []struct {
		name      string
		warnings  []string
		excludes  []string
		candidate string
		expected  bool
	}{
		{
			name:      "nothing is found in empty sets",
			warnings:  []string{},
			excludes:  []string{},
			candidate: "missing",
			expected:  false,
		},
		{
			name:      "explicit warning is found",
			warnings:  []string{"found"},
			excludes:  []string{},
			candidate: "found",
			expected:  true,
		},
		{
			name:      "explicit warning that is excluded is not found",
			warnings:  []string{"found"},
			excludes:  []string{"found"},
			candidate: "found",
			expected:  false,
		},
	}

	for _, testCase := range testCases {
		opt := options{
			warnings:        flagutil.NewStrings(testCase.warnings...),
			excludeWarnings: flagutil.NewStrings(testCase.excludes...),
		}
		if actual, expected := opt.warningEnabled(testCase.candidate), testCase.expected; actual != expected {
			t.Errorf("%s: expected warning %s enablement to be %v but got %v", testCase.name, testCase.candidate, expected, actual)
		}
	}
}

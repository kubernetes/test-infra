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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/diff"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
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
				t.Errorf("%s: did not get expected config:\n%v", tc.name, cmp.Diff(tc.expected, got))
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
		expectedErr    string
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
			expectedErr: "",
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
			expectedErr: "notconfig_updater",
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
			expectedErr: "xs",
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
			expectedErr: "repoz",
		},
		// Options like DisallowUnknownFields can not be passed when using
		// a custon json.Unmarshaler like we do here for defaulting:
		// https://github.com/golang/go/issues/41144
		//		{
		//			name:     "invalid map entry",
		//			filename: "map.yaml",
		//			cfg:      &plugins.Configuration{},
		//			configBytes: []byte(`plugins:
		//  kube/kube:
		//  - size
		//  - config-updater
		//config_updater:
		//  maps:
		//    # Update the plugins configmap whenever plugins.yaml changes
		//    kube/plugins.yaml:
		//      name: plugins
		//    kube/config.yaml:
		//      validation: config
		//size:
		//  s: 1`),
		//			expectedErr: "validation",
		//		},
		{
			//only one invalid element is printed in the error
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
			expectedErr: "xs",
		},
		{
			name:     "embedded structs - kube",
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
        command: ["/bin/printenv"]`),
			expectedErr: "never_run",
		},
		{
			name:     "embedded structs - tide",
			filename: "embedded.yaml",
			cfg:      &config.Config{},
			configBytes: []byte(`tide:
  squash_label: sq
  not-a-property: true`),
			expectedErr: "not-a-property",
		},
		{
			name:     "embedded structs - size",
			filename: "embedded.yaml",
			cfg:      &config.Config{},
			configBytes: []byte(`size:
  s: 1
  xs: 1`),
			expectedErr: "size",
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
			expectedErr: "extra",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := yaml.Unmarshal(tc.configBytes, tc.cfg); err != nil {
				t.Fatalf("Unable to unmarhsal yaml: %v", err)
			}
			got := validateUnknownFields(tc.cfg, tc.configBytes, tc.filename)

			if tc.expectedErr == "" {
				if got != nil {
					t.Errorf("%s: expected nil error but got:\n%v", tc.name, got)
				}
			} else { // check substrings in case yaml lib changes err fmt
				var errMsg string
				if got != nil {
					errMsg = got.Error()
				}
				for _, s := range []string{"unknown field", tc.filename, tc.expectedErr} {
					if !strings.Contains(errMsg, s) {
						t.Errorf("%s: did not get expected validation error: expected substring in error message:\n%s\n but got:\n%v", tc.name, s, got)
					}
				}
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

func TestValidateManagedWebhooks(t *testing.T) {
	testCases := []struct {
		name      string
		config    config.ProwConfig
		expectErr bool
	}{
		{
			name:      "empty config",
			config:    config.ProwConfig{},
			expectErr: false,
		},
		{
			name: "no duplicate webhooks",
			config: config.ProwConfig{
				ManagedWebhooks: config.ManagedWebhooks{
					RespectLegacyGlobalToken: false,
					OrgRepoConfig: map[string]config.ManagedWebhookInfo{
						"foo1":     {TokenCreatedAfter: time.Now()},
						"foo2":     {TokenCreatedAfter: time.Now()},
						"foo/bar":  {TokenCreatedAfter: time.Now()},
						"foo/bar1": {TokenCreatedAfter: time.Now()},
						"foo/bar2": {TokenCreatedAfter: time.Now()},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "has duplicate webhooks",
			config: config.ProwConfig{
				ManagedWebhooks: config.ManagedWebhooks{
					OrgRepoConfig: map[string]config.ManagedWebhookInfo{
						"foo":      {TokenCreatedAfter: time.Now()},
						"foo1":     {TokenCreatedAfter: time.Now()},
						"foo2":     {TokenCreatedAfter: time.Now()},
						"foo/bar":  {TokenCreatedAfter: time.Now()},
						"foo/bar1": {TokenCreatedAfter: time.Now()},
						"foo/bar2": {TokenCreatedAfter: time.Now()},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "has multiple duplicate webhooks",
			config: config.ProwConfig{
				ManagedWebhooks: config.ManagedWebhooks{
					RespectLegacyGlobalToken: true,
					OrgRepoConfig: map[string]config.ManagedWebhookInfo{
						"foo":       {TokenCreatedAfter: time.Now()},
						"foo1":      {TokenCreatedAfter: time.Now()},
						"foo2":      {TokenCreatedAfter: time.Now()},
						"foo/bar":   {TokenCreatedAfter: time.Now()},
						"foo/bar1":  {TokenCreatedAfter: time.Now()},
						"foo1/bar1": {TokenCreatedAfter: time.Now()},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		err := validateManagedWebhooks(&config.Config{ProwConfig: testCase.config})
		if testCase.expectErr && err == nil {
			t.Errorf("%s: expected the config %+v to have errors but not", testCase.name, testCase.config)
		}
		if !testCase.expectErr && err != nil {
			t.Errorf("%s: expected the config %+v to be correct but got an error in validation: %v",
				testCase.name, testCase.config, err)
		}
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

type fakeGHContent map[string]map[string]map[string]bool // org[repo][path] -> exist/does not exist

type fakeGH struct {
	files    fakeGHContent
	archived map[string]bool // org/repo -> true/false
}

func (f fakeGH) GetFile(org, repo, filepath, _ string) ([]byte, error) {
	if _, hasOrg := f.files[org]; !hasOrg {
		return nil, &github.FileNotFound{}
	}
	if _, hasRepo := f.files[org][repo]; !hasRepo {
		return nil, &github.FileNotFound{}
	}
	if _, hasPath := f.files[org][repo][filepath]; !hasPath {
		return nil, &github.FileNotFound{}
	}

	return []byte("CONTENT"), nil
}

func (f fakeGH) GetRepos(org string, isUser bool) ([]github.Repo, error) {
	if _, hasOrg := f.files[org]; !hasOrg {
		return nil, fmt.Errorf("no such org")
	}
	var repos []github.Repo
	for repo := range f.files[org] {
		fullname := fmt.Sprintf("%s/%s", org, repo)
		_, archived := f.archived[fullname]
		repos = append(
			repos,
			github.Repo{
				Owner:    github.User{Login: org},
				Name:     repo,
				FullName: fullname,
				Archived: archived,
			})
	}
	return repos, nil
}

func TestVerifyOwnersPresence(t *testing.T) {
	testCases := []struct {
		description string
		cfg         *plugins.Configuration
		gh          fakeGH

		expected string
	}{
		{
			description: "org with blunderbuss enabled contains a repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org": {"blunderbuss"}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with approve enable contains a repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org": {"approve"}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains a repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org": {"owners-label"}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains an *archived* repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org": {"owners-label"}}},
			gh: fakeGH{
				files:    fakeGHContent{"org": {"repo": {"NOOWNERS": true}}},
				archived: map[string]bool{"org/repo": true},
			},
			expected: "",
		}, {
			description: "repo with owners-label enabled does not contain OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org/repo": {"owners-label"}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains only repos with OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org": {"owners-label"}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"OWNERS": true}}}},
			expected:    "",
		}, {
			description: "repo with owners-label enabled contains OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org/repo": {"owners-label"}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"OWNERS": true}}}},
			expected:    "",
		}, {
			description: "repo with unrelated plugin enabled does not contain OWNERS",
			cfg:         &plugins.Configuration{Plugins: map[string][]string{"org/repo": {"cat"}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var errMessage string
			if err := verifyOwnersPresence(tc.cfg, tc.gh); err != nil {
				errMessage = err.Error()
			}
			if errMessage != tc.expected {
				t.Errorf("result differs:\n%s", diff.StringDiff(tc.expected, errMessage))
			}
		})
	}
}

func TestOptions(t *testing.T) {

	var defaultGitHubOptions flagutil.GitHubOptions
	defaultGitHubOptions.AddFlags(flag.NewFlagSet("", flag.ContinueOnError))
	defaultGitHubOptions.AllowAnonymous = true

	StringsFlag := func(vals []string) flagutil.Strings {
		var flag flagutil.Strings
		for _, val := range vals {
			flag.Set(val)
		}
		return flag
	}

	testCases := []struct {
		name            string
		args            []string
		expectedOptions *options
		expectedError   bool
	}{
		{
			name: "cannot parse argument, reject",
			args: []string{
				"--config-path=prow/config.yaml",
				"--strict=non-boolean-string",
			},
			expectedOptions: nil,
			expectedError:   true,
		},
		{
			name:            "forgot config-path, reject",
			args:            []string{"--job-config-path=config/jobs/org/job.yaml"},
			expectedOptions: nil,
			expectedError:   true,
		},
		{
			name: "config-path with two warnings but one unknown, reject",
			args: []string{
				"--config-path=prow/config.yaml",
				"--warnings=mismatched-tide",
				"--warnings=unknown-warning",
			},
			expectedOptions: nil,
			expectedError:   true,
		},
		{
			name: "config-path with many valid options",
			args: []string{
				"--config-path=prow/config.yaml",
				"--plugin-config=prow/plugins/plugin.yaml",
				"--job-config-path=config/jobs/org/job.yaml",
				"--warnings=mismatched-tide",
				"--warnings=mismatched-tide-lenient",
				"--exclude-warning=tide-strict-branch",
				"--exclude-warning=mismatched-tide",
				"--exclude-warning=ok-if-unknown-warning",
				"--strict=true",
				"--expensive-checks=false",
			},
			expectedOptions: &options{
				configPath:      "prow/config.yaml",
				pluginConfig:    "prow/plugins/plugin.yaml",
				jobConfigPath:   "config/jobs/org/job.yaml",
				warnings:        StringsFlag([]string{"mismatched-tide", "mismatched-tide-lenient"}),
				excludeWarnings: StringsFlag([]string{"tide-strict-branch", "mismatched-tide", "ok-if-unknown-warning"}),
				strict:          true,
				expensive:       false,
				github:          defaultGitHubOptions,
			},
			expectedError: false,
		},
		{
			name: "prow-yaml-path gets defaulted",
			args: []string{
				"--config-path=prow/config.yaml",
				"--plugin-config=prow/plugins/plugin.yaml",
				"--job-config-path=config/jobs/org/job.yaml",
				"--prow-yaml-repo-name=my/repo",
			},
			expectedOptions: &options{
				configPath:       "prow/config.yaml",
				pluginConfig:     "prow/plugins/plugin.yaml",
				jobConfigPath:    "config/jobs/org/job.yaml",
				prowYAMLRepoName: "my/repo",
				prowYAMLPath:     "/home/prow/go/src/github.com/my/repo/.prow.yaml",
				github:           defaultGitHubOptions,
			},
			expectedError: false,
		},
		{
			name: "prow-yaml-path without prow-yaml-repo-name is invalid",
			args: []string{
				"--prow-yaml-path=my-file",
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			var actualOptions options
			switch actualErr := actualOptions.gatherOptions(flags, tc.args); {
			case tc.expectedError:
				if actualErr == nil {
					t.Error("failed to receive an error")
				}
			case actualErr != nil:
				t.Errorf("unexpected error: %v", actualErr)
			case !reflect.DeepEqual(&actualOptions, tc.expectedOptions):
				t.Errorf("actual %#v != expected %#v", actualOptions, *tc.expectedOptions)
			}
		})
	}
}

func TestValidateJobExtraRefs(t *testing.T) {
	testCases := []struct {
		name      string
		extraRefs []prowapi.Refs
		expected  error
	}{
		{
			name: "validation error if extra ref specifies the repo for which the job is configured",
			extraRefs: []prowapi.Refs{
				{
					Org:  "org",
					Repo: "repo",
				},
			},
			expected: fmt.Errorf("Invalid job test on repo org/repo: the following refs specified more than once: %s",
				"org/repo"),
		},
		{
			name: "no errors if there are no duplications",
			extraRefs: []prowapi.Refs{
				{
					Org:  "foo",
					Repo: "bar",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := config.JobConfig{
				PresubmitsStatic: map[string][]config.Presubmit{
					"org/repo": {
						{
							JobBase: config.JobBase{
								Name: "test",
								UtilityConfig: config.UtilityConfig{
									ExtraRefs: tc.extraRefs,
								},
							},
						},
					},
				},
			}
			if err := validateJobExtraRefs(config); !reflect.DeepEqual(err, utilerrors.NewAggregate([]error{tc.expected})) {
				t.Errorf("%s: did not get expected validation error:\n%v", tc.name,
					cmp.Diff(tc.expected, err))
			}
		})
	}
}

func TestValidateInRepoConfig(t *testing.T) {
	testCases := []struct {
		name         string
		prowYAMLData []byte
		expectedErr  string
	}{
		{
			name:         "Valid prowYAML, no err",
			prowYAMLData: []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
		},
		{
			name:         "Invalid prowYAML presubmit, err",
			prowYAMLData: []byte(`presubmits: [{"name": "hans"}]`),
			expectedErr:  "failed to validate .prow.yaml: invalid presubmit job hans: kubernetes jobs require a spec",
		},
		{
			name:         "Invalid prowYAML postsubmit, err",
			prowYAMLData: []byte(`postsubmits: [{"name": "hans"}]`),
			expectedErr:  "failed to validate .prow.yaml: invalid postsubmit job hans: kubernetes jobs require a spec",
		},
		{
			name: "Absent prowYAML, no err",
		},
	}

	for _, tc := range testCases {
		prowYAMLFileName := "/this-must-not-exist"

		if tc.prowYAMLData != nil {
			tempFile, err := ioutil.TempFile("", "prow-test")
			if err != nil {
				t.Fatalf("failed to get tempfile: %v", err)
			}
			defer func() {
				if err := tempFile.Close(); err != nil {
					t.Errorf("failed to close tempFile: %v", err)
				}
				if err := os.Remove(tempFile.Name()); err != nil {
					t.Errorf("failed to remove tempfile: %v", err)
				}
			}()

			if _, err := tempFile.Write(tc.prowYAMLData); err != nil {
				t.Fatalf("failed to write to tempfile: %v", err)
			}

			prowYAMLFileName = tempFile.Name()
		}

		// Need an empty file to load the config from so we go through its defaulting
		tempConfig, err := ioutil.TempFile("/tmp", "prow-test")
		if err != nil {
			t.Fatalf("failed to get tempfile: %v", err)
		}
		defer func() {
			if err := os.Remove(tempConfig.Name()); err != nil {
				t.Errorf("failed to remove tempfile: %v", err)
			}
		}()
		if err := tempConfig.Close(); err != nil {
			t.Errorf("failed to close tempFile: %v", err)
		}

		cfg, err := config.Load(tempConfig.Name(), "")
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		err = validateInRepoConfig(cfg, prowYAMLFileName, "my/repo")
		var errString string
		if err != nil {
			errString = err.Error()
		}

		if errString != tc.expectedErr {
			t.Errorf("expected error %q does not match actual error %q", tc.expectedErr, errString)
		}
	}
}

func TestValidateTideContextPolicy(t *testing.T) {
	cfg := func(m ...func(*config.Config)) *config.Config {
		cfg := &config.Config{}
		cfg.PresubmitsStatic = map[string][]config.Presubmit{}
		for _, mod := range m {
			mod(cfg)
		}
		return cfg
	}

	testCases := []struct {
		name          string
		cfg           *config.Config
		expectedError string
	}{
		{
			name: "overlapping branch config, error",
			cfg: cfg(func(c *config.Config) {
				c.PresubmitsStatic["a/b"] = []config.Presubmit{
					{Reporter: config.Reporter{Context: "a"}, Brancher: config.Brancher{Branches: []string{"a"}}},
					{AlwaysRun: true, Reporter: config.Reporter{Context: "a"}},
				}
			}),
			expectedError: "context policy for a branch in a/b is invalid: contexts a are defined as required and required if present",
		},
		{
			name: "overlapping branch config with empty branch configs, error",
			cfg: cfg(func(c *config.Config) {
				c.PresubmitsStatic["a/b"] = []config.Presubmit{
					{Reporter: config.Reporter{Context: "a"}},
					{AlwaysRun: true, Reporter: config.Reporter{Context: "a"}},
				}
			}),
			expectedError: "context policy for master branch in a/b is invalid: contexts a are defined as required and required if present",
		},
		{
			name: "overlapping branch config, inrepoconfig enabled, error",
			cfg: cfg(func(c *config.Config) {
				c.InRepoConfig.Enabled = map[string]*bool{"*": utilpointer.BoolPtr(true)}
				c.PresubmitsStatic["a/b"] = []config.Presubmit{
					{Reporter: config.Reporter{Context: "a"}, Brancher: config.Brancher{Branches: []string{"a"}}},
					{AlwaysRun: true, Reporter: config.Reporter{Context: "a"}},
				}
			}),
			expectedError: "context policy for a branch in a/b is invalid: contexts a are defined as required and required if present",
		},
		{
			name: "no overlapping branch config, no error",
			cfg: cfg(func(c *config.Config) {
				c.PresubmitsStatic["a/b"] = []config.Presubmit{
					{Reporter: config.Reporter{Context: "a"}, Brancher: config.Brancher{Branches: []string{"a"}}},
					{AlwaysRun: true, Reporter: config.Reporter{Context: "a"}, Brancher: config.Brancher{Branches: []string{"b"}}},
				}
			}),
		},
		{
			name: "repo key is not in org/repo format, no error",
			cfg: cfg(func(c *config.Config) {
				c.PresubmitsStatic["https://kunit-review.googlesource.com/linux"] = []config.Presubmit{
					{Reporter: config.Reporter{Context: "a"}, Brancher: config.Brancher{Branches: []string{"a"}}},
					{AlwaysRun: true, Reporter: config.Reporter{Context: "a"}, Brancher: config.Brancher{Branches: []string{"b"}}},
				}
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Needed so regexes get compiled
			tc.cfg.SetPresubmits(tc.cfg.PresubmitsStatic)

			errMsg := ""
			if err := validateTideContextPolicy(tc.cfg); err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.expectedError {
				t.Errorf("expected error %q, got error %q", tc.expectedError, errMsg)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	testCases := []struct {
		name string
		opts options
	}{
		{
			name: "combined config",
			opts: options{
				configPath: "testdata/combined.yaml",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validate(tc.opts); err != nil {
				t.Fatalf("validation failed: %v", err)
			}
		})
	}
}

func TestValidateClusterField(t *testing.T) {
	testCases := []struct {
		name          string
		cfg           *config.Config
		expectedError string
	}{
		{
			name: "Jenkins job with unset cluster",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Agent: "jenkins",
								},
							}}}}},
		},
		{
			name: "jenkins job with defaulted cluster",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Agent:   "jenkins",
									Cluster: "default",
									Name:    "some-job",
								},
							}}}}},
		},
		{
			name: "jenkins job must not set cluster",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Agent:   "jenkins",
									Cluster: "build1",
									Name:    "some-job",
								},
							}}}}},
			expectedError: "org1/repo1: some-job: cannot set cluster field if agent is jenkins",
		},
		{
			name: "k8s job can set cluster",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Agent:   "kubernetes",
									Cluster: "default",
								},
							}}}}},
		},
		{
			name: "empty agent job can set cluster",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Cluster: "default",
								},
							}}}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errMsg := ""
			if err := validateCluster(tc.cfg); err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.expectedError {
				t.Errorf("expected error %q, got error %q", tc.expectedError, errMsg)
			}
		})
	}
}

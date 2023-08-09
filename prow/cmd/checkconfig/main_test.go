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
	"context"
	"flag"
	"fmt"
	stdio "io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
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
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	pluginsflagutil "k8s.io/test-infra/prow/flagutil/plugins"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/plank"
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
			pluginsSubSet: newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo without tide makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, nil),
			pluginsSubSet: newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo with tide on repo makes no error",
			tideSubSet:    newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			tideSuperSet:  newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo with tide on org makes error",
			tideSubSet:    newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on org with tide on repo makes no error",
			tideSubSet:    newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			tideSuperSet:  newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on org with tide on org makes no error",
			tideSubSet:    newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			expectedErr:   false,
		},
		{
			name:          "tide enabled on org without plugin makes error",
			tideSubSet:    newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   true,
		},
		{
			name:          "tide enabled on repo without plugin makes error",
			tideSubSet:    newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			tideSuperSet:  newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on org with any tide record but no specific tide requirement makes error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on repo with any tide record but no specific tide requirement makes error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			expectedErr:   true,
		},
		{
			name:          "any tide org record but no specific tide requirement or plugin makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   false,
		},
		{
			name:          "any tide repo record but no specific tide requirement or plugin makes no error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			pluginsSubSet: newOrgRepoConfig(nil, nil),
			expectedErr:   false,
		},
		{
			name:          "irrelevant repo exception in tide superset doesn't stop missing req error",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			expectedErr:   true,
		},
		{
			name:          "repo exception in tide superset (no missing req error)",
			tideSubSet:    newOrgRepoConfig(nil, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, nil),
			pluginsSubSet: newOrgRepoConfig(nil, sets.New[string]("org/repo")),
			expectedErr:   false,
		},
		{
			name:          "repo exception in tide subset (new missing req error)",
			tideSubSet:    newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, nil),
			tideSuperSet:  newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
			pluginsSubSet: newOrgRepoConfig(map[string]sets.Set[string]{"org": nil}, nil),
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
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]()),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "no overlap",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "subtract self",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]()),
		},
		{
			name:     "subtract superset",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo"), "org2": nil}, sets.New[string]("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]()),
		},
		{
			name:     "remove org with org",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/foo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/foo"), "2": nil}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "shrink org with org",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/foo"), "2": nil}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("org/foo", "4/1", "4/2")),
		},
		{
			name:     "shrink org with repo",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil}, sets.New[string]("org/foo", "3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/foo")}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "remove repo with org",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil, "4": sets.New[string]("4/2")}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/2", "5/1")),
		},
		{
			name:     "remove repo with repo",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil}, sets.New[string]("3/1", "4/2", "4/3")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "5/1")),
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
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]()),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]()),
		},
		{
			name:     "no overlap",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]()),
		},
		{
			name:     "intersect self",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "intersect superset",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo"), "org2": nil}, sets.New[string]("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "remove org",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/foo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org2": sets.New[string]("org2/repo1")}, sets.New[string]("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "shrink org with org",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/bar")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/foo"), "2": nil}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/foo", "org/bar")}, sets.New[string]()),
		},
		{
			name:     "shrink org with repo",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil}, sets.New[string]("org/repo", "org/foo", "3/1", "4/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("org/foo", "4/1")),
		},
		{
			name:     "remove repo with org",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil, "4": sets.New[string]("4/2")}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("4/1", "4/3")),
		},
		{
			name:     "remove repo with repo",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2", "4/3", "5/1")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": nil}, sets.New[string]("3/1", "4/2", "4/3")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("4/2", "4/3")),
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
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]()),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "no overlap, simple union",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"2": sets.New[string]()}, sets.New[string]("3/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo"), "2": sets.New[string]()}, sets.New[string]("4/1", "4/2", "3/1")),
		},
		{
			name:     "union self, get self back",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
		},
		{
			name:     "union superset, get superset back",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]("4/1", "4/2")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo"), "org2": sets.New[string]()}, sets.New[string]("4/1", "4/2", "5/1")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo"), "org2": sets.New[string]()}, sets.New[string]("4/1", "4/2", "5/1")),
		},
		{
			name:     "keep only common denied items for an org",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/bar")}, sets.New[string]()),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo", "org/foo")}, sets.New[string]()),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]()),
		},
		{
			name:     "remove items from an org denylist if they're in a repo allowlist",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]("org/repo")}, sets.New[string]()),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("org/repo")),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"org": sets.New[string]()}, sets.New[string]()),
		},
		{
			name:     "remove repos when they're covered by an org allowlist",
			a:        newOrgRepoConfig(map[string]sets.Set[string]{}, sets.New[string]("4/1", "4/2", "4/3")),
			b:        newOrgRepoConfig(map[string]sets.Set[string]{"4": sets.New[string]("4/2")}, sets.New[string]()),
			expected: newOrgRepoConfig(map[string]sets.Set[string]{"4": sets.New[string]()}, sets.New[string]()),
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
		// config_updater:
		//  maps:
		//    # Update the plugins configmap whenever plugins.yaml changes
		//    kube/plugins.yaml:
		//      name: plugins
		//    kube/config.yaml:
		//      validation: config
		// size:
		//  s: 1`),
		//			expectedErr: "validation",
		//		},
		{
			// only one invalid element is printed in the error
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

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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

func TestValidateUnknownFieldsAll(t *testing.T) {
	testcases := []struct {
		name             string
		configContent    string
		jobConfigContent map[string]string
		expectedErr      bool
	}{
		{
			name: "no separate job-config, all known fields",
			configContent: `
plank:
  default_decoration_config_entries:
    - config:
        timeout: 2h
        grace_period: 15s
        utility_images:
          clonerefs: "clonerefs:default"
          initupload: "initupload:default"
          entrypoint: "entrypoint:default"
          sidecar: "sidecar:default"
        gcs_configuration:
          bucket: "default-bucket"
          path_strategy: "legacy"
          default_org: "kubernetes"
          default_repo: "kubernetes"
        gcs_credentials_secret: "default-service-account"

presubmits:
  kube/kube:
  - name: test-presubmit
    decorate: true
    spec:
      containers:
      - image: alpine
        command: ["/bin/printenv"]
`,
		},
		{
			name: "no separate job-config, unknown field",
			configContent: `
presubmits:
  kube/kube:
  - name: test-presubmit
    never_run: true      // I'm unknown
    spec:
      containers:
      - image: alpine
`,
			expectedErr: true,
		},
		{
			name: "separate job-configs, all known field",
			configContent: `
presubmits:
  kube/kube:
  - name: kube-presubmit
    run_if_changed: "^src/"
    spec:
      containers:
      - image: alpine
`,
			jobConfigContent: map[string]string{
				"org-repo-presubmits.yaml": `
presubmits:
  org/repo:
  - name: org-repo-presubmit
    always_run: true
    spec:
      containers:
      - image: alpine
`,
				"org-repo2-presubmits.yaml": `
presubmits:
  org/repo2:
  - name: org-repo2-presubmit
    always_run: true
    spec:
      containers:
      - image: alpine
`,
			},
		},
		{
			name: "separate job-configs, unknown field in second job config",
			configContent: `
presubmits:
  kube/kube:
  - name: kube-presubmit
    never_run: true      // I'm unknown
    spec:
      containers:
      - image: alpine
`,
			jobConfigContent: map[string]string{
				"org-repo-presubmits.yaml": `
presubmits:
  org/repo:
  - name: org-repo-presubmit
    always_run: true
    spec:
      containers:
      - image: alpine
`,
				"org-repo2-presubmits.yaml": `
presubmits:
  org/repo2:
  - name: org-repo2-presubmit
    never_run: true       // I'm unknown
    spec:
      containers:
      - image: alpine
`,
			},
			expectedErr: true,
		},
	}
	for i := range testcases {
		tc := testcases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Set up config files
			root := t.TempDir()

			prowConfigFile := filepath.Join(root, "config.yaml")
			if err := os.WriteFile(prowConfigFile, []byte(tc.configContent), 0666); err != nil {
				t.Fatalf("Error writing config.yaml file: %v.", err)
			}
			var jobConfigDir string
			if len(tc.jobConfigContent) > 0 {
				jobConfigDir = filepath.Join(root, "job-config")
				if err := os.Mkdir(jobConfigDir, 0777); err != nil {
					t.Fatalf("Error creating job-config directory: %v.", err)
				}
				for file, content := range tc.jobConfigContent {
					file = filepath.Join(jobConfigDir, file)
					if err := os.WriteFile(file, []byte(content), 0666); err != nil {
						t.Fatalf("Error writing %q file: %v.", file, err)
					}
				}
			}
			// Test validation
			_, err := config.LoadStrict(prowConfigFile, jobConfigDir, nil, "")
			if (err != nil) != tc.expectedErr {
				if tc.expectedErr {
					t.Error("Expected an error, but did not receive one.")
				} else {
					content, _ := os.ReadFile(prowConfigFile)
					t.Log(string(content))
					t.Errorf("Unexpected error: %v.", err)
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Orgs: []string{"kubernetes"},
							},
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Orgs:          []string{"kubernetes"},
								ExcludedRepos: []string{"kubernetes/test-infra"},
							},
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Repos: []string{"kubernetes/test-infra"},
							},
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Orgs: []string{"kubernetes"},
							},
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Repos: []string{"kubernetes/test-infra"},
							},
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Orgs: []string{"kubernetes", "k8s"},
							},
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Repos: []string{"kubernetes/kubernetes"},
							},
							{
								Repos: []string{"kubernetes/test-infra"},
							},
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
					TideGitHubConfig: config.TideGitHubConfig{
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
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: []config.TideQuery{
							{
								Repos: []string{"kubernetes/test-infra"},
							},
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
			description: "org with blunderbuss enabled contains a repo without OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org": {"blunderbuss"}})},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with approve enable contains a repo without OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org": {"approve"}})},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains a repo without OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org": {"owners-label"}})},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains an *archived* repo without OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org": {"owners-label"}})},
			gh: fakeGH{
				files:    fakeGHContent{"org": {"repo": {"NOOWNERS": true}}},
				archived: map[string]bool{"org/repo": true},
			},
			expected: "",
		}, {
			description: "repo with owners-label enabled does not contain OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org": {"owners-label"}})},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains only repos with OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org": {"owners-label"}})},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"OWNERS": true}}}},
			expected:    "",
		}, {
			description: "repo with owners-label enabled contains OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org": {"owners-label"}})},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"OWNERS": true}}}},
			expected:    "",
		}, {
			description: "repo with unrelated plugin enabled does not contain OWNERS (legacy config)",
			cfg:         &plugins.Configuration{Plugins: plugins.OldToNewPlugins(map[string][]string{"org/repo": {"cat"}})},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected:    "",
		}, {
			description: "org with blunderbuss enabled contains a repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org": {Plugins: []string{"blunderbuss"}}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with approve enable contains a repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org": {Plugins: []string{"approve"}}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with approve excluded contains a repo without OWNERS",
			cfg: &plugins.Configuration{Plugins: plugins.Plugins{"org": {
				Plugins:       []string{"approve"},
				ExcludedRepos: []string{"repo"},
			}}},
			gh:       fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "",
		}, {
			description: "org with approve repo-enabled contains a repo without OWNERS",
			cfg: &plugins.Configuration{Plugins: plugins.Plugins{
				"org": {
					Plugins:       []string{"approve"},
					ExcludedRepos: []string{"repo"},
				},
				"org/repo": {Plugins: []string{"approve"}},
			}},
			gh: fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains a repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org": {Plugins: []string{"owners-label"}}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains an *archived* repo without OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org": {Plugins: []string{"owners-label"}}}},
			gh: fakeGH{
				files:    fakeGHContent{"org": {"repo": {"NOOWNERS": true}}},
				archived: map[string]bool{"org/repo": true},
			},
			expected: "",
		}, {
			description: "repo with owners-label enabled does not contain OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org/repo": {Plugins: []string{"owners-label"}}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"NOOWNERS": true}}}},
			expected: "the following orgs or repos enable at least one" +
				" plugin that uses OWNERS files (approve, blunderbuss, owners-label), but" +
				" its master branch does not contain a root level OWNERS file: [org/repo]",
		}, {
			description: "org with owners-label enabled contains only repos with OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org": {Plugins: []string{"owners-label"}}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"OWNERS": true}}}},
			expected:    "",
		}, {
			description: "repo with owners-label enabled contains OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org/repo": {Plugins: []string{"owners-label"}}}},
			gh:          fakeGH{files: fakeGHContent{"org": {"repo": {"OWNERS": true}}}},
			expected:    "",
		}, {
			description: "repo with unrelated plugin enabled does not contain OWNERS",
			cfg:         &plugins.Configuration{Plugins: plugins.Plugins{"org/repo": {Plugins: []string{"cat"}}}},
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
	defaultGitHubOptions.AddCustomizedFlags(flag.NewFlagSet("", flag.ContinueOnError), throttlerDefaults)
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
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "prow/config.yaml",
					JobConfigPath:                         "config/jobs/org/job.yaml",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 200,
				},
				pluginsConfig: pluginsflagutil.PluginOptions{
					PluginConfigPath:                         "prow/plugins/plugin.yaml",
					SupplementalPluginsConfigsFileNameSuffix: "_pluginconfig.yaml",
					CheckUnknownPlugins:                      true,
				},
				warnings:        StringsFlag([]string{"mismatched-tide", "mismatched-tide-lenient"}),
				excludeWarnings: StringsFlag([]string{"tide-strict-branch", "mismatched-tide", "ok-if-unknown-warning"}),
				strict:          true,
				expensive:       false,
				github:          defaultGitHubOptions,
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
				t.Errorf("actual differs from expected: %s", cmp.Diff(actualOptions, *tc.expectedOptions, cmp.Exporter(func(_ reflect.Type) bool { return true })))
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
			expected: fmt.Errorf("invalid job test on repo org/repo: the following refs specified more than once: %s",
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
		strict       bool
		expectedErr  string
	}{
		{
			name:         "Valid prowYAML, no err",
			prowYAMLData: []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
		},
		{
			name:         "Invalid prowYAML presubmit, err",
			prowYAMLData: []byte(`presubmits: [{"name": "hans"}]`),
			expectedErr:  "failed to validate Prow YAML: invalid presubmit job hans: kubernetes jobs require a spec",
		},
		{
			name:         "Invalid prowYAML postsubmit, err",
			prowYAMLData: []byte(`postsubmits: [{"name": "hans"}]`),
			expectedErr:  "failed to validate Prow YAML: invalid postsubmit job hans: kubernetes jobs require a spec",
		},
		{
			name: "Absent prowYAML, no err",
		},
		{
			name:         "unknown field prowYAML fails strict validation",
			strict:       true,
			prowYAMLData: []byte(`presubmits: [{"name": "hans", "never_run": "true", "spec": {"containers": [{}]}}]`),
			expectedErr:  "error unmarshaling JSON: while decoding JSON: json: unknown field \"never_run\"",
		},
	}

	for _, tc := range testCases {
		prowYAMLFileName := "/this-must-not-exist"

		if tc.prowYAMLData != nil {
			fileName := filepath.Join(t.TempDir(), ".prow.yaml")
			if err := os.WriteFile(fileName, tc.prowYAMLData, 0666); err != nil {
				t.Fatalf("failed to write to tempfile: %v", err)
			}

			prowYAMLFileName = fileName
		}

		// Need an empty file to load the config from so we go through its defaulting
		tempConfig, err := os.CreateTemp("", "prow-test")
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

		cfg, err := config.Load(tempConfig.Name(), "", nil, "")
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		err = validateInRepoConfig(cfg, prowYAMLFileName, "my/repo", tc.strict)
		var errString string
		if err != nil {
			errString = err.Error()
		}

		if errString != tc.expectedErr && !strings.Contains(errString, tc.expectedErr) {
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
				c.InRepoConfig.Enabled = map[string]*bool{"*": utilpointer.Bool(true)}
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
				config: configflagutil.ConfigOptions{ConfigPath: "testdata/combined.yaml"},
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

type fakeOpener struct {
	io.Opener
	content   string
	readError error
}

func (fo *fakeOpener) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	if fo.readError != nil {
		return nil, fo.readError
	}
	return stdio.NopCloser(strings.NewReader(fo.content)), nil
}

func (fo *fakeOpener) Close() error {
	return nil
}

func TestValidateClusterField(t *testing.T) {
	testCases := []struct {
		name              string
		cfg               *config.Config
		clusterStatusFile string
		readError         error
		expectedError     string
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
		{
			name: "cluster validates with lone reachable default cluster",
			cfg: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{BuildClusterStatusFile: "gs://my-bucket/build-cluster-status.json"},
				},
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Cluster: "default",
								},
							}}}}},
			clusterStatusFile: fmt.Sprintf(`{"default": %q}`, plank.ClusterStatusReachable),
		},
		{
			name: "cluster validates with multiple clusters, specified is reachable",
			cfg: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{BuildClusterStatusFile: "gs://my-bucket/build-cluster-status.json"},
				},
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Cluster: "build1",
								},
							}}}}},
			clusterStatusFile: fmt.Sprintf(`{"default": %q, "build1": %q, "build2": %q}`, plank.ClusterStatusReachable, plank.ClusterStatusReachable, plank.ClusterStatusUnreachable),
		},
		{
			name: "cluster validates with multiple clusters, specified is unreachable (just warn)",
			cfg: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{BuildClusterStatusFile: "gs://my-bucket/build-cluster-status.json"},
				},
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Name:    "my-job",
									Cluster: "build2",
								},
							}}}}},
			clusterStatusFile: fmt.Sprintf(`{"default": %q, "build1": %q, "build2": %q}`, plank.ClusterStatusReachable, plank.ClusterStatusReachable, plank.ClusterStatusUnreachable),
		},
		{
			name: "cluster fails validation with multiple clusters, specified is unrecognized",
			cfg: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{BuildClusterStatusFile: "gs://my-bucket/build-cluster-status.json"},
				},
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Name:    "my-job",
									Cluster: "build3",
								},
							}}}}},
			clusterStatusFile: fmt.Sprintf(`{"default": %q, "build1": %q, "build2": %q}`, plank.ClusterStatusReachable, plank.ClusterStatusReachable, plank.ClusterStatusUnreachable),
			expectedError:     "org1/repo1: job configuration for \"my-job\" specifies unknown 'cluster' value \"build3\"",
		},
		{
			name: "cluster validation skipped if status file does not exist yet",
			cfg: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{BuildClusterStatusFile: "gs://my-bucket/build-cluster-status.json"},
				},
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org1/repo1": {
							{
								JobBase: config.JobBase{
									Cluster: "build1",
								},
							}}}}},
			readError: os.ErrNotExist,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opener := fakeOpener{content: tc.clusterStatusFile, readError: tc.readError}
			errMsg := ""
			if err := validateCluster(tc.cfg, &opener); err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.expectedError {
				t.Errorf("expected error %q, got error %q", tc.expectedError, errMsg)
			}
		})
	}
}

func TestValidateAdditionalProwConfigIsInOrgRepoDirectoryStructure(t *testing.T) {
	t.Parallel()
	const root = "root"
	const invalidConfig = `[]`
	const validGlobalConfig = `
sinker:
  exclude_clusters:
    - default
slack_reporter_configs:
  '*':
    channel: '#general-announcements'
    job_states_to_report:
      - failure
      - error
	  - success
	report_template: Job {{.Spec.Job}} ended with status {{.Status.State}}.`
	const validOrgConfig = `
branch-protection:
  orgs:
    my-org:
      protect: true
tide:
  merge_method:
    my-org: squash
slack_reporter_configs:
  my-org:
    channel: '#my-org-announcements'
    job_states_to_report:
      - failure
      - error
    report_template: Job {{.Spec.Job}} needs my-org maintainers attention.`
	const validRepoConfig = `
branch-protection:
  orgs:
    my-org:
      repos:
        my-repo:
          protect: true
tide:
  merge_method:
    my-org/my-repo: squash
slack_reporter_configs:
  my-org/my-repo:
    channel: '#my-repo-announcements'
    job_states_to_report:
      - failure
    report_template: Job {{.Spec.Job}} needs my-repo maintainers attention.`
	const validGlobalPluginsConfig = `
blunderbuss:
  max_request_count: 2
  request_count: 2
  use_status_availability: true`
	const validOrgPluginsConfig = `
label:
  restricted_labels:
    my-org:
    - label: cherry-pick-approved
      allowed_teams:
      - patch-managers
plugins:
  my-org:
    plugins:
    - assign`
	const validRepoPluginsConfig = `
plugins:
  my-org/my-repo:
    plugins:
    - assign`

	tests := []struct {
		name string
		fs   fstest.MapFS

		expectedErrorMessage string
	}{
		{
			name: "No configs, no error",
			fs:   testfs(map[string]string{root + "/OWNERS": "some-owners"}),
		},
		{
			name: "Config directly below root, no error",
			fs: testfs(map[string]string{
				root + "/cfg.yaml":     validGlobalConfig,
				root + "/plugins.yaml": validGlobalPluginsConfig,
			}),
		},
		{
			name: "Valid org config",
			fs: testfs(map[string]string{
				root + "/my-org/cfg.yaml":     validOrgConfig,
				root + "/my-org/plugins.yaml": validOrgPluginsConfig,
			}),
		},
		{
			name: "Valid org config for wrong org",
			fs: testfs(map[string]string{
				root + "/my-other-org/cfg.yaml":     validOrgConfig,
				root + "/my-other-org/plugins.yaml": validOrgPluginsConfig,
			}),
			expectedErrorMessage: `[config root/my-other-org/cfg.yaml is invalid: Must contain only config for org my-other-org, but contains config for org my-org, config root/my-other-org/plugins.yaml is invalid: Must contain only config for org my-other-org, but contains config for org my-org]`,
		},
		{
			name: "Invalid org config",
			fs: testfs(map[string]string{
				root + "/my-org/cfg.yaml":     invalidConfig,
				root + "/my-org/plugins.yaml": invalidConfig,
			}),
			expectedErrorMessage: `[failed to unmarshal root/my-org/cfg.yaml into *config.Config: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go value of type config.Config, failed to unmarshal root/my-org/plugins.yaml into *plugins.Configuration: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go value of type plugins.Configuration]`,
		},
		{
			name: "Repo config at org level",
			fs: testfs(map[string]string{
				root + "/my-org/cfg.yaml":     validRepoConfig,
				root + "/my-org/plugins.yaml": validRepoPluginsConfig,
			}),
			expectedErrorMessage: `[config root/my-org/cfg.yaml is invalid: Must contain only config for org my-org, but contains config for repo my-org/my-repo, config root/my-org/plugins.yaml is invalid: Must contain only config for org my-org, but contains config for repo my-org/my-repo]`,
		},
		{
			name: "Valid repo config",
			fs: testfs(map[string]string{
				root + "/my-org/my-repo/cfg.yaml":     validRepoConfig,
				root + "/my-org/my-repo/plugins.yaml": validRepoPluginsConfig,
			}),
		},
		{
			name: "Valid repo config for wrong repo",
			fs: testfs(map[string]string{
				root + "/my-org/my-other-repo/cfg.yaml":     validRepoConfig,
				root + "/my-org/my-other-repo/plugins.yaml": validRepoPluginsConfig,
			}),
			expectedErrorMessage: `[config root/my-org/my-other-repo/cfg.yaml is invalid: Must only contain config for repo my-org/my-other-repo, but contains config for repo my-org/my-repo, config root/my-org/my-other-repo/plugins.yaml is invalid: Must only contain config for repo my-org/my-other-repo, but contains config for repo my-org/my-repo]`,
		},
		{
			name: "Invalid repo config",
			fs: testfs(map[string]string{
				root + "/my-org/my-repo/cfg.yaml":     invalidConfig,
				root + "/my-org/my-repo/plugins.yaml": invalidConfig,
			}),
			expectedErrorMessage: `[failed to unmarshal root/my-org/my-repo/cfg.yaml into *config.Config: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go value of type config.Config, failed to unmarshal root/my-org/my-repo/plugins.yaml into *plugins.Configuration: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal array into Go value of type plugins.Configuration]`,
		},
		{
			name: "Org config at repo level",
			fs: testfs(map[string]string{
				root + "/my-org/my-repo/cfg.yaml":     validOrgConfig,
				root + "/my-org/my-repo/plugins.yaml": validOrgPluginsConfig,
			}),
			expectedErrorMessage: `[config root/my-org/my-repo/cfg.yaml is invalid: Must only contain config for repo my-org/my-repo, but contains config for org my-org, config root/my-org/my-repo/plugins.yaml is invalid: Must only contain config for repo my-org/my-repo, but contains config for org my-org]`,
		},
		{
			name: "Nested too deeply",
			fs: testfs(map[string]string{
				root + "/my-org/my-repo/nest/cfg.yaml":     validOrgConfig,
				root + "/my-org/my-repo/nest/plugins.yaml": validOrgPluginsConfig,
			}),

			expectedErrorMessage: `[config root/my-org/my-repo/nest/cfg.yaml is at an invalid location. All configs must be below root. If they are org-specific, they must be in a folder named like the org. If they are repo-specific, they must be in a folder named like the repo below a folder named like the org., config root/my-org/my-repo/nest/plugins.yaml is at an invalid location. All configs must be below root. If they are org-specific, they must be in a folder named like the org. If they are repo-specific, they must be in a folder named like the repo below a folder named like the org.]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var errMsg string
			err := validateAdditionalProwConfigIsInOrgRepoDirectoryStructure(tc.fs, []string{root}, []string{root}, "cfg.yaml", "plugins.yaml")
			if err != nil {
				errMsg = err.Error()
			}
			if tc.expectedErrorMessage != errMsg {
				t.Errorf("expected error %s, got %s", tc.expectedErrorMessage, errMsg)
			}
		})
	}
}

func testfs(files map[string]string) fstest.MapFS {
	filesystem := fstest.MapFS{}
	for path, content := range files {
		filesystem[path] = &fstest.MapFile{Data: []byte(content)}
	}
	return filesystem
}

func TestValidateUnmanagedBranchprotectionConfigDoesntHaveSubconfig(t *testing.T) {
	t.Parallel()
	bpConfigWithSettingsOnAllLayers := func(m ...func(*config.BranchProtection)) config.BranchProtection {
		cfg := config.BranchProtection{
			Policy: config.Policy{Exclude: []string{"some-regex"}},
			Orgs: map[string]config.Org{
				"my-org": {
					Policy: config.Policy{Exclude: []string{"some-regex"}},
					Repos: map[string]config.Repo{
						"my-repo": {
							Policy: config.Policy{Exclude: []string{"some-regex"}},
							Branches: map[string]config.Branch{
								"my-branch": {
									Policy: config.Policy{Exclude: []string{"some-regex"}},
								},
							},
						},
					},
				},
			},
		}

		for _, modify := range m {
			modify(&cfg)
		}

		return cfg
	}

	testCases := []struct {
		name   string
		config config.BranchProtection

		expectedErrorMsg string
	}{
		{
			name: "Empty config, no error",
		},
		{
			name: "Globally disabled, errors for global and org config",
			config: bpConfigWithSettingsOnAllLayers(func(bp *config.BranchProtection) {
				bp.Unmanaged = utilpointer.Bool(true)
			}),

			expectedErrorMsg: `[branch protection is globally set to unmanaged, but has configuration, branch protection config is globally set to unmanaged but has configuration for org my-org without setting the org to unmanaged: false]`,
		},
		{
			name: "Org-level disabled, errors for org policy and repos",
			config: bpConfigWithSettingsOnAllLayers(func(bp *config.BranchProtection) {
				p := bp.Orgs["my-org"]
				p.Unmanaged = utilpointer.Bool(true)
				bp.Orgs["my-org"] = p
			}),

			expectedErrorMsg: `[branch protection config for org my-org is set to unmanaged, but it defines settings, branch protection config for repo my-org/my-repo is defined, but branch protection is unmanaged for org my-org without setting the repo to unmanaged: false]`,
		},

		{
			name: "Repo-level disabled, errors for repo policy and branches",
			config: bpConfigWithSettingsOnAllLayers(func(bp *config.BranchProtection) {
				p := bp.Orgs["my-org"].Repos["my-repo"]
				p.Unmanaged = utilpointer.Bool(true)
				bp.Orgs["my-org"].Repos["my-repo"] = p
			}),

			expectedErrorMsg: `[branch protection config for repo my-org/my-repo is set to unmanaged, but it defines settings, branch protection for repo my-org/my-repo is set to unmanaged, but it defines settings for branch my-branch without setting the branch to unmanaged: false]`,
		},

		{
			name: "Branch-level disabled, errors for branch policy",
			config: bpConfigWithSettingsOnAllLayers(func(bp *config.BranchProtection) {
				p := bp.Orgs["my-org"].Repos["my-repo"].Branches["my-branch"]
				p.Unmanaged = utilpointer.Bool(true)
				bp.Orgs["my-org"].Repos["my-repo"].Branches["my-branch"] = p
			}),

			expectedErrorMsg: `branch protection config for branch my-branch in repo my-org/my-repo is set to unmanaged but defines settings`,
		},
		{
			name: "unmanaged repo level is overridden by branch level, no errors",
			config: bpConfigWithSettingsOnAllLayers(func(bp *config.BranchProtection) {
				repoP := bp.Orgs["my-org"].Repos["my-repo"]
				repoP.Unmanaged = utilpointer.Bool(true)
				bp.Orgs["my-org"].Repos["my-repo"] = repoP
				p := bp.Orgs["my-org"].Repos["my-repo"].Branches["my-branch"]
				p.Unmanaged = utilpointer.Bool(false)
				bp.Orgs["my-org"].Repos["my-repo"].Branches["my-branch"] = p
			}),
		},
	}

	for _, tc := range testCases {
		var errMsg string
		err := validateUnmanagedBranchprotectionConfigDoesntHaveSubconfig(tc.config)
		if err != nil {
			errMsg = err.Error()
		}
		if tc.expectedErrorMsg != errMsg {
			t.Errorf("expected error message\n%s\ngot error message\n%s", tc.expectedErrorMsg, errMsg)
		}
	}
}

type fakeGhAppListingClient struct {
	installations []github.AppInstallation
}

func (f *fakeGhAppListingClient) ListAppInstallations() ([]github.AppInstallation, error) {
	return f.installations, nil
}

func TestValidateGitHubAppIsInstalled(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		allRepos      sets.Set[string]
		installations []github.AppInstallation

		expectedErrorMsg string
	}{
		{
			name:     "Installations exist",
			allRepos: sets.New[string]("org/repo", "org-a/repo-a", "org-b/repo-b"),
			installations: []github.AppInstallation{
				{Account: github.User{Login: "org"}},
				{Account: github.User{Login: "org-a"}},
				{Account: github.User{Login: "org-b"}},
			},
		},
		{
			name:     "Some installations exist",
			allRepos: sets.New[string]("org/repo", "org-a/repo-a", "org-b/repo-b"),
			installations: []github.AppInstallation{
				{Account: github.User{Login: "org"}},
				{Account: github.User{Login: "org-a"}},
			},

			expectedErrorMsg: `There is configuration for the GitHub org "org-b" but the GitHub app is not installed there`,
		},
		{
			name:     "No installations exist",
			allRepos: sets.New[string]("org/repo", "org-a/repo-a", "org-b/repo-b"),

			expectedErrorMsg: `[There is configuration for the GitHub org "org-a" but the GitHub app is not installed there, There is configuration for the GitHub org "org-b" but the GitHub app is not installed there, There is configuration for the GitHub org "org" but the GitHub app is not installed there]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualErrMsg string
			if err := validateGitHubAppIsInstalled(&fakeGhAppListingClient{installations: tc.installations}, tc.allRepos); err != nil {
				actualErrMsg = err.Error()
			}

			if actualErrMsg != tc.expectedErrorMsg {
				t.Errorf("expected error %q, got error %q", tc.expectedErrorMsg, actualErrMsg)
			}
		})
	}
}

func TestVerifyLabelPlugin(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name             string
		label            plugins.Label
		expectedErrorMsg string
	}{
		{
			name: "empty label config is valid",
		},
		{
			name: "cannot use the empty string as label name",
			label: plugins.Label{
				RestrictedLabels: map[string][]plugins.RestrictedLabel{
					"openshift/machine-config-operator": {
						{
							Label:        "",
							AllowedTeams: []string{"openshift-patch-managers"},
						},
						{
							Label:        "backport-risk-assessed",
							AllowedUsers: []string{"kikisdeliveryservice", "sinnykumari", "yuqi-zhang"},
						},
					},
				},
			},
			expectedErrorMsg: "the following orgs or repos have configuration of label plugin using the empty string as label name in restricted labels: openshift/machine-config-operator",
		},
		{
			name: "valid after removing the restricted labels for the empty string",
			label: plugins.Label{
				RestrictedLabels: map[string][]plugins.RestrictedLabel{
					"openshift/machine-config-operator": {
						{
							Label:        "backport-risk-assessed",
							AllowedUsers: []string{"kikisdeliveryservice", "sinnykumari", "yuqi-zhang"},
						},
					},
				},
			},
		},
		{
			name: "two invalid label configs",
			label: plugins.Label{
				RestrictedLabels: map[string][]plugins.RestrictedLabel{
					"orgRepo1": {
						{
							Label:        "",
							AllowedTeams: []string{"some-team"},
						},
					},
					"orgRepo2": {
						{
							Label:        "",
							AllowedUsers: []string{"some-user"},
						},
					},
				},
			},
			expectedErrorMsg: "the following orgs or repos have configuration of label plugin using the empty string as label name in restricted labels: orgRepo1, orgRepo2",
		},
		{
			name: "invalid when additional and restricted labels are the same",
			label: plugins.Label{
				AdditionalLabels: []string{"cherry-pick-approved"},
				RestrictedLabels: map[string][]plugins.RestrictedLabel{
					"orgRepo1": {
						{
							Label: "cherry-pick-approved",
						},
					},
				},
			},
			expectedErrorMsg: "the following orgs or repos have configuration of label plugin using the restricted label cherry-pick-approved which is also configured as an additional label: orgRepo1",
		},
		{
			name: "invalid when additional and restricted labels are the same in multiple orgRepos and empty string",
			label: plugins.Label{
				AdditionalLabels: []string{"cherry-pick-approved"},
				RestrictedLabels: map[string][]plugins.RestrictedLabel{
					"orgRepo1": {
						{
							Label: "cherry-pick-approved",
						},
					},
					"orgRepo2": {
						{
							Label: "",
						},
					},
					"orgRepo3": {
						{
							Label: "cherry-pick-approved",
						},
					},
				},
			},
			expectedErrorMsg: "[the following orgs or repos have configuration of label plugin using the restricted label cherry-pick-approved which is also configured as an additional label: orgRepo1, orgRepo3, " +
				"the following orgs or repos have configuration of label plugin using the empty string as label name in restricted labels: orgRepo2]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualErrMsg string
			if err := verifyLabelPlugin(tc.label); err != nil {
				actualErrMsg = err.Error()
			}
			if actualErrMsg != tc.expectedErrorMsg {
				t.Errorf("expected error %q, got error %q", tc.expectedErrorMsg, actualErrMsg)
			}
		})
	}
}

func TestValidateRequiredJobAnnotations(t *testing.T) {
	tc := []struct {
		name                string
		presubmits          []config.Presubmit
		postsubmits         []config.Postsubmit
		periodics           []config.Periodic
		expectedErr         bool
		expectedAnnotations []string
	}{
		{
			name: "no annotation is required, pass",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{},
				},
			},
			postsubmits: []config.Postsubmit{
				{
					JobBase: config.JobBase{
						Annotations: map[string]string{"prow.k8s.io/cat": "meow"},
					},
				},
			},
			periodics: []config.Periodic{
				{
					JobBase: config.JobBase{},
				},
			},
			expectedErr:         false,
			expectedAnnotations: nil,
		},
		{
			name: "jobs don't have required annotation, fail",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{},
				},
			},
			postsubmits: []config.Postsubmit{
				{
					JobBase: config.JobBase{
						Annotations: map[string]string{"prow.k8s.io/cat": "meow"},
					},
				},
			},
			periodics: []config.Periodic{
				{
					JobBase: config.JobBase{},
				},
			},
			expectedAnnotations: []string{"prow.k8s.io/maintainer"},
			expectedErr:         true,
		},
		{
			name: "jobs have required annotations, pass",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Annotations: map[string]string{"prow.k8s.io/maintainer": "job-maintainer"},
					},
				},
			},
			postsubmits: []config.Postsubmit{
				{
					JobBase: config.JobBase{
						Annotations: map[string]string{"prow.k8s.io/maintainer": "job-maintainer"},
					},
				},
			},
			periodics: []config.Periodic{
				{
					JobBase: config.JobBase{
						Annotations: map[string]string{"prow.k8s.io/maintainer": "job-maintainer"},
					},
				},
			},
			expectedAnnotations: []string{"prow.k8s.io/maintainer"},
			expectedErr:         false,
		},
	}

	for _, c := range tc {
		t.Run(c.name, func(t *testing.T) {
			jcfg := config.JobConfig{
				PresubmitsStatic:  map[string][]config.Presubmit{"org/repo": c.presubmits},
				PostsubmitsStatic: map[string][]config.Postsubmit{"org/repo": c.postsubmits},
				Periodics:         c.periodics,
			}
			err := validateRequiredJobAnnotations(c.expectedAnnotations, jcfg)
			if c.expectedErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !c.expectedErr && err != nil {
				t.Errorf("Got error but didn't expect one: %v", err)
			}
		})
	}
}

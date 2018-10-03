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
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
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

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

import "testing"

func TestEnsureValidConfiguration(t *testing.T) {
	var testCases = []struct {
		name                                    string
		tideSubSet, tideSuperSet, pluginsSubSet orgRepoConfig
		expectedErr                             bool
	}{
		{
			name:          "nothing enabled makes no error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{}),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on org without tide makes no error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{"org"}, []string{}),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo without tide makes no error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{"org/repo"}),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo with tide on repo makes no error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{"org/repo"}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{"org/repo"}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{"org/repo"}),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on repo with tide on org makes error",
			tideSubSet:    newOrgRepoConfig([]string{"org"}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{"org"}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{"org/repo"}),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on org with tide on repo makes no error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{"org/repo"}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{"org/repo"}),
			pluginsSubSet: newOrgRepoConfig([]string{"org"}, []string{}),
			expectedErr:   false,
		},
		{
			name:          "plugin enabled on org with tide on org makes no error",
			tideSubSet:    newOrgRepoConfig([]string{"org"}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{"org"}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{"org"}, []string{}),
			expectedErr:   false,
		},
		{
			name:          "tide enabled on org without plugin makes error",
			tideSubSet:    newOrgRepoConfig([]string{"org"}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{"org"}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{}),
			expectedErr:   true,
		},
		{
			name:          "tide enabled on repo without plugin makes error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{"org/repo"}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{"org/repo"}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{}),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on org with any tide record but no specific tide requirement makes error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{"org"}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{"org"}, []string{}),
			expectedErr:   true,
		},
		{
			name:          "plugin enabled on repo with any tide record but no specific tide requirement makes error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{"org/repo"}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{"org/repo"}),
			expectedErr:   true,
		},
		{
			name:          "any tide org record but no specific tide requirement or plugin makes no error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{"org"}, []string{}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{}),
			expectedErr:   false,
		},
		{
			name:          "any tide repo record but no specific tide requirement or plugin makes no error",
			tideSubSet:    newOrgRepoConfig([]string{}, []string{}),
			tideSuperSet:  newOrgRepoConfig([]string{}, []string{"org/repo"}),
			pluginsSubSet: newOrgRepoConfig([]string{}, []string{}),
			expectedErr:   false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ensureValidConfiguration("plugin", "label", testCase.tideSubSet, testCase.tideSuperSet, testCase.pluginsSubSet)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
		})
	}
}

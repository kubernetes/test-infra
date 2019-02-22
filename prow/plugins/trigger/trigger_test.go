/*
Copyright 2019 The Kubernetes Authors.

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

package trigger

import (
	"testing"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/plugins"
)

func TestHelpProvider(t *testing.T) {
	cases := []struct {
		name         string
		config       *plugins.Configuration
		enabledRepos []string
		err          bool
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo"},
		},
		{
			name:         "Overlapping org and org/repo",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org2", "org2/repo"},
		},
		{
			name:         "Invalid enabledRepos",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo/extra"},
			err:          true,
		},
		{
			name: "All configs enabled",
			config: &plugins.Configuration{
				Triggers: []plugins.Trigger{
					{
						Repos:          []string{"org2"},
						TrustedOrg:     "org2",
						JoinOrgURL:     "https://join.me",
						OnlyOrgMembers: true,
						IgnoreOkToTest: true,
					},
				},
			},
			enabledRepos: []string{"org1", "org2/repo"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := helpProvider(c.config, c.enabledRepos)
			if err != nil && !c.err {
				t.Fatalf("helpProvider error: %v", err)
			}
		})
	}
}

func TestValidateContextOverlap(t *testing.T) {
	var testCases = []struct {
		name          string
		toRun, toSkip []config.Presubmit
		expectedErr   bool
	}{
		{
			name:   "empty inputs mean no error",
			toRun:  []config.Presubmit{},
			toSkip: []config.Presubmit{},
		},
		{
			name:   "disjoint sets mean no error",
			toRun:  []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}},
			toSkip: []config.Presubmit{{Reporter: config.Reporter{Context: "bar"}}},
		},
		{
			name:   "complex disjoint sets mean no error",
			toRun:  []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip: []config.Presubmit{{Reporter: config.Reporter{Context: "bar"}}, {Reporter: config.Reporter{Context: "otherbar"}}},
		},
		{
			name:        "overlapping sets error",
			toRun:       []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip:      []config.Presubmit{{Reporter: config.Reporter{Context: "bar"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			expectedErr: true,
		},
		{
			name:        "identical sets error",
			toRun:       []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			expectedErr: true,
		},
		{
			name:        "superset callErrors",
			toRun:       []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}, {Reporter: config.Reporter{Context: "thirdfoo"}}},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		validateErr := validateContextOverlap(testCase.toRun, testCase.toSkip)
		if validateErr == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if validateErr != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, validateErr)
		}
	}
}

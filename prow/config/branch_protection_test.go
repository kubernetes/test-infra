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

package config

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestJobRequirements(t *testing.T) {
	cases := []struct {
		name                          string
		config                        []Presubmit
		masterExpected, otherExpected []string
	}{
		{
			name: "basic",
			config: []Presubmit{
				{
					Context:    "always-run",
					AlwaysRun:  true,
					SkipReport: false,
				},
				{
					Context:      "run-if-changed",
					RunIfChanged: "foo",
					AlwaysRun:    false,
					SkipReport:   false,
				},
				{
					Context:    "optional",
					AlwaysRun:  false,
					SkipReport: false,
				},
				{
					Context:    "optional",
					AlwaysRun:  true,
					SkipReport: false,
					Optional:   true,
					Brancher: Brancher{
						SkipBranches: []string{"master"},
					},
				},
			},
			masterExpected: []string{"always-run", "run-if-changed"},
			otherExpected:  []string{"always-run", "run-if-changed"},
		},
		{
			name: "children",
			config: []Presubmit{
				{
					Context:    "always-run",
					AlwaysRun:  true,
					SkipReport: false,
					RunAfterSuccess: []Presubmit{
						{
							Context: "include-me",
						},
					},
				},
				{
					Context:      "run-if-changed",
					RunIfChanged: "foo",
					SkipReport:   true,
					AlwaysRun:    false,
					RunAfterSuccess: []Presubmit{
						{
							Context: "me2",
						},
					},
				},
				{
					Context:    "run-and-skip",
					AlwaysRun:  true,
					SkipReport: true,
					RunAfterSuccess: []Presubmit{
						{
							Context: "also-me-3",
						},
					},
				},
				{
					Context:    "optional",
					AlwaysRun:  false,
					SkipReport: false,
					RunAfterSuccess: []Presubmit{
						{
							Context: "no thanks",
						},
					},
				},
				{
					Context:    "hidden-grandpa",
					AlwaysRun:  true,
					SkipReport: true,
					RunAfterSuccess: []Presubmit{
						{
							Context:    "hidden-parent",
							SkipReport: true,
							AlwaysRun:  false,
							RunAfterSuccess: []Presubmit{
								{
									Context: "visible-kid",
									Brancher: Brancher{
										Branches: []string{"master"},
									},
								},
							},
						},
					},
				},
			},
			masterExpected: []string{
				"always-run", "include-me",
				"me2",
				"also-me-3",
				"visible-kid",
			},
			otherExpected: []string{
				"always-run", "include-me",
				"me2",
				"also-me-3",
			},
		},
	}

	for _, tc := range cases {
		masterActual := jobRequirements(tc.config, "master", false)
		if !reflect.DeepEqual(masterActual, tc.masterExpected) {
			t.Errorf("branch: master - %s: actual %v != expected %v", tc.name, masterActual, tc.masterExpected)
		}
		otherActual := jobRequirements(tc.config, "other", false)
		if !reflect.DeepEqual(masterActual, tc.masterExpected) {
			t.Errorf("branch: other - %s: actual %v != expected %v", tc.name, otherActual, tc.otherExpected)
		}
	}
}

func TestConfig_GetBranchProtection(t *testing.T) {
	yes := true
	no := false
	type orgRepoBranch struct {
		org, repo, branch string
	}
	type expected struct {
		b *Branch
		e string
	}

	testCases := []struct {
		name          string
		config        Config
		expected      []expected
		orgRepoBranch []orgRepoBranch
	}{
		{
			name:          "nothing",
			orgRepoBranch: []orgRepoBranch{{"org", "repo", "branch"}},
			expected:      []expected{{}},
		},
		{
			name:          "unknown org",
			orgRepoBranch: []orgRepoBranch{{"unknown", "unknown", "master"}},
			config: Config{
				BranchProtection: BranchProtection{
					Protect: &yes,
					Orgs: map[string]Org{
						"unknown": {},
					},
				},
			},
			expected: []expected{{b: &Branch{Protect: &yes}}},
		},
		{
			name:          "protect org via config default",
			orgRepoBranch: []orgRepoBranch{{"org", "repo", "branch"}},
			config: Config{
				BranchProtection: BranchProtection{
					Protect: &yes,
					Orgs: map[string]Org{
						"org": {},
					},
				},
			},
			expected: []expected{{b: &Branch{Protect: &yes}}},
		},
		{
			name: "protect this but not that org",
			orgRepoBranch: []orgRepoBranch{
				{"this", "repo", "branch"},
				{"that", "repo", "branch"},
			},
			config: Config{
				BranchProtection: BranchProtection{
					Protect: &no,
					Orgs: map[string]Org{
						"this": {Protect: &yes},
						"that": {},
					},
				},
			},
			expected: []expected{
				{b: &Branch{Protect: &yes}},
				{b: &Branch{Protect: &no}},
			},
		},
		{
			name:          "require a defined branch to make a protection decision",
			orgRepoBranch: []orgRepoBranch{{"org", "repo", "branch"}},
			config: Config{
				BranchProtection: BranchProtection{
					Orgs: map[string]Org{
						"org": {
							Repos: map[string]Repo{
								"repo": {
									Branches: map[string]Branch{
										"branch": {},
									},
								},
							},
						},
					},
				},
			},
			expected: []expected{{e: "protect should not be nil"}},
		},
		{
			name:          "require pushers to set protection",
			orgRepoBranch: []orgRepoBranch{{"org", "repo", "push"}},
			config: Config{
				BranchProtection: BranchProtection{
					Protect: &no,
					Pushers: []string{"oncall"},
					Orgs: map[string]Org{
						"org": {},
					},
				},
			},
			expected: []expected{{e: "setting pushers or contexts requires protection"}},
		},
		{
			name:          "require required contexts to set protection",
			orgRepoBranch: []orgRepoBranch{{"org", "repo", "context"}},
			config: Config{
				BranchProtection: BranchProtection{
					Protect:  &no,
					Contexts: []string{"test-foo"},
					Orgs: map[string]Org{
						"org": {},
					},
				},
			},
			expected: []expected{{e: "setting pushers or contexts requires protection"}},
		},
		{
			name: "protect org but skip a repo",
			orgRepoBranch: []orgRepoBranch{
				{"org", "repo1", "master"},
				{"org", "repo1", "branch"},
				{"org", "skip", "master"},
			},
			config: Config{
				BranchProtection: BranchProtection{
					Protect: &no,
					Orgs: map[string]Org{
						"org": {
							Protect: &yes,
							Repos: map[string]Repo{
								"skip": {
									Protect: &no,
								},
							},
						},
					},
				},
			},
			expected: []expected{
				{b: &Branch{Protect: &yes}},
				{b: &Branch{Protect: &yes}},
				{b: &Branch{Protect: &no}},
			},
		},
		{
			name:          "append contexts",
			orgRepoBranch: []orgRepoBranch{{"org", "repo", "master"}},
			config: Config{
				BranchProtection: BranchProtection{
					Protect:  &yes,
					Contexts: []string{"config-presubmit"},
					Orgs: map[string]Org{
						"org": {
							Contexts: []string{"org-presubmit"},
							Repos: map[string]Repo{
								"repo": {
									Contexts: []string{"repo-presubmit"},
									Branches: map[string]Branch{
										"master": {
											Contexts: []string{"branch-presubmit"},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []expected{
				{
					b: &Branch{
						Protect:  &yes,
						Contexts: []string{"config-presubmit", "org-presubmit", "repo-presubmit", "branch-presubmit"},
					},
				},
			},
		},
		{
			name:          "append pushers",
			orgRepoBranch: []orgRepoBranch{{"org", "repo", "master"}},
			config: Config{
				BranchProtection: BranchProtection{
					Protect: &yes,
					Pushers: []string{"config-team"},
					Orgs: map[string]Org{
						"org": {
							Pushers: []string{"org-team"},
							Repos: map[string]Repo{
								"repo": {
									Pushers: []string{"repo-team"},
									Branches: map[string]Branch{
										"master": {
											Pushers: []string{"branch-team"},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []expected{
				{
					b: &Branch{
						Protect: &yes,
						Pushers: []string{"config-team", "org-team", "repo-team", "branch-team"},
					},
				},
			},
		},
	}

	cmpBranch := func(a, b *Branch) bool {
		if a == nil {
			if b == nil {
				return true
			}
			return false
		}
		if b == nil {
			return false
		}
		if !reflect.DeepEqual(a.Protect, b.Protect) {
			return false
		}
		if !sets.NewString(a.Contexts...).Equal(sets.NewString(b.Contexts...)) {
			return false
		}
		if !sets.NewString(a.Pushers...).Equal(sets.NewString(b.Pushers...)) {
			return false
		}
		return true
	}

	for _, tc := range testCases {
		for i, orb := range tc.orgRepoBranch {
			actual, err := tc.config.GetBranchProtection(orb.org, orb.repo, orb.branch)
			expectedBranch := tc.expected[i].b
			expectedError := tc.expected[i].e
			if err != nil {
				if expectedError != err.Error() {
					t.Errorf("%s - Expected error '%v' received '%v'", tc.name, expectedError, err)
				}
			} else {
				if expectedError != "" {
					t.Errorf("%s - Expected error '%v' received '%v'", tc.name, expectedError, err)
				}
			}
			if !cmpBranch(expectedBranch, actual) {
				t.Errorf("%s - Expected %v received %v", tc.name, expectedBranch, actual)
			}
		}
	}
}

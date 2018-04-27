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
	"sort"
	"testing"
)

var (
	y   = true
	n   = false
	yes = &y
	no  = &n
)

func fixup(policy Policy) PolicyStruct {
	if policy == nil {
		return PolicyStruct{}
	}
	p := policy.Get()
	if p.Protect != nil {
		x := new(bool)
		*x = *p.Protect
		p.Protect = x
	}
	sort.Strings(p.Pushers)
	sort.Strings(p.Contexts)
	return p
}

func TestSelectBool(t *testing.T) {
	cases := []struct {
		name     string
		parent   *bool
		child    *bool
		expected *bool
	}{
		{
			name: "default is nil",
		},
		{
			name:     "use child if set",
			child:    yes,
			expected: yes,
		},
		{
			name:     "child overrides parent",
			child:    yes,
			parent:   no,
			expected: yes,
		},
		{
			name:     "use parent if child unset",
			parent:   no,
			expected: no,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := selectBool(tc.parent, tc.child)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("actual %v != expected %v", actual, tc.expected)
			}
		})
	}
}

func TestUnionStrings(t *testing.T) {
	cases := []struct {
		name     string
		parent   []string
		child    []string
		expected []string
	}{
		{
			name: "empty list",
		},
		{
			name:     "all parent items",
			parent:   []string{"hi", "there"},
			expected: []string{"hi", "there"},
		},
		{
			name:     "all child items",
			child:    []string{"hi", "there"},
			expected: []string{"hi", "there"},
		},
		{
			name:     "both child and parent items, no duplicates",
			child:    []string{"hi", "world"},
			parent:   []string{"hi", "there"},
			expected: []string{"hi", "there", "world"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := unionStrings(tc.parent, tc.child)
			sort.Strings(actual)
			sort.Strings(tc.expected)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("actual %v != expected %v", actual, tc.expected)
			}
		})
	}
}

func TestApply(t *testing.T) {
	cases := []struct {
		name     string
		parent   PolicyStruct
		child    PolicyStruct
		expected PolicyStruct
	}{
		{
			name: "default policy",
		},
		{
			name: "merge parent and child",
			parent: PolicyStruct{
				Protect:  yes,
				Contexts: []string{"cP"},
				Pushers:  []string{"pP"},
			},
			child: PolicyStruct{
				Protect:  no,
				Contexts: []string{"cC"},
				Pushers:  []string{"pC"},
			},
			expected: PolicyStruct{
				Protect:  no,
				Contexts: []string{"cP", "cC"},
				Pushers:  []string{"pP", "pC"},
			},
		},
		{
			name: "only parent",
			parent: PolicyStruct{
				Protect:  yes,
				Contexts: []string{"cP"},
				Pushers:  []string{"pP"},
			},
			expected: PolicyStruct{
				Protect:  yes,
				Contexts: []string{"cP"},
				Pushers:  []string{"pP"},
			},
		},
		{
			name: "only child",
			child: PolicyStruct{
				Protect:  yes,
				Contexts: []string{"cC"},
				Pushers:  []string{"pC"},
			},
			expected: PolicyStruct{
				Protect:  yes,
				Contexts: []string{"cC"},
				Pushers:  []string{"pC"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.parent.Apply(tc.child)
			actual = fixup(actual)
			tc.expected = fixup(tc.expected)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("actual %v != expected %v", actual, tc.expected)
			}
		})
	}
}

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
	testCases := []struct {
		name              string
		config            Config
		org, repo, branch string
		err               bool
		expected          PolicyStruct
	}{
		{
			name: "unprotected by default",
		},
		{
			name: "undefined org not protected",
			config: Config{
				BranchProtection: BranchProtection{
					PolicyStruct: PolicyStruct{
						Protect: yes,
					},
					Orgs: map[string]Org{
						"other": {},
						// org not present
					},
				},
			},
		},
		{
			name: "protect via config default",
			config: Config{
				BranchProtection: BranchProtection{
					PolicyStruct: PolicyStruct{
						Protect: yes,
					},
					Orgs: map[string]Org{
						"org": {},
					},
				},
			},
			expected: PolicyStruct{Protect: yes},
		},
		{
			name: "protect via org default",
			config: Config{
				BranchProtection: BranchProtection{
					Orgs: map[string]Org{
						"org": {
							PolicyStruct: PolicyStruct{
								Protect: yes,
							},
						},
					},
				},
			},
			expected: PolicyStruct{Protect: yes},
		},
		{
			name: "protect via repo default",
			config: Config{
				BranchProtection: BranchProtection{
					Orgs: map[string]Org{
						"org": {
							Repos: map[string]Repo{
								"repo": {
									PolicyStruct: PolicyStruct{
										Protect: yes,
									},
								},
							},
						},
					},
				},
			},
			expected: PolicyStruct{Protect: yes},
		},
		{
			name: "protect specific branch",
			config: Config{
				BranchProtection: BranchProtection{
					Orgs: map[string]Org{
						"org": {
							Repos: map[string]Repo{
								"repo": {
									Branches: map[string]Branch{
										"branch": {
											PolicyStruct: PolicyStruct{
												Protect: yes,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: PolicyStruct{Protect: yes},
		},
		{
			name: "ignore other org settings",
			config: Config{
				BranchProtection: BranchProtection{
					PolicyStruct: PolicyStruct{
						Protect: no,
					},
					Orgs: map[string]Org{
						"other": {
							PolicyStruct: PolicyStruct{Protect: yes},
						},
						"org": {},
					},
				},
			},
			expected: PolicyStruct{Protect: no},
		},
		{
			name: "defined branches must make a protection decision",
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
			err: true,
		},
		{
			name: "pushers require protection",
			config: Config{
				BranchProtection: BranchProtection{
					PolicyStruct: PolicyStruct{
						Protect: no,
						Pushers: []string{"oncall"},
					},
					Orgs: map[string]Org{
						"org": {},
					},
				},
			},
			err: true,
		},
		{
			name: "required contexts require protection",
			config: Config{
				BranchProtection: BranchProtection{
					PolicyStruct: PolicyStruct{
						Protect:  no,
						Contexts: []string{"test-foo"},
					},
					Orgs: map[string]Org{
						"org": {},
					},
				},
			},
			err: true,
		},
		{
			name: "Make required presubmits required",
			config: Config{
				BranchProtection: BranchProtection{
					PolicyStruct: PolicyStruct{
						Protect:  yes,
						Contexts: []string{"cla"},
					},
					Orgs: map[string]Org{
						"org": {},
					},
				},
				Presubmits: map[string][]Presubmit{
					"org/repo": {
						{
							Name:      "required presubmit",
							Context:   "required presubmit",
							AlwaysRun: true,
						},
					},
				},
			},
			expected: PolicyStruct{
				Protect:  yes,
				Contexts: []string{"required presubmit", "cla"},
			},
		},
		{
			name: "ProtectTested opts into protection",
			config: Config{
				BranchProtection: BranchProtection{
					ProtectTested: true,
					Orgs: map[string]Org{
						"org": {},
					},
				},
				Presubmits: map[string][]Presubmit{
					"org/repo": {
						{
							Name:      "required presubmit",
							Context:   "required presubmit",
							AlwaysRun: true,
						},
					},
				},
			},
			expected: PolicyStruct{
				Protect:  yes,
				Contexts: []string{"required presubmit"},
			},
		},
		{
			name: "required presubmits require protection",
			config: Config{
				BranchProtection: BranchProtection{
					PolicyStruct: PolicyStruct{
						Protect: no,
					},
					Orgs: map[string]Org{
						"org": {},
					},
				},
				Presubmits: map[string][]Presubmit{
					"org/repo": {
						{
							Name:      "required presubmit",
							Context:   "required presubmit",
							AlwaysRun: true,
						},
					},
				},
			},
			err: true,
		},
		{
			name: "Optional presubmits do not force protection",
			config: Config{
				BranchProtection: BranchProtection{
					ProtectTested: true,
					Orgs: map[string]Org{
						"org": {},
					},
				},
				Presubmits: map[string][]Presubmit{
					"org/repo": {
						{
							Name:      "optional presubmit",
							Context:   "optional presubmit",
							AlwaysRun: true,
							Optional:  true,
						},
					},
				},
			},
		},
		{
			name: "Explicit configuration takes precedence over ProtectTested",
			config: Config{
				BranchProtection: BranchProtection{
					ProtectTested: true,
					Orgs: map[string]Org{
						"org": {
							PolicyStruct: PolicyStruct{
								Protect: yes,
							},
						},
					},
				},
				Presubmits: map[string][]Presubmit{
					"org/repo": {
						{
							Name:      "optional presubmit",
							Context:   "optional presubmit",
							AlwaysRun: true,
							Optional:  true,
						},
					},
				},
			},
			expected: PolicyStruct{Protect: yes},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.config.GetBranchProtection("org", "repo", "branch")
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case err == nil && tc.err:
				t.Errorf("failed to receive an error")
			default:
				actual = fixup(actual)
				tc.expected = fixup(tc.expected)
				if !reflect.DeepEqual(actual.Get(), tc.expected.Get()) {
					t.Errorf("actual %+v != expected %+v", actual, tc.expected)
				}
			}
		})
	}
}

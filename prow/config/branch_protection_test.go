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

	"github.com/google/go-cmp/cmp"
	fuzz "github.com/google/gofuzz"
	"github.com/mohae/deepcopy"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"
)

var (
	y   = true
	n   = false
	yes = &y
	no  = &n
)

func normalize(policy *Policy) {
	if policy == nil || policy.RequiredStatusChecks == nil {
		return
	}
	sort.Strings(policy.RequiredStatusChecks.Contexts)
	sort.Strings(policy.Exclude)
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

func TestSelectInt(t *testing.T) {
	one := 1
	two := 2
	cases := []struct {
		name     string
		parent   *int
		child    *int
		expected *int
	}{
		{
			name: "default is nil",
		},
		{
			name:     "use child if set",
			child:    &one,
			expected: &one,
		},
		{
			name:     "child overrides parent",
			child:    &one,
			parent:   &two,
			expected: &one,
		},
		{
			name:     "use parent if child unset",
			parent:   &two,
			expected: &two,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := selectInt(tc.parent, tc.child)
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

func TestApply(test *testing.T) {
	t := true
	f := false
	basic := Policy{
		Protect: &t,
	}
	ebasic := Policy{
		Protect: &t,
	}
	cases := []struct {
		name     string
		parent   Policy
		child    Policy
		expected Policy
	}{
		{
			name:     "nil child",
			parent:   basic,
			expected: ebasic,
		},
		{
			name: "merge parent and child",
			parent: Policy{
				Protect: &t,
			},
			child: Policy{
				Admins:                &f,
				RequiredLinearHistory: &t,
				AllowForcePushes:      &t,
				AllowDeletions:        &t,
			},
			expected: Policy{
				Protect:               &t,
				Admins:                &f,
				RequiredLinearHistory: &t,
				AllowForcePushes:      &t,
				AllowDeletions:        &t,
			},
		},
		{
			name: "child overrides parent",
			parent: Policy{
				Protect: &t,
			},
			child: Policy{
				Protect: &f,
			},
			expected: Policy{
				Protect: &f,
			},
		},
		{
			name: "append strings",
			parent: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Contexts: []string{"hello", "world"},
				},
			},
			child: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Contexts: []string{"world", "of", "thrones"},
				},
			},
			expected: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Contexts: []string{"hello", "of", "thrones", "world"},
				},
			},
		},
		{
			name: "merge struct",
			parent: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Contexts: []string{"hi"},
				},
			},
			child: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Strict: &t,
				},
			},
			expected: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Contexts: []string{"hi"},
					Strict:   &t,
				},
			},
		},
		{
			name: "nil child struct",
			parent: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Strict: &f,
				},
			},
			child: Policy{
				Protect: &t,
			},
			expected: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Strict: &f,
				},
				Protect: &t,
			},
		},
		{
			name: "nil parent struct",
			child: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Strict: &f,
				},
			},
			parent: Policy{
				Protect: &t,
			},
			expected: Policy{
				RequiredStatusChecks: &ContextPolicy{
					Strict: &f,
				},
				Protect: &t,
			},
		},
		{
			name: "merge exclusion strings",
			child: Policy{
				Exclude: []string{"foo*"},
			},
			parent: Policy{
				Exclude: []string{"bar*"},
			},
			expected: Policy{
				Exclude: []string{"bar*", "foo*"},
			},
		},
	}

	for _, tc := range cases {
		test.Run(tc.name, func(test *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					test.Errorf("unexpected panic: %s", r)
				}
			}()
			actual := tc.parent.Apply(tc.child)
			normalize(&actual)
			normalize(&tc.expected)
			if !reflect.DeepEqual(actual, tc.expected) {
				test.Errorf("bad merged policy:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
			}
		})
	}
}

func TestBranchRequirements(t *testing.T) {
	cases := []struct {
		name                            string
		config                          []Presubmit
		masterExpected, otherExpected   []string
		masterOptional, otherOptional   []string
		masterIfPresent, otherIfPresent []string
	}{
		{
			name: "basic",
			config: []Presubmit{
				{
					AlwaysRun: true,
					Reporter: Reporter{
						Context:    "always-run",
						SkipReport: false,
					},
				},
				{
					RegexpChangeMatcher: RegexpChangeMatcher{
						RunIfChanged: "foo",
					},
					AlwaysRun: false,
					Reporter: Reporter{
						Context:    "run-if-changed",
						SkipReport: false,
					},
				},
				{
					AlwaysRun: false,
					Reporter: Reporter{
						Context:    "not-always",
						SkipReport: false,
					},
				},
				{
					AlwaysRun: true,
					Reporter: Reporter{
						Context:    "skip-report",
						SkipReport: true,
					},
					Brancher: Brancher{
						SkipBranches: []string{"master"},
					},
				},
				{
					AlwaysRun: true,
					Reporter: Reporter{
						Context:    "optional",
						SkipReport: false,
					},
					Optional: true,
				},
			},
			masterExpected:  []string{"always-run"},
			masterIfPresent: []string{"run-if-changed", "not-always"},
			masterOptional:  []string{"optional"},
			otherExpected:   []string{"always-run"},
			otherIfPresent:  []string{"run-if-changed", "not-always"},
			otherOptional:   []string{"skip-report", "optional"},
		},
	}

	for _, tc := range cases {
		if err := SetPresubmitRegexes(tc.config); err != nil {
			t.Fatalf("could not set regexes: %v", err)
		}
		presubmits := map[string][]Presubmit{
			"o/r": tc.config,
		}
		masterActual, masterActualIfPresent, masterOptional := BranchRequirements("master", presubmits["o/r"])
		if !reflect.DeepEqual(masterActual, tc.masterExpected) {
			t.Errorf("%s: identified incorrect required contexts on branch master: %s", tc.name, diff.ObjectReflectDiff(masterActual, tc.masterExpected))
		}
		if !reflect.DeepEqual(masterOptional, tc.masterOptional) {
			t.Errorf("%s: identified incorrect optional contexts on branch master: %s", tc.name, diff.ObjectReflectDiff(masterOptional, tc.masterOptional))
		}
		if !reflect.DeepEqual(masterActualIfPresent, tc.masterIfPresent) {
			t.Errorf("%s: identified incorrect if-present contexts on branch master: %s", tc.name, diff.ObjectReflectDiff(masterActualIfPresent, tc.masterIfPresent))
		}
		otherActual, otherActualIfPresent, otherOptional := BranchRequirements("other", presubmits["o/r"])
		if !reflect.DeepEqual(masterActual, tc.masterExpected) {
			t.Errorf("%s: identified incorrect required contexts on branch other: : %s", tc.name, diff.ObjectReflectDiff(otherActual, tc.otherExpected))
		}
		if !reflect.DeepEqual(otherOptional, tc.otherOptional) {
			t.Errorf("%s: identified incorrect optional contexts on branch other: %s", tc.name, diff.ObjectReflectDiff(otherOptional, tc.otherOptional))
		}
		if !reflect.DeepEqual(otherActualIfPresent, tc.otherIfPresent) {
			t.Errorf("%s: identified incorrect if-present contexts on branch other: %s", tc.name, diff.ObjectReflectDiff(otherActualIfPresent, tc.otherIfPresent))
		}
	}
}

func TestConfig_GetBranchProtection(t *testing.T) {
	testCases := []struct {
		name     string
		config   Config
		err      bool
		expected *Policy
	}{
		{
			name: "unprotected by default",
		},
		{
			name: "undefined org not protected",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: yes,
						},
						Orgs: map[string]Org{
							"unknown": {},
						},
					},
				},
			},
		},
		{
			name: "protect via config default",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: yes,
						},
						Orgs: map[string]Org{
							"org": {},
						},
					},
				},
			},
			expected: &Policy{Protect: yes},
		},
		{
			name: "protect via org default",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org": {
								Policy: Policy{
									Protect: yes,
								},
							},
						},
					},
				},
			},
			expected: &Policy{Protect: yes},
		},
		{
			name: "protect via repo default",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org": {
								Repos: map[string]Repo{
									"repo": {
										Policy: Policy{
											Protect: yes,
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &Policy{Protect: yes},
		},
		{
			name: "protect specific branch",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org": {
								Repos: map[string]Repo{
									"repo": {
										Branches: map[string]Branch{
											"branch": {
												Policy: Policy{
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
			},
			expected: &Policy{Protect: yes},
		},
		{
			name: "ignore other org settings",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: no,
						},
						Orgs: map[string]Org{
							"other": {
								Policy: Policy{Protect: yes},
							},
							"org": {},
						},
					},
				},
			},
			expected: &Policy{Protect: no},
		},
		{
			name: "defined branches must make a protection decision",
			config: Config{
				ProwConfig: ProwConfig{
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
			},
			err: true,
		},
		{
			name: "pushers require protection",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: no,
							Restrictions: &Restrictions{
								Teams: []string{"oncall"},
							},
						},
						Orgs: map[string]Org{
							"org": {},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "required contexts require protection",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: no,
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"test-foo"},
							},
						},
						Orgs: map[string]Org{
							"org": {},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "child policy with defined parent can disable protection",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						AllowDisabledPolicies: utilpointer.BoolPtr(true),
						Policy: Policy{
							Protect: yes,
							Restrictions: &Restrictions{
								Teams: []string{"oncall"},
							},
						},
						Orgs: map[string]Org{
							"org": {
								Policy: Policy{
									Protect: no,
								},
							},
						},
					},
				},
			},
			expected: &Policy{
				Protect: no,
				Restrictions: &Restrictions{
					Teams: []string{"oncall"},
				},
			},
		},
		{
			name: "Make required presubmits required",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: yes,
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"cla"},
							},
						},
						Orgs: map[string]Org{
							"org": {},
						},
					},
				},
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							{
								JobBase: JobBase{
									Name: "required presubmit",
								},
								Reporter: Reporter{
									Context: "required presubmit",
								},
								AlwaysRun: true,
							},
						},
					},
				},
			},
			expected: &Policy{
				Protect: yes,
				RequiredStatusChecks: &ContextPolicy{
					Contexts: []string{"required presubmit", "cla"},
				},
			},
		},
		{
			name: "ProtectTested opts into protection",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						ProtectTested: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org": {},
						},
					},
				},
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							{
								JobBase: JobBase{
									Name: "required presubmit",
								},
								Reporter: Reporter{
									Context: "required presubmit",
								},
								AlwaysRun: true,
							},
						},
					},
				},
			},
			expected: &Policy{
				Protect: yes,
				RequiredStatusChecks: &ContextPolicy{
					Contexts: []string{"required presubmit"},
				},
			},
		},
		{
			name: "required presubmits require protection",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: no,
						},
						Orgs: map[string]Org{
							"org": {},
						},
					},
				},
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							{
								JobBase: JobBase{
									Name: "required presubmit",
								},
								Reporter: Reporter{
									Context: "required presubmit",
								},
								AlwaysRun: true,
							},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "Optional presubmits do not force protection",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						ProtectTested: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org": {},
						},
					},
				},
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							{
								JobBase: JobBase{
									Name: "optional presubmit",
								},
								Reporter: Reporter{
									Context: "optional presubmit",
								},
								AlwaysRun: true,
								Optional:  true,
							},
						},
					},
				},
			},
		},
		{
			name: "Explicit configuration takes precedence over ProtectTested",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						ProtectTested: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org": {
								Policy: Policy{
									Protect: yes,
								},
							},
						},
					},
				},
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							{
								JobBase: JobBase{
									Name: "optional presubmit",
								},
								Reporter: Reporter{
									Context: "optional presubmit",
								},
								AlwaysRun: true,
								Optional:  true,
							},
						},
					},
				},
			},
			expected: &Policy{Protect: yes},
		},
		{
			name: "Explicit non-configuration takes precedence over ProtectTested",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						AllowDisabledJobPolicies: utilpointer.BoolPtr(true),
						ProtectTested:            utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org": {
								Repos: map[string]Repo{
									"repo": {
										Policy: Policy{
											Protect: no,
										},
									},
								},
							},
						},
					},
				},
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							{
								JobBase: JobBase{
									Name: "required presubmit",
								},
								Reporter: Reporter{
									Context: "required presubmit",
								},
								AlwaysRun: true,
							},
						},
					},
				},
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.config.GetBranchProtection("org", "repo", "branch", tc.config.PresubmitsStatic["org/repo"])
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case err == nil && tc.err:
				t.Errorf("failed to receive an error")
			default:
				normalize(actual)
				normalize(tc.expected)
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("actual %+v != expected %+v", actual, tc.expected)
				}
			}
		})
	}
}

func TestReposWithDisabledPolicy(t *testing.T) {
	testCases := []struct {
		name              string
		config            Config
		expectedRepoWarns []string
	}{
		{
			name: "Warning is generated for repos with disabled policies",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: no,
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"hello", "world"},
							},
						},
						AllowDisabledPolicies: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {},
									"repo2": {},
								},
							},
						},
					},
				},
			},
			expectedRepoWarns: []string{"org1/repo1", "org1/repo2"},
		},
		{
			name: "No warnings if disabled policies are not allowed",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: no,
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"hello", "world"},
							},
						},
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {},
								},
							},
						},
					},
				},
			},
			expectedRepoWarns: []string{},
		},
		{
			name: "No warnings if repo has no policies",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {},
								},
							},
						},
					},
				},
			},
			expectedRepoWarns: []string{},
		},
		{
			name: "No warnings if repo's defined policy is protected",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Policy: Policy{
											Protect: yes,
											RequiredStatusChecks: &ContextPolicy{
												Contexts: []string{"hello", "world"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedRepoWarns: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repoWarns := tc.config.reposWithDisabledPolicy()
			if !reflect.DeepEqual(repoWarns, tc.expectedRepoWarns) {
				t.Errorf("actual repo warnings %+v != expected %+v", repoWarns, tc.expectedRepoWarns)
			}
		})
	}
}

func TestUnprotectedBranches(t *testing.T) {
	testCases := []struct {
		name                string
		config              Config
		presubmits          map[string][]Presubmit
		expectedBranchWarns []string
	}{
		{
			name: "Repos with unprotected branches are added to the warning list",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"hello", "world"},
							},
						},
						AllowDisabledPolicies: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
												},
											},
										},
									},
									"repo2": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
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
			expectedBranchWarns: []string{"org1/repo1=branch1", "org1/repo2=branch1"},
		},
		{
			name: "Warn only once about repos with multiple unprotected branches",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"hello", "world"},
							},
						},
						AllowDisabledPolicies: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
												},
											},
											"branch2": {
												Policy{
													Protect: no,
												},
											},
										},
									},
									"repo2": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
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
			expectedBranchWarns: []string{"org1/repo1=branch1,branch2", "org1/repo2=branch1"},
		},
		{
			name: "No warnings if repo has no policies",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
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
			expectedBranchWarns: []string{},
		},
		{
			name: "No warnings if repo's defined policy is protected",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Policy: Policy{
											Protect: yes,
											RequiredStatusChecks: &ContextPolicy{
												Contexts: []string{"hello", "world"},
											},
										},
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
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
			expectedBranchWarns: []string{},
		},
		{
			name: "Warning if a branch has a required context but has protect: false",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						AllowDisabledJobPolicies: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
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
			presubmits: map[string][]Presubmit{
				"org1/repo1": {
					{
						JobBase: JobBase{
							Name: "always-run",
						},
						AlwaysRun: true,
					},
				},
			},
			expectedBranchWarns: []string{"org1/repo1=branch1"},
		},
		{
			name: "No warnings for a branch with no required context and protect: false",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						AllowDisabledJobPolicies: utilpointer.BoolPtr(true),
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
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
			presubmits: map[string][]Presubmit{
				"org1/repo1": {
					{
						JobBase: JobBase{
							Name: "optional",
						},
						Optional: true,
					},
				},
			},
			expectedBranchWarns: []string{},
		},
		{
			name: "No warnings if allow_disabled_job_policies is not set",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org1": {
								Repos: map[string]Repo{
									"repo1": {
										Branches: map[string]Branch{
											"branch1": {
												Policy{
													Protect: no,
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
			presubmits: map[string][]Presubmit{
				"org1/repo1": {
					{
						JobBase: JobBase{
							Name: "always-run",
						},
						AlwaysRun: true,
					},
				},
			},
			expectedBranchWarns: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			branchWarns := tc.config.unprotectedBranches(tc.presubmits)
			if !reflect.DeepEqual(branchWarns, tc.expectedBranchWarns) {
				t.Errorf("actual branch warnings %+v != expected %+v", branchWarns, tc.expectedBranchWarns)
			}
		})
	}
}

// TestBranchProtectionMergeMergesAnythingWithoutAnError verifies that merging any
// BranchProtection into an empty Branchprotection succeeds and that the resulting
// Branchprotection is equal to the one we merged from.
func TestBranchProtectionMergeMergesAnythingWithoutAnError(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i++ {
		fuzzedBP := &BranchProtection{}
		fuzz.New().Funcs(func(p *Policy, c fuzz.Continue) {
			// Make sure we always have a good sample of non-nil but empty Policies so
			// we check that they get copied over. Today, the meaning of an empty and
			// an unset Policy is identical because all the fields are pointers that
			// will get ignored if unset. However, this might change in the future and
			// caused flakes when we didn't copy over map entries with an empty Policy,
			// as an entry with no value and no entry are different things for cmp.Diff.
			if i%2 == 0 {
				c.Fuzz(p)
			}
		}).Fuzz(fuzzedBP)

		mergedBP := &BranchProtection{}
		if err := mergedBP.merge(fuzzedBP); err != nil {
			t.Fatalf("failed to merge branchprotection: %v", err)
		}

		if diff := cmp.Diff(mergedBP, fuzzedBP); diff != "" {
			t.Errorf("after merging the fuzzed BP, the merged and the fuzzed BP still differ: %s", diff)
		}
	}
}

// TestBranchprotectionMergeErrorsOnConflicts verifies that merging any non-empty BranchProtectionConfig
// into itself errors.
func TestBranchprotectionMergeErrorsOnConflicts(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i++ {
		fuzzedBP := &BranchProtection{}
		fuzz.New().Fuzz(fuzzedBP)

		if err := fuzzedBP.merge(fuzzedBP); err == nil && !apiequality.Semantic.DeepEqual(fuzzedBP, &BranchProtection{}) {
			serialized, serializeErr := yaml.Marshal(fuzzedBP)
			if serializeErr != nil {
				t.Fatalf("merging non-empty branchproteciton config into itself did not yield an error and serializing it afterwards failed: %v. Raw object: %+v", serializeErr, fuzzedBP)
			}
			t.Fatalf("merging non-empty branchproteciton config into itself did not yield an error; Serialized config:\n%s", string(serialized))
		}

	}
}

// TestBranchprotectionMergeRemainsUnchangedWhenMergingEmptyIn verifies that when merging
// any empty BranchProtectionConfig into a non-empty BranchProtectionConfig does not result
// in changes to the latter.
func TestBranchprotectionMergeRemainsUnchangedWhenMergingEmptyIn(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i++ {
		fuzzedBP := &BranchProtection{}
		fuzz.New().Fuzz(fuzzedBP)

		fuzzedBPDeepCopy := deepcopy.Copy(fuzzedBP)
		if err := fuzzedBP.merge(&BranchProtection{}); err != nil {
			t.Fatalf("failed to merge branchprotection: %v", err)
		}

		if diff := cmp.Diff(fuzzedBPDeepCopy, fuzzedBP); diff != "" {
			t.Errorf("after merging the the empty BP, the merged and the fuzzed BP differ: %s", diff)
		}
	}
}

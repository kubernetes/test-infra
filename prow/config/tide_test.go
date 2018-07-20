/*
Copyright 2017 The Kubernetes Authors.

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
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

var testQuery = TideQuery{
	Orgs:                   []string{"org"},
	Repos:                  []string{"k/k", "k/t-i"},
	Labels:                 []string{"lgtm", "approved"},
	MissingLabels:          []string{"foo"},
	Milestone:              "milestone",
	ReviewApprovedRequired: true,
}

func TestTideQuery(t *testing.T) {
	q := " " + testQuery.Query() + " "
	checkTok := func(tok string) {
		if !strings.Contains(q, " "+tok+" ") {
			t.Errorf("Expected query to contain \"%s\", got \"%s\"", tok, q)
		}
	}

	checkTok("is:pr")
	checkTok("state:open")
	checkTok("org:\"org\"")
	checkTok("repo:\"k/k\"")
	checkTok("repo:\"k/t-i\"")
	checkTok("label:\"lgtm\"")
	checkTok("label:\"approved\"")
	checkTok("-label:\"foo\"")
	checkTok("milestone:\"milestone\"")
	checkTok("review:approved")
}

func TestAllPRsSince(t *testing.T) {
	testTime, err := time.Parse(time.UnixDate, "Sat Mar  7 11:06:39 PST 2015")
	if err != nil {
		t.Fatalf("Error parsing test time string: %v.", err)
	}
	testTimeOld, err := time.Parse(time.UnixDate, "Sat Mar  7 11:06:39 PST 1915")
	if err != nil {
		t.Fatalf("Error parsing test time string: %v.", err)
	}
	var q string
	checkTok := func(tok string, shouldExist bool) {
		if shouldExist == strings.Contains(q, " "+tok+" ") {
			return
		} else if shouldExist {
			t.Errorf("Expected query to contain \"%s\", got \"%s\"", tok, q)
		} else {
			t.Errorf("Expected query to not contain \"%s\", got \"%s\"", tok, q)

		}
	}

	queries := TideQueries([]TideQuery{
		testQuery,
		{
			Orgs:   []string{"foo"},
			Repos:  []string{"k/foo"},
			Labels: []string{"lgtm", "mergeable"},
		},
	})
	q = " " + queries.AllPRsSince(testTime) + " "
	checkTok("is:pr", true)
	checkTok("state:open", true)
	checkTok("org:\"org\"", true)
	checkTok("org:\"foo\"", true)
	checkTok("repo:\"k/k\"", true)
	checkTok("repo:\"k/t-i\"", true)
	checkTok("repo:\"k/foo\"", true)
	checkTok("label:\"lgtm\"", false)
	checkTok("label:\"approved\"", false)
	checkTok("label:\"mergeable\"", false)
	checkTok("-label:\"foo\"", false)
	checkTok("milestone:\"milestone\"", false)
	checkTok("review:approved", false)
	checkTok("updated:>=2015-03-07T11:06:39Z", true)

	// Test that if time is the zero time value, the token is not included.
	q = " " + queries.AllPRsSince(time.Time{}) + " "
	checkTok("updated:>=0001-01-01T00:00:00Z", false)
	// Test that if time is before 1970, the token is not included.
	q = " " + queries.AllPRsSince(testTimeOld) + " "
	checkTok("updated:>=1915-03-07T11:06:39Z", false)
}

func TestMergeMethod(t *testing.T) {
	ti := &Tide{
		MergeType: map[string]github.PullRequestMergeType{
			"kubernetes/kops":             github.MergeRebase,
			"kubernetes/charts":           github.MergeSquash,
			"helm/charts":                 github.MergeSquash,
			"kubernetes-helm":             github.MergeSquash,
			"kubernetes-helm/chartmuseum": github.MergeMerge,
		},
	}

	var testcases = []struct {
		org      string
		repo     string
		expected github.PullRequestMergeType
	}{
		{
			"kubernetes",
			"kubernetes",
			github.MergeMerge,
		},
		{
			"kubernetes",
			"kops",
			github.MergeRebase,
		},
		{
			"kubernetes",
			"charts",
			github.MergeSquash,
		},
		{
			"kubernetes-helm",
			"monocular",
			github.MergeSquash,
		},
		{
			"kubernetes-helm",
			"chartmuseum",
			github.MergeMerge,
		},
	}

	for _, test := range testcases {
		if ti.MergeMethod(test.org, test.repo) != test.expected {
			t.Errorf("Expected merge method %q but got %q for %s/%s", test.expected, ti.MergeMethod(test.org, test.repo), test.org, test.repo)
		}
	}
}

func TestParseTideContextPolicyOptions(t *testing.T) {
	yes := true
	no := false
	org, repo, branch := "org", "repo", "branch"
	testCases := []struct {
		name     string
		config   TideContextPolicyOptions
		expected TideContextPolicy
	}{
		{
			name: "empty",
		},
		{
			name: "global config",
			config: TideContextPolicyOptions{
				TideContextPolicy: TideContextPolicy{
					FromBranchProtection: &yes,
					SkipUnknownContexts:  &yes,
					RequiredContexts:     []string{"r1"},
					OptionalContexts:     []string{"o1"},
				},
			},
			expected: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
				FromBranchProtection: &yes,
			},
		},
		{
			name: "org config",
			config: TideContextPolicyOptions{
				TideContextPolicy: TideContextPolicy{
					RequiredContexts:     []string{"r1"},
					OptionalContexts:     []string{"o1"},
					FromBranchProtection: &no,
				},
				Orgs: map[string]TideOrgContextPolicy{
					"org": {
						TideContextPolicy: TideContextPolicy{
							SkipUnknownContexts:  &yes,
							RequiredContexts:     []string{"r2"},
							OptionalContexts:     []string{"o2"},
							FromBranchProtection: &yes,
						},
					},
				},
			},
			expected: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				RequiredContexts:     []string{"r1", "r2"},
				OptionalContexts:     []string{"o1", "o2"},
				FromBranchProtection: &yes,
			},
		},
		{
			name: "repo config",
			config: TideContextPolicyOptions{
				TideContextPolicy: TideContextPolicy{
					RequiredContexts:     []string{"r1"},
					OptionalContexts:     []string{"o1"},
					FromBranchProtection: &no,
				},
				Orgs: map[string]TideOrgContextPolicy{
					"org": {
						TideContextPolicy: TideContextPolicy{
							SkipUnknownContexts:  &no,
							RequiredContexts:     []string{"r2"},
							OptionalContexts:     []string{"o2"},
							FromBranchProtection: &no,
						},
						Repos: map[string]TideRepoContextPolicy{
							"repo": {
								TideContextPolicy: TideContextPolicy{
									SkipUnknownContexts:  &yes,
									RequiredContexts:     []string{"r3"},
									OptionalContexts:     []string{"o3"},
									FromBranchProtection: &yes,
								},
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				RequiredContexts:     []string{"r1", "r2", "r3"},
				OptionalContexts:     []string{"o1", "o2", "o3"},
				FromBranchProtection: &yes,
			},
		},
		{
			name: "branch config",
			config: TideContextPolicyOptions{
				TideContextPolicy: TideContextPolicy{
					RequiredContexts: []string{"r1"},
					OptionalContexts: []string{"o1"},
				},
				Orgs: map[string]TideOrgContextPolicy{
					"org": {
						TideContextPolicy: TideContextPolicy{
							RequiredContexts: []string{"r2"},
							OptionalContexts: []string{"o2"},
						},
						Repos: map[string]TideRepoContextPolicy{
							"repo": {
								TideContextPolicy: TideContextPolicy{
									RequiredContexts: []string{"r3"},
									OptionalContexts: []string{"o3"},
								},
								Branches: map[string]TideContextPolicy{
									"branch": {
										RequiredContexts: []string{"r4"},
										OptionalContexts: []string{"o4"},
									},
								},
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts: []string{"r1", "r2", "r3", "r4"},
				OptionalContexts: []string{"o1", "o2", "o3", "o4"},
			},
		},
	}
	for _, tc := range testCases {
		policy := parseTideContextPolicyOptions(org, repo, branch, tc.config)
		if !reflect.DeepEqual(policy, tc.expected) {
			t.Errorf("%s - expected %v got %v", tc.name, tc.expected, policy)
		}
	}
}

func TestConfigGetTideContextPolicy(t *testing.T) {
	yes := true
	no := false
	org, repo, branch := "org", "repo", "branch"
	testCases := []struct {
		name     string
		config   Config
		expected TideContextPolicy
		error    string
	}{
		{
			name: "no policy - use prow jobs",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: &yes,
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"r1", "r2"},
							},
						},
					},
				},
				JobConfig: JobConfig{
					Presubmits: map[string][]Presubmit{
						"org/repo": {
							Presubmit{
								Context:   "pr1",
								AlwaysRun: true,
							},
							Presubmit{
								Context:   "po1",
								AlwaysRun: true,
								Optional:  true,
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts: []string{"pr1"},
				OptionalContexts: []string{"po1"},
			},
		},
		{
			name: "no policy no prow jobs defined - empty",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Policy: Policy{
							Protect: &yes,
							RequiredStatusChecks: &ContextPolicy{
								Contexts: []string{"r1", "r2"},
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts: []string{},
				OptionalContexts: []string{},
			},
		},
		{
			name: "no branch protection",
			config: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						ContextOptions: TideContextPolicyOptions{
							TideContextPolicy: TideContextPolicy{
								FromBranchProtection: &yes,
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts: []string{},
				OptionalContexts: []string{},
			},
		},
		{
			name: "invalid branch protection",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org": {
								Policy: Policy{
									Protect: &no,
								},
							},
						},
					},
					Tide: Tide{
						ContextOptions: TideContextPolicyOptions{
							TideContextPolicy: TideContextPolicy{
								FromBranchProtection: &yes,
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts: []string{},
				OptionalContexts: []string{},
			},
		},
		{
			name: "manually defined policy",
			config: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						ContextOptions: TideContextPolicyOptions{
							TideContextPolicy: TideContextPolicy{
								RequiredContexts:    []string{"r1"},
								OptionalContexts:    []string{"o1"},
								SkipUnknownContexts: &yes,
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:    []string{"r1"},
				OptionalContexts:    []string{"o1"},
				SkipUnknownContexts: &yes,
			},
		},
	}

	for _, tc := range testCases {
		p, err := tc.config.GetTideContextPolicy(org, repo, branch)
		if !reflect.DeepEqual(p, &tc.expected) {
			t.Errorf("%s - expected contexts %v got %v", tc.name, &tc.expected, p)
		}
		if err != nil {
			if err.Error() != tc.error {
				t.Errorf("%s - expected error %v got %v", tc.name, tc.error, err.Error())
			}
		} else if tc.error != "" {
			t.Errorf("%s - expected error %v got nil", tc.name, tc.error)
		}
	}
}

func TestMergeTideContextPolicyConfig(t *testing.T) {
	yes := true
	no := false
	testCases := []struct {
		name    string
		a, b, c TideContextPolicy
	}{
		{
			name: "all empty",
		},
		{
			name: "empty a",
			b: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				FromBranchProtection: &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
			c: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				FromBranchProtection: &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
		},
		{
			name: "empty b",
			a: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				FromBranchProtection: &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
			c: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				FromBranchProtection: &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
		},
		{
			name: "merging unset boolean",
			a: TideContextPolicy{
				FromBranchProtection: &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
			b: TideContextPolicy{
				SkipUnknownContexts: &yes,
				RequiredContexts:    []string{"r2"},
				OptionalContexts:    []string{"o2"},
			},
			c: TideContextPolicy{
				SkipUnknownContexts:  &yes,
				FromBranchProtection: &no,
				RequiredContexts:     []string{"r1", "r2"},
				OptionalContexts:     []string{"o1", "o2"},
			},
		},
		{
			name: "merging unset contexts in a",
			a: TideContextPolicy{
				FromBranchProtection: &no,
				SkipUnknownContexts:  &yes,
			},
			b: TideContextPolicy{
				FromBranchProtection: &yes,
				SkipUnknownContexts:  &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
			c: TideContextPolicy{
				FromBranchProtection: &yes,
				SkipUnknownContexts:  &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
		},
		{
			name: "merging unset contexts in b",
			a: TideContextPolicy{
				FromBranchProtection: &yes,
				SkipUnknownContexts:  &no,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
			b: TideContextPolicy{
				FromBranchProtection: &no,
				SkipUnknownContexts:  &yes,
			},
			c: TideContextPolicy{
				FromBranchProtection: &no,
				SkipUnknownContexts:  &yes,
				RequiredContexts:     []string{"r1"},
				OptionalContexts:     []string{"o1"},
			},
		},
	}

	for _, tc := range testCases {
		c := mergeTideContextPolicy(tc.a, tc.b)
		if !reflect.DeepEqual(c, tc.c) {
			t.Errorf("%s - expected %v got %v", tc.name, tc.c, c)
		}
	}
}

func TestTideQuery_Validate(t *testing.T) {
	testCases := []struct {
		name   string
		t      TideContextPolicy
		failed bool
	}{
		{
			name: "good policy",
			t: TideContextPolicy{
				OptionalContexts: []string{"o1"},
				RequiredContexts: []string{"r1"},
			},
		},
		{
			name: "good policy",
			t: TideContextPolicy{
				OptionalContexts: []string{"c1"},
				RequiredContexts: []string{"c1"},
			},
			failed: true,
		},
		{
			name: "good policy",
			t: TideContextPolicy{
				OptionalContexts: []string{"c1", "c2", "c3", "c4"},
				RequiredContexts: []string{"c1", "c4"},
			},
			failed: true,
		},
	}
	for _, tc := range testCases {
		err := tc.t.Validate()
		failed := err != nil
		if failed != tc.failed {
			t.Errorf("%s - expected %v got %v", tc.name, tc.failed, err)
		}
	}
}

func TestTideContextPolicy_IsOptional(t *testing.T) {
	testCases := []struct {
		name                string
		skipUnknownContexts bool
		required, optional  []string
		contexts            []string
		results             []bool
	}{
		{
			name:     "only optional contexts registered - skipUnknownContexts false",
			contexts: []string{"c1", "o1", "o2"},
			optional: []string{"o1", "o2"},
			results:  []bool{false, true, true},
		},
		{
			name:     "no contexts registered - skipUnknownContexts false",
			contexts: []string{"t2"},
			results:  []bool{false},
		},
		{
			name:     "only required contexts registered - skipUnknownContexts false",
			required: []string{"c1", "c2", "c3"},
			contexts: []string{"c1", "c2", "c3", "t1"},
			results:  []bool{false, false, false, false},
		},
		{
			name:     "optional and required contexts registered - skipUnknownContexts false",
			optional: []string{"o1", "o2"},
			required: []string{"c1", "c2", "c3"},
			contexts: []string{"o1", "o2", "c1", "c2", "c3", "t1"},
			results:  []bool{true, true, false, false, false, false},
		},
		{
			name:                "only optional contexts registered - skipUnknownContexts true",
			contexts:            []string{"c1", "o1", "o2"},
			optional:            []string{"o1", "o2"},
			skipUnknownContexts: true,
			results:             []bool{true, true, true},
		},
		{
			name:                "no contexts registered - skipUnknownContexts true",
			contexts:            []string{"t2"},
			skipUnknownContexts: true,
			results:             []bool{true},
		},
		{
			name:                "only required contexts registered - skipUnknownContexts true",
			required:            []string{"c1", "c2", "c3"},
			contexts:            []string{"c1", "c2", "c3", "t1"},
			skipUnknownContexts: true,
			results:             []bool{false, false, false, true},
		},
		{
			name:                "optional and required contexts registered - skipUnknownContexts true",
			optional:            []string{"o1", "o2"},
			required:            []string{"c1", "c2", "c3"},
			contexts:            []string{"o1", "o2", "c1", "c2", "c3", "t1"},
			skipUnknownContexts: true,
			results:             []bool{true, true, false, false, false, true},
		},
	}

	for _, tc := range testCases {
		cp := TideContextPolicy{
			SkipUnknownContexts: &tc.skipUnknownContexts,
			RequiredContexts:    tc.required,
			OptionalContexts:    tc.optional,
		}
		for i, c := range tc.contexts {
			if cp.IsOptional(c) != tc.results[i] {
				t.Errorf("%s - IsOptional for %s should return %t", tc.name, c, tc.results[i])
			}
		}
	}
}

func TestTideContextPolicy_MissingRequiredContexts(t *testing.T) {
	testCases := []struct {
		name                               string
		skipUnknownContexts                bool
		required, optional                 []string
		existingContexts, expectedContexts []string
	}{
		{
			name:             "no contexts registered",
			existingContexts: []string{"c1", "c2"},
		},
		{
			name:             "optional contexts registered / no missing contexts",
			optional:         []string{"o1", "o2", "o3"},
			existingContexts: []string{"c1", "c2"},
		},
		{
			name:             "required  contexts registered / missing contexts",
			required:         []string{"c1", "c2", "c3"},
			existingContexts: []string{"c1", "c2"},
			expectedContexts: []string{"c3"},
		},
		{
			name:             "required contexts registered / no missing contexts",
			required:         []string{"c1", "c2", "c3"},
			existingContexts: []string{"c1", "c2", "c3"},
		},
		{
			name:             "optional and required contexts registered / missing contexts",
			optional:         []string{"o1", "o2", "o3"},
			required:         []string{"c1", "c2", "c3"},
			existingContexts: []string{"c1", "c2"},
			expectedContexts: []string{"c3"},
		},
		{
			name:             "optional and required contexts registered / no missing contexts",
			optional:         []string{"o1", "o2", "o3"},
			required:         []string{"c1", "c2"},
			existingContexts: []string{"c1", "c2", "c4"},
		},
	}

	for _, tc := range testCases {
		cp := TideContextPolicy{
			SkipUnknownContexts: &tc.skipUnknownContexts,
			RequiredContexts:    tc.required,
			OptionalContexts:    tc.optional,
		}
		missingContexts := cp.MissingRequiredContexts(tc.existingContexts)
		if !sets.NewString(missingContexts...).Equal(sets.NewString(tc.expectedContexts...)) {
			t.Errorf("%s - expected %v got %v", tc.name, tc.expectedContexts, missingContexts)
		}
	}
}

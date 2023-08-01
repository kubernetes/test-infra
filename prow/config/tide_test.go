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
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/labels"
)

var testQuery = TideQuery{
	Orgs:                   []string{"org"},
	Repos:                  []string{"k/k", "k/t-i"},
	ExcludedRepos:          []string{"org/repo"},
	Labels:                 []string{labels.LGTM, labels.Approved},
	MissingLabels:          []string{"foo"},
	Author:                 "batman",
	Milestone:              "milestone",
	ReviewApprovedRequired: true,
}

var expectedQueryComponents = []string{
	"is:pr",
	"state:open",
	"archived:false",
	"label:\"lgtm\"",
	"label:\"approved\"",
	"-label:\"foo\"",
	"author:\"batman\"",
	"milestone:\"milestone\"",
	"review:approved",
}

func TestMarshalMergeMethod(t *testing.T) {
	testCases := []struct {
		testName   string
		tideConfig TideGitHubConfig
		wantYaml   string
	}{
		{
			testName: "Org-wide",
			tideConfig: TideGitHubConfig{
				MergeType: map[string]TideOrgMergeType{
					"org1": {
						MergeType: "squash",
					},
				},
			},
			wantYaml: `context_options: {}
merge_method:
  org1: squash
`,
		},
		{
			testName: "Empty org",
			tideConfig: TideGitHubConfig{
				MergeType: map[string]TideOrgMergeType{
					"org1": {},
				},
			},
			wantYaml: `context_options: {}
merge_method:
  org1: ""
`,
		},
		{
			testName: "Repo-wide",
			tideConfig: TideGitHubConfig{
				MergeType: map[string]TideOrgMergeType{
					"org1": {
						Repos: map[string]TideRepoMergeType{
							"repo1": {
								MergeType: "rebase",
							},
						},
					},
				},
			},
			wantYaml: `context_options: {}
merge_method:
  org1:
    repo1: rebase
`,
		},
		{
			testName: "Empty repo",
			tideConfig: TideGitHubConfig{
				MergeType: map[string]TideOrgMergeType{
					"org1": {
						Repos: map[string]TideRepoMergeType{
							"repo1": {},
						},
					},
				},
			},
			wantYaml: `context_options: {}
merge_method:
  org1:
    repo1: ""
`,
		},
		{
			testName: "Multiple branches",
			tideConfig: TideGitHubConfig{
				MergeType: map[string]TideOrgMergeType{
					"org1": {
						Repos: map[string]TideRepoMergeType{
							"repo1": {
								Branches: map[string]TideBranchMergeType{
									"branch1": {
										Regexpr:   regexp.MustCompile("branch1"),
										MergeType: types.MergeMerge,
									},
									"branch2": {
										Regexpr:   regexp.MustCompile("branch2"),
										MergeType: types.MergeSquash,
									},
								},
							},
						},
					},
				},
			},
			wantYaml: `context_options: {}
merge_method:
  org1:
    repo1:
      branch1: merge
      branch2: squash
`,
		},
		{
			testName: "Complex",
			tideConfig: TideGitHubConfig{
				MergeType: map[string]TideOrgMergeType{
					"org1": {
						Repos: map[string]TideRepoMergeType{
							"repo1": {
								Branches: map[string]TideBranchMergeType{
									"branch1": {
										Regexpr:   regexp.MustCompile("branch1"),
										MergeType: types.MergeMerge,
									},
									"branch2": {
										Regexpr:   regexp.MustCompile("branch2"),
										MergeType: types.MergeSquash,
									},
								},
							},
						},
					},
					"org2/repo1@master": {
						MergeType: "squash",
					},
					"org2": {
						Repos: map[string]TideRepoMergeType{
							"repo2": {
								MergeType: "merge",
							},
						},
					},
					"org3": {
						MergeType: "rebase",
					},
				},
			},
			wantYaml: `context_options: {}
merge_method:
  org1:
    repo1:
      branch1: merge
      branch2: squash
  org2:
    repo2: merge
  org2/repo1@master: squash
  org3: rebase
`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			yaml, err := yaml.Marshal(testCase.tideConfig)
			if err != nil {
				t.Errorf("unmarshal error: %v", err)
			}
			if diff := cmp.Diff(testCase.wantYaml, string(yaml)); diff != "" {
				t.Errorf("unexpected yaml: %s", diff)
			}
		})
	}
}

func TestUnmarshalMergeMethod(t *testing.T) {
	testCases := []struct {
		testName   string
		yamlConfig string
		wantConfig Config
	}{
		{
			testName:   "No merge config",
			yamlConfig: `tide:`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{},
				},
			},
		},
		{
			testName: "Mix OrgRepo and branches config",
			yamlConfig: `
tide:
  merge_method:
    org1:
      repo1:
        branch1: merge
    org2/repo1: rebase`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								"org1": {
									Repos: map[string]TideRepoMergeType{
										"repo1": {
											Branches: map[string]TideBranchMergeType{
												"branch1": {
													MergeType: types.MergeMerge,
												},
											},
										},
									},
								},
								"org2/repo1": {
									MergeType: types.MergeRebase,
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "The same repo-wide and per branch config",
			yamlConfig: `
tide:
  merge_method:
    org1:
      repo1:
        branch1: merge
    org1/repo1: rebase`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								"org1": {
									Repos: map[string]TideRepoMergeType{
										"repo1": {
											Branches: map[string]TideBranchMergeType{
												"branch1": {
													MergeType: types.MergeMerge,
												},
											},
										},
									},
								},
								"org1/repo1": {
									MergeType: types.MergeRebase,
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "Legacy repo only config",
			yamlConfig: `
tide:
  merge_method:
    org1/repo1: merge`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								"org1/repo1": {
									MergeType: types.MergeMerge,
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "Repo only config",
			yamlConfig: `
tide:
  merge_method:
    org1:
      repo1: merge`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								"org1": {
									Repos: map[string]TideRepoMergeType{
										"repo1": {
											MergeType: types.MergeMerge,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "Org only config",
			yamlConfig: `
tide:
  merge_method:
    org1: rebase`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								"org1": {
									MergeType: types.MergeRebase,
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "Branches only config",
			yamlConfig: `
tide:
  merge_method:
    org1:
      repo1:
        branch1: merge`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								"org1": {
									Repos: map[string]TideRepoMergeType{
										"repo1": {
											Branches: map[string]TideBranchMergeType{
												"branch1": {
													MergeType: types.MergeMerge,
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
		},
		{
			testName: "Branches config: wildcard on branch and repo",
			yamlConfig: `
tide:
  merge_method:
    org1:
      ".*":
        ".+": merge`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								"org1": {
									Repos: map[string]TideRepoMergeType{
										".*": {
											Branches: map[string]TideBranchMergeType{
												".+": {
													MergeType: types.MergeMerge,
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
		},
		{
			testName: "Branches config: no repos at all",
			yamlConfig: `
tide:
  merge_method:
    ".*":`,
			wantConfig: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							MergeType: map[string]TideOrgMergeType{
								".*": {},
							},
						},
					},
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			var config Config
			if err := yaml.Unmarshal([]byte(testCase.yamlConfig), &config); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if diff := cmp.Diff(&testCase.wantConfig, &config); diff != "" {
				t.Errorf("merge method configurations differ: %s", diff)
			}
		})
	}
}

func TestTideQuery(t *testing.T) {
	q := " " + testQuery.Query() + " "
	checkTok := checkTok(t, q)

	checkTok("org:\"org\"")
	checkTok("repo:\"k/k\"")
	checkTok("repo:\"k/t-i\"")
	checkTok("-repo:\"org/repo\"")
	for _, expectedComponent := range expectedQueryComponents {
		checkTok(expectedComponent)
	}

	elements := strings.Fields(q)
	alreadySeen := sets.Set[string]{}
	for _, element := range elements {
		if alreadySeen.Has(element) {
			t.Errorf("element %q was multiple times in the query string", element)
		}
		alreadySeen.Insert(element)
	}
}

func checkTok(t *testing.T, q string) func(tok string) {
	return func(tok string) {
		t.Run("Query string contains "+tok, func(t *testing.T) {
			if !strings.Contains(q, " "+tok+" ") {
				t.Errorf("Expected query to contain \"%s\", got \"%s\"", tok, q)
			}
		})
	}
}

func TestOrgQueries(t *testing.T) {
	queries := testQuery.OrgQueries()
	if n := len(queries); n != 2 {
		t.Errorf("expected exactly two queries, got %d", n)
	}
	if queries["org"] == "" {
		t.Error("no query for org org found")
	}
	if queries["k"] == "" {
		t.Error("no query for org k found")
	}

	for org, query := range queries {
		t.Run(org, func(t *testing.T) {
			checkTok := checkTok(t, " "+query+" ")
			t.Logf("query: %s", query)

			for _, expectedComponent := range expectedQueryComponents {
				checkTok(expectedComponent)
			}

			elements := strings.Fields(query)
			alreadySeen := sets.Set[string]{}
			for _, element := range elements {
				if alreadySeen.Has(element) {
					t.Errorf("element %q was multiple times in the query string", element)
				}
				alreadySeen.Insert(element)
			}

			if org == "org" {
				checkTok(`org:"org"`)
				checkTok(`-repo:"org/repo"`)
			}

			if org == "k" {
				for _, repo := range testQuery.Repos {
					checkTok(fmt.Sprintf(`repo:"%s"`, repo))
				}
			}
		})
	}
}

func TestOrgExceptionsAndRepos(t *testing.T) {
	queries := TideQueries{
		{
			Orgs:          []string{"k8s"},
			ExcludedRepos: []string{"k8s/k8s"},
		},
		{
			Orgs:          []string{"kuber"},
			Repos:         []string{"foo/bar", "baz/bar"},
			ExcludedRepos: []string{"kuber/netes"},
		},
		{
			Orgs:          []string{"k8s"},
			ExcludedRepos: []string{"k8s/k8s", "k8s/t-i"},
		},
		{
			Orgs:          []string{"org", "org2"},
			ExcludedRepos: []string{"org2/repo", "org2/repo2", "org2/repo3"},
		},
		{
			Orgs:  []string{"foo"},
			Repos: []string{"org2/repo3"},
		},
	}

	expectedOrgs := map[string]sets.Set[string]{
		"foo":   sets.New[string](),
		"k8s":   sets.New[string]("k8s/k8s"),
		"kuber": sets.New[string]("kuber/netes"),
		"org":   sets.New[string](),
		"org2":  sets.New[string]("org2/repo", "org2/repo2"),
	}
	expectedRepos := sets.New[string]("foo/bar", "baz/bar", "org2/repo3")

	orgs, repos := queries.OrgExceptionsAndRepos()
	if !reflect.DeepEqual(orgs, expectedOrgs) {
		t.Errorf("Expected org map %v, but got %v.", expectedOrgs, orgs)
	}
	if !repos.Equal(expectedRepos) {
		t.Errorf("Expected repo set %v, but got %v.", expectedRepos, repos)
	}
}

func TestMergeMethod(t *testing.T) {
	ti := &Tide{
		TideGitHubConfig: TideGitHubConfig{
			MergeType: map[string]TideOrgMergeType{
				"kubernetes/kops":             {MergeType: types.MergeRebase},
				"kubernetes-helm":             {MergeType: types.MergeSquash},
				"kubernetes-helm/chartmuseum": {MergeType: types.MergeMerge},
			},
		},
	}

	var testcases = []struct {
		org      string
		repo     string
		expected types.PullRequestMergeType
	}{
		{
			"kubernetes",
			"kubernetes",
			types.MergeMerge,
		},
		{
			"kubernetes",
			"kops",
			types.MergeRebase,
		},
		{
			"kubernetes-helm",
			"monocular",
			types.MergeSquash,
		},
		{
			"kubernetes-helm",
			"chartmuseum",
			types.MergeMerge,
		},
	}

	for _, test := range testcases {
		actual := ti.MergeMethod(OrgRepo{Org: test.org, Repo: test.repo})
		if actual != test.expected {
			t.Errorf("Expected merge method %q but got %q for %s/%s", test.expected, actual, test.org, test.repo)
		}
	}
}

func TestOrgRepoMatchMergeMethod(t *testing.T) {
	var testCases = []struct {
		name     string
		config   Tide
		org      string
		repo     string
		branch   string
		expected types.PullRequestMergeType
	}{
		// Edge cases
		{
			name: "No input at all",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"test-infra": {
									Branches: map[string]TideBranchMergeType{
										"master": {
											Regexpr:   regexp.MustCompile("master"),
											MergeType: types.MergeRebase,
										},
									},
								},
							},
						},
					},
				},
			},
			expected: types.MergeMerge,
		},
		{
			name:     "Empty tide config",
			config:   Tide{},
			org:      "kubernetes",
			repo:     "test-infra",
			branch:   "master",
			expected: types.MergeMerge,
		},
		{
			name:     "Empty tide config and no input",
			config:   Tide{},
			expected: types.MergeMerge,
		},
		// Shorthands
		{
			name: "org shorthand: match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes-helm": {MergeType: types.MergeSquash},
					},
				},
			},
			org:      "kubernetes-helm",
			expected: types.MergeSquash,
		},
		{
			name: "org shorthand: no match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes-helm": {MergeType: types.MergeSquash},
					},
				},
			},
			org:      "kubernetes",
			expected: types.MergeMerge,
		},
		{
			name: "org shorthand: no org provided",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {MergeType: types.MergeRebase},
					},
				},
			},
			expected: types.MergeMerge,
		},
		{
			name: "org shorthand: neither repo nor branch matches",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {MergeType: types.MergeRebase},
					},
				},
			},
			org:      "kubernetes",
			repo:     "test-infra",
			branch:   "dev",
			expected: types.MergeRebase,
		},
		{
			name: "org/repo shorthand: match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes-helm/chartmuseum": {MergeType: types.MergeSquash},
					},
				},
			},
			org:      "kubernetes-helm",
			repo:     "chartmuseum",
			expected: types.MergeSquash,
		},
		{
			name: "org/repo shorthand: no match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes-helm/chartmuseum": {MergeType: types.MergeSquash},
					},
				},
			},
			org:      "kubernetes-helm",
			repo:     "test-infra",
			expected: types.MergeMerge,
		},
		{
			name: "org/repo shorthand: no repo provided",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes-helm/chartmuseum": {MergeType: types.MergeSquash},
					},
				},
			},
			org:      "kubernetes-helm",
			expected: types.MergeMerge,
		},
		{
			name: "org/repo shorthand: org only match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes-helm": {MergeType: types.MergeSquash},
					},
				},
			},
			org:      "kubernetes-helm",
			repo:     "chartmuseum",
			expected: types.MergeSquash,
		},
		{
			name: "org/repo shorthand: fallback to org/repo when branch doesn't match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes-helm/chartmuseum": {MergeType: types.MergeSquash},
					},
				},
			},
			org:      "kubernetes-helm",
			repo:     "chartmuseum",
			branch:   "master",
			expected: types.MergeSquash,
		},
		{
			name: "org/repo@branch shorthand: match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes/kops@main": {MergeType: types.MergeRebase},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kops",
			branch:   "main",
			expected: types.MergeRebase,
		},
		{
			name: "org/repo@branch shorthand: no match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes/kops@main": {MergeType: types.MergeRebase},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kops",
			branch:   "master",
			expected: types.MergeMerge,
		},
		{
			name: "org/repo@branch shorthand: no branch provided",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes/kops@main": {MergeType: types.MergeRebase},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kops",
			expected: types.MergeMerge,
		},
		// Repo-wide config
		{
			name: "Repo-wide config: match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"kubernetes": {MergeType: types.MergeIfNecessary},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kubernetes",
			expected: types.MergeIfNecessary,
		},
		{
			name: "Repo-wide config: no match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"kubernetes": {MergeType: types.MergeIfNecessary},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "test-infra",
			expected: types.MergeMerge,
		},
		{
			name: "Repo-wide config: match using '*'",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"*":          {MergeType: types.MergeIfNecessary},
								"kubernetes": {MergeType: types.MergeSquash},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "test-infra",
			expected: types.MergeIfNecessary,
		},
		// Branch level config
		{
			name: "Branch level config: no match",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"test-infra": {
									Branches: map[string]TideBranchMergeType{
										"master": {
											Regexpr:   regexp.MustCompile("master"),
											MergeType: types.MergeRebase,
										},
									},
								},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "test-infra",
			branch:   "main",
			expected: types.MergeMerge,
		},
		{
			name: "Branch level config: match no regex",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"test-infra": {
									Branches: map[string]TideBranchMergeType{
										"master": {
											Regexpr:   regexp.MustCompile("master"),
											MergeType: types.MergeRebase,
										},
									},
								},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "test-infra",
			branch:   "master",
			expected: types.MergeRebase,
		},
		{
			name: "Branch level config: match regex",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"test-infra": {
									Branches: map[string]TideBranchMergeType{
										`release-\d+(.\d+)?`: {
											Regexpr:   regexp.MustCompile(`release-\d+(.\d+)?`),
											MergeType: types.MergeSquash,
										},
									},
								},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "test-infra",
			branch:   "release-0.2",
			expected: types.MergeSquash,
		},
		{
			name: "Branch level config: multiple regex matches, pick the first one",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"test-infra": {
									Branches: map[string]TideBranchMergeType{
										`ma.*`: {
											Regexpr:   regexp.MustCompile(`ma.*`),
											MergeType: types.MergeSquash,
										},
										`mast.*`: {
											Regexpr:   regexp.MustCompile(`ma.*`),
											MergeType: types.MergeIfNecessary,
										},
									},
								},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "test-infra",
			branch:   "master",
			expected: types.MergeSquash,
		},
		{
			name: "Branch level config: match '*' wildcard at repository level",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"golang": {
							Repos: map[string]TideRepoMergeType{
								"*": {
									Branches: map[string]TideBranchMergeType{
										"main": {
											Regexpr:   regexp.MustCompile("main"),
											MergeType: types.MergeIfNecessary,
										},
									},
								},
							},
						},
					},
				},
			},
			org:      "golang",
			repo:     "docs",
			branch:   "main",
			expected: types.MergeIfNecessary,
		},
		// Precedences
		{
			name: "Precedence: org/repo@branch shorthand over branch level config",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes/kops@main": {MergeType: types.MergeSquash},
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"kops": {
									Branches: map[string]TideBranchMergeType{
										"main": {
											Regexpr:   regexp.MustCompile("main"),
											MergeType: types.MergeIfNecessary,
										},
									},
								},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kops",
			branch:   "main",
			expected: types.MergeSquash,
		},
		{
			name: "Precedence: branch level config over org/repo shorthand",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes/kops": {MergeType: types.MergeSquash},
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"kops": {
									Branches: map[string]TideBranchMergeType{
										"main": {
											Regexpr:   regexp.MustCompile("main"),
											MergeType: types.MergeIfNecessary,
										},
									},
								},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kops",
			branch:   "main",
			expected: types.MergeIfNecessary,
		},
		{
			name: "Precedence: org/repo shorthand over repo-wide config",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes/kops": {MergeType: types.MergeSquash},
						"kubernetes": {
							Repos: map[string]TideRepoMergeType{
								"kops": {MergeType: types.MergeRebase},
							},
						},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kops",
			expected: types.MergeSquash,
		},
		{
			name: "Precedence: org/repo shorthand over org config",
			config: Tide{
				TideGitHubConfig: TideGitHubConfig{
					MergeType: map[string]TideOrgMergeType{
						"kubernetes/kops": {MergeType: types.MergeSquash},
						"kubernetes":      {MergeType: types.MergeIfNecessary},
					},
				},
			},
			org:      "kubernetes",
			repo:     "kops",
			expected: types.MergeSquash,
		},
	}
	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			actual := test.config.OrgRepoBranchMergeMethod(OrgRepo{Org: test.org, Repo: test.repo}, test.branch)
			if actual != test.expected {
				t.Errorf("Expected merge method %q but got %q for org: %q, repo: %q, branch: %q",
					test.expected, actual, test.org, test.repo, test.branch)
			}
		})
	}
}
func TestMergeTemplate(t *testing.T) {
	ti := &Tide{
		TideGitHubConfig: TideGitHubConfig{
			MergeTemplate: map[string]TideMergeCommitTemplate{
				"kubernetes/kops": {
					TitleTemplate: "",
					BodyTemplate:  "",
				},
				"kubernetes-helm": {
					TitleTemplate: "{{ .Title }}",
					BodyTemplate:  "{{ .Body }}",
				},
			},
		},
	}

	var testcases = []struct {
		org      string
		repo     string
		expected TideMergeCommitTemplate
	}{
		{
			org:      "kubernetes",
			repo:     "kubernetes",
			expected: TideMergeCommitTemplate{},
		},
		{
			org:  "kubernetes",
			repo: "kops",
			expected: TideMergeCommitTemplate{
				TitleTemplate: "",
				BodyTemplate:  "",
			},
		},
		{
			org:  "kubernetes-helm",
			repo: "monocular",
			expected: TideMergeCommitTemplate{
				TitleTemplate: "{{ .Title }}",
				BodyTemplate:  "{{ .Body }}",
			},
		},
	}

	for _, test := range testcases {
		actual := ti.MergeCommitTemplate(OrgRepo{Org: test.org, Repo: test.repo})

		if actual.TitleTemplate != test.expected.TitleTemplate || actual.BodyTemplate != test.expected.BodyTemplate {
			t.Errorf("Expected title \"%v\", body \"%v\", but got title \"%v\", body \"%v\" for %v/%v", test.expected.TitleTemplate, test.expected.BodyTemplate, actual.TitleTemplate, actual.BodyTemplate, test.org, test.repo)
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
			t.Errorf("%s - did not get expected policy: %s", tc.name, diff.ObjectReflectDiff(tc.expected, policy))
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
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							Presubmit{
								Reporter: Reporter{
									Context: "pr1",
								},
								AlwaysRun: true,
							},
							Presubmit{
								Reporter: Reporter{
									Context: "po1",
								},
								AlwaysRun: true,
								Optional:  true,
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:          []string{"pr1"},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{"po1"},
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
				RequiredContexts:          []string{},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{},
			},
		},
		{
			name: "no branch protection",
			config: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							ContextOptions: TideContextPolicyOptions{
								TideContextPolicy: TideContextPolicy{
									FromBranchProtection: &yes,
								},
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:          []string{},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{},
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
						TideGitHubConfig: TideGitHubConfig{
							ContextOptions: TideContextPolicyOptions{
								TideContextPolicy: TideContextPolicy{
									FromBranchProtection: &yes,
								},
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:          []string{},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{},
			},
		},
		{
			name: "branch protection with manually required triggered jobs",
			config: Config{
				ProwConfig: ProwConfig{
					BranchProtection: BranchProtection{
						Orgs: map[string]Org{
							"org": {
								Policy: Policy{
									RequireManuallyTriggeredJobs: &yes,
								},
							},
						},
					},
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							ContextOptions: TideContextPolicyOptions{
								TideContextPolicy: TideContextPolicy{
									FromBranchProtection: &yes,
								},
							},
						},
					},
				},
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							Presubmit{
								Reporter: Reporter{
									Context: "pr1",
								},
								AlwaysRun: false,
								Optional:  false,
							},
							Presubmit{
								Reporter: Reporter{
									Context: "pr2",
								},
								AlwaysRun: true,
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:          []string{"pr1", "pr2"},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{},
			},
		},
		{
			name: "manually defined policy",
			config: Config{
				ProwConfig: ProwConfig{
					Tide: Tide{
						TideGitHubConfig: TideGitHubConfig{
							ContextOptions: TideContextPolicyOptions{
								TideContextPolicy: TideContextPolicy{
									RequiredContexts:          []string{"r1"},
									RequiredIfPresentContexts: []string{},
									OptionalContexts:          []string{"o1"},
									SkipUnknownContexts:       &yes,
								},
							},
						},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:          []string{"r1"},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{"o1"},
				SkipUnknownContexts:       &yes,
			},
		},
		{
			name: "jobs from inrepoconfig are considered",
			config: Config{
				JobConfig: JobConfig{
					ProwYAMLGetterWithDefaults: fakeProwYAMLGetterFactory(
						[]Presubmit{
							{
								AlwaysRun: true,
								Reporter:  Reporter{Context: "ir0"},
							},
							{
								AlwaysRun: true,
								Optional:  true,
								Reporter:  Reporter{Context: "ir1"},
							},
						},
						nil,
					),
				},
				ProwConfig: ProwConfig{
					InRepoConfig: InRepoConfig{
						Enabled: map[string]*bool{"*": utilpointer.Bool(true)},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:          []string{"ir0"},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{"ir1"},
			},
		},
		{
			name: "both static and inrepoconfig jobs are consired",
			config: Config{
				JobConfig: JobConfig{
					PresubmitsStatic: map[string][]Presubmit{
						"org/repo": {
							Presubmit{
								Reporter: Reporter{
									Context: "pr1",
								},
								AlwaysRun: true,
							},
							Presubmit{
								Reporter: Reporter{
									Context: "po1",
								},
								AlwaysRun: true,
								Optional:  true,
							},
						},
					},
					ProwYAMLGetterWithDefaults: fakeProwYAMLGetterFactory(
						[]Presubmit{
							{
								AlwaysRun: true,
								Reporter:  Reporter{Context: "ir0"},
							},
							{
								AlwaysRun: true,
								Optional:  true,
								Reporter:  Reporter{Context: "ir1"},
							},
						},
						nil,
					),
				},
				ProwConfig: ProwConfig{
					InRepoConfig: InRepoConfig{
						Enabled: map[string]*bool{"*": utilpointer.Bool(true)},
					},
				},
			},
			expected: TideContextPolicy{
				RequiredContexts:          []string{"ir0", "pr1"},
				RequiredIfPresentContexts: []string{},
				OptionalContexts:          []string{"ir1", "po1"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			baseSHAGetter := func() (string, error) {
				return "baseSHA", nil
			}
			p, err := tc.config.GetTideContextPolicy(nil, org, repo, branch, baseSHAGetter, "some-sha")
			if !reflect.DeepEqual(p, &tc.expected) {
				t.Errorf("%s - did not get expected policy: %s", tc.name, diff.ObjectReflectDiff(&tc.expected, p))
			}
			if err != nil {
				if err.Error() != tc.error {
					t.Errorf("%s - expected error %v got %v", tc.name, tc.error, err.Error())
				}
			} else if tc.error != "" {
				t.Errorf("%s - expected error %v got nil", tc.name, tc.error)
			}
		})
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
		name        string
		query       TideQuery
		expectError bool
	}{
		{
			name: "good query",
			query: TideQuery{
				Orgs:                   []string{"kuber"},
				Repos:                  []string{"foo/bar", "baz/bar"},
				ExcludedRepos:          []string{"kuber/netes"},
				IncludedBranches:       []string{"master"},
				Milestone:              "backlog-forever",
				Labels:                 []string{labels.LGTM, labels.Approved},
				MissingLabels:          []string{"do-not-merge/evil-code"},
				ReviewApprovedRequired: true,
			},
			expectError: false,
		},
		{
			name: "simple org query is valid",
			query: TideQuery{
				Orgs: []string{"kuber"},
			},
			expectError: false,
		},
		{
			name: "org with slash is invalid",
			query: TideQuery{
				Orgs: []string{"kube/r"},
			},
			expectError: true,
		},
		{
			name: "empty org is invalid",
			query: TideQuery{
				Orgs: []string{""},
			},
			expectError: true,
		},
		{
			name: "duplicate org is invalid",
			query: TideQuery{
				Orgs: []string{"kuber", "kuber"},
			},
			expectError: true,
		},
		{
			name: "simple repo query is valid",
			query: TideQuery{
				Repos: []string{"kuber/netes"},
			},
			expectError: false,
		},
		{
			name: "repo without slash is invalid",
			query: TideQuery{
				Repos: []string{"foobar", "baz/bar"},
			},
			expectError: true,
		},
		{
			name: "repo included with parent org is invalid",
			query: TideQuery{
				Orgs:  []string{"kuber"},
				Repos: []string{"foo/bar", "kuber/netes"},
			},
			expectError: true,
		},
		{
			name: "duplicate repo is invalid",
			query: TideQuery{
				Repos: []string{"baz/bar", "foo/bar", "baz/bar"},
			},
			expectError: true,
		},
		{
			name: "empty orgs and repos is invalid",
			query: TideQuery{
				IncludedBranches:       []string{"master"},
				Milestone:              "backlog-forever",
				Labels:                 []string{labels.LGTM, labels.Approved},
				MissingLabels:          []string{"do-not-merge/evil-code"},
				ReviewApprovedRequired: true,
			},
			expectError: true,
		},
		{
			name: "simple excluded repo query is valid",
			query: TideQuery{
				Orgs:          []string{"kuber"},
				ExcludedRepos: []string{"kuber/netes"},
			},
			expectError: false,
		},
		{
			name: "excluded repo without slash is invalid",
			query: TideQuery{
				Orgs:          []string{"kuber"},
				ExcludedRepos: []string{"kubernetes"},
			},
			expectError: true,
		},
		{
			name: "excluded repo included without parent org is invalid",
			query: TideQuery{
				Repos:         []string{"foo/bar", "baz/bar"},
				ExcludedRepos: []string{"kuber/netes"},
			},
			expectError: true,
		},
		{
			name: "duplicate excluded repo is invalid",
			query: TideQuery{
				Orgs:                   []string{"kuber"},
				ExcludedRepos:          []string{"kuber/netes", "kuber/netes"},
				ReviewApprovedRequired: true,
			},
			expectError: true,
		},
		{
			name: "label cannot be required and forbidden",
			query: TideQuery{
				Orgs:          []string{"kuber"},
				Labels:        []string{labels.LGTM, labels.Approved},
				MissingLabels: []string{"do-not-merge/evil-code", labels.LGTM},
			},
			expectError: true,
		},
		{
			name: "simple excluded branches query is valid",
			query: TideQuery{
				Orgs:             []string{"kuber"},
				ExcludedBranches: []string{"dev"},
			},
			expectError: false,
		},
		{
			name: "specifying both included and excluded branches is invalid",
			query: TideQuery{
				Orgs:             []string{"kuber"},
				IncludedBranches: []string{"master"},
				ExcludedBranches: []string{"dev"},
			},
			expectError: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.query.Validate()
			if err != nil && !tc.expectError {
				t.Errorf("Unexpected error: %v.", err)
			} else if err == nil && tc.expectError {
				t.Error("Expected a validation error, but didn't get one.")
			}
		})

	}
}

func TestTideContextPolicy_Validate(t *testing.T) {
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
			name: "optional contexts must differ from required contexts",
			t: TideContextPolicy{
				OptionalContexts: []string{"c1"},
				RequiredContexts: []string{"c1"},
			},
			failed: true,
		},
		{
			name: "individual contexts cannot be both optional and required",
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
		if !sets.New[string](missingContexts...).Equal(sets.New[string](tc.expectedContexts...)) {
			t.Errorf("%s - expected %v got %v", tc.name, tc.expectedContexts, missingContexts)
		}
	}
}

func fakeProwYAMLGetterFactory(presubmits []Presubmit, postsubmits []Postsubmit) ProwYAMLGetter {
	return func(_ *Config, _ git.ClientFactory, _, _ string, _ ...string) (*ProwYAML, error) {
		return &ProwYAML{
			Presubmits:  presubmits,
			Postsubmits: postsubmits,
		}, nil
	}
}

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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

type FakeClient struct {
	repos    map[string][]github.Repo
	branches map[string][]github.Branch
	pushers  map[string][]string
	contexts map[string][]string
	deleted  map[string]bool
}

func (c FakeClient) GetRepos(org string, user bool) ([]github.Repo, error) {
	r, ok := c.repos[org]
	if !ok {
		return nil, fmt.Errorf("Unknown org: %s", org)
	}
	return r, nil
}

func (c FakeClient) GetBranches(org, repo string) ([]github.Branch, error) {
	b, ok := c.branches[org+"/"+repo]
	if !ok {
		return nil, fmt.Errorf("Unknown repo: %s/%s", org, repo)
	}
	return b, nil
}

func (c *FakeClient) UpdateBranchProtection(org, repo, branch string, contexts, pushers []string) error {
	if branch == "error" {
		return errors.New("failed to update branch protection")
	}
	ctx := org + "/" + repo + "=" + branch
	if len(pushers) > 0 {
		c.pushers[ctx] = pushers
	}
	if len(contexts) > 0 {
		c.contexts[ctx] = contexts
	}
	return nil
}

func (c *FakeClient) RemoveBranchProtection(org, repo, branch string) error {
	if branch == "error" {
		return errors.New("failed to remove branch protection")
	}
	ctx := org + "/" + repo + "=" + branch
	c.deleted[ctx] = true
	return nil
}

func TestConfigureBranches(t *testing.T) {
	cases := []struct {
		name     string
		updates  []Requirements
		deletes  []string
		contexts map[string][]string
		pushers  map[string][]string
		errors   int
	}{
		{
			name: "remove-protection",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "delete", Protect: false},
				{Org: "one", Repo: "1", Branch: "remove", Protect: false},
				{Org: "two", Repo: "2", Branch: "remove", Protect: false},
			},
			deletes: []string{"one/1=delete", "one/1=remove", "two/2=remove"},
		},
		{
			name: "error-remove-protection",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "error", Protect: false},
			},
			errors: 1,
		},
		{
			name: "update-protection-context",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "master", Protect: true, Contexts: []string{"this-context", "that-context"}},
				{Org: "one", Repo: "1", Branch: "other", Protect: true, Contexts: []string{"hello", "world"}},
			},
			contexts: map[string][]string{
				"one/1=master": {"this-context", "that-context"},
				"one/1=other":  {"hello", "world"},
			},
		},
		{
			name: "update-protection-pushers",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "master", Protect: true, Pushers: []string{"admins", "oncall"}},
				{Org: "one", Repo: "1", Branch: "other", Protect: true, Pushers: []string{"me", "myself", "I"}},
			},
			pushers: map[string][]string{
				"one/1=master": {"admins", "oncall"},
				"one/1=other":  {"me", "myself", "I"},
			},
		},
		{
			name: "complex",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "master", Protect: true, Pushers: []string{"admins", "oncall"}},
				{Org: "one", Repo: "1", Branch: "remove", Protect: false},
				{Org: "two", Repo: "2", Branch: "push-and-context", Protect: true, Pushers: []string{"team"}, Contexts: []string{"presubmit"}},
				{Org: "three", Repo: "3", Branch: "push-and-context", Protect: true, Pushers: []string{"team"}, Contexts: []string{"presubmit"}},
				{Org: "four", Repo: "4", Branch: "error", Protect: true, Pushers: []string{"team"}, Contexts: []string{"presubmit"}},
				{Org: "five", Repo: "5", Branch: "error", Protect: false},
			},
			errors:  2, // four and five
			deletes: []string{"one/1=remove"},
			pushers: map[string][]string{
				"one/1=master":             {"admins", "oncall"},
				"two/2=push-and-context":   {"team"},
				"three/3=push-and-context": {"team"},
			},
			contexts: map[string][]string{
				"two/2=push-and-context":   {"presubmit"},
				"three/3=push-and-context": {"presubmit"},
			},
		},
	}

	for _, tc := range cases {
		fc := FakeClient{
			deleted:  make(map[string]bool),
			contexts: make(map[string][]string),
			pushers:  make(map[string][]string),
		}
		p := Protector{
			client:  &fc,
			updates: make(chan Requirements),
			done:    make(chan []error),
		}
		go p.ConfigureBranches()
		for _, u := range tc.updates {
			p.updates <- u
		}
		close(p.updates)
		errs := <-p.done
		if len(errs) != tc.errors {
			t.Errorf("%s: %d errors != expected %d: %v", tc.name, len(errs), tc.errors, errs)
		}
		if len(fc.deleted) != len(tc.deletes) {
			t.Errorf("%s: wrong number of deletes %d not expected %d: %v", tc.name, len(fc.deleted), len(tc.deletes), fc.deleted)
		}
		for _, d := range tc.deletes {
			if fc.deleted[d] != true {
				t.Errorf("%s: did not delete %s", tc.name, d)
			}
		}

		if len(fc.contexts) != len(tc.contexts) {
			t.Errorf("%s: wrong number of contexts %d not expected %d: %v", tc.name, len(fc.contexts), len(tc.contexts), fc.contexts)
		}
		for branch, actual := range fc.contexts {
			e := tc.contexts[branch]
			if !reflect.DeepEqual(actual, e) {
				t.Errorf("%s: for %s actual %v != expected %v", tc.name, branch, actual, e)
			}
		}
		if len(fc.pushers) != len(tc.pushers) {
			t.Errorf("%s: wrong number of pushers %d not expected %d: %v", tc.name, len(fc.pushers), len(tc.pushers), fc.pushers)
		}
		for branch, actual := range fc.pushers {
			e := tc.pushers[branch]
			if !reflect.DeepEqual(actual, e) {
				t.Errorf("%s: for %s actual %v != expected %v", tc.name, branch, actual, e)
			}
		}
	}
}

func split(branch string) (string, string, string) {
	parts := strings.Split(branch, "=")
	b := parts[1]
	parts = strings.Split(parts[0], "/")
	return parts[0], parts[1], b
}

func TestProtect(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		name     string
		branches []string
		config   config.Config
		expected []Requirements
		errors   int
	}{
		{
			name: "nothing",
		},
		{
			name: "unknown org",
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect: &yes,
					Orgs: map[string]config.Org{
						"unknown": {},
					},
				},
			},
			errors: 1,
		},
		{
			name:     "protect org via config default",
			branches: []string{"org/repo1=master", "org/repo1=branch", "org/repo2=master"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect: &yes,
					Orgs: map[string]config.Org{
						"org": {},
					},
				},
			},
			expected: []Requirements{
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "master",
					Protect: true,
				},
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "branch",
					Protect: true,
				},
				{
					Org:     "org",
					Repo:    "repo2",
					Branch:  "master",
					Protect: true,
				},
			},
		},
		{
			name:     "protect this but not that org",
			branches: []string{"this/yes=master", "that/no=master"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect: &no,
					Orgs: map[string]config.Org{
						"this": {Protect: &yes},
						"that": {},
					},
				},
			},
			expected: []Requirements{
				{
					Org:     "this",
					Repo:    "yes",
					Branch:  "master",
					Protect: true,
				},
				{
					Org:     "that",
					Repo:    "no",
					Branch:  "master",
					Protect: false,
				},
			},
		},
		{
			name:     "require a defined branch to make a protection decision",
			branches: []string{"org/repo=branch"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Orgs: map[string]config.Org{
						"org": {
							Repos: map[string]config.Repo{
								"repo": {
									Branches: map[string]config.Branch{
										"branch": {},
									},
								},
							},
						},
					},
				},
			},
			errors: 1,
		},
		{
			name:     "require pushers to set protection",
			branches: []string{"org/repo=push"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect: &no,
					Pushers: []string{"oncall"},
					Orgs: map[string]config.Org{
						"org": {},
					},
				},
			},
			errors: 1,
		},
		{
			name:     "require requiring contexts to set protection",
			branches: []string{"org/repo=context"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect:  &no,
					Contexts: []string{"test-foo"},
					Orgs: map[string]config.Org{
						"org": {},
					},
				},
			},
			errors: 1,
		},
		{
			name:     "protect org but skip a repo",
			branches: []string{"org/repo1=master", "org/repo1=branch", "org/skip=master"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect: &no,
					Orgs: map[string]config.Org{
						"org": {
							Protect: &yes,
							Repos: map[string]config.Repo{
								"skip": {
									Protect: &no,
								},
							},
						},
					},
				},
			},
			expected: []Requirements{
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "master",
					Protect: true,
				},
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "branch",
					Protect: true,
				},
				{
					Org:     "org",
					Repo:    "skip",
					Branch:  "master",
					Protect: false,
				},
			},
		},
		{
			name:     "append contexts",
			branches: []string{"org/repo=master"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect:  &yes,
					Contexts: []string{"config-presubmit"},
					Orgs: map[string]config.Org{
						"org": {
							Contexts: []string{"org-presubmit"},
							Repos: map[string]config.Repo{
								"repo": {
									Contexts: []string{"repo-presubmit"},
									Branches: map[string]config.Branch{
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
			expected: []Requirements{
				{
					Org:      "org",
					Repo:     "repo",
					Branch:   "master",
					Protect:  true,
					Contexts: []string{"config-presubmit", "org-presubmit", "repo-presubmit", "branch-presubmit"},
				},
			},
		},
		{
			name:     "append pushers",
			branches: []string{"org/repo=master"},
			config: config.Config{
				BranchProtection: config.BranchProtection{
					Protect: &yes,
					Pushers: []string{"config-team"},
					Orgs: map[string]config.Org{
						"org": {
							Pushers: []string{"org-team"},
							Repos: map[string]config.Repo{
								"repo": {
									Pushers: []string{"repo-team"},
									Branches: map[string]config.Branch{
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
			expected: []Requirements{
				{
					Org:     "org",
					Repo:    "repo",
					Branch:  "master",
					Protect: true,
					Pushers: []string{"config-team", "org-team", "repo-team", "branch-team"},
				},
			},
		},
	}

	for _, tc := range cases {
		repos := map[string]map[string]bool{}
		branches := map[string][]github.Branch{}
		for _, b := range tc.branches {
			org, repo, branch := split(b)
			k := org + "/" + repo
			branches[k] = append(branches[k], github.Branch{Name: branch})
			r := repos[org]
			if r == nil {
				repos[org] = make(map[string]bool)
			}
			repos[org][repo] = true
		}
		fc := FakeClient{
			deleted:  make(map[string]bool),
			contexts: make(map[string][]string),
			pushers:  make(map[string][]string),
			branches: branches,
			repos:    make(map[string][]github.Repo),
		}
		for org, r := range repos {
			for rname := range r {
				fc.repos[org] = append(fc.repos[org], github.Repo{Name: rname, FullName: org + "/" + rname})
			}
		}

		p := Protector{
			client:         &fc,
			cfg:            &tc.config,
			errors:         Errors{},
			updates:        make(chan Requirements),
			done:           make(chan []error),
			completedRepos: make(map[string]bool),
		}
		go func() {
			p.Protect()
			close(p.updates)
		}()

		var actual []Requirements
		for r := range p.updates {
			actual = append(actual, r)
		}
		errors := p.errors.errs
		if len(errors) != tc.errors {
			t.Errorf("%s: actual errors %d != expected %d", tc.name, len(errors), tc.errors)
		}
		switch {
		case len(actual) != len(tc.expected):
			t.Errorf("%s: actual updates %v != expected %v", tc.name, actual, tc.expected)
		default:
			for _, a := range actual {
				found := false
				for _, e := range tc.expected {
					if e.Org == a.Org && e.Repo == a.Repo && e.Branch == a.Branch {
						found = true
						if e.Protect != a.Protect {
							t.Errorf("%s: %s/%s=%s actual protect %t != expected %t", tc.name, e.Org, e.Repo, e.Branch, a.Protect, e.Protect)
						}
						if !reflect.DeepEqual(e.Contexts, a.Contexts) {
							t.Errorf("%s: %s/%s=%s actual contexts %v != expected %v", tc.name, e.Org, e.Repo, e.Branch, a.Contexts, e.Contexts)
						}
						if !reflect.DeepEqual(e.Pushers, a.Pushers) {
							t.Errorf("%s: %s/%s=%s actual pushers %v != expected %v", tc.name, e.Org, e.Repo, e.Branch, a.Pushers, e.Pushers)
						}
						break
					}
				}
				if !found {
					t.Errorf("%s: actual updates %v not in expected %v", tc.name, a, tc.expected)
				}
			}
		}
	}
}

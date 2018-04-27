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
	"sort"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"

	"github.com/ghodss/yaml"
)

func TestOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		opt         options
		expectedErr bool
	}{
		{
			name: "all ok",
			opt: options{
				config:   "dummy",
				token:    "fake",
				endpoint: "https://api.github.com",
			},
			expectedErr: false,
		},
		{
			name: "no config",
			opt: options{
				config:   "",
				token:    "fake",
				endpoint: "https://api.github.com",
			},
			expectedErr: true,
		},
		{
			name: "no token",
			opt: options{
				config:   "dummy",
				token:    "",
				endpoint: "https://api.github.com",
			},
			expectedErr: true,
		},
		{
			name: "invalid endpoint",
			opt: options{
				config:   "dummy",
				token:    "fake",
				endpoint: ":",
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		err := testCase.opt.Validate()
		if testCase.expectedErr && err == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if !testCase.expectedErr && err != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}
	}
}

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
		config   string
		expected []Requirements
		errors   int
	}{
		{
			name: "nothing",
		},
		{
			name: "unknown org",
			config: `
branch-protection:
  protect-by-default: true
  orgs:
    unknown:
`,
			errors: 1,
		},
		{
			name:     "protect org via config default",
			branches: []string{"org/repo1=master", "org/repo1=branch", "org/repo2=master"},
			config: `
branch-protection:
  protect-by-default: true
  orgs:
    org:
`,
			expected: []Requirements{
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "master",
					Protect: yes,
				},
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "branch",
					Protect: yes,
				},
				{
					Org:     "org",
					Repo:    "repo2",
					Branch:  "master",
					Protect: yes,
				},
			},
		},
		{
			name:     "protect this but not that org",
			branches: []string{"this/yes=master", "that/no=master"},
			config: `
branch-protection:
  protect-by-default: false
  orgs:
    this:
      protect-by-default: true
    that:
`,
			expected: []Requirements{
				{
					Org:     "this",
					Repo:    "yes",
					Branch:  "master",
					Protect: yes,
				},
				{
					Org:     "that",
					Repo:    "no",
					Branch:  "master",
					Protect: no,
				},
			},
		},
		{
			name:     "require a defined branch to make a protection decision",
			branches: []string{"org/repo=branch"},
			config: `
branch-protection:
  orgs:
    org:
      repos:
        repo:
          branches:
            branch: # empty
`,
			errors: 1,
		},
		{
			name:     "require pushers to set protection",
			branches: []string{"org/repo=push"},
			config: `
branch-protection:
  protect-by-default: false
  allow-push:
  - oncall
  orgs:
    org:
`,
			errors: 1,
		},
		{
			name:     "required contexts must set protection",
			branches: []string{"org/repo=context"},
			config: `
branch-protection:
  protect-by-default: false
  require-contexts:
  - test-foo
  orgs:
    org:
`,
			errors: 1,
		},
		{
			name:     "protect org but skip a repo",
			branches: []string{"org/repo1=master", "org/repo1=branch", "org/skip=master"},
			config: `
branch-protection:
  protect-by-default: false
  orgs:
    org:
      protect-by-default: true
      repos:
        skip:
          protect-by-default: false
`,
			expected: []Requirements{
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "master",
					Protect: yes,
				},
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "branch",
					Protect: yes,
				},
				{
					Org:     "org",
					Repo:    "skip",
					Branch:  "master",
					Protect: no,
				},
			},
		},
		{
			name:     "append contexts",
			branches: []string{"org/repo=master"},
			config: `
branch-protection:
  protect-by-default: true
  require-contexts:
  - config-presubmit
  orgs:
    org:
      require-contexts:
      - org-presubmit
      repos:
        repo:
          require-contexts:
          - repo-presubmit
          branches:
            master:
              require-contexts:
              - branch-presubmit
`,
			expected: []Requirements{
				{
					Org:      "org",
					Repo:     "repo",
					Branch:   "master",
					Protect:  yes,
					Contexts: []string{"config-presubmit", "org-presubmit", "repo-presubmit", "branch-presubmit"},
				},
			},
		},
		{
			name:     "append pushers",
			branches: []string{"org/repo=master"},
			config: `
branch-protection:
  protect-by-default: true
  allow-push:
  - config-team
  orgs:
    org:
      allow-push:
      - org-team
      repos:
        repo:
          allow-push:
          - repo-team
          branches:
            master:
              allow-push:
              - branch-team
`,
			expected: []Requirements{
				{
					Org:     "org",
					Repo:    "repo",
					Branch:  "master",
					Protect: yes,
					Pushers: []string{"config-team", "org-team", "repo-team", "branch-team"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			var cfg config.Config
			if err := yaml.Unmarshal([]byte(tc.config), &cfg); err != nil {
				t.Fatalf("failed to parse config: %v", err)
			}
			p := Protector{
				client:         &fc,
				cfg:            &cfg,
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
				t.Errorf("actual errors %d != expected %d", len(errors), tc.errors)
			}
			switch {
			case len(actual) != len(tc.expected):
				t.Errorf("actual updates %v != expected %v", actual, tc.expected)
			default:
				for _, a := range actual {
					found := false
					for _, e := range tc.expected {
						if e.Org == a.Org && e.Repo == a.Repo && e.Branch == a.Branch {
							found = true
							fixup(&a)
							fixup(&e)
							if !reflect.DeepEqual(e, a) {
								t.Errorf("bad policy actual %v != expected %v", a, e)
							}
							break
						}
					}
					if !found {
						t.Errorf("actual updates %v not in expected %v", a, tc.expected)
					}
				}
			}
		})
	}
}

func fixup(r *Requirements) {
	if r == nil {
		return
	}
	if len(r.Contexts) == 0 {
		r.Contexts = nil
	} else {
		sort.Strings(r.Contexts)
	}
	if len(r.Pushers) == 0 {
		r.Pushers = nil
	} else {
		sort.Strings(r.Pushers)
	}
}

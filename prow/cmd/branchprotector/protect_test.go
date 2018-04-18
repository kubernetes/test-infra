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
	"github.com/ghodss/yaml"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/branchprotection"
	"k8s.io/test-infra/prow/github"
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

func TestJobRequirements(t *testing.T) {
	cases := []struct {
		name     string
		config   []config.Presubmit
		expected []string
	}{
		{
			name: "basic",
			config: []config.Presubmit{
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
			},
			expected: []string{"always-run", "run-if-changed"},
		},
		{
			name: "children",
			config: []config.Presubmit{
				{
					Context:    "always-run",
					AlwaysRun:  true,
					SkipReport: false,
					RunAfterSuccess: []config.Presubmit{
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
					RunAfterSuccess: []config.Presubmit{
						{
							Context: "me2",
						},
					},
				},
				{
					Context:    "run-and-skip",
					AlwaysRun:  true,
					SkipReport: true,
					RunAfterSuccess: []config.Presubmit{
						{
							Context: "also-me-3",
						},
					},
				},
				{
					Context:    "optional",
					AlwaysRun:  false,
					SkipReport: false,
					RunAfterSuccess: []config.Presubmit{
						{
							Context: "no thanks",
						},
					},
				},
				{
					Context:    "hidden-grandpa",
					AlwaysRun:  true,
					SkipReport: true,
					RunAfterSuccess: []config.Presubmit{
						{
							Context:    "hidden-parent",
							SkipReport: true,
							AlwaysRun:  false,
							RunAfterSuccess: []config.Presubmit{
								{
									Context: "visible-kid",
								},
							},
						},
					},
				},
			},
			expected: []string{
				"always-run", "include-me",
				"me2",
				"also-me-3",
				"visible-kid",
			},
		},
	}

	for _, tc := range cases {
		actual := jobRequirements(tc.config, false)
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("%s: actual %v != expected %v", tc.name, actual, tc.expected)
		}
	}
}

type FakeClient struct {
	repos    map[string][]github.Repo
	branches map[string][]github.Branch
	requests map[string]github.BranchProtectionRequest
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

func (c *FakeClient) UpdateBranchProtection(org, repo, branch string, request github.BranchProtectionRequest) error {
	if branch == "error" {
		return errors.New("failed to update branch protection")
	}
	ctx := org + "/" + repo + "=" + branch
	if c.requests == nil {
		c.requests = map[string]github.BranchProtectionRequest{}
	}
	c.requests[ctx] = request
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

func depPolicy(protect bool, contexts, pushers []string) branchprotection.Policy {
	p := branchprotection.Policy{
		Protect: &protect,
	}

	if contexts != nil {
		p.ContextPolicy = &branchprotection.ContextPolicy{
			Contexts: contexts,
		}
	}
	if pushers != nil {
		p.Restrictions = &branchprotection.Restrictions{
			Teams: pushers,
		}
	}

	return p
}

func pushers(teams ...string) github.BranchProtectionRequest {
	return github.BranchProtectionRequest{
		Restrictions: &github.Restrictions{
			Teams: teams,
		},
	}
}

func contexts(c ...string) github.BranchProtectionRequest {
	return github.BranchProtectionRequest{
		RequiredStatusChecks: &github.RequiredStatusChecks{
			Contexts: c,
		},
	}
}

func TestConfigureBranches(t *testing.T) {
	cases := []struct {
		name     string
		updates  []Requirements
		deletes  []string
		expected map[string]github.BranchProtectionRequest
		errors   int
	}{
		{
			name: "remove-protection",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "delete", Policy: depPolicy(false, nil, nil)},
				{Org: "one", Repo: "1", Branch: "remove", Policy: depPolicy(false, nil, nil)},
				{Org: "two", Repo: "2", Branch: "remove", Policy: depPolicy(false, nil, nil)},
			},
			deletes: []string{"one/1=delete", "one/1=remove", "two/2=remove"},
		},
		{
			name: "error-remove-protection",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "error", Policy: depPolicy(false, nil, nil)},
			},
			errors: 1,
		},
		{
			name: "update-protection-context",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "master", Policy: depPolicy(true, []string{"this-context", "that-context"}, nil)},
				{Org: "one", Repo: "1", Branch: "other", Policy: depPolicy(true, []string{"hello", "world"}, nil)},
			},
			expected: map[string]github.BranchProtectionRequest{
				"one/1=master": contexts("this-context", "that-context"),
				"one/1=other":  contexts("hello", "world"),
			},
		},
		{
			name: "update-protection-pushers",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "master", Policy: depPolicy(true, nil, []string{"admins", "oncall"})},
				{Org: "one", Repo: "1", Branch: "other", Policy: depPolicy(true, nil, []string{"me", "myself", "I"})},
			},
			expected: map[string]github.BranchProtectionRequest{
				"one/1=master": pushers("admins", "oncall"),
				"one/1=other":  pushers("me", "myself", "I"),
			},
		},
		{
			name: "complex",
			updates: []Requirements{
				{Org: "one", Repo: "1", Branch: "master", Policy: depPolicy(true, nil, []string{"admins", "oncall"})},
				{Org: "one", Repo: "1", Branch: "remove", Policy: depPolicy(false, nil, nil)},
				{Org: "two", Repo: "2", Branch: "push-and-context", Policy: depPolicy(true, []string{"presubmit"}, []string{"team"})},
				{Org: "three", Repo: "3", Branch: "push-and-context", Policy: depPolicy(true, []string{"presubmit"}, []string{"team"})},
				{Org: "four", Repo: "4", Branch: "error", Policy: depPolicy(true, []string{"presubmit"}, []string{"team"})},
				{Org: "five", Repo: "5", Branch: "error", Policy: depPolicy(false, nil, nil)},
			},
			errors:  2, // four and five
			deletes: []string{"one/1=remove"},
			expected: map[string]github.BranchProtectionRequest{
				"one/1=master": pushers("admins", "oncall"),
				"two/2=push-and-context": {
					Restrictions: &github.Restrictions{
						Teams: []string{"team"},
					},
					RequiredStatusChecks: &github.RequiredStatusChecks{
						Contexts: []string{"presubmit"},
					},
				},
				"three/3=push-and-context": {
					Restrictions: &github.Restrictions{
						Teams: []string{"team"},
					},
					RequiredStatusChecks: &github.RequiredStatusChecks{
						Contexts: []string{"presubmit"},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		fc := FakeClient{
			deleted: make(map[string]bool),
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

		if !reflect.DeepEqual(tc.expected, fc.requests) {
			t.Errorf("%s: expected updates %v != actual %v", tc.name, tc.expected, fc.requests)
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
	five := 5
	cases := []struct {
		name     string
		branches []string
		config   string
		expected map[string]branchprotection.Policy
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
    unknown: # should fail
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
    org: # will list repos
`,
			expected: map[string]branchprotection.Policy{
				"org/repo1=master": depPolicy(true, nil, nil),
				"org/repo1=branch": depPolicy(true, nil, nil),
				"org/repo2=master": depPolicy(true, nil, nil),
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
      # defaults
`,
			expected: map[string]branchprotection.Policy{
				"that/no=master":  depPolicy(false, nil, nil),
				"this/yes=master": depPolicy(true, nil, nil),
			},
		},
		{
			name:     "require a defined branch to make a protection decision",
			branches: []string{"org/repo=indecisive"},
			config: `
branch-protection:
  orgs:
    org:
      repos:
        repo:
          branches:
            indecisive:
              # nothing set
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
			expected: map[string]branchprotection.Policy{
				"org/repo1=master": depPolicy(true, nil, nil),
				"org/repo1=branch": depPolicy(true, nil, nil),
				"org/skip=master":  depPolicy(false, nil, nil),
			},
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
			name:     "require requiring contexts to set protection",
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
			expected: map[string]branchprotection.Policy{
				"org/repo=master": depPolicy(true, []string{"branch-presubmit", "config-presubmit", "org-presubmit", "repo-presubmit"}, nil),
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
			expected: map[string]branchprotection.Policy{
				"org/repo=master": depPolicy(true, nil, []string{"branch-team", "config-team", "org-team", "repo-team"}),
			},
		},
		{
			name:     "refuse mixing global policy with deprecated fields",
			branches: []string{"hello/world=master"},
			config: `
branch-protection:
  protect-by-default: true
  protect: true
  orgs:
    hello:
`,
			errors: 1,
		},
		{
			name:     "refuse mixing org policy with deprecated fields",
			branches: []string{"hello/world=master"},
			config: `
branch-protection:
  orgs:
    hello:
      protect-by-default: true
      protect: true
`,
			errors: 1,
		},
		{
			name:     "refuse mixing repo policy with deprecated fields",
			branches: []string{"hello/world=master"},
			config: `
branch-protection:
  orgs:
    hello:
      repos:
        world:
          protect-by-default: true
          protect: true
`,
			errors: 1,
		},
		{
			name:     "refuse mixing branch policy with deprecated fields",
			branches: []string{"hello/world=master"},
			config: `
branch-protection:
  orgs:
    hello:
      repos:
        world:
          branches:
            master:
              protect-by-default: true
              protect: true
`,
			errors: 1,
		},
		{
			name:     "replace deprecated fields",
			branches: []string{"hello/world=master"},
			config: `
branch-protection:
  protect-by-default: false
  require-contexts:
  - this
  allow-push:
  - us
  orgs:
    hello:
      protect: true
      required_status_checks:
        contexts:
        - that
      restrictions:
        teams:
        - them
`,
			expected: map[string]branchprotection.Policy{
				"hello/world=master": depPolicy(true, []string{"that", "this"}, []string{"them", "us"}),
			},
		},
		{
			name:     "full policy",
			branches: []string{"hello/world=master"},
			config: `
branch-protection:
  protect: true
  required_status_checks:
    strict: true
    contexts:
    - failing-test
  enforce_admins: true
  restrictions:
    teams:
    - blue
    - red
    users:
    - cindy
    - david
  required_pull_request_reviews:
    dismissal_restrictions:
      teams:
      - alpha
      - beta
      users:
      - alice
      - bob
    dismiss_stale_reviews: true
    require_code_owner_reviews: true
    required_approving_review_count: 5
  orgs:
    hello:
      required_status_checks:
        strict: false
      repos:
        world:
          required_pull_request_reviews:
            dismiss_stale_reviews: false
          branches:
            master:
              enforce_admins: false
`,
			expected: map[string]branchprotection.Policy{
				"hello/world=master": {
					Protect: &yes,
					ReviewPolicy: &branchprotection.ReviewPolicy{
						Restrictions: &branchprotection.Restrictions{
							Users: []string{"alice", "bob"},
							Teams: []string{"alpha", "beta"},
						},
						DismissStale:  &no,
						RequireOwners: &yes,
						Approvals:     &five,
					},
					Restrictions: &branchprotection.Restrictions{
						Users: []string{"cindy", "david"},
						Teams: []string{"blue", "red"},
					},
					ContextPolicy: &branchprotection.ContextPolicy{
						Strict:   &no,
						Contexts: []string{"failing-test"},
					},
					Admins: &no,
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
				branches: branches,
				repos:    make(map[string][]github.Repo),
			}
			for org, r := range repos {
				for rname := range r {
					fc.repos[org] = append(fc.repos[org], github.Repo{Name: rname, FullName: org + "/" + rname})
				}
			}

			var cfg config.Config
			err := yaml.Unmarshal([]byte(tc.config), &cfg)
			if err != nil {
				t.Errorf("failed to parse configString: %v", err)
				return
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
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("unexpected panic: %v", r)
						panic(r)
					}
				}()
				p.Protect()
				close(p.updates)
			}()

			var actual map[string]branchprotection.Policy
			for r := range p.updates {
				if actual == nil {
					actual = map[string]branchprotection.Policy{}
				}
				actual[r.Org+"/"+r.Repo+"="+r.Branch] = r.Policy
			}
			errors := p.errors.errs
			if len(errors) != tc.errors {
				t.Errorf("actual errors %d != expected %d", len(errors), tc.errors)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				a, _ := yaml.Marshal(actual)
				e, _ := yaml.Marshal(tc.expected)
				t.Errorf("wrong yaml:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
				t.Errorf("expected:\n%s", e)
				t.Errorf("actual:\n%s", a)
			}
		})
	}
}

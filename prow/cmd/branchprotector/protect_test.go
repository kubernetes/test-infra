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

	"k8s.io/apimachinery/pkg/util/diff"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
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
				config: "dummy",
				github: flagutil.GitHubOptions{TokenPath: "fake"},
			},
			expectedErr: false,
		},
		{
			name: "no config",
			opt: options{
				config: "",
				github: flagutil.GitHubOptions{TokenPath: "fake"},
			},
			expectedErr: true,
		},
		{
			name: "no token, allow",
			opt: options{
				config: "dummy",
			},
			expectedErr: false,
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

type fakeClient struct {
	repos    map[string][]github.Repo
	branches map[string][]github.Branch
	deleted  map[string]bool
	updated  map[string]github.BranchProtectionRequest
}

func (c fakeClient) GetRepos(org string, user bool) ([]github.Repo, error) {
	r, ok := c.repos[org]
	if !ok {
		return nil, fmt.Errorf("Unknown org: %s", org)
	}
	return r, nil
}

func (c fakeClient) GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error) {
	b, ok := c.branches[org+"/"+repo]
	if !ok {
		return nil, fmt.Errorf("Unknown repo: %s/%s", org, repo)
	}
	var out []github.Branch
	if onlyProtected {
		for _, item := range b {
			if !item.Protected {
				continue
			}
			out = append(out, item)
		}
	} else {
		// when !onlyProtected, github does not set Protected
		// match that behavior here to ensure we handle this correctly
		for _, item := range b {
			item.Protected = false
			out = append(out, item)
		}
	}
	return b, nil
}

func (c *fakeClient) UpdateBranchProtection(org, repo, branch string, config github.BranchProtectionRequest) error {
	if branch == "error" {
		return errors.New("failed to update branch protection")
	}
	if c.updated == nil {
		c.updated = map[string]github.BranchProtectionRequest{}
	}
	ctx := org + "/" + repo + "=" + branch
	c.updated[ctx] = config
	return nil
}

func (c *fakeClient) RemoveBranchProtection(org, repo, branch string) error {
	if branch == "error" {
		return errors.New("failed to remove branch protection")
	}
	if c.deleted == nil {
		c.deleted = map[string]bool{}
	}
	ctx := org + "/" + repo + "=" + branch
	c.deleted[ctx] = true
	return nil
}

func TestConfigureBranches(t *testing.T) {
	yes := true

	prot := github.BranchProtectionRequest{}
	diffprot := github.BranchProtectionRequest{
		EnforceAdmins: &yes,
	}

	cases := []struct {
		name    string
		updates []requirements
		deletes map[string]bool
		sets    map[string]github.BranchProtectionRequest
		errors  int
	}{
		{
			name: "remove-protection",
			updates: []requirements{
				{Org: "one", Repo: "1", Branch: "delete", Request: nil},
				{Org: "one", Repo: "1", Branch: "remove", Request: nil},
				{Org: "two", Repo: "2", Branch: "remove", Request: nil},
			},
			deletes: map[string]bool{
				"one/1=delete": true,
				"one/1=remove": true,
				"two/2=remove": true,
			},
		},
		{
			name: "error-remove-protection",
			updates: []requirements{
				{Org: "one", Repo: "1", Branch: "error", Request: nil},
			},
			errors: 1,
		},
		{
			name: "update-protection-context",
			updates: []requirements{
				{
					Org:     "one",
					Repo:    "1",
					Branch:  "master",
					Request: &prot,
				},
				{
					Org:     "one",
					Repo:    "1",
					Branch:  "other",
					Request: &diffprot,
				},
			},
			sets: map[string]github.BranchProtectionRequest{
				"one/1=master": prot,
				"one/1=other":  diffprot,
			},
		},
		{
			name: "complex",
			updates: []requirements{
				{Org: "update", Repo: "1", Branch: "master", Request: &prot},
				{Org: "update", Repo: "2", Branch: "error", Request: &prot},
				{Org: "remove", Repo: "3", Branch: "master", Request: nil},
				{Org: "remove", Repo: "4", Branch: "error", Request: nil},
			},
			errors: 2, // four and five
			deletes: map[string]bool{
				"remove/3=master": true,
			},
			sets: map[string]github.BranchProtectionRequest{
				"update/1=master": prot,
			},
		},
	}

	for _, tc := range cases {
		fc := fakeClient{}
		p := protector{
			client:  &fc,
			updates: make(chan requirements),
			done:    make(chan []error),
		}
		go p.configureBranches()
		for _, u := range tc.updates {
			p.updates <- u
		}
		close(p.updates)
		errs := <-p.done
		if len(errs) != tc.errors {
			t.Errorf("%s: %d errors != expected %d: %v", tc.name, len(errs), tc.errors, errs)
		}
		if !reflect.DeepEqual(fc.deleted, tc.deletes) {
			t.Errorf("%s: deletes %v != expected %v", tc.name, fc.deleted, tc.deletes)
		}
		if !reflect.DeepEqual(fc.updated, tc.sets) {
			t.Errorf("%s: updates %v != expected %v", tc.name, fc.updated, tc.sets)
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

	cases := []struct {
		name             string
		branches         []string
		startUnprotected bool
		config           string
		expected         []requirements
		errors           int
	}{
		{
			name: "nothing",
		},
		{
			name: "unknown org",
			config: `
branch-protection:
  protect: true
  orgs:
    unknown:
`,
			errors: 1,
		},
		{
			name:     "protect org via config default",
			branches: []string{"cfgdef/repo1=master", "cfgdef/repo1=branch", "cfgdef/repo2=master"},
			config: `
branch-protection:
  protect: true
  orgs:
    cfgdef:
`,
			expected: []requirements{
				{
					Org:     "cfgdef",
					Repo:    "repo1",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{},
				},
				{
					Org:     "cfgdef",
					Repo:    "repo1",
					Branch:  "branch",
					Request: &github.BranchProtectionRequest{},
				},
				{
					Org:     "cfgdef",
					Repo:    "repo2",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{},
				},
			},
		},
		{
			name:     "protect this but not that org",
			branches: []string{"this/yes=master", "that/no=master"},
			config: `
branch-protection:
  protect: false
  orgs:
    this:
      protect: true
    that:
`,
			expected: []requirements{
				{
					Org:     "this",
					Repo:    "yes",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{},
				},
				{
					Org:     "that",
					Repo:    "no",
					Branch:  "master",
					Request: nil,
				},
			},
		},
		{
			name:     "protect all repos when protection configured at org level",
			branches: []string{"kubernetes/test-infra=master", "kubernetes/publishing-bot=master"},
			config: `
branch-protection:
  orgs:
    kubernetes:
      protect: true
      repos:
        test-infra:
          required_status_checks:
            contexts:
            - hello-world
`,
			expected: []requirements{
				{
					Org:    "kubernetes",
					Repo:   "test-infra",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						RequiredStatusChecks: &github.RequiredStatusChecks{
							Contexts: []string{"hello-world"},
						},
					},
				},
				{
					Org:     "kubernetes",
					Repo:    "publishing-bot",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{},
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
  protect: false
  restrictions:
    teams:
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
  protect: false
  required_status_checks:
    contexts:
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
  protect: false
  orgs:
    org:
      protect: true
      repos:
        skip:
          protect: false
`,
			expected: []requirements{
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{},
				},
				{
					Org:     "org",
					Repo:    "repo1",
					Branch:  "branch",
					Request: &github.BranchProtectionRequest{},
				},
				{
					Org:     "org",
					Repo:    "skip",
					Branch:  "master",
					Request: nil,
				},
			},
		},
		{
			name:     "collapse duplicated contexts",
			branches: []string{"org/repo=master"},
			config: `
branch-protection:
  protect: true
  required_status_checks:
    contexts:
    - hello-world
    - duplicate-context
    - duplicate-context
    - hello-world
  orgs:
    org:
`,
			expected: []requirements{
				{
					Org:    "org",
					Repo:   "repo",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						RequiredStatusChecks: &github.RequiredStatusChecks{
							Contexts: []string{"duplicate-context", "hello-world"},
						},
					},
				},
			},
		},
		{
			name:     "append contexts",
			branches: []string{"org/repo=master"},
			config: `
branch-protection:
  protect: true
  required_status_checks:
    contexts:
    - config-presubmit
  orgs:
    org:
      required_status_checks:
        contexts:
        - org-presubmit
      repos:
        repo:
          required_status_checks:
            contexts:
            - repo-presubmit
          branches:
            master:
              required_status_checks:
                contexts:
                - branch-presubmit
`,
			expected: []requirements{
				{
					Org:    "org",
					Repo:   "repo",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						RequiredStatusChecks: &github.RequiredStatusChecks{
							Contexts: []string{"config-presubmit", "org-presubmit", "repo-presubmit", "branch-presubmit"},
						},
					},
				},
			},
		},
		{
			name:     "append pushers",
			branches: []string{"org/repo=master"},
			config: `
branch-protection:
  protect: true
  restrictions:
    teams:
    - config-team
  orgs:
    org:
      restrictions:
        teams:
        - org-team
      repos:
        repo:
          restrictions:
            teams:
            - repo-team
          branches:
            master:
              restrictions:
                teams:
                - branch-team
`,
			expected: []requirements{
				{
					Org:    "org",
					Repo:   "repo",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						Restrictions: &github.Restrictions{
							Users: &[]string{},
							Teams: &[]string{"config-team", "org-team", "repo-team", "branch-team"},
						},
					},
				},
			},
		},
		{
			name:     "all modern fields",
			branches: []string{"all/modern=master"},
			config: `
branch-protection:
  protect: true
  enforce_admins: true
  required_status_checks:
    contexts:
    - config-presubmit
    strict: true
  required_pull_request_reviews:
    required_approving_review_count: 3
    dismiss_stale: false
    require_code_owner_reviews: true
    dismissal_restrictions:
      users:
      - bob
      - jane
      teams:
      - oncall
      - sres
  restrictions:
    teams:
    - config-team
    users:
    - cindy
  orgs:
    all:
      required_status_checks:
        contexts:
        - org-presubmit
      restrictions:
        teams:
        - org-team
`,
			expected: []requirements{
				{
					Org:    "all",
					Repo:   "modern",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &yes,
						RequiredStatusChecks: &github.RequiredStatusChecks{
							Strict:   true,
							Contexts: []string{"config-presubmit", "org-presubmit"},
						},
						RequiredPullRequestReviews: &github.RequiredPullRequestReviews{
							DismissStaleReviews:          false,
							RequireCodeOwnerReviews:      true,
							RequiredApprovingReviewCount: 3,
							DismissalRestrictions: github.Restrictions{
								Users: &[]string{"bob", "jane"},
								Teams: &[]string{"oncall", "sres"},
							},
						},
						Restrictions: &github.Restrictions{
							Users: &[]string{"cindy"},
							Teams: &[]string{"config-team", "org-team"},
						},
					},
				},
			},
		},
		{
			name:     "child cannot disable parent policy by default",
			branches: []string{"parent/child=unprotected"},
			config: `
branch-protection:
  protect: true
  enforce_admins: true
  orgs:
    parent:
      protect: false
`,
			errors: 1,
		},
		{
			name:     "child disables parent",
			branches: []string{"parent/child=unprotected"},
			config: `
branch-protection:
  allow_disabled_policies: true
  protect: true
  enforce_admins: true
  orgs:
    parent:
      protect: false
`,
			expected: []requirements{
				{
					Org:    "parent",
					Repo:   "child",
					Branch: "unprotected",
				},
			},
		},
		{
			name:     "do not unprotect unprotected",
			branches: []string{"protect/update=master", "unprotected/skip=master"},
			config: `
branch-protection:
  protect: true
  orgs:
    protect:
      protect: true
    unprotected:
      protect: false
`,
			startUnprotected: true,
			expected: []requirements{
				{
					Org:     "protect",
					Repo:    "update",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{},
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
				branches[k] = append(branches[k], github.Branch{
					Name:      branch,
					Protected: !tc.startUnprotected,
				})
				r := repos[org]
				if r == nil {
					repos[org] = make(map[string]bool)
				}
				repos[org][repo] = true
			}
			fc := fakeClient{
				branches: branches,
				repos:    map[string][]github.Repo{},
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
			p := protector{
				client:         &fc,
				cfg:            &cfg,
				errors:         Errors{},
				updates:        make(chan requirements),
				done:           make(chan []error),
				completedRepos: make(map[string]bool),
			}
			go func() {
				p.protect()
				close(p.updates)
			}()

			var actual []requirements
			for r := range p.updates {
				actual = append(actual, r)
			}
			errors := p.errors.errs
			if len(errors) != tc.errors {
				t.Errorf("actual errors %d != expected %d: %v", len(errors), tc.errors, errors)
			}
			switch {
			case len(actual) != len(tc.expected):
				t.Errorf("%+v %+v", cfg.BranchProtection, actual)
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
								t.Errorf("actual != expected: %s", diff.ObjectDiff(a.Request, e.Request))
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

func fixup(r *requirements) {
	if r == nil || r.Request == nil {
		return
	}
	req := r.Request
	if req.RequiredStatusChecks != nil {
		sort.Strings(req.RequiredStatusChecks.Contexts)
	}
	if restr := req.Restrictions; restr != nil {
		sort.Strings(*restr.Teams)
		sort.Strings(*restr.Users)
	}
}

func TestIgnoreArchivedRepos(t *testing.T) {
	repos := map[string]map[string]bool{}
	branches := map[string][]github.Branch{}
	org, repo, branch := "organization", "repository", "branch"
	k := org + "/" + repo
	branches[k] = append(branches[k], github.Branch{
		Name: branch,
	})
	r := repos[org]
	if r == nil {
		repos[org] = make(map[string]bool)
	}
	repos[org][repo] = true
	fc := fakeClient{
		branches: branches,
		repos:    map[string][]github.Repo{},
	}
	for org, r := range repos {
		for rname := range r {
			fc.repos[org] = append(fc.repos[org], github.Repo{Name: rname, FullName: org + "/" + rname, Archived: true})
		}
	}

	var cfg config.Config
	if err := yaml.Unmarshal([]byte(`
branch-protection:
  protect-by-default: true
  orgs:
    organization:
`), &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	p := protector{
		client:         &fc,
		cfg:            &cfg,
		errors:         Errors{},
		updates:        make(chan requirements),
		done:           make(chan []error),
		completedRepos: make(map[string]bool),
	}
	go func() {
		p.protect()
		close(p.updates)
	}()

	protectionErrors := p.errors.errs
	if len(protectionErrors) != 0 {
		t.Errorf("expected no errors, got %d errors: %v", len(protectionErrors), protectionErrors)
	}
	var actual []requirements
	for r := range p.updates {
		actual = append(actual, r)
	}
	if len(actual) != 0 {
		t.Errorf("expected no updates, got: %v", actual)
	}
}

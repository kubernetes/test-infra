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
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
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
				config: configflagutil.ConfigOptions{
					ConfigPath: "dummy",
				},
				github: flagutil.GitHubOptions{TokenPath: "fake"},
			},
			expectedErr: false,
		},
		{
			name: "no config",
			opt: options{
				github: flagutil.GitHubOptions{TokenPath: "fake"},
			},
			expectedErr: true,
		},
		{
			name: "no token, allow",
			opt: options{
				config: configflagutil.ConfigOptions{
					ConfigPath: "dummy",
				},
			},
			expectedErr: false,
		},
		{
			name: "override default tokens allowed",
			opt: options{
				config: configflagutil.ConfigOptions{
					ConfigPath: "dummy",
				},
				tokens:     5000,
				tokenBurst: 200,
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
	repos             map[string][]github.Repo
	branches          map[string][]github.Branch
	deleted           map[string]bool
	updated           map[string]github.BranchProtectionRequest
	branchProtections map[string]github.BranchProtection
	collaborators     []github.User
	teams             []github.Team
}

func (c fakeClient) GetRepo(org string, repo string) (github.FullRepo, error) {
	r, ok := c.repos[org]
	if !ok {
		return github.FullRepo{}, fmt.Errorf("Unknown org: %s", org)
	}
	for _, item := range r {
		if item.Name == repo {
			return github.FullRepo{Repo: item}, nil
		}
	}
	return github.FullRepo{}, fmt.Errorf("Unknown repo: %s", repo)
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

func (c *fakeClient) GetBranchProtection(org, repo, branch string) (*github.BranchProtection, error) {
	ctx := org + "/" + repo + "=" + branch
	if bp, ok := c.branchProtections[ctx]; ok {
		return &bp, nil
	}
	return nil, nil
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

func (c *fakeClient) ListCollaborators(org, repo string) ([]github.User, error) {
	return c.collaborators, nil
}

func (c *fakeClient) ListRepoTeams(org, repo string) ([]github.Team, error) {
	return c.teams, nil
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
	no := false

	cases := []struct {
		name                   string
		branches               []string
		startUnprotected       bool
		config                 string
		archived               string
		expected               []requirements
		branchProtections      map[string]github.BranchProtection
		collaborators          []github.User
		teams                  []github.Team
		skipVerifyRestrictions bool
		errors                 int
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
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "branch",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo2",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
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
					Org:    "this",
					Repo:   "yes",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
				{
					Org:     "that",
					Repo:    "no",
					Branch:  "master",
					Request: nil,
				},
			},
			branchProtections: map[string]github.BranchProtection{"that/no=master": {}},
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
						EnforceAdmins: &no,
						RequiredStatusChecks: &github.RequiredStatusChecks{
							Contexts: []string{"hello-world"},
						},
					},
				},
				{
					Org:    "kubernetes",
					Repo:   "publishing-bot",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
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
					Org:    "org",
					Repo:   "repo1",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
				{
					Org:    "org",
					Repo:   "repo1",
					Branch: "branch",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
				{
					Org:     "org",
					Repo:    "skip",
					Branch:  "master",
					Request: nil,
				},
			},
			branchProtections: map[string]github.BranchProtection{"org/skip=master": {}},
		},
		{
			name:     "protect org but skip a repo due to archival",
			branches: []string{"org/repo1=master", "org/repo1=branch", "org/skip=master"},
			config: `
branch-protection:
  protect: false
  orgs:
    org:
      protect: true
`,
			archived: "skip",
			expected: []requirements{
				{
					Org:    "org",
					Repo:   "repo1",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
				{
					Org:    "org",
					Repo:   "repo1",
					Branch: "branch",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
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
						EnforceAdmins: &no,
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
						EnforceAdmins: &no,
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
			teams: []github.Team{
				{
					Slug:       "config-team",
					Permission: github.RepoPush,
				},
				{
					Slug:       "org-team",
					Permission: github.RepoPush,
				},
				{
					Slug:       "repo-team",
					Permission: github.RepoPush,
				},
				{
					Slug:       "branch-team",
					Permission: github.RepoPush,
				},
			},
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
						EnforceAdmins: &no,
						Restrictions: &github.RestrictionsRequest{
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
			collaborators: []github.User{
				{
					Login:       "cindy",
					Permissions: github.RepoPermissions{Push: true},
				},
			},
			teams: []github.Team{
				{
					Slug:       "config-team",
					Permission: github.RepoPush,
				},
				{
					Slug:       "org-team",
					Permission: github.RepoPush,
				},
			},
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
						RequiredPullRequestReviews: &github.RequiredPullRequestReviewsRequest{
							DismissStaleReviews:          false,
							RequireCodeOwnerReviews:      true,
							RequiredApprovingReviewCount: 3,
							DismissalRestrictions: github.RestrictionsRequest{
								Users: &[]string{"bob", "jane"},
								Teams: &[]string{"oncall", "sres"},
							},
						},
						Restrictions: &github.RestrictionsRequest{
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
			branchProtections: map[string]github.BranchProtection{"parent/child=unprotected": {}},
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
					Org:    "protect",
					Repo:   "update",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
			},
		},
		{
			name:     "do not make update request if the branch is already up-to-date",
			branches: []string{"kubernetes/test-infra=master"},
			collaborators: []github.User{
				{
					Login:       "cindy",
					Permissions: github.RepoPermissions{Push: true},
				},
			},
			teams: []github.Team{
				{
					Slug:       "config-team",
					Permission: github.RepoPush,
				},
			},
			config: `
branch-protection:
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
  protect: true
  orgs:
    kubernetes:
      repos:
        test-infra:
`,
			branchProtections: map[string]github.BranchProtection{
				"kubernetes/test-infra=master": {
					EnforceAdmins: github.EnforceAdmins{Enabled: true},
					RequiredStatusChecks: &github.RequiredStatusChecks{
						Strict:   true,
						Contexts: []string{"config-presubmit"},
					},
					RequiredPullRequestReviews: &github.RequiredPullRequestReviews{
						DismissStaleReviews:          false,
						RequireCodeOwnerReviews:      true,
						RequiredApprovingReviewCount: 3,
						DismissalRestrictions: &github.Restrictions{
							Users: []github.User{{Login: "bob"}, {Login: "jane"}},
							Teams: []github.Team{{Slug: "oncall"}, {Slug: "sres"}},
						},
					},
					Restrictions: &github.Restrictions{
						Users: []github.User{{Login: "cindy"}},
						Teams: []github.Team{{Slug: "config-team"}},
					},
				},
			},
		},
		{
			name:     "make request if branch protection is present, but out of date",
			branches: []string{"kubernetes/test-infra=master"},
			config: `
branch-protection:
  enforce_admins: true
  required_pull_request_reviews:
    required_approving_review_count: 3
  protect: true
  orgs:
    kubernetes:
      repos:
        test-infra:
`,
			branchProtections: map[string]github.BranchProtection{
				"kubernetes/test-infra=master": {
					EnforceAdmins: github.EnforceAdmins{Enabled: true},
					RequiredStatusChecks: &github.RequiredStatusChecks{
						Strict:   true,
						Contexts: []string{"config-presubmit"},
					},
				},
			},
			expected: []requirements{
				{
					Org:    "kubernetes",
					Repo:   "test-infra",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &yes,
						RequiredPullRequestReviews: &github.RequiredPullRequestReviewsRequest{
							RequiredApprovingReviewCount: 3,
						},
					},
				},
			},
		},
		{
			name:     "excluded branches are not protected",
			branches: []string{"kubernetes/test-infra=master", "kubernetes/test-infra=skip"},
			config: `
branch-protection:
  protect: true
  orgs:
    kubernetes:
      repos:
        test-infra:
          exclude:
          - sk.*
`,

			expected: []requirements{
				{
					Org:     "kubernetes",
					Repo:    "test-infra",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{EnforceAdmins: &no},
				},
			},
		},
		{
			name:     "org and repo level branch exclusions are combined",
			branches: []string{"kubernetes/test-infra=master", "kubernetes/test-infra=skip", "kubernetes/test-infra=foobar1"},
			config: `
branch-protection:
  protect: true
  orgs:
    kubernetes:
      exclude:
      - foo.*
      repos:
        test-infra:
          exclude:
          - sk.*
`,
			expected: []requirements{
				{
					Org:     "kubernetes",
					Repo:    "test-infra",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{EnforceAdmins: &no},
				},
			},
		},
		{
			name:     "explicitly specified branches are not affected by Exclude",
			branches: []string{"kubernetes/test-infra=master"},
			config: `
branch-protection:
  protect: true
  orgs:
    kubernetes:
      exclude:
      - master.*
      repos:
        test-infra:
          branches:
            master:
`,
			expected: []requirements{
				{
					Org:     "kubernetes",
					Repo:    "test-infra",
					Branch:  "master",
					Request: &github.BranchProtectionRequest{EnforceAdmins: &no},
				},
			},
		},
		{
			name:     "do not make update request if the team or collaborator is not authorized",
			branches: []string{"org/unauthorized-collaborator=master", "org/unauthorized-team=master"},
			config: `
branch-protection:
  protect: true
  orgs:
    org:
      repos:
        unauthorized-collaborator:
          restrictions:
            users:
            - cindy
        unauthorized-team:
          restrictions:
            teams:
            - config-team
`,
			errors: 1,
		},
		{
			name:     "make request for unauthorized collaborators/teams if the verify-restrictions feature flag is not set",
			branches: []string{"org/unauthorized=master"},
			config: `
branch-protection:
  restrictions:
    teams:
    - config-team
    users:
    - cindy
  protect: true
  orgs:
    org:
      repos:
        unauthorized:
          protect: true
`,
			skipVerifyRestrictions: true,
			expected: []requirements{
				{
					Org:    "org",
					Repo:   "unauthorized",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
						Restrictions: &github.RestrictionsRequest{
							Users: &[]string{"cindy"},
							Teams: &[]string{"config-team"},
						},
					},
				},
			},
		},
		{
			name:     "protect branches with special characters",
			branches: []string{"cfgdef/repo1=test_#123"},
			config: `
branch-protection:
  protect: true
  orgs:
    cfgdef:
`,
			expected: []requirements{
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "test_#123",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins: &no,
					},
				},
			},
		},
		{
			name:     "require linear history",
			branches: []string{"cfgdef/repo1=master", "cfgdef/repo1=branch", "cfgdef/repo2=master"},
			config: `
branch-protection:
  protect: true
  required_linear_history: true
  orgs:
    cfgdef:
`,
			expected: []requirements{
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:         &no,
						RequiredLinearHistory: true,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "branch",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:         &no,
						RequiredLinearHistory: true,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo2",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:         &no,
						RequiredLinearHistory: true,
					},
				},
			},
		},
		{
			name:     "allow force pushes",
			branches: []string{"cfgdef/repo1=master", "cfgdef/repo1=branch", "cfgdef/repo2=master"},
			config: `
branch-protection:
  protect: true
  allow_force_pushes: true
  orgs:
    cfgdef:
`,
			expected: []requirements{
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:    &no,
						AllowForcePushes: true,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "branch",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:    &no,
						AllowForcePushes: true,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo2",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:    &no,
						AllowForcePushes: true,
					},
				},
			},
		},
		{
			name:     "allow deletions",
			branches: []string{"cfgdef/repo1=master", "cfgdef/repo1=branch", "cfgdef/repo2=master"},
			config: `
branch-protection:
  protect: true
  allow_deletions: true
  orgs:
    cfgdef:
`,
			expected: []requirements{
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:  &no,
						AllowDeletions: true,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo1",
					Branch: "branch",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:  &no,
						AllowDeletions: true,
					},
				},
				{
					Org:    "cfgdef",
					Repo:   "repo2",
					Branch: "master",
					Request: &github.BranchProtectionRequest{
						EnforceAdmins:  &no,
						AllowDeletions: true,
					},
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
				branches:          branches,
				repos:             map[string][]github.Repo{},
				branchProtections: tc.branchProtections,
				collaborators:     tc.collaborators,
				teams:             tc.teams,
			}
			for org, r := range repos {
				for rname := range r {
					fc.repos[org] = append(fc.repos[org], github.Repo{Name: rname, FullName: org + "/" + rname, Archived: rname == tc.archived})
				}
			}

			var cfg config.Config
			if err := yaml.Unmarshal([]byte(tc.config), &cfg); err != nil {
				t.Fatalf("failed to parse config: %v", err)
			}
			p := protector{
				client:             &fc,
				cfg:                &cfg,
				errors:             Errors{},
				updates:            make(chan requirements),
				done:               make(chan []error),
				completedRepos:     make(map[string]bool),
				verifyRestrictions: !tc.skipVerifyRestrictions,
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
	testBranches := []string{"organization/repository=branch", "organization/archived=branch"}
	repos := map[string]map[string]bool{}
	branches := map[string][]github.Branch{}
	for _, b := range testBranches {
		org, repo, branch := split(b)
		k := org + "/" + repo
		branches[k] = append(branches[k], github.Branch{
			Name: branch,
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
			fc.repos[org] = append(fc.repos[org], github.Repo{Name: rname, FullName: org + "/" + rname, Archived: rname == "archived"})
		}
	}

	var cfg config.Config
	if err := yaml.Unmarshal([]byte(`
branch-protection:
  protect: true
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
	if len(actual) != 1 {
		t.Errorf("expected one update, got: %v", actual)
	}
}

func TestIgnorePrivateSecurityRepos(t *testing.T) {
	testBranches := []string{"organization/repository=branch", "organization/repo-ghsa-1234abcd=branch"}
	repos := map[string]map[string]bool{}
	branches := map[string][]github.Branch{}
	for _, b := range testBranches {
		org, repo, branch := split(b)
		k := org + "/" + repo
		branches[k] = append(branches[k], github.Branch{
			Name: branch,
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
			fc.repos[org] = append(fc.repos[org], github.Repo{Name: rname, FullName: org + "/" + rname, Private: true})
		}
	}

	var cfg config.Config
	if err := yaml.Unmarshal([]byte(`
branch-protection:
  protect: true
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
	if len(actual) != 1 {
		t.Errorf("expected one update, got: %v", actual)
	}
}

func TestEqualBranchProtection(t *testing.T) {
	yes := true
	var testCases = []struct {
		name     string
		state    *github.BranchProtection
		request  *github.BranchProtectionRequest
		expected bool
	}{
		{
			name:     "neither set matches",
			expected: true,
		},
		{
			name:     "request unset doesn't match",
			state:    &github.BranchProtection{},
			expected: false,
		},
		{
			name:     "state unset doesn't match",
			request:  &github.BranchProtectionRequest{},
			expected: false,
		},
		{
			name: "matching requests work",
			state: &github.BranchProtection{
				RequiredStatusChecks: &github.RequiredStatusChecks{
					Strict:   true,
					Contexts: []string{"a", "b", "c"},
				},
				EnforceAdmins: github.EnforceAdmins{
					Enabled: true,
				},
				RequiredPullRequestReviews: &github.RequiredPullRequestReviews{
					DismissStaleReviews:          true,
					RequireCodeOwnerReviews:      true,
					RequiredApprovingReviewCount: 1,
					DismissalRestrictions: &github.Restrictions{
						Users: []github.User{{Login: "user"}},
						Teams: []github.Team{{Slug: "team"}},
					},
				},
				Restrictions: &github.Restrictions{
					Users: []github.User{{Login: "user"}},
					Teams: []github.Team{{Slug: "team"}},
				},
			},
			request: &github.BranchProtectionRequest{
				RequiredStatusChecks: &github.RequiredStatusChecks{
					Strict:   true,
					Contexts: []string{"a", "b", "c"},
				},
				EnforceAdmins: &yes,
				RequiredPullRequestReviews: &github.RequiredPullRequestReviewsRequest{
					DismissStaleReviews:          true,
					RequireCodeOwnerReviews:      true,
					RequiredApprovingReviewCount: 1,
					DismissalRestrictions: github.RestrictionsRequest{
						Users: &[]string{"user"},
						Teams: &[]string{"team"},
					},
				},
				Restrictions: &github.RestrictionsRequest{
					Users: &[]string{"user"},
					Teams: &[]string{"team"},
				},
			},
			expected: true,
		},
	}

	for _, testCase := range testCases {
		if actual, expected := equalBranchProtections(testCase.state, testCase.request), testCase.expected; actual != expected {
			t.Errorf("%s: didn't compute equality correctly, expected %v got %v", testCase.name, expected, actual)
		}
	}
}

func TestEqualStatusChecks(t *testing.T) {
	var testCases = []struct {
		name     string
		state    *github.RequiredStatusChecks
		request  *github.RequiredStatusChecks
		expected bool
	}{
		{
			name:     "neither set matches",
			expected: true,
		},
		{
			name:     "request unset doesn't match",
			state:    &github.RequiredStatusChecks{},
			expected: false,
		},
		{
			name:     "state unset doesn't match",
			request:  &github.RequiredStatusChecks{},
			expected: false,
		},
		{
			name: "matching requests work",
			state: &github.RequiredStatusChecks{
				Strict:   true,
				Contexts: []string{"a", "b", "c"},
			},

			request: &github.RequiredStatusChecks{
				Strict:   true,
				Contexts: []string{"a", "b", "c"},
			},
			expected: true,
		},
		{
			name: "not matching on strict",
			state: &github.RequiredStatusChecks{
				Strict:   true,
				Contexts: []string{"a", "b", "c"},
			},

			request: &github.RequiredStatusChecks{
				Strict:   false,
				Contexts: []string{"a", "b", "c"},
			},
			expected: false,
		},
		{
			name: "not matching on contexts",
			state: &github.RequiredStatusChecks{
				Strict:   true,
				Contexts: []string{"a", "b", "d"},
			},

			request: &github.RequiredStatusChecks{
				Strict:   true,
				Contexts: []string{"a", "b", "c"},
			},
			expected: false,
		},
	}

	for _, testCase := range testCases {
		if actual, expected := equalRequiredStatusChecks(testCase.state, testCase.request), testCase.expected; actual != expected {
			t.Errorf("%s: didn't compute equality correctly, expected %v got %v", testCase.name, expected, actual)
		}
	}
}

func TestEqualStringSlices(t *testing.T) {
	var testCases = []struct {
		name     string
		state    *[]string
		request  *[]string
		expected bool
	}{
		{
			name:     "no slices",
			expected: true,
		},
		{
			name:     "a unset doesn't match",
			state:    &[]string{},
			expected: false,
		},
		{
			name:     "b unset doesn't match",
			request:  &[]string{},
			expected: false,
		},
		{
			name:     "matching slices work",
			state:    &[]string{"a", "b", "c"},
			request:  &[]string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "ordering doesn't matter",
			state:    &[]string{"a", "c", "b"},
			request:  &[]string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "unequal slices don't match",
			state:    &[]string{"a", "b"},
			request:  &[]string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "disoint slices don't match",
			state:    &[]string{"e", "f", "g"},
			request:  &[]string{"a", "b", "c"},
			expected: false,
		},
	}

	for _, testCase := range testCases {
		if actual, expected := equalStringSlices(testCase.state, testCase.request), testCase.expected; actual != expected {
			t.Errorf("%s: didn't compute equality correctly, expected %v got %v", testCase.name, expected, actual)
		}
	}
}

func TestEqualAdminEnforcement(t *testing.T) {
	yes, no := true, false
	var testCases = []struct {
		name     string
		state    github.EnforceAdmins
		request  *bool
		expected bool
	}{
		{
			name:     "unset request matches no enforcement",
			state:    github.EnforceAdmins{Enabled: false},
			expected: true,
		},
		{
			name:     "set request matches enforcement",
			state:    github.EnforceAdmins{Enabled: false},
			request:  &no,
			expected: true,
		},
		{
			name:     "set request doesn't match enforcement",
			state:    github.EnforceAdmins{Enabled: false},
			request:  &yes,
			expected: false,
		},
	}

	for _, testCase := range testCases {
		if actual, expected := equalAdminEnforcement(testCase.state, testCase.request), testCase.expected; actual != expected {
			t.Errorf("%s: didn't compute equality correctly, expected %v got %v", testCase.name, expected, actual)
		}
	}
}

func TestEqualRequiredPullRequestReviews(t *testing.T) {
	var testCases = []struct {
		name     string
		state    *github.RequiredPullRequestReviews
		request  *github.RequiredPullRequestReviewsRequest
		expected bool
	}{
		{
			name:     "neither set matches",
			expected: true,
		},
		{
			name:     "request unset doesn't match",
			state:    &github.RequiredPullRequestReviews{},
			expected: false,
		},
		{
			name:     "state unset doesn't match",
			request:  &github.RequiredPullRequestReviewsRequest{},
			expected: false,
		},
		{
			name: "matching requests work",
			state: &github.RequiredPullRequestReviews{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
				DismissalRestrictions: &github.Restrictions{
					Users: []github.User{{Login: "user"}},
					Teams: []github.Team{{Slug: "team"}},
				},
			},
			request: &github.RequiredPullRequestReviewsRequest{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
				DismissalRestrictions: github.RestrictionsRequest{
					Users: &[]string{"user"},
					Teams: &[]string{"team"},
				},
			},
			expected: true,
		},
		{
			name: "not matching on dismissal",
			state: &github.RequiredPullRequestReviews{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
			},
			request: &github.RequiredPullRequestReviewsRequest{
				DismissStaleReviews:          false,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
			},
			expected: false,
		},
		{
			name: "not matching on reviews",
			state: &github.RequiredPullRequestReviews{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
			},
			request: &github.RequiredPullRequestReviewsRequest{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      false,
				RequiredApprovingReviewCount: 1,
			},
			expected: false,
		},
		{
			name: "not matching on count",
			state: &github.RequiredPullRequestReviews{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
			},
			request: &github.RequiredPullRequestReviewsRequest{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 2,
			},
			expected: false,
		},
		{
			name: "not matching on restrictions",
			state: &github.RequiredPullRequestReviews{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
				DismissalRestrictions: &github.Restrictions{
					Users: []github.User{{Login: "user"}},
					Teams: []github.Team{{Slug: "team"}},
				},
			},
			request: &github.RequiredPullRequestReviewsRequest{
				DismissStaleReviews:          true,
				RequireCodeOwnerReviews:      true,
				RequiredApprovingReviewCount: 1,
				DismissalRestrictions: github.RestrictionsRequest{
					Users: &[]string{"other"},
					Teams: &[]string{"team"},
				},
			},
			expected: false,
		},
	}

	for _, testCase := range testCases {
		if actual, expected := equalRequiredPullRequestReviews(testCase.state, testCase.request), testCase.expected; actual != expected {
			t.Errorf("%s: didn't compute equality correctly, expected %v got %v", testCase.name, expected, actual)
		}
	}
}

func TestEqualRestrictions(t *testing.T) {
	var testCases = []struct {
		name     string
		state    *github.Restrictions
		request  *github.RestrictionsRequest
		expected bool
	}{
		{
			name:     "neither set matches",
			expected: true,
		},
		{
			name:     "request unset doesn't match",
			state:    &github.Restrictions{},
			expected: false,
		},
		{
			name: "matching requests work",
			state: &github.Restrictions{
				Users: []github.User{{Login: "user"}},
				Teams: []github.Team{{Slug: "team"}},
			},
			request: &github.RestrictionsRequest{
				Users: &[]string{"user"},
				Teams: &[]string{"team"},
			},
			expected: true,
		},
		{
			name: "user login casing is ignored",
			state: &github.Restrictions{
				Users: []github.User{{Login: "User"}, {Login: "OTHer"}},
				Teams: []github.Team{{Slug: "team"}},
			},
			request: &github.RestrictionsRequest{
				Users: &[]string{"uSer", "oThER"},
				Teams: &[]string{"team"},
			},
			expected: true,
		},
		{
			name: "not matching on users",
			state: &github.Restrictions{
				Users: []github.User{{Login: "user"}},
				Teams: []github.Team{{Slug: "team"}},
			},
			request: &github.RestrictionsRequest{
				Users: &[]string{"other"},
				Teams: &[]string{"team"},
			},
			expected: false,
		},
		{
			name: "not matching on team",
			state: &github.Restrictions{
				Users: []github.User{{Login: "user"}},
				Teams: []github.Team{{Slug: "team"}},
			},
			request: &github.RestrictionsRequest{
				Users: &[]string{"user"},
				Teams: &[]string{"other"},
			},
			expected: false,
		},
		{
			name:     "both unset",
			request:  &github.RestrictionsRequest{},
			expected: true,
		},
		{
			name: "partially unset",
			request: &github.RestrictionsRequest{
				Teams: &[]string{"team"},
			},
			expected: false,
		},
	}

	for _, testCase := range testCases {
		if actual, expected := equalRestrictions(testCase.state, testCase.request), testCase.expected; actual != expected {
			t.Errorf("%s: didn't compute equality correctly, expected %v got %v", testCase.name, expected, actual)
		}
	}
}

func TestValidateRequest(t *testing.T) {
	var testCases = []struct {
		name          string
		request       *github.BranchProtectionRequest
		collaborators []string
		teams         []string
		errs          []error
	}{
		{
			name: "restrict to unathorized collaborator results in error",
			request: &github.BranchProtectionRequest{
				Restrictions: &github.RestrictionsRequest{
					Users: &[]string{"foo"},
				},
			},
			errs: []error{fmt.Errorf("the following collaborators are not authorized for %s/%s: [%s]", "org", "repo", "foo")},
		},
		{
			name: "restrict to unauthorized team results in error",
			request: &github.BranchProtectionRequest{
				Restrictions: &github.RestrictionsRequest{
					Teams: &[]string{"bar"},
				},
			},
			errs: []error{fmt.Errorf("the following teams are not authorized for %s/%s: [%s]", "org", "repo", "bar")},
		},
		{
			name: "authorized user and team result in no errors",
			request: &github.BranchProtectionRequest{
				Restrictions: &github.RestrictionsRequest{
					Users: &[]string{"foo"},
					Teams: &[]string{"bar"},
				},
			},
			collaborators: []string{"foo"},
			teams:         []string{"bar"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateRestrictions("org", "repo", tc.request, tc.collaborators, tc.teams)
			if !reflect.DeepEqual(errs, tc.errs) {
				t.Errorf("%s: errors %v != expected %v", tc.name, errs, tc.errs)
			}
		})
	}
}

func TestAuthorizedCollaborators(t *testing.T) {
	var testCases = []struct {
		name          string
		collaborators []github.User
		expected      []string
	}{
		{
			name: "Collaborator with pull permission is not included",
			collaborators: []github.User{
				{
					Login: "foo",
					Permissions: github.RepoPermissions{
						Pull: true,
					},
				},
			},
		},
		{
			name: "Collaborators with Push or Admin permission are included",
			collaborators: []github.User{
				{
					Login: "foo",
					Permissions: github.RepoPermissions{
						Push: true,
					},
				},
				{
					Login: "bar",
					Permissions: github.RepoPermissions{
						Admin: true,
					},
				},
			},
			expected: []string{"foo", "bar"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fc := fakeClient{collaborators: tc.collaborators}
			p := protector{
				client: &fc,
				errors: Errors{},
			}

			collaborators, err := p.authorizedCollaborators("org", "repo")
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			sort.Strings(tc.expected)
			sort.Strings(collaborators)
			if !reflect.DeepEqual(tc.expected, collaborators) {
				t.Errorf("expected: %v, got: %v", tc.expected, collaborators)
			}
		})
	}
}

func TestAuthorizedTeams(t *testing.T) {
	var testCases = []struct {
		name     string
		teams    []github.Team
		expected []string
	}{
		{
			name: "Team with pull permission is not included",
			teams: []github.Team{
				{
					Slug:       "foo",
					Permission: github.RepoPull,
				},
			},
		},
		{
			name: "Teams with Push or Admin permission are included",
			teams: []github.Team{
				{
					Slug:       "foo",
					Permission: github.RepoPush,
				},
				{
					Slug:       "bar",
					Permission: github.RepoAdmin,
				},
			},
			expected: []string{"foo", "bar"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fc := fakeClient{teams: tc.teams}
			p := protector{
				client: &fc,
				errors: Errors{},
			}

			teams, err := p.authorizedTeams("org", "repo")
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			sort.Strings(tc.expected)
			sort.Strings(teams)
			if !reflect.DeepEqual(tc.expected, teams) {
				t.Errorf("expected: %v, got: %v", tc.expected, teams)
			}
		})
	}
}

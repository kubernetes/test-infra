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

package branchprotection

import "errors"

// BranchProtection declares a branch protection configuration for a related set of github orgs and its repos and branches.
//
// Policies may be set a top-level, org, repo or branch level.
// Merging policies from a parent to child level:
// - nil: inherit the parent policy.
// - int/bool: replaces any parent policy if set at child level
// - []string: append child and parent items
//
// Github documentation of settings:
// * https://developer.github.com/v3/repos/branches/#update-branch-protection for details
// * https://help.github.com/articles/about-protected-branches/
type BranchProtection struct {
	// Orgs lists each org to configure (and any org-level extensions/overrides)
	Orgs map[string]Org `json:"orgs,omitempty"`

	Policy
	// ProtectTested will always set Protect = true for branches with prow jobs.
	ProtectTested bool `json:"protect-tested-repos,omitempty"`
}

// Org specifies default policies to apply to all repos in an org
type Org struct {
	// Repos extends/overrides org policies for a given repo
	Repos map[string]Repo `json:"repos,omitempty"`

	Policy
}

// Repo specifies default policies to apply to all branches in a repo
type Repo struct {
	// Branches extends/overrides org
	// TODO(fejta): replace with map[string]Policy after deprecation period
	Branches map[string]Branch `json:"branches,omitempty"`

	Policy
}

// Branch is a Policy including deprecated fields. See Policy.
type Branch struct {
	Policy
}

// Policy for the config/org/repo/branch.
// When merging policies, a nil value results in inheriting the parent policy.
type Policy struct {
	deprecatedPolicy
	// Protect overrides whether branch protection is enabled if set.
	Protect *bool `json:"protect,omitempty"`
	// ContextPolicy configures github contexts
	*ContextPolicy `json:"required_status_checks,omitempty"`
	// Admins overrides whether protections apply to admins if set.
	Admins *bool `json:"enforce_admins,omitempty"`
	// Restrictions limits who can merge
	*Restrictions `json:"restrictions,omitempty"`
	// ReviewPolicy specifies github approval/review criteria.
	*ReviewPolicy `json:"required_pull_request_reviews,omitempty"`
}

// defined returns true if any policy options are set
func (p Policy) defined() bool {
	return p.Protect != nil || p.ContextPolicy != nil || p.Admins != nil || p.Restrictions != nil || p.ReviewPolicy != nil
}

// Make returns the modern policy, converting any deprecated fields.
// If the policy uses any deprecated fields it returns true
// An error is returned if both deprecated and non-deprecated fields are set.
func (p Policy) MakePolicy() (*Policy, bool, error) {
	dep := p.deprecatedPolicy.modernize()

	if dep == nil {
		if !p.defined() {
			return nil, false, nil
		}
		return &p, false, nil
	}

	if p.defined() {
		return nil, true, errors.New("Cannot combine current with deprecated protect-by-default, require-contexts, allow-push fields")
	}

	return dep, true, nil
}

// ContextPolicy configures required github contexts.
// When merging policies, contexts are appended to context list from parent.
// Strict determines whether merging to the branch invalidates existing contexts.
type ContextPolicy struct {
	// Contexts appends required contexts that must be green to merge
	Contexts []string `json:"contexts,omitempty"`
	// Strict overrides whether new commits in the base branch require updating the PR if set
	Strict *bool `json:"strict,omitempty"`
}

// ReviewPolicy specifies github approval/review criteria.
// Any nil values inherit the policy from the parent, otherwise bool/ints are overridden.
// Non-empty lists are appended to parent lists.
type ReviewPolicy struct {
	// Restrictions appends users/teams that are allowed to merge
	*Restrictions `json:"dismissal_restrictions,omitempty"`
	// DismissStale overrides whether new commits automatically dismiss old reviews if set
	DismissStale *bool `json:"dismiss_stale_reviews,omitempty"`
	// RequireOwners overrides whether CODEOWNERS must approve PRs if set
	RequireOwners *bool `json:"require_code_owner_reviews,omitempty"`
	// Approvals overrides the number of approvals required if set (set to 0 to disable)
	Approvals *int `json:"required_approving_review_count,omitempty"`
}

// Restrictions limits who can merge
// Users and Teams items are appended to parent lists.
type Restrictions struct {
	Users []string `json:"users"`
	Teams []string `json:"teams"`
}

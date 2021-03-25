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
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Policy for the config/org/repo/branch.
// When merging policies, a nil value results in inheriting the parent policy.
type Policy struct {
	// Protect overrides whether branch protection is enabled if set.
	Protect *bool `json:"protect,omitempty"`
	// RequiredStatusChecks configures github contexts
	RequiredStatusChecks *ContextPolicy `json:"required_status_checks,omitempty"`
	// Admins overrides whether protections apply to admins if set.
	Admins *bool `json:"enforce_admins,omitempty"`
	// Restrictions limits who can merge
	Restrictions *Restrictions `json:"restrictions,omitempty"`
	// RequiredPullRequestReviews specifies github approval/review criteria.
	RequiredPullRequestReviews *ReviewPolicy `json:"required_pull_request_reviews,omitempty"`
	// RequiredLinearHistory enforces a linear commit Git history, which prevents anyone from pushing merge commits to a branch.
	RequiredLinearHistory *bool `json:"required_linear_history,omitempty"`
	// AllowForcePushes permits force pushes to the protected branch by anyone with write access to the repository.
	AllowForcePushes *bool `json:"allow_force_pushes,omitempty"`
	// AllowDeletions allows deletion of the protected branch by anyone with write access to the repository.
	AllowDeletions *bool `json:"allow_deletions,omitempty"`
	// Exclude specifies a set of regular expressions which identify branches
	// that should be excluded from the protection policy
	Exclude []string `json:"exclude,omitempty"`
}

func (p Policy) defined() bool {
	return p.Protect != nil || p.RequiredStatusChecks != nil || p.Admins != nil || p.Restrictions != nil || p.RequiredPullRequestReviews != nil ||
		p.RequiredLinearHistory != nil || p.AllowForcePushes != nil || p.AllowDeletions != nil
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
	DismissalRestrictions *Restrictions `json:"dismissal_restrictions,omitempty"`
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
	Users []string `json:"users,omitempty"`
	Teams []string `json:"teams,omitempty"`
}

// selectInt returns the child if set, else parent
func selectInt(parent, child *int) *int {
	if child != nil {
		return child
	}
	return parent
}

// selectBool returns the child argument if set, otherwise the parent
func selectBool(parent, child *bool) *bool {
	if child != nil {
		return child
	}
	return parent
}

// unionStrings merges the parent and child items together
func unionStrings(parent, child []string) []string {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	s := sets.NewString(parent...)
	s.Insert(child...)
	return s.List()
}

func mergeContextPolicy(parent, child *ContextPolicy) *ContextPolicy {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	return &ContextPolicy{
		Contexts: unionStrings(parent.Contexts, child.Contexts),
		Strict:   selectBool(parent.Strict, child.Strict),
	}
}

func mergeReviewPolicy(parent, child *ReviewPolicy) *ReviewPolicy {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	return &ReviewPolicy{
		DismissalRestrictions: mergeRestrictions(parent.DismissalRestrictions, child.DismissalRestrictions),
		DismissStale:          selectBool(parent.DismissStale, child.DismissStale),
		RequireOwners:         selectBool(parent.RequireOwners, child.RequireOwners),
		Approvals:             selectInt(parent.Approvals, child.Approvals),
	}
}

func mergeRestrictions(parent, child *Restrictions) *Restrictions {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	return &Restrictions{
		Users: unionStrings(parent.Users, child.Users),
		Teams: unionStrings(parent.Teams, child.Teams),
	}
}

// Apply returns a policy that merges the child into the parent
func (p Policy) Apply(child Policy) Policy {
	return Policy{
		Protect:                    selectBool(p.Protect, child.Protect),
		RequiredStatusChecks:       mergeContextPolicy(p.RequiredStatusChecks, child.RequiredStatusChecks),
		Admins:                     selectBool(p.Admins, child.Admins),
		RequiredLinearHistory:      selectBool(p.RequiredLinearHistory, child.RequiredLinearHistory),
		AllowForcePushes:           selectBool(p.AllowForcePushes, child.AllowForcePushes),
		AllowDeletions:             selectBool(p.AllowDeletions, child.AllowDeletions),
		Restrictions:               mergeRestrictions(p.Restrictions, child.Restrictions),
		RequiredPullRequestReviews: mergeReviewPolicy(p.RequiredPullRequestReviews, child.RequiredPullRequestReviews),
		Exclude:                    unionStrings(p.Exclude, child.Exclude),
	}
}

// BranchProtection specifies the global branch protection policy
type BranchProtection struct {
	Policy `json:",inline"`
	// ProtectTested determines if branch protection rules are set for all repos
	// that Prow has registered jobs for, regardless of if those repos are in the
	// branch protection config.
	ProtectTested *bool `json:"protect-tested-repos,omitempty"`
	// Orgs holds branch protection options for orgs by name
	Orgs map[string]Org `json:"orgs,omitempty"`
	// AllowDisabledPolicies allows a child to disable all protection even if the
	// branch has inherited protection options from a parent.
	AllowDisabledPolicies *bool `json:"allow_disabled_policies,omitempty"`
	// AllowDisabledJobPolicies allows a branch to choose to opt out of branch protection
	// even if Prow has registered required jobs for that branch.
	AllowDisabledJobPolicies *bool `json:"allow_disabled_job_policies,omitempty"`
}

func isPolicySet(p Policy) bool {
	return !apiequality.Semantic.DeepEqual(p, Policy{})
}

func (bp *BranchProtection) merge(additional *BranchProtection) error {
	var errs []error
	if isPolicySet(bp.Policy) && isPolicySet(additional.Policy) {
		errs = append(errs, errors.New("both brachprotection configs set a top-level policy"))
	} else if isPolicySet(additional.Policy) {
		bp.Policy = additional.Policy
	}
	if bp.ProtectTested != nil && additional.ProtectTested != nil {
		errs = append(errs, errors.New("both branchprotection configs set protect-tested-repos"))
	} else if additional.ProtectTested != nil {
		bp.ProtectTested = additional.ProtectTested
	}
	if bp.AllowDisabledPolicies != nil && additional.AllowDisabledPolicies != nil {
		errs = append(errs, errors.New("both branchprotection configs set allow_disabled_policies"))
	} else if additional.AllowDisabledPolicies != nil {
		bp.AllowDisabledPolicies = additional.AllowDisabledPolicies
	}
	if bp.AllowDisabledJobPolicies != nil && additional.AllowDisabledJobPolicies != nil {
		errs = append(errs, errors.New("both branchprotection configs set allow_disabled_job_policies"))
	} else if additional.AllowDisabledJobPolicies != nil {
		bp.AllowDisabledJobPolicies = additional.AllowDisabledJobPolicies
	}
	for org := range additional.Orgs {
		if bp.Orgs == nil {
			bp.Orgs = map[string]Org{}
		}
		if isPolicySet(bp.Orgs[org].Policy) && isPolicySet(additional.Orgs[org].Policy) {
			errs = append(errs, fmt.Errorf("both branchprotection configs define a policy for org %s", org))
		} else if _, ok := additional.Orgs[org]; ok && !isPolicySet(bp.Orgs[org].Policy) {
			orgSettings := bp.Orgs[org]
			orgSettings.Policy = additional.Orgs[org].Policy
			bp.Orgs[org] = orgSettings
		}

		for repo := range additional.Orgs[org].Repos {
			if bp.Orgs[org].Repos == nil {
				orgSettings := bp.Orgs[org]
				orgSettings.Repos = map[string]Repo{}
				bp.Orgs[org] = orgSettings
			}
			if isPolicySet(bp.Orgs[org].Repos[repo].Policy) && isPolicySet(additional.Orgs[org].Repos[repo].Policy) {
				errs = append(errs, fmt.Errorf("both branchprotection configs define a policy for repo %s/%s", org, repo))
			} else if _, ok := additional.Orgs[org].Repos[repo]; ok && !isPolicySet(bp.Orgs[org].Repos[repo].Policy) {
				repoSettings := bp.Orgs[org].Repos[repo]
				repoSettings.Policy = additional.Orgs[org].Repos[repo].Policy
				bp.Orgs[org].Repos[repo] = repoSettings
			}

			for branch := range additional.Orgs[org].Repos[repo].Branches {
				if bp.Orgs[org].Repos[repo].Branches == nil {
					branchSettings := bp.Orgs[org].Repos[repo]
					branchSettings.Branches = map[string]Branch{}
					bp.Orgs[org].Repos[repo] = branchSettings
				}

				if isPolicySet(bp.Orgs[org].Repos[repo].Branches[branch].Policy) && isPolicySet(additional.Orgs[org].Repos[repo].Branches[branch].Policy) {
					errs = append(errs, fmt.Errorf("both branchprotection configs define a policy for branch %s in repo %s/%s", branch, org, repo))
				} else if _, ok := additional.Orgs[org].Repos[repo].Branches[branch]; ok && !isPolicySet(bp.Orgs[org].Repos[repo].Branches[branch].Policy) {
					branchSettings := bp.Orgs[org].Repos[repo].Branches[branch]
					branchSettings.Policy = additional.Orgs[org].Repos[repo].Branches[branch].Policy
					bp.Orgs[org].Repos[repo].Branches[branch] = branchSettings
				}
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

// GetOrg returns the org config after merging in any global policies.
func (bp BranchProtection) GetOrg(name string) *Org {
	o, ok := bp.Orgs[name]
	if ok {
		o.Policy = bp.Apply(o.Policy)
	} else {
		o.Policy = bp.Policy
	}
	return &o
}

// Org holds the default protection policy for an entire org, as well as any repo overrides.
type Org struct {
	Policy `json:",inline"`
	Repos  map[string]Repo `json:"repos,omitempty"`
}

// GetRepo returns the repo config after merging in any org policies.
func (o Org) GetRepo(name string) *Repo {
	r, ok := o.Repos[name]
	if ok {
		r.Policy = o.Apply(r.Policy)
	} else {
		r.Policy = o.Policy
	}
	return &r
}

// Repo holds protection policy overrides for all branches in a repo, as well as specific branch overrides.
type Repo struct {
	Policy   `json:",inline"`
	Branches map[string]Branch `json:"branches,omitempty"`
}

// GetBranch returns the branch config after merging in any repo policies.
func (r Repo) GetBranch(name string) (*Branch, error) {
	b, ok := r.Branches[name]
	if ok {
		b.Policy = r.Apply(b.Policy)
		if b.Protect == nil {
			return nil, errors.New("defined branch policies must set protect")
		}
	} else {
		b.Policy = r.Policy
	}
	return &b, nil
}

// Branch holds protection policy overrides for a particular branch.
type Branch struct {
	Policy `json:",inline"`
}

// GetBranchProtection returns the policy for a given branch.
//
// Handles merging any policies defined at repo/org/global levels into the branch policy.
func (c *Config) GetBranchProtection(org, repo, branch string, presubmits []Presubmit) (*Policy, error) {
	if _, present := c.BranchProtection.Orgs[org]; !present {
		return nil, nil // only consider branches in configured orgs
	}
	b, err := c.BranchProtection.GetOrg(org).GetRepo(repo).GetBranch(branch)
	if err != nil {
		return nil, err
	}

	return c.GetPolicy(org, repo, branch, *b, presubmits)
}

// GetPolicy returns the protection policy for the branch, after merging in presubmits.
func (c *Config) GetPolicy(org, repo, branch string, b Branch, presubmits []Presubmit) (*Policy, error) {
	policy := b.Policy

	// Automatically require contexts from prow which must always be present
	if prowContexts, _, _ := BranchRequirements(branch, presubmits); len(prowContexts) > 0 {
		// Error if protection is disabled
		if policy.Protect != nil && !*policy.Protect {
			if c.BranchProtection.AllowDisabledJobPolicies != nil && *c.BranchProtection.AllowDisabledJobPolicies {
				return nil, nil
			}
			return nil, fmt.Errorf("required prow jobs require branch protection")
		}
		ps := Policy{
			RequiredStatusChecks: &ContextPolicy{
				Contexts: prowContexts,
			},
		}
		// Require protection by default if ProtectTested is true
		if c.BranchProtection.ProtectTested != nil && *c.BranchProtection.ProtectTested {
			yes := true
			ps.Protect = &yes
		}
		policy = policy.Apply(ps)
	}

	if policy.Protect != nil && !*policy.Protect {
		// Ensure that protection is false => no protection settings
		var old *bool
		old, policy.Protect = policy.Protect, old
		if policy.defined() && !boolValFromPtr(c.BranchProtection.AllowDisabledPolicies) {
			return nil, fmt.Errorf("%s/%s=%s defines a policy, which requires protect: true", org, repo, branch)
		}
		policy.Protect = old
	}

	if !policy.defined() {
		return nil, nil
	}
	return &policy, nil
}

func isUnprotected(policy Policy, allowDisabledPolicies bool, hasRequiredContexts bool, allowDisabledJobPolicies bool) bool {
	if policy.Protect != nil && !*policy.Protect {
		if hasRequiredContexts && allowDisabledJobPolicies {
			return true
		}
		if allowDisabledPolicies {
			policy.Protect = nil
			if policy.defined() {
				return true
			}
		}
	}
	return false
}

func (c *Config) reposWithDisabledPolicy() []string {
	repoWarns := sets.NewString()
	for orgName, org := range c.BranchProtection.Orgs {
		for repoName := range org.Repos {
			repoPolicy := c.BranchProtection.GetOrg(orgName).GetRepo(repoName)
			if isUnprotected(repoPolicy.Policy, boolValFromPtr(c.BranchProtection.AllowDisabledPolicies), false, false) {
				repoWarns.Insert(fmt.Sprintf("%s/%s", orgName, repoName))
			}
		}
	}
	return repoWarns.List()
}

// boolValFromPtr returns the bool value from a bool pointer.
// Nil counts as false. We need the boolpointers to be able
// to differentiate unset from false in the serialization.
func boolValFromPtr(b *bool) bool {
	return b != nil && *b
}

// unprotectedBranches returns the set of names of branches
// which have protection flag set to false, but have either:
// a. a protection policy, or
// b. a required context
func (c *Config) unprotectedBranches(presubmits map[string][]Presubmit) []string {
	branchWarns := sets.NewString()
	for orgName, org := range c.BranchProtection.Orgs {
		for repoName, repo := range org.Repos {
			branches := sets.NewString()
			for branchName := range repo.Branches {
				b, err := c.BranchProtection.GetOrg(orgName).GetRepo(repoName).GetBranch(branchName)
				if err != nil {
					continue
				}
				policy, err := c.GetPolicy(orgName, repoName, branchName, *b, []Presubmit{})
				if err != nil || policy == nil {
					continue
				}
				requiredContexts, _, _ := BranchRequirements(branchName, presubmits[orgName+"/"+repoName])
				if isUnprotected(*policy, boolValFromPtr(c.BranchProtection.AllowDisabledPolicies), len(requiredContexts) > 0, boolValFromPtr(c.BranchProtection.AllowDisabledJobPolicies)) {
					branches.Insert(branchName)
				}
			}
			if branches.Len() > 0 {
				branchWarns.Insert(fmt.Sprintf("%s/%s=%s", orgName, repoName, strings.Join(branches.List(), ",")))
			}
		}
	}
	return branchWarns.List()
}

// BranchProtectionWarnings logs two sets of warnings:
// - The list of repos with unprotected branches,
// - The list of repos with disabled policies, i.e. Protect set to false,
//     because any branches not explicitly specified in the configuration will be unprotected.
func (c *Config) BranchProtectionWarnings(logger *logrus.Entry, presubmits map[string][]Presubmit) {
	if warnings := c.reposWithDisabledPolicy(); len(warnings) > 0 {

		logger.WithField("repos", strings.Join(warnings, ",")).Warn("The following repos define a policy, but have protect: false")
	}
	if warnings := c.unprotectedBranches(presubmits); len(warnings) > 0 {
		logger.WithField("repos", strings.Join(warnings, ",")).Warn("The following repos define a policy or require context(s), but have one or more branches with protect: false")
	}
}

// BranchRequirements partitions status contexts for a given org, repo branch into three buckets:
//  - contexts that are always required to be present
//  - contexts that are required, _if_ present
//  - contexts that are always optional
func BranchRequirements(branch string, jobs []Presubmit) ([]string, []string, []string) {
	var required, requiredIfPresent, optional []string
	for _, j := range jobs {
		if !j.CouldRun(branch) {
			continue
		}

		if j.ContextRequired() {
			if j.TriggersConditionally() {
				// jobs that trigger conditionally cannot be
				// required as their status may not exist on PRs
				requiredIfPresent = append(requiredIfPresent, j.Context)
			} else {
				// jobs that produce required contexts and will
				// always run should be required at all times
				required = append(required, j.Context)
			}
		} else {
			optional = append(optional, j.Context)
		}
	}
	return required, requiredIfPresent, optional
}

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
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
)

type Policy struct {
	Protect *bool `json:"protect-by-default,omitempty"`
	// TODO(fejta): add all protection options
	Contexts []string `json:"require-contexts,omitempty"`
	Pushers  []string `json:"allow-push,omitempty"`
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

// apply returns a policy that merges the child into the parent
func (parent Policy) Apply(child Policy) Policy {
	return Policy{
		Protect:  selectBool(parent.Protect, child.Protect),
		Contexts: unionStrings(parent.Contexts, child.Contexts),
		Pushers:  unionStrings(parent.Pushers, child.Pushers),
	}
}

// BranchProtection specifies the global branch protection policy
type BranchProtection struct {
	Policy
	ProtectTested bool           `json:"protect-tested-repos,omitempty"`
	Orgs          map[string]Org `json:"orgs,omitempty"`
}

type Org struct {
	Policy
	Repos map[string]Repo `json:"repos,omitempty"`
}

type Repo struct {
	Policy
	Branches map[string]Branch `json:"branches,omitempty"`
}

type Branch struct {
	Policy
}

func (c *Config) GetBranchProtection(org, repo, branch string) (*Policy, error) {
	policy := c.BranchProtection.Policy

	if o, ok := c.BranchProtection.Orgs[org]; ok {
		policy = policy.Apply(o.Policy)
		if r, ok := o.Repos[repo]; ok {
			policy = policy.Apply(r.Policy)
			if b, ok := r.Branches[branch]; ok {
				policy = policy.Apply(b.Policy)
				if policy.Protect == nil {
					return nil, fmt.Errorf("protect should not be nil")
				}
			}
		}
	} else {
		return nil, nil
	}

	// Automatically require any required prow jobs
	if prowContexts := branchRequirements(org, repo, branch, c.Presubmits); len(prowContexts) > 0 {
		// Error if protection is disabled
		if policy.Protect != nil && !*policy.Protect {
			return nil, fmt.Errorf("required prow jobs require branch protection")
		}
		ps := Policy{
			Contexts: prowContexts,
			Protect:  nil,
		}
		// Require protection by default if ProtectTested is true
		if c.BranchProtection.ProtectTested {
			yes := true
			ps.Protect = &yes
		}
		policy = policy.Apply(ps)
	}

	if policy.Protect == nil {
		return nil, nil
	}

	if policy.Protect != nil && !*policy.Protect {
		if len(policy.Contexts) > 0 {
			return nil, fmt.Errorf("required contexts requires branch protection")
		}
		if len(policy.Pushers) > 0 {
			return nil, fmt.Errorf("push restrictions requires branch protection")
		}
	}

	return &policy, nil
}

func jobRequirements(jobs []Presubmit, branch string, after bool) []string {
	var required []string
	for _, j := range jobs {
		if !j.Brancher.RunsAgainstBranch(branch) {
			continue
		}
		// Does this job require a context or have kids that might need one?
		if !after && !j.AlwaysRun && j.RunIfChanged == "" {
			continue // No
		}
		if !j.SkipReport && !j.Optional { // This job needs a context
			required = append(required, j.Context)
		}
		// Check which children require contexts
		required = append(required, jobRequirements(j.RunAfterSuccess, branch, true)...)
	}
	return required
}

func branchRequirements(org, repo, branch string, presubmits map[string][]Presubmit) []string {
	p, ok := presubmits[org+"/"+repo]
	if !ok {
		return nil
	}
	return jobRequirements(p, branch, false)
}

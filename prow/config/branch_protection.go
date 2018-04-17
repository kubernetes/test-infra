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

type BranchProtection struct {
	ProtectTested bool           `json:"protect-tested-repos,omitempty"`
	Protect       *bool          `json:"protect-by-default,omitempty"`
	Contexts      []string       `json:"require-contexts,omitempty"`
	Pushers       []string       `json:"allow-push,omitempty"`
	Orgs          map[string]Org `json:"orgs,omitempty"`
}

type Org struct {
	Protect  *bool           `json:"protect-by-default,omitempty"`
	Contexts []string        `json:"require-contexts,omitempty"`
	Pushers  []string        `json:"allow-push,omitempty"`
	Repos    map[string]Repo `json:"repos,omitempty"`
}

type Repo struct {
	Protect  *bool             `json:"protect-by-default,omitempty"`
	Contexts []string          `json:"require-contexts,omitempty"`
	Pushers  []string          `json:"allow-push,omitempty"`
	Branches map[string]Branch `json:"branches,omitempty"`
}

type Branch struct {
	Protect  *bool    `json:"protect-by-default,omitempty"`
	Contexts []string `json:"require-contexts,omitempty"`
	Pushers  []string `json:"allow-push,omitempty"`
}

func (bp BranchProtection) isSet() bool {
	switch {
	case bp.ProtectTested:
		return true
	case bp.Protect != nil:
		return true
	case len(bp.Orgs) > 0:
		return true
	case len(bp.Pushers) > 0:
		return true
	default:
		return false
	}
}

func (c *Config) GetBranchProtection(org, repo, branch string) (*Branch, error) {
	if !c.BranchProtection.isSet() {
		return nil, nil
	}

	var protect *bool
	pushers := sets.NewString()
	contexts := sets.NewString()

	update := func(b *bool, c, p []string) {
		if b != nil {
			protect = b

		}
		pushers.Insert(p...)
		contexts.Insert(c...)
	}

	if c.BranchProtection.ProtectTested {
		// Adding ProwJobs
		prowContexts := branchRequirements(org, repo, branch, c.Presubmits)
		if len(prowContexts) > 0 {
			yes := true
			update(&yes, prowContexts, nil)
		}
	}

	update(c.BranchProtection.Protect, c.BranchProtection.Contexts, c.BranchProtection.Pushers)
	if orgP, exists := c.BranchProtection.Orgs[org]; exists {
		update(orgP.Protect, orgP.Contexts, orgP.Pushers)
		if repoP, exists := orgP.Repos[repo]; exists {
			update(repoP.Protect, repoP.Contexts, repoP.Pushers)
			if branchP, exists := repoP.Branches[branch]; exists {
				update(branchP.Protect, branchP.Contexts, branchP.Pushers)
			}
		}
	}

	if protect == nil {
		return nil, fmt.Errorf("protect should not be nil")
	}

	if contexts.Len() > 0 || pushers.Len() > 0 {
		if !*protect {
			return nil, fmt.Errorf("setting pushers or contexts requires protection")
		}
	}

	return &Branch{
		Protect:  protect,
		Contexts: contexts.List(),
		Pushers:  pushers.List(),
	}, nil
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

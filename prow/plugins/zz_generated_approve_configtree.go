/*
Copyright 2020 The Kubernetes Authors.

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

package plugins

// ApproveConfigTree specifies the global generic config for a plugin.
type ApproveConfigTree struct {
	Approve
	Orgs map[string]ApproveOrg `json:"orgs,omitempty"`
}

// ApproveOrg holds the default config for an entire org, as well as any repo overrides.
type ApproveOrg struct {
	Approve
	Repos map[string]ApproveRepo `json:"repos,omitempty"`
}

// ApproveRepo holds the default config for all branches in a repo, as well as specific branch overrides.
type ApproveRepo struct {
	Approve
	Branches map[string]Approve `json:"branches,omitempty"`
}

// GetOrg returns the org config after merging in any global config.
func (t ApproveConfigTree) GetOrg(name string) *ApproveOrg {
	o, ok := t.Orgs[name]
	if ok {
		o.Approve = t.Approve.Apply(o.Approve)
	} else {
		o.Approve = t.Approve
	}
	return &o
}

// GetRepo returns the repo config after merging in any org config.
func (o ApproveOrg) GetRepo(name string) *ApproveRepo {
	r, ok := o.Repos[name]
	if ok {
		r.Approve = o.Apply(r.Approve)
	} else {
		r.Approve = o.Approve
	}
	return &r
}

// GetBranch returns the branch config after merging in any repo config.
func (r ApproveRepo) GetBranch(name string) *Approve {
	b, ok := r.Branches[name]
	if ok {
		b = r.Apply(b)
	} else {
		b = r.Approve
	}
	return &b
}

// BranchOptions returns the plugin configuration for a given org/repo/branch.
func (t *ApproveConfigTree) BranchOptions(org, repo, branch string) *Approve {
	return t.GetOrg(org).GetRepo(repo).GetBranch(branch)
}

// RepoOptions returns the plugin configuration for a given org/repo.
func (t *ApproveConfigTree) RepoOptions(org, repo string) *Approve {
	return &t.GetOrg(org).GetRepo(repo).Approve
}

// OrgOptions returns the plugin configuration for a given org.
func (t *ApproveConfigTree) OrgOptions(org string) *Approve {
	return &t.GetOrg(org).Approve
}

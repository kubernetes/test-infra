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

// GenConfigTree specifies the global generic config for a plugin.
type GenConfigTree struct {
	Gen
	Orgs map[string]GenOrg `json:"orgs,omitempty"`
}

// GenOrg holds the default config for an entire org, as well as any repo overrides.
type GenOrg struct {
	Gen
	Repos map[string]GenRepo `json:"repos,omitempty"`
}

// GenRepo holds the default config for all branches in a repo, as well as specific branch overrides.
type GenRepo struct {
	Gen
	Branches map[string]Gen `json:"branches,omitempty"`
}

// GetOrg returns the org config after merging in any global config.
func (t GenConfigTree) GetOrg(name string) *GenOrg {
	o, ok := t.Orgs[name]
	if ok {
		o.Gen = t.Apply(o.Gen)
	} else {
		o.Gen = t.Gen
	}
	return &o
}

// GetRepo returns the repo config after merging in any org config.
func (o GenOrg) GetRepo(name string) *GenRepo {
	r, ok := o.Repos[name]
	if ok {
		r.Gen = o.Apply(r.Gen)
	} else {
		r.Gen = o.Gen
	}
	return &r
}

// GetBranch returns the branch config after merging in any repo config.
func (r GenRepo) GetBranch(name string) *Gen {
	b, ok := r.Branches[name]
	if ok {
		b = r.Apply(b)
	} else {
		b = r.Gen
	}
	return &b
}

// BranchOptions returns the plugin configuration for a given org/repo/branch.
func (t *GenConfigTree) BranchOptions(org, repo, branch string) *Gen {
	return t.GetOrg(org).GetRepo(repo).GetBranch(branch)
}

// RepoOptions returns the plugin configuration for a given org/repo.
func (t *GenConfigTree) RepoOptions(org, repo string) *Gen {
	return &t.GetOrg(org).GetRepo(repo).Gen
}

// OrgOptions returns the plugin configuration for a given org.
func (t *GenConfigTree) OrgOptions(org string) *Gen {
	return &t.GetOrg(org).Gen
}

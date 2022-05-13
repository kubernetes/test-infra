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

type ProwConfig interface {
	Apply(config ProwConfig) ProwConfig
}

// ConfigTree specifies the global generic config for a plugin.
type ConfigTree[T ProwConfig] struct {
	Config T
	Orgs   map[string]Org[T] `json:"orgs,omitempty"`
}

// Org holds the default config for an entire org, as well as any repo overrides.
type Org[T ProwConfig] struct {
	Config T
	Repos  map[string]Repo[T] `json:"repos,omitempty"`
}

// Repo holds the default config for all branches in a repo, as well as specific branch overrides.
type Repo[T ProwConfig] struct {
	Config   T
	Branches map[string]T `json:"branches,omitempty"`
}

// GetOrg returns the org config after merging in any global config.
func (t ConfigTree[T]) GetOrg(name string) *Org[T] {
	o, ok := t.Orgs[name]
	if ok {
		o.Config = t.Config.Apply(o.Config).(T)
	} else {
		o.Config = t.Config
	}
	return &o
}

// GetRepo returns the repo config after merging in any org config.
func (o Org[T]) GetRepo(name string) *Repo[T] {
	r, ok := o.Repos[name]
	if ok {
		r.Config = o.Config.Apply(r.Config).(T)
	} else {
		r.Config = o.Config
	}
	return &r
}

// GetBranch returns the branch config after merging in any repo config.
func (r Repo[T]) GetBranch(name string) *T {
	b, ok := r.Branches[name]
	if ok {
		b = r.Config.Apply(b).(T)
	} else {
		b = r.Config
	}
	return &b
}

// BranchOptions returns the plugin configuration for a given org/repo/branch.
func (t *ConfigTree[T]) BranchOptions(org, repo, branch string) *T {
	return t.GetOrg(org).GetRepo(repo).GetBranch(branch)
}

// RepoOptions returns the plugin configuration for a given org/repo.
func (t *ConfigTree[T]) RepoOptions(org, repo string) *T {
	return &t.GetOrg(org).GetRepo(repo).Config
}

// OrgOptions returns the plugin configuration for a given org.
func (t *ConfigTree[T]) OrgOptions(org string) *T {
	return &t.GetOrg(org).Config
}

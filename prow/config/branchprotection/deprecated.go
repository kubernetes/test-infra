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

// deprecatedPolicy defines deprecated fields
type deprecatedPolicy struct {
	// Deprecated below
	// Protect is deprecated, please use Policy
	DeprecatedProtect *bool `json:"protect-by-default,omitempty"`
	// Contexts are deprecated, please use Policy
	DeprecatedContexts []string `json:"require-contexts,omitempty"`
	// Pusers are deprecated, please use Policy
	DeprecatedPushers []string `json:"allow-push,omitempty"`
}

// defined returns true if any deprecated fields are set
func (d deprecatedPolicy) defined() bool {
	return d.DeprecatedProtect != nil || d.DeprecatedContexts != nil || d.DeprecatedPushers != nil
}

// modernize converts the deprecatedPolicy into its modern equivalent
func (d deprecatedPolicy) modernize() *Policy {
	if !d.defined() {
		return nil
	}
	p := Policy{
		Protect: d.DeprecatedProtect,
	}
	if d.DeprecatedContexts != nil {
		p.ContextPolicy = &ContextPolicy{
			Contexts: d.DeprecatedContexts,
		}
	}
	if d.DeprecatedPushers != nil {
		p.Restrictions = &Restrictions{
			Teams: d.DeprecatedPushers,
		}
	}
	return &p
}

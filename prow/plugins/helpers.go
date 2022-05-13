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

// Apply returns a policy that merges the child into the parent
func (parent Approve) Apply(child ProwConfig) ProwConfig {
	new := Approve{
		IssueRequired:       child.(Approve).IssueRequired,
		RequireSelfApproval: selectBool(parent.RequireSelfApproval, child.(Approve).RequireSelfApproval),
		LgtmActsAsApprove:   child.(Approve).LgtmActsAsApprove,
		IgnoreReviewState:   selectBool(parent.IgnoreReviewState, child.(Approve).IgnoreReviewState),
		CommandHelpLink:     child.(Approve).CommandHelpLink,
		PrProcessLink:       child.(Approve).PrProcessLink,
	}
	return new
}

// Apply returns a policy tree that merges the child into the parent
func (parent ConfigTree[T]) Apply(child ConfigTree[T]) ConfigTree[T] {
	parent.Config = parent.Config.Apply(child.Config).(T)
	for org, childOrg := range child.Orgs {
		if parentOrg, ok := parent.Orgs[org]; ok {
			parentOrg.Config = parentOrg.Config.Apply(childOrg.Config).(T)
			for repo, childRepo := range childOrg.Repos {
				if parentRepo, ok := parentOrg.Repos[repo]; ok {
					parentRepo.Config = parentRepo.Config.Apply(childRepo.Config).(T)
					for branch, childBranch := range childRepo.Branches {
						if parentBranch, ok := parentRepo.Branches[branch]; ok {
							parentBranch.Apply(childBranch)
						} else {
							parentRepo.Branches[branch] = childBranch
						}
					}
				} else {
					parentOrg.Repos[repo] = childRepo
				}
			}
		} else {
			parent.Orgs[org] = childOrg
		}
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

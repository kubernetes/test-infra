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

// Gen is a generic type used in ConfigTree as a leaf
type Gen interface {
	Apply(Gen) Gen
}

// Apply returns a policy that merges the child into the parent
func (parent Approve) Apply(child Approve) Approve {
	new := Approve{
		IssueRequired:       child.IssueRequired,
		RequireSelfApproval: selectBool(parent.RequireSelfApproval, child.RequireSelfApproval),
		LgtmActsAsApprove:   child.LgtmActsAsApprove,
		IgnoreReviewState:   selectBool(parent.IgnoreReviewState, child.IgnoreReviewState),
		CommandHelpLink:     child.CommandHelpLink,
		PrProcessLink:       child.PrProcessLink,
	}
	return new
}

// Apply returns a policy tree that merges the child into the parent
func (parent ApproveConfigTree) Apply(child ApproveConfigTree) ApproveConfigTree {
	parent.Approve = parent.Approve.Apply(child.Approve)
	for org, childOrg := range child.Orgs {
		if parentOrg, ok := parent.Orgs[org]; ok {
			parentOrg.Approve = parentOrg.Approve.Apply(childOrg.Approve)
			for repo, childRepo := range childOrg.Repos {
				if parentRepo, ok := parentOrg.Repos[repo]; ok {
					parentRepo.Approve = parentRepo.Approve.Apply(childRepo.Approve)
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

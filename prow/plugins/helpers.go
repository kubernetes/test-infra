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

// selectBool returns the child argument if set, otherwise the parent
func selectBool(parent, child *bool) *bool {
	if child != nil {
		return child
	}
	return parent
}

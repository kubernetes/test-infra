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

import (
	"sort"

	"k8s.io/apimachinery/pkg/util/sets"
)

// MergePolicy returns a copy of the parent policy after replacing/appending any fields set in child.
func MergePolicy(parent, child *Policy) *Policy {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}

	return &Policy{
		Protect:       selectBool(parent.Protect, child.Protect),
		ContextPolicy: mergeContextPolicy(parent.ContextPolicy, child.ContextPolicy),
		Admins:        selectBool(parent.Admins, child.Admins),
		Restrictions:  mergeRestrictions(parent.Restrictions, child.Restrictions),
		ReviewPolicy:  mergeReviewPolicy(parent.ReviewPolicy, child.ReviewPolicy),
	}
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
		Restrictions:  mergeRestrictions(parent.Restrictions, child.Restrictions),
		DismissStale:  selectBool(parent.DismissStale, child.DismissStale),
		RequireOwners: selectBool(parent.RequireOwners, child.RequireOwners),
		Approvals:     selectInt(parent.Approvals, child.Approvals),
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

// selectBool returns the child if set, else parent
func selectBool(parent, child *bool) *bool {
	if child != nil {
		return child
	}
	return parent
}

// selectInt returns the child if set, else parent
func selectInt(parent, child *int) *int {
	if child != nil {
		return child
	}
	return parent
}

// unionStrings returns a new list after appending both parent and child items.
func unionStrings(parent, child []string) []string {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	s := sets.NewString(parent...)
	s.Insert(child...)
	out := s.List()
	sort.Strings(out)
	return out
}

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

package main

import (
	branchprotection "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

// makeRequest renders a branch protection policy into the corresponding GitHub api request.
func makeRequest(policy branchprotection.Policy) github.BranchProtectionRequest {
	return github.BranchProtectionRequest{
		EnforceAdmins:              makeAdmins(policy.Admins),
		RequiredPullRequestReviews: makeReviews(policy.RequiredPullRequestReviews),
		RequiredStatusChecks:       makeChecks(policy.RequiredStatusChecks),
		Restrictions:               makeRestrictions(policy.Restrictions),
	}

}

// makeAdmins returns true iff *val == true, else nil
func makeAdmins(val *bool) *bool {
	if v := makeBool(val); v {
		return &v
	}
	return nil
}

// makeBool returns true iff *val == true
func makeBool(val *bool) bool {
	return val != nil && *val
}

// makeChecks renders a ContextPolicy into the corresponding GitHub api object.
//
// Returns nil when input policy is nil.
// Otherwise returns non-nil Contexts (empty if unset) and Strict iff Strict is true
func makeChecks(cp *branchprotection.ContextPolicy) *github.RequiredStatusChecks {
	if cp == nil {
		return nil
	}
	return &github.RequiredStatusChecks{
		Contexts: append([]string{}, sets.NewString(cp.Contexts...).List()...),
		Strict:   makeBool(cp.Strict),
	}
}

// makeRestrictions renders restrictions into the corresponding GitHub api object.
//
// Returns nil when input restrictions is nil.
// Otherwise Teams and Users are both non-nil (empty list if unset)
func makeRestrictions(rp *branchprotection.Restrictions) *github.Restrictions {
	if rp == nil {
		return nil
	}
	teams := append([]string{}, sets.NewString(rp.Teams...).List()...)
	users := append([]string{}, sets.NewString(rp.Users...).List()...)
	return &github.Restrictions{
		Teams: &teams,
		Users: &users,
	}
}

// makeReviews renders review policy into the corresponding GitHub api object.
//
// Returns nil if the policy is nil, or approvals is nil or 0.
func makeReviews(rp *branchprotection.ReviewPolicy) *github.RequiredPullRequestReviews {
	switch {
	case rp == nil:
		return nil
	case rp.Approvals == nil:
		logrus.Warn("WARNING: required_pull_request_reviews policy does not specify required_approving_review_count, disabling")
		return nil
	case *rp.Approvals == 0:
		return nil
	}
	rprr := github.RequiredPullRequestReviews{
		DismissStaleReviews:          makeBool(rp.DismissStale),
		RequireCodeOwnerReviews:      makeBool(rp.RequireOwners),
		RequiredApprovingReviewCount: *rp.Approvals,
	}
	if rp.DismissalRestrictions != nil {
		rprr.DismissalRestrictions = *makeRestrictions(rp.DismissalRestrictions)
	}
	return &rprr
}

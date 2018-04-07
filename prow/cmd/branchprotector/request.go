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
	"log"

	"k8s.io/test-infra/prow/config/branchprotection"
	"k8s.io/test-infra/prow/github"
)

// makeRequest renders a branch protectio policy into the corresponding github api request.
func makeRequest(policy branchprotection.Policy) github.BranchProtectionRequest {
	return github.BranchProtectionRequest{
		EnforceAdmins:              policy.Admins,
		RequiredPullRequestReviews: makeReviews(policy.ReviewPolicy),
		RequiredStatusChecks:       makeChecks(policy.ContextPolicy),
		Restrictions:               makeRestrictions(policy.Restrictions),
	}

}

// makeChecks renders a ContextPolicy into the corresponding github api object.
func makeChecks(policy *branchprotection.ContextPolicy) *github.RequiredStatusChecks {
	if policy == nil {
		return nil
	}
	rsc := github.RequiredStatusChecks{
		Contexts: policy.Contexts,
	}
	if policy.Strict != nil {
		rsc.Strict = *policy.Strict
	}
	return &rsc
}

// makeBool returns true if b points to a true value, else false.
func makeBool(b *bool) bool {
	return b != nil && *b
}

// makeReviews renders a ReviewPolicy into the corresponding github api object.
func makeReviews(policy *branchprotection.ReviewPolicy) *github.RequiredPullRequestReviews {
	switch {
	case policy == nil:
		return nil
	case policy.Approvals == nil:
		log.Printf("WARNING: required_pull_request_reviews policy does not specify required_approving_review_count, disabling")
		return nil
	case *policy.Approvals == 0:
		return nil
	}
	rprr := github.RequiredPullRequestReviews{
		DismissStaleReviews:         makeBool(policy.DismissStale),
		RequireCodeOwnerReviews:     makeBool(policy.RequireOwners),
		RequireApprovingReviewCount: *policy.Approvals,
	}
	if policy.Restrictions != nil {
		rprr.DismissalRestrictions = *makeRestrictions(policy.Restrictions)
	}
	return &rprr
}

// makeRestrictions renders restrictions into the corresponding github api object.
func makeRestrictions(policy *branchprotection.Restrictions) *github.Restrictions {
	if policy == nil {
		return nil
	}
	return &github.Restrictions{
		Users: policy.Users,
		Teams: policy.Teams,
	}
}

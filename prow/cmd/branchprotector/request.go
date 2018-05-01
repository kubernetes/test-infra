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
)

// makeRequest renders a branch protection policy into the corresponding GitHub api request.
func makeRequest(policy branchprotection.Policy) github.BranchProtectionRequest {
	return github.BranchProtectionRequest{
		RequiredStatusChecks: makeChecks(policy.Contexts),
		Restrictions:         makeRestrictions(policy.Pushers),
	}

}

// makeChecks renders a ContextPolicy into the corresponding GitHub api object.
func makeChecks(contexts []string) *github.RequiredStatusChecks {
	if contexts == nil {
		return nil
	}
	return &github.RequiredStatusChecks{
		Contexts: contexts,
	}
}

// makeRestrictions renders restrictions into the corresponding GitHub api object.
func makeRestrictions(pushers []string) *github.Restrictions {
	if pushers == nil {
		return nil
	}
	return &github.Restrictions{
		Teams: pushers,
		Users: []string{},
	}
}

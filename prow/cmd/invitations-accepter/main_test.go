/*
Copyright 2021 The Kubernetes Authors.

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
	"testing"

	"github.com/google/go-cmp/cmp"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestAcceptInvitations(t *testing.T) {

	testCases := []struct {
		id              string
		repoInvitations map[int]github.UserRepoInvitation
		orgInvitations  map[string]github.UserOrgInvitation
	}{
		{
			id: "no invitations to accept",
		},
		{
			id: "one repo invitation to accept",
			repoInvitations: map[int]github.UserRepoInvitation{
				1: {InvitationID: 1, Repository: &github.Repo{FullName: "foo/bar"}},
			},
		},
		{
			id: "multiple repo invitations to accept",
			repoInvitations: map[int]github.UserRepoInvitation{
				1: {InvitationID: 1, Repository: &github.Repo{FullName: "foo/bar"}},
				2: {InvitationID: 2, Repository: &github.Repo{FullName: "james/bond"}},
				3: {InvitationID: 3, Repository: &github.Repo{FullName: "captain/hook"}},
			},
		},

		{
			id: "one org invitation to accept",
			orgInvitations: map[string]github.UserOrgInvitation{
				"org-1": {Org: github.UserOrganization{Login: "org-1"}},
			},
		},
		{
			id: "multiple org invitations to accept",
			orgInvitations: map[string]github.UserOrgInvitation{
				"org-1": {Org: github.UserOrganization{Login: "org-1"}},
				"org-2": {Org: github.UserOrganization{Login: "org-2"}},
				"org-3": {Org: github.UserOrganization{Login: "org-3"}},
			},
		},
		{
			id: "multiple org and repo invitations to accept",
			repoInvitations: map[int]github.UserRepoInvitation{
				1: {InvitationID: 1, Repository: &github.Repo{FullName: "foo/bar"}},
				2: {InvitationID: 2, Repository: &github.Repo{FullName: "james/bond"}},
				3: {InvitationID: 3, Repository: &github.Repo{FullName: "captain/hook"}},
			},
			orgInvitations: map[string]github.UserOrgInvitation{
				"org-1": {Org: github.UserOrganization{Login: "org-1"}},
				"org-2": {Org: github.UserOrganization{Login: "org-2"}},
				"org-3": {Org: github.UserOrganization{Login: "org-3"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			fgh := &fakegithub.FakeClient{UserRepoInvitations: tc.repoInvitations, UserOrgInvitations: tc.orgInvitations}

			if err := acceptInvitations(fgh, false); err != nil {
				t.Fatalf("error wasn't expected: %v", err)
			}

			actualOrgInvitations, err := fgh.ListCurrentUserOrgInvitations()
			if err != nil {
				t.Fatalf("error not expected: %v", err)
			}

			var orgInvitationsExpected []github.UserOrgInvitation
			if diff := cmp.Diff(actualOrgInvitations, orgInvitationsExpected); diff != "" {
				t.Fatal(diff)
			}

			actualRepoInvitations, err := fgh.ListCurrentUserRepoInvitations()
			if err != nil {
				t.Fatalf("error not expected: %v", err)
			}

			var repoInvitationsExpected []github.UserRepoInvitation
			if diff := cmp.Diff(actualRepoInvitations, repoInvitationsExpected); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

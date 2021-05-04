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
		id          string
		invitations map[int]github.UserRepoInvitation
	}{
		{
			id: "no invitations to accept",
		},
		{
			id: "one invitation to accept",
			invitations: map[int]github.UserRepoInvitation{
				1: {InvitationID: 1, Repository: &github.Repo{FullName: "foo/bar"}},
			},
		},
		{
			id: "multiple invitations per client to accept",
			invitations: map[int]github.UserRepoInvitation{
				1: {InvitationID: 1, Repository: &github.Repo{FullName: "foo/bar"}},
				2: {InvitationID: 2, Repository: &github.Repo{FullName: "james/bond"}},
				3: {InvitationID: 3, Repository: &github.Repo{FullName: "captain/hook"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			fgh := &fakegithub.FakeClient{UserRepoInvitations: tc.invitations}

			if err := acceptInvitations(fgh, false); err != nil {
				t.Fatalf("error wasn't expected: %v", err)
			}

			actualInvitations, err := fgh.ListCurrentUserRepoInvitations()
			if err != nil {
				t.Fatalf("error not expected: %v", err)
			}

			var expected []github.UserRepoInvitation
			if diff := cmp.Diff(actualInvitations, expected); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

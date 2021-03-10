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
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

type fakeInvitationStatus string

var (
	fakeNew      fakeInvitationStatus = "new"
	fakeAccepted fakeInvitationStatus = "accepted"
	fakeRejected fakeInvitationStatus = "rejected"
)

type fakeGhClient struct {
	UserInvitations    []github.UserInvitation
	Errs               []error
	InvitationStatuses map[int]fakeInvitationStatus
}

func NewFakeGhClient(userInvitations []github.UserInvitation, errs []error) *fakeGhClient {
	return &fakeGhClient{
		UserInvitations:    userInvitations,
		Errs:               errs,
		InvitationStatuses: map[int]fakeInvitationStatus{},
	}
}

func (fc *fakeGhClient) ListCurrentUserInvitations() ([]github.UserInvitation, error) {
	return fc.UserInvitations, fc.getNextError()
}

func (fc *fakeGhClient) AcceptUserInvitation(invitationID int) error {
	status := fakeNew
	err := fc.getNextError()
	if err == nil {
		status = fakeAccepted
	}
	fc.InvitationStatuses[invitationID] = status
	return err
}

func (fc *fakeGhClient) DeclineUserInvitation(invitationID int) error {
	status := fakeNew
	err := fc.getNextError()
	if err == nil {
		status = fakeRejected
	}
	fc.InvitationStatuses[invitationID] = status
	return err
}

func (fc *fakeGhClient) getNextError() error {
	var err error
	if len(fc.Errs) > 0 {
		err = fc.Errs[0]
		fc.Errs = fc.Errs[1:]
	}
	return err
}

func TestIsTrusted(t *testing.T) {
	tests := []struct {
		name    string
		orgrepo string
		trusted map[string]config.ManagedWebhookInfo
		want    bool
	}{
		{
			name:    "match org",
			orgrepo: "fake-org/fake-repo",
			trusted: map[string]config.ManagedWebhookInfo{"fake-org": {}},
			want:    true,
		},
		{
			name:    "match full",
			orgrepo: "fake-org/fake-repo",
			trusted: map[string]config.ManagedWebhookInfo{"fake-org/fake-repo": {}},
			want:    true,
		},
		{
			name:    "org not match",
			orgrepo: "fake-org/fake-repo",
			trusted: map[string]config.ManagedWebhookInfo{"fake-org2": {}},
			want:    false,
		},
		{
			name:    "repo not match",
			orgrepo: "fake-org/fake-repo",
			trusted: map[string]config.ManagedWebhookInfo{"fake-org/fake-repo2": {}},
			want:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pc := config.ProwConfig{
				ManagedWebhooks: config.ManagedWebhooks{
					OrgRepoConfig: tc.trusted,
				},
			}
			if want, got := tc.want, isTrusted("fake-org/fake-repo", &pc); want != got {
				t.Fatalf("Test repo trusted, want: %v, got: %v", want, got)
			}
		})
	}
}

func TestHandle(t *testing.T) {
	var (
		errListing   = errors.New("failed listing")
		errAccepting = errors.New("failed acception")
		errRejecting = errors.New("failed rejection")
	)
	tests := []struct {
		name         string
		ivs          []github.UserInvitation
		trusted      map[string]config.ManagedWebhookInfo
		errs         []error
		wantStatuses map[int]fakeInvitationStatus
		wantErr      error
	}{
		{
			name: "normal invitation accepted",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo1",
					},
					Permission: github.Admin,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			wantStatuses: map[int]fakeInvitationStatus{0: fakeAccepted},
			wantErr:      nil,
		},
		{
			name: "normal invitation failed acception",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo1",
					},
					Permission: github.Admin,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			errs:         []error{nil, errAccepting},
			wantStatuses: map[int]fakeInvitationStatus{0: fakeNew},
			wantErr:      errAccepting,
		},
		{
			name: "invalid invitation rejected",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo1",
					},
					Permission: github.Write,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			errs:         []error{nil, nil},
			wantStatuses: map[int]fakeInvitationStatus{0: fakeRejected},
			wantErr:      nil,
		},
		{
			name: "invalid invitation failed rejection",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo1",
					},
					Permission: github.Write,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			errs:         []error{nil, errRejecting},
			wantStatuses: map[int]fakeInvitationStatus{0: fakeNew},
			wantErr:      errRejecting,
		},
		{
			name: "success then fail",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo1",
					},
					Permission: github.Admin,
				},
				github.UserInvitation{
					InvitationID: 1,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo2",
					},
					Permission: github.Admin,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			errs:         []error{nil, nil, errAccepting},
			wantStatuses: map[int]fakeInvitationStatus{0: fakeAccepted, 1: fakeNew},
			wantErr:      errAccepting,
		},
		{
			name: "fail then success",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo1",
					},
					Permission: github.Admin,
				},
				github.UserInvitation{
					InvitationID: 1,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo2",
					},
					Permission: github.Admin,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			errs:         []error{nil, errAccepting, nil},
			wantStatuses: map[int]fakeInvitationStatus{0: fakeNew, 1: fakeAccepted},
			wantErr:      errAccepting,
		},
		{
			name: "keep last error",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo1",
					},
					Permission: github.Admin,
				},
				github.UserInvitation{
					InvitationID: 1,
					Repository: &github.Repo{
						FullName: "fake-org/fake-repo2",
					},
					Permission: github.Write,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			errs:         []error{nil, errAccepting, errRejecting},
			wantStatuses: map[int]fakeInvitationStatus{0: fakeNew, 1: fakeNew},
			wantErr:      errRejecting,
		},
		{
			name: "falied listing",
			ivs: []github.UserInvitation{
				github.UserInvitation{
					InvitationID: 0,
					Repository: &github.Repo{
						FullName: "fake-org1/fake-repo1",
					},
					Permission: github.Admin,
				},
			},
			trusted:      map[string]config.ManagedWebhookInfo{"fake-org": {}},
			errs:         []error{errListing},
			wantStatuses: map[int]fakeInvitationStatus{},
			wantErr:      errListing,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fgc := NewFakeGhClient(tc.ivs, tc.errs)
			pc := config.ProwConfig{
				ManagedWebhooks: config.ManagedWebhooks{
					OrgRepoConfig: tc.trusted,
				},
			}
			if want, got := tc.wantErr, handle(fgc, &pc); want != got {
				t.Fatalf("handle invitations, want error: %v, got error: %v", want, got)
			}
			if diff := cmp.Diff(tc.wantStatuses, fgc.InvitationStatuses); diff != "" {
				t.Fatalf("Diff error. Want(-), got(+):\n%v", diff)
			}
		})
	}
}

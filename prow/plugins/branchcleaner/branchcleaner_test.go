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

package branchcleaner

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestBranchCleaner(t *testing.T) {
	baseRepoOrg := "my-org"
	baseRepoRepo := "repo"
	baseRepoFullName := fmt.Sprintf("%s/%s", baseRepoOrg, baseRepoRepo)

	testcases := []struct {
		name                 string
		prAction             github.PullRequestEventAction
		merged               bool
		headRepoFullName     string
		branchDeleteExpected bool
	}{
		{
			name:                 "Opened PR nothing to do",
			prAction:             github.PullRequestActionOpened,
			merged:               false,
			branchDeleteExpected: false,
		},
		{
			name:                 "Closed PR unmerged nothing to do",
			prAction:             github.PullRequestActionClosed,
			merged:               false,
			branchDeleteExpected: false,
		},
		{
			name:                 "PR from different repo nothing to do",
			prAction:             github.PullRequestActionClosed,
			merged:               true,
			headRepoFullName:     "different-org/repo",
			branchDeleteExpected: false,
		},
		{
			name:                 "PR from same repo delete head ref",
			prAction:             github.PullRequestActionClosed,
			merged:               true,
			headRepoFullName:     "my-org/repo",
			branchDeleteExpected: true,
		},
	}

	mergeSHA := "abc"
	prNumber := 1

	for _, tc := range testcases {

		t.Run(tc.name, func(t *testing.T) {
			log := logrus.WithField("plugin", pluginName)
			event := github.PullRequestEvent{
				Action: tc.prAction,
				Number: prNumber,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Ref: "master",
						Repo: github.Repo{
							DefaultBranch: "master",
							FullName:      baseRepoFullName,
							Name:          baseRepoRepo,
							Owner:         github.User{Login: baseRepoOrg},
						},
					},
					Head: github.PullRequestBranch{
						Ref: "my-feature",
						Repo: github.Repo{
							FullName: tc.headRepoFullName,
						},
					},
					Merged: tc.merged},
			}
			if tc.merged {
				event.PullRequest.MergeSHA = &mergeSHA
			}

			fgc := fakegithub.NewFakeClient()
			fgc.PullRequests = map[int]*github.PullRequest{
				prNumber: {
					Number: prNumber,
				},
			}
			if err := handle(fgc, log, event); err != nil {
				t.Fatalf("error in handle: %v", err)
			}
			if tc.branchDeleteExpected != (len(fgc.RefsDeleted) == 1) {
				t.Fatalf("branchDeleteExpected: %v, refsDeleted: %d", tc.branchDeleteExpected, len(fgc.RefsDeleted))
			}

			if tc.branchDeleteExpected {
				if fgc.RefsDeleted[0].Org != event.PullRequest.Base.Repo.Owner.Login {
					t.Errorf("Expected org of deleted ref to be %s but was %s", event.PullRequest.Base.Repo.Owner.Login, fgc.RefsDeleted[0].Org)
				}
				if fgc.RefsDeleted[0].Repo != event.PullRequest.Base.Repo.Name {
					t.Errorf("Expected repo of deleted ref to be %s but was %s", baseRepoRepo, fgc.RefsDeleted[0].Repo)
				}
				expectedRefName := fmt.Sprintf("heads/%s", event.PullRequest.Head.Ref)
				if fgc.RefsDeleted[0].Ref != expectedRefName {
					t.Errorf("Expected name of deleted ref to be %s but was %s", expectedRefName, fgc.RefsDeleted[0].Ref)
				}
			}

		})

	}
}

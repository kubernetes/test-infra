/*
Copyright 2019 The Kubernetes Authors.

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

package milestoneapplier

import (
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestMilestoneApplier(t *testing.T) {
	var milestonesMap = map[string]int{"v1.0": 1, "v2.0": 2}
	testcases := []struct {
		name                string
		baseBranch          string
		prAction            github.PullRequestEventAction
		merged              bool
		previousMilestone   int
		configuredMilestone int
		expectedMilestone   int
	}{
		{
			name:                "opened PR on default branch => do nothing",
			baseBranch:          "master",
			prAction:            github.PullRequestActionOpened,
			expectedMilestone:   0,
			configuredMilestone: 1,
		},
		{
			name:                "closed (not merged) PR on default branch => do nothing",
			baseBranch:          "master",
			prAction:            github.PullRequestActionClosed,
			merged:              false,
			expectedMilestone:   0,
			configuredMilestone: 1,
		},
		{
			name:                "merged PR but has existing milestone on default branch => apply configured milestone",
			baseBranch:          "master",
			prAction:            github.PullRequestActionClosed,
			merged:              true,
			previousMilestone:   1,
			configuredMilestone: 2,
			expectedMilestone:   2,
		},
		{
			name:                "merged PR but already has configured milestone on default branch => do nothing",
			baseBranch:          "master",
			prAction:            github.PullRequestActionClosed,
			merged:              true,
			previousMilestone:   2,
			configuredMilestone: 2,
			expectedMilestone:   2,
		},
		{
			name:                "merged PR but does not have existing milestone on default branch => add milestone",
			prAction:            github.PullRequestActionClosed,
			baseBranch:          "master",
			merged:              true,
			previousMilestone:   0,
			configuredMilestone: 2,
			expectedMilestone:   2,
		},
		{
			name:                "opened PR on non-default branch => add milestone",
			baseBranch:          "release-1.0",
			prAction:            github.PullRequestActionOpened,
			configuredMilestone: 1,
			expectedMilestone:   1,
		},
		{
			name:                "synced PR on non-default branch => do nothing",
			baseBranch:          "release-1.0",
			prAction:            github.PullRequestActionSynchronize,
			previousMilestone:   0,
			configuredMilestone: 1,
			expectedMilestone:   0,
		},
		{
			name:                "closed (not merged) PR on non-default branch => do nothing",
			baseBranch:          "release-1.0",
			prAction:            github.PullRequestActionClosed,
			merged:              false,
			expectedMilestone:   0,
			configuredMilestone: 1,
		},
		{
			name:                "merged PR but has existing milestone on non-default branch => add configured milestone",
			baseBranch:          "release-1.0",
			prAction:            github.PullRequestActionClosed,
			merged:              true,
			previousMilestone:   1,
			configuredMilestone: 2,
			expectedMilestone:   2,
		},
		{
			name:                "merged PR but already has configured milestone on non-default branch => do nothing",
			baseBranch:          "release-1.0",
			prAction:            github.PullRequestActionClosed,
			merged:              true,
			previousMilestone:   2,
			configuredMilestone: 2,
			expectedMilestone:   2,
		},
		{
			name:                "merged PR but does not have existing milestone on non-default branch => add milestone",
			prAction:            github.PullRequestActionClosed,
			baseBranch:          "release-1.0",
			merged:              true,
			previousMilestone:   0,
			configuredMilestone: 1,
			expectedMilestone:   1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			basicPR := github.PullRequest{
				Number: 1,
				Base: github.PullRequestBranch{
					Repo: github.Repo{
						Owner: github.User{
							Login: "kubernetes",
						},
						Name:          "kubernetes",
						DefaultBranch: "master",
					},
					Ref: tc.baseBranch,
				},
			}

			basicPR.Merged = tc.merged
			if tc.previousMilestone != 0 {
				basicPR.Milestone = &github.Milestone{
					Number: tc.previousMilestone,
				}
			}

			event := github.PullRequestEvent{
				Action:      tc.prAction,
				Number:      basicPR.Number,
				PullRequest: basicPR,
			}

			fakeClient := fakegithub.NewFakeClient()
			fakeClient.PullRequests = map[int]*github.PullRequest{
				basicPR.Number: &basicPR,
			}
			fakeClient.MilestoneMap = milestonesMap
			fakeClient.Milestone = tc.previousMilestone

			var configuredMilestoneTitle string
			for title, number := range milestonesMap {
				if number == tc.configuredMilestone {
					configuredMilestoneTitle = title
				}
			}

			if err := handle(fakeClient, logrus.WithField("plugin", pluginName), configuredMilestoneTitle, event); err != nil {
				t.Fatalf("(%s): Unexpected error from handle: %v.", tc.name, err)
			}

			if fakeClient.Milestone != tc.expectedMilestone {
				t.Fatalf("%s: expected milestone: %d, received milestone: %d", tc.name, tc.expectedMilestone, fakeClient.Milestone)
			}
		})
	}
}

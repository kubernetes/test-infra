/*
Copyright 2016 The Kubernetes Authors.

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

package heart

import (
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/scallywag/github/fakegithub"
	"k8s.io/test-infra/prow/scallywag"
)

func TestHandlePR(t *testing.T) {
	basicPR := scallywag.PullRequest{
		Number: 1,
		Base: scallywag.PullRequestBranch{
			Repo: scallywag.Repo{
				Owner: scallywag.User{
					Login: "kubernetes",
				},
				Name: "kubernetes",
			},
		},
	}

	testcases := []struct {
		prAction              scallywag.PullRequestEventAction
		changes               []scallywag.PullRequestChange
		expectedReactionAdded bool
	}{
		// PR opened against kubernetes/kubernetes that adds 1 line to
		// an OWNERS file
		{
			prAction: scallywag.PullRequestActionOpened,
			changes: []scallywag.PullRequestChange{
				{
					Filename:  "foo/bar/OWNERS",
					Additions: 1,
				},
			},
			expectedReactionAdded: true,
		},
		// PR opened against kubernetes/kubernetes that deletes 1 line
		// from an OWNERS file
		{
			prAction: scallywag.PullRequestActionOpened,
			changes: []scallywag.PullRequestChange{
				{
					Filename:  "foo/bar/OWNERS",
					Deletions: 1,
				},
			},
			expectedReactionAdded: false,
		},
		// PR opened against kubernetes/kubernetes with no changes to
		// OWNERS
		{
			prAction: scallywag.PullRequestActionOpened,
			changes: []scallywag.PullRequestChange{
				{
					Filename:  "foo/bar/foo.go",
					Additions: 1,
				},
			},
			expectedReactionAdded: false,
		},
		// PR reopened against kubernetes/kubernetes
		{
			prAction: scallywag.PullRequestActionReopened,
			changes: []scallywag.PullRequestChange{
				{
					Filename:  "foo/bar/OWNERS",
					Additions: 1,
				},
			},
			expectedReactionAdded: false,
		},
		// PR opened against kubernetes/kubernetes that adds 1 line to
		// an OWNERS_ALIASES file
		{
			prAction: scallywag.PullRequestActionOpened,
			changes: []scallywag.PullRequestChange{
				{
					Filename:  "foo/bar/OWNERS_ALIASES",
					Additions: 1,
				},
			},
			expectedReactionAdded: true,
		},
	}

	for _, tc := range testcases {
		event := scallywag.PullRequestEvent{
			Action:      tc.prAction,
			Number:      basicPR.Number,
			PullRequest: basicPR,
		}
		fakeGitHubClient := &fakegithub.FakeClient{
			PullRequests: map[int]*scallywag.PullRequest{
				basicPR.Number: &basicPR,
			},
			PullRequestChanges: map[int][]scallywag.PullRequestChange{
				basicPR.Number: tc.changes,
			},
		}
		fakeClient := client{
			GitHubClient: fakeGitHubClient,
			Logger:       logrus.WithField("plugin", pluginName),
		}

		err := handlePR(fakeClient, event)
		if err != nil {
			t.Fatal(err)
		}

		if len(fakeGitHubClient.IssueReactionsAdded) > 0 && !tc.expectedReactionAdded {
			t.Fatalf("Expected no reactions to be added for %+v", tc)

		} else if len(fakeGitHubClient.IssueReactionsAdded) == 0 && tc.expectedReactionAdded {
			t.Fatalf("Expected reaction to be added for %+v", tc)
		}
	}
}

/*
Copyright 2023 The Kubernetes Authors.

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

package phased

import (
	"testing"

	"github.com/sirupsen/logrus"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/diff"

	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

func TestHandlePullRequest(t *testing.T) {
	var testcases = []struct {
		name string

		Author          string
		ShouldComment   bool
		prLabel         string
		prAction        github.PullRequestEventAction
		prIsDraft       bool
		HasApprove      bool
		expectedComment string
	}{
		{
			name: "user labeled PR with lgtm, PR already had approve, should comment",

			Author:          "t",
			prAction:        github.PullRequestActionLabeled,
			prLabel:         labels.LGTM,
			HasApprove:      true,
			ShouldComment:   true,
			expectedComment: "org/repo#0:/test jub\n/test jub2\n",
		},
	}
	for _, tc := range testcases {
		t.Logf("running scenario %q", tc.name)
		t.Run(tc.name, func(t *testing.T) {
			g := fakegithub.NewFakeClient()
			g.IssueComments = map[int][]github.IssueComment{}
			g.OrgMembers = map[string][]string{"org": {"t"}}
			g.PullRequests = map[int]*github.PullRequest{
				0: {
					Number: 0,
					User:   github.User{Login: tc.Author},
					Base: github.PullRequestBranch{
						Ref: "master",
						Repo: github.Repo{
							Owner: github.User{Login: "org"},
							Name:  "repo",
						},
					},
					Draft: tc.prIsDraft,
				},
			}
			fakeProwJobClient := fake.NewSimpleClientset()
			c := Client{
				GitHubClient:  g,
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs("namespace"),
				Config:        &config.Config{},
				Logger:        logrus.WithField("plugin", PluginName),
				GitClient:     nil,
			}

			presubmits := map[string][]config.Presubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "jib",
						},
						AlwaysRun: true,
					},
					{
						JobBase: config.JobBase{
							Name: "jub",
						},
						AlwaysRun: false,
						Optional:  false,
					},
					{
						JobBase: config.JobBase{
							Name: "jub2",
						},
						AlwaysRun: false,
						Optional:  false,
					},
				},
			}
			if err := c.Config.SetPresubmits(presubmits); err != nil {
				t.Fatalf("failed to set presubmits: %v", err)
			}

			if tc.HasApprove {
				g.IssueLabelsExisting = append(g.IssueLabelsExisting, issueLabels(labels.Approved)...)
			}

			pr := github.PullRequestEvent{
				Action: tc.prAction,
				Label:  github.Label{Name: tc.prLabel},
				PullRequest: github.PullRequest{
					Number: 0,
					User:   github.User{Login: tc.Author},
					Base: github.PullRequestBranch{
						Ref: "master",
						Repo: github.Repo{
							Owner:    github.User{Login: "org"},
							Name:     "repo",
							FullName: "org/repo",
						},
					},
					Draft: tc.prIsDraft,
				},
				Sender: github.User{
					Login: tc.Author,
				},
			}
			trigger := plugins.Trigger{
				TrustedOrg:     "org",
				OnlyOrgMembers: true,
			}
			trigger.SetDefaults()
			if err := handlePR(c, trigger, pr); err != nil {
				t.Fatalf("Didn't expect error: %s", err)
			}
			var numStarted int
			for _, action := range fakeProwJobClient.Actions() {
				switch action.(type) {
				case clienttesting.CreateActionImpl:
					numStarted++
				}
			}

			if tc.ShouldComment && len(g.IssueCommentsAdded) == 0 {
				t.Error("Expected comment to github")
			} else if !tc.ShouldComment && len(g.IssueCommentsAdded) > 0 {
				t.Errorf("Expected no comments to github, but got %d", len(g.IssueCommentsAdded))
			}
			if tc.expectedComment != "" && len(g.IssueCommentsAdded) == 1 {
				if tc.expectedComment != g.IssueCommentsAdded[0] {
					t.Errorf("%s: got incorrect comment: %v", tc.name, diff.StringDiff(tc.expectedComment, g.IssueCommentsAdded[0]))
				}
			}
		})
	}
}

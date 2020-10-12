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

package trigger

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clienttesting "k8s.io/client-go/testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

func TestTrusted(t *testing.T) {
	const rando = "random-person"
	const member = "org-member"
	const sister = "trusted-org-member"
	const friend = "repo-collaborator"

	var testcases = []struct {
		name     string
		author   string
		labels   []string
		onlyOrg  bool
		expected bool
	}{
		{
			name:     "trust org member",
			author:   member,
			labels:   []string{},
			expected: true,
		},
		{
			name:     "trust member of other trusted org",
			author:   sister,
			labels:   []string{},
			expected: true,
		},
		{
			name:     "accept random PR with ok-to-test",
			author:   rando,
			labels:   []string{labels.OkToTest},
			expected: true,
		},
		{
			name:     "accept random PR with both labels",
			author:   rando,
			labels:   []string{labels.OkToTest, labels.NeedsOkToTest},
			expected: true,
		},
		{
			name:     "reject random PR with needs-ok-to-test",
			author:   rando,
			labels:   []string{labels.NeedsOkToTest},
			expected: false,
		},
		{
			name:     "reject random PR with no label",
			author:   rando,
			labels:   []string{},
			expected: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakegithub.FakeClient{
				OrgMembers:    map[string][]string{"kubernetes": {sister}, "kubernetes-sigs": {member, fakegithub.Bot}},
				Collaborators: []string{friend},
				IssueComments: map[int][]github.IssueComment{},
			}
			trigger := plugins.Trigger{
				TrustedOrg:     "kubernetes",
				OnlyOrgMembers: tc.onlyOrg,
			}
			var labels []github.Label
			for _, label := range tc.labels {
				labels = append(labels, github.Label{
					Name: label,
				})
			}
			_, actual, err := TrustedPullRequest(g, trigger, tc.author, "kubernetes-sigs", "random-repo", 1, labels)
			if err != nil {
				t.Fatalf("Didn't expect error: %s", err)
			}
			if actual != tc.expected {
				t.Errorf("actual result %t != expected %t", actual, tc.expected)
			}
		})
	}
}

func TestHandlePullRequest(t *testing.T) {
	jobToAbort := &prowapi.ProwJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-to-abort",
			Namespace: "namespace",
			Labels: map[string]string{
				kube.OrgLabel:         "org",
				kube.RepoLabel:        "repo",
				kube.PullLabel:        "0",
				kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
			},
		},
	}

	var testcases = []struct {
		name string

		Author        string
		ShouldBuild   bool
		ShouldComment bool
		HasOkToTest   bool
		prLabel       string
		prChanges     bool
		prAction      github.PullRequestEventAction
		prIsDraft     bool
		eventSender   string
		jobToAbort    *prowapi.ProwJob
	}{
		{
			name: "Trusted user open PR should build",

			Author:      "t",
			ShouldBuild: true,
			prAction:    github.PullRequestActionOpened,
		},
		{
			name: "Trusted user open draft PR should not build and should comment",

			Author:        "t",
			ShouldBuild:   false,
			prAction:      github.PullRequestActionOpened,
			prIsDraft:     true,
			ShouldComment: true,
		},
		{
			name: "Untrusted user open PR should not build and should comment",

			Author:        "u",
			ShouldBuild:   false,
			ShouldComment: true,
			prAction:      github.PullRequestActionOpened,
		},
		{
			name: "Untrusted user open draft PR should not build and should comment",

			Author:        "u",
			ShouldBuild:   false,
			ShouldComment: true,
			prAction:      github.PullRequestActionOpened,
			prIsDraft:     true,
		},
		{
			name: "Trusted user reopen PR should build",

			Author:      "t",
			ShouldBuild: true,
			prAction:    github.PullRequestActionReopened,
		},
		{
			name: "Trusted user reopen draft PR should not build",

			Author:      "t",
			ShouldBuild: false,
			prAction:    github.PullRequestActionReopened,
			prIsDraft:   true,
		},
		{
			name: "Trusted user switch PR from draft to normal shoud build",

			Author:      "t",
			ShouldBuild: true,
			prAction:    github.PullRequestActionReadyForReview,
		},
		{
			name: "Untrusted user switch PR from draft to normal should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    github.PullRequestActionReadyForReview,
		},
		{
			name: "Untrusted user switch PR from draft to normal with ok-to-test should build",

			Author:      "u",
			HasOkToTest: true,
			ShouldBuild: true,
			prAction:    github.PullRequestActionReadyForReview,
		},
		{
			name: "Untrusted user reopen PR with ok-to-test should build",

			Author:      "u",
			ShouldBuild: true,
			HasOkToTest: true,
			prAction:    github.PullRequestActionReopened,
		},
		{
			name: "Untrusted user reopen PR without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    github.PullRequestActionReopened,
		},
		{
			name: "Untrusted user reopen draft PR should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    github.PullRequestActionReopened,
			prIsDraft:   true,
		},
		{
			name: "Trusted user edit PR with changes should build",

			Author:      "t",
			ShouldBuild: true,
			prChanges:   true,
			prAction:    github.PullRequestActionEdited,
		},
		{
			name: "Trusted user edit draft PR with changes should not build",

			Author:      "t",
			ShouldBuild: false,
			prChanges:   true,
			prAction:    github.PullRequestActionEdited,
			prIsDraft:   true,
		},
		{
			name: "Trusted user edit PR without changes should not build",

			Author:      "t",
			ShouldBuild: false,
			prAction:    github.PullRequestActionEdited,
		},
		{
			name: "Untrusted user edit PR without changes and without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    github.PullRequestActionEdited,
		},
		{
			name: "Untrusted user edit PR with changes and without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prChanges:   true,
			prAction:    github.PullRequestActionEdited,
		},
		{
			name: "Untrusted user edit PR without changes and with ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			HasOkToTest: true,
			prAction:    github.PullRequestActionEdited,
		},
		{
			name: "Untrusted user edit PR with changes and with ok-to-test should build",

			Author:      "u",
			ShouldBuild: true,
			HasOkToTest: true,
			prChanges:   true,
			prAction:    github.PullRequestActionEdited,
		},
		{
			name: "Trusted user sync PR should build",

			Author:      "t",
			ShouldBuild: true,
			prAction:    github.PullRequestActionSynchronize,
		},
		{
			name: "Untrusted user sync PR without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    github.PullRequestActionSynchronize,
		},
		{
			name: "Untrusted user sync PR with ok-to-test should build",

			Author:      "u",
			ShouldBuild: true,
			HasOkToTest: true,
			prAction:    github.PullRequestActionSynchronize,
		},
		{
			name: "Trusted user labeled PR with lgtm should not build",

			Author:      "t",
			ShouldBuild: false,
			prAction:    github.PullRequestActionLabeled,
			prLabel:     labels.LGTM,
		},
		{
			name: "Untrusted user labeled PR with lgtm should build",

			Author:      "u",
			ShouldBuild: true,
			prAction:    github.PullRequestActionLabeled,
			prLabel:     labels.LGTM,
		},
		{
			name: "Untrusted user labeled PR without lgtm should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    github.PullRequestActionLabeled,
			prLabel:     "test",
		},
		{
			name: "Trusted user closed PR should not build",

			Author:      "t",
			ShouldBuild: false,
			prAction:    github.PullRequestActionClosed,
		},
		{
			name: "Trusted user labeled PR with ok-to-test should build",

			Author:      "t",
			ShouldBuild: true,
			eventSender: "not-k8s-ci-robot",
			prAction:    github.PullRequestActionLabeled,
			prLabel:     labels.OkToTest,
		},
		{
			name: "Untrusted user labeled PR with ok-to-test should build",

			Author:      "u",
			ShouldBuild: true,
			eventSender: "not-k8s-ci-robot",
			prAction:    github.PullRequestActionLabeled,
			prLabel:     labels.OkToTest,
		},
		{
			name: "Label added by a bot. Build should not be triggered in this case.",

			Author:      "u",
			eventSender: "k8s-ci-robot",
			prLabel:     labels.OkToTest,
			prAction:    github.PullRequestActionLabeled,
			ShouldBuild: false,
		},
		{
			name: "Abort jobs if PR is closed",

			Author:      "t",
			HasOkToTest: true,
			prAction:    github.PullRequestActionClosed,
			ShouldBuild: false,
			jobToAbort:  jobToAbort,
		},
		{
			name: "Abort jobs if PR is changed to draft",

			Author:      "t",
			HasOkToTest: true,
			prAction:    github.PullRequestActionConvertedToDraft,
			ShouldBuild: false,
			jobToAbort:  jobToAbort,
		},
	}
	for _, tc := range testcases {
		t.Logf("running scenario %q", tc.name)
		t.Run(tc.name, func(t *testing.T) {
			g := &fakegithub.FakeClient{
				IssueComments: map[int][]github.IssueComment{},
				OrgMembers:    map[string][]string{"org": {"t"}},
				PullRequests: map[int]*github.PullRequest{
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
				},
			}
			fakeProwJobClient := fake.NewSimpleClientset(jobToAbort)
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
				},
			}
			if err := c.Config.SetPresubmits(presubmits); err != nil {
				t.Fatalf("failed to set presubmits: %v", err)
			}

			if tc.HasOkToTest {
				g.IssueLabelsExisting = append(g.IssueLabelsExisting, issueLabels(labels.OkToTest)...)
			}
			if tc.eventSender == "" {
				tc.eventSender = tc.Author
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
					Login: tc.eventSender,
				},
			}
			if tc.prChanges {
				data := []byte(`{"base":{"ref":{"from":"REF"}, "sha":{"from":"SHA"}}}`)
				pr.Changes = (json.RawMessage)(data)
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
			if numStarted > 0 && !tc.ShouldBuild {
				t.Errorf("Built but should not have: %+v", tc)
			} else if numStarted == 0 && tc.ShouldBuild {
				t.Errorf("Not built but should have: %+v", tc)
			}
			if tc.ShouldComment && len(g.IssueCommentsAdded) == 0 {
				t.Error("Expected comment to github")
			} else if !tc.ShouldComment && len(g.IssueCommentsAdded) > 0 {
				t.Errorf("Expected no comments to github, but got %d", len(g.IssueCommentsAdded))
			}
			if tc.jobToAbort != nil {
				pj, err := fakeProwJobClient.ProwV1().ProwJobs("namespace").Get(context.Background(), tc.jobToAbort.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("failed to get prowjob: %v", err)
				}

				if pj.Status.State != prowapi.AbortedState {
					t.Errorf("exptected job %s to be aborted, found state: %v", tc.jobToAbort.Name, pj.Status.State)
				}
				if pj.Complete() {
					t.Errorf("exptected job %s to not be set to complete.", tc.jobToAbort.Name)
				}
			}
		})
	}
}

func TestAbortAllJobs(t *testing.T) {
	t.Parallel()
	const org, repo, number = "org", "repo", 1
	pj := func(modifier ...func(*prowapi.ProwJob)) *prowapi.ProwJob {
		job := &prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-pj",
				Labels: map[string]string{
					kube.OrgLabel:         org,
					kube.RepoLabel:        repo,
					kube.PullLabel:        strconv.Itoa(number),
					kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
				},
			},
		}

		for _, m := range modifier {
			m(job)
		}

		return job
	}
	testCases := []struct {
		name                   string
		pj                     *prowapi.ProwJob
		expectedAbortedProwJob bool
	}{
		{
			name:                   "Job gets aborted",
			pj:                     pj(),
			expectedAbortedProwJob: true,
		},
		{
			name:                   "Wrong org, ignored",
			pj:                     pj(func(pj *prowapi.ProwJob) { pj.Labels[kube.OrgLabel] = "wrong" }),
			expectedAbortedProwJob: false,
		},
		{
			name:                   "Wrong repo, ignored",
			pj:                     pj(func(pj *prowapi.ProwJob) { pj.Labels[kube.RepoLabel] = "wrong" }),
			expectedAbortedProwJob: false,
		},
		{
			name:                   "Wrong pr, ignored",
			pj:                     pj(func(pj *prowapi.ProwJob) { pj.Labels[kube.PullLabel] = "99" }),
			expectedAbortedProwJob: false,
		},
		{
			name:                   "Wrong type, ignored",
			pj:                     pj(func(pj *prowapi.ProwJob) { pj.Labels[kube.ProwJobTypeLabel] = "wrong" }),
			expectedAbortedProwJob: false,
		},
		{
			name: "Job completed, ignored",
			pj: pj(func(pj *prowapi.ProwJob) {
				pj.Status.CompletionTime = &[]metav1.Time{metav1.Now()}[0]
			}),
			expectedAbortedProwJob: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pjClient := fake.NewSimpleClientset(tc.pj)
			client := Client{
				ProwJobClient: pjClient.ProwV1().ProwJobs(""),
				Logger:        logrus.NewEntry(logrus.New()),
			}
			pr := &github.PullRequest{
				Base: github.PullRequestBranch{
					Repo: github.Repo{
						Owner: github.User{
							Login: org,
						},
						Name: repo,
					},
				},
				Number: number,
			}

			if err := abortAllJobs(client, pr); err != nil {
				t.Fatalf("error caling abortAllJobs: %v", err)
			}

			pj, err := pjClient.ProwV1().ProwJobs("").Get(context.Background(), pj().Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("failed to get prowjob: %v", err)
			}

			if isAborted := pj.Status.State == prowapi.AbortedState; isAborted != tc.expectedAbortedProwJob {
				t.Errorf("IsAborted: %t, but expected aborted: %t", isAborted, tc.expectedAbortedProwJob)
			}
		})
	}
}

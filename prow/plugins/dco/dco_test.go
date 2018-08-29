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

package dco

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakePruner struct{}

func (fp *fakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}

func strP(str string) *string {
	return &str
}

func TestCheckDCO(t *testing.T) {
	var testcases = []struct {
		// test settings
		name string

		// PR settings
		pullRequest github.PullRequest
		commits     []github.RepositoryCommit
		issueState  string
		hasDCOYes   bool
		hasDCONo    bool
		// status of the DCO github context
		status string

		// expectations
		addedLabel     string
		removedLabel   string
		expectedStatus string
		// org/repo#number:body
		addedComment string
		// org/repo#issuecommentid
		removedComment string
	}{
		{
			name: "should add 'no' label & status context and add a comment if no commits have sign off",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("not a sign off")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    false,
			hasDCOYes:   false,

			addedLabel:     fmt.Sprintf("/#3:%s", dcoNoLabel),
			expectedStatus: github.StatusFailure,
			addedComment: `/#3:Thanks for your pull request. Before we can look at it, you'll need to add a 'DCO signoff' to your commits.

:memo: **Please follow instructions in the [contributing guide](https://github.com///blob/master/CONTRIBUTING.md) to update your commits with the DCO**

Full details of the Developer Certificate of Origin can be found at [developercertificate.org](https://developercertificate.org/).

**The list of commits missing DCO signoff**:

- [sha](https://github.com///commits/sha) not a sign off

<details>

Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository. I understand the commands that are listed [here](https://go.k8s.io/bot-commands).
</details>
`,
		},
		{
			name: "should add 'no' label & status context, remove old labels and add a comment if no commits have sign off",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("not a sign off")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    false,
			hasDCOYes:   true,

			addedLabel:     fmt.Sprintf("/#3:%s", dcoNoLabel),
			removedLabel:   fmt.Sprintf("/#3:%s", dcoYesLabel),
			expectedStatus: github.StatusFailure,
			addedComment: `/#3:Thanks for your pull request. Before we can look at it, you'll need to add a 'DCO signoff' to your commits.

:memo: **Please follow instructions in the [contributing guide](https://github.com///blob/master/CONTRIBUTING.md) to update your commits with the DCO**

Full details of the Developer Certificate of Origin can be found at [developercertificate.org](https://developercertificate.org/).

**The list of commits missing DCO signoff**:

- [sha](https://github.com///commits/sha) not a sign off

<details>

Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository. I understand the commands that are listed [here](https://go.k8s.io/bot-commands).
</details>
`,
		},
		{
			name: "should update comment if labels and status are up to date and sign off is failing",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("not a sign off")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    true,
			hasDCOYes:   false,
			status:      github.StatusFailure,

			expectedStatus: github.StatusFailure,
			addedComment: `/#3:Thanks for your pull request. Before we can look at it, you'll need to add a 'DCO signoff' to your commits.

:memo: **Please follow instructions in the [contributing guide](https://github.com///blob/master/CONTRIBUTING.md) to update your commits with the DCO**

Full details of the Developer Certificate of Origin can be found at [developercertificate.org](https://developercertificate.org/).

**The list of commits missing DCO signoff**:

- [sha](https://github.com///commits/sha) not a sign off

<details>

Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository. I understand the commands that are listed [here](https://go.k8s.io/bot-commands).
</details>
`,
		},
		{
			name: "should mark the PR as failed if just one commit is missing sign-off",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha1"), Commit: &github.GitCommit{Message: strP("Signed-off-by: someone")}},
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("not signed off")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    false,
			hasDCOYes:   true,

			addedLabel:     fmt.Sprintf("/#3:%s", dcoNoLabel),
			removedLabel:   fmt.Sprintf("/#3:%s", dcoYesLabel),
			expectedStatus: github.StatusFailure,
			addedComment: `/#3:Thanks for your pull request. Before we can look at it, you'll need to add a 'DCO signoff' to your commits.

:memo: **Please follow instructions in the [contributing guide](https://github.com///blob/master/CONTRIBUTING.md) to update your commits with the DCO**

Full details of the Developer Certificate of Origin can be found at [developercertificate.org](https://developercertificate.org/).

**The list of commits missing DCO signoff**:

- [sha](https://github.com///commits/sha) not signed off

<details>

Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository. I understand the commands that are listed [here](https://go.k8s.io/bot-commands).
</details>
`,
		},
		{
			name: "should add label and update status context if all commits are signed-off",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("Signed-off-by: someone")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    false,
			hasDCOYes:   false,

			addedLabel:     fmt.Sprintf("/#3:%s", dcoYesLabel),
			expectedStatus: github.StatusSuccess,
		},
		{
			name: "should add label and update status context and remove old labels if all commits are signed-off",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("Signed-off-by: someone")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    true,
			hasDCOYes:   false,

			addedLabel:     fmt.Sprintf("/#3:%s", dcoYesLabel),
			removedLabel:   fmt.Sprintf("/#3:%s", dcoNoLabel),
			expectedStatus: github.StatusSuccess,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakegithub.FakeClient{
				CreatedStatuses: make(map[string][]github.Status),
				PullRequests:    map[int]*github.PullRequest{tc.pullRequest.Number: &tc.pullRequest},
				IssueComments:   make(map[int][]github.IssueComment),
				CommitMap: map[string][]github.RepositoryCommit{
					"/#3": tc.commits,
				},
			}
			if tc.hasDCOYes {
				fc.LabelsAdded = append(fc.LabelsAdded, fmt.Sprintf("/#3:%s", dcoYesLabel))
			}
			if tc.hasDCONo {
				fc.LabelsAdded = append(fc.LabelsAdded, fmt.Sprintf("/#3:%s", dcoNoLabel))
			}
			if tc.status != "" {
				fc.CreatedStatuses["sha"] = []github.Status{
					{Context: dcoContextName, State: tc.status},
				}
			}
			if err := handle(fc, &fakePruner{}, logrus.WithField("plugin", pluginName), "", "", tc.pullRequest); err != nil {
				t.Errorf("For case %s, didn't expect error from dco plugin: %v", tc.name, err)
			}
			ok := tc.addedLabel == ""
			if !ok {
				for _, label := range fc.LabelsAdded {
					if reflect.DeepEqual(tc.addedLabel, label) {
						ok = true
						break
					}
				}
			}
			if !ok {
				t.Errorf("Expected to add: %#v, Got %#v in case %s.", tc.addedLabel, fc.LabelsAdded, tc.name)
			}
			ok = tc.removedLabel == ""
			if !ok {
				for _, label := range fc.LabelsRemoved {
					if reflect.DeepEqual(tc.removedLabel, label) {
						ok = true
						break
					}
				}
			}
			if !ok {
				t.Errorf("Expected to remove: %#v, Got %#v in case %s.", tc.removedLabel, fc.LabelsRemoved, tc.name)
			}

			// check status is set as expected
			statuses := fc.CreatedStatuses["sha"]
			if len(statuses) == 0 && tc.expectedStatus != "" {
				t.Errorf("Expected dco status to be %q, but it was not set", tc.expectedStatus)
			}
			found := false
			for _, s := range statuses {
				if s.Context == dcoContextName {
					found = true
					if s.State != tc.expectedStatus {
						t.Errorf("Expected dco status to be %q but it was %q", tc.expectedStatus, s.State)
					}
				}
			}
			if !found && tc.expectedStatus != "" {
				t.Errorf("Expect dco status to be %q, but it was not found", tc.expectedStatus)
			}

			comments := fc.IssueCommentsAdded
			if len(comments) == 0 && tc.addedComment != "" {
				t.Errorf("Expected comment with body %q to be added, but it was not", tc.addedComment)
				return
			}
			if len(comments) > 1 {
				t.Errorf("did not expect more than one comment to be created")
			}
			if len(comments) != 0 && comments[0] != tc.addedComment {
				t.Errorf("expected comment to be %q but it was %q", tc.addedComment, comments[0])
			}
		})
	}
}

func TestMarkdownSHAList(t *testing.T) {
	var testcases = []struct {
		name string

		org, repo    string
		commits      []github.GitCommit
		expectedList string
	}{
		{
			name: "return a single git commit in a list",
			org:  "org",
			repo: "repo",
			commits: []github.GitCommit{
				{SHA: strP("sha"), Message: strP("msg")},
			},
			expectedList: `- [sha](https://github.com/org/repo/commits/sha) msg`,
		},
		{
			name: "return two git commits in a list",
			org:  "org",
			repo: "repo",
			commits: []github.GitCommit{
				{SHA: strP("sha1"), Message: strP("msg1")},
				{SHA: strP("sha2"), Message: strP("msg2")},
			},
			expectedList: `- [sha1](https://github.com/org/repo/commits/sha1) msg1
- [sha2](https://github.com/org/repo/commits/sha2) msg2`,
		},
	}
	for _, tc := range testcases {
		actualList := markdownSHAList(tc.org, tc.repo, tc.commits)
		if actualList != tc.expectedList {
			t.Errorf("Expected returned list to be %q but it was %q", tc.expectedList, actualList)
		}
	}
}

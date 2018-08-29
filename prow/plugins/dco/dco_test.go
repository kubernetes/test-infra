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
	}{
		{
			name: "should add 'dco-signoff: no' label and status context if no commits have sign off",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("not a sign off")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    false,
			hasDCOYes:   false,

			addedLabel:     fmt.Sprintf("/#3:%s", dcoNoLabel),
			expectedStatus: github.StatusFailure,
		},
		{
			name: "should add 'dco-signoff: no' label and status context and remove old labels if no commits have sign off",
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
		},
		{
			name: "should do nothing if labels and status are up to date and sign off is failing",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("not a sign off")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    true,
			hasDCOYes:   false,
			status:      github.StatusFailure,

			expectedStatus: github.StatusFailure,
		},
		{
			name: "should mark the PR as failed if just one commit is missing sign-off",
			commits: []github.RepositoryCommit{
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("Signed-off-by: someone")}},
				{SHA: strP("sha"), Commit: &github.GitCommit{Message: strP("not signed off")}},
			},
			issueState:  "open",
			pullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			hasDCONo:    false,
			hasDCOYes:   true,

			addedLabel:     fmt.Sprintf("/#3:%s", dcoNoLabel),
			removedLabel:   fmt.Sprintf("/#3:%s", dcoYesLabel),
			expectedStatus: github.StatusFailure,
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
		})
	}
}

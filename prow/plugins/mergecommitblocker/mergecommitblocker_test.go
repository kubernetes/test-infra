/*
Copyright 2017 The Kubernetes Authors.

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

package mergecommitblocker

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"reflect"
	"testing"
)
const (
	helpWanted = "help-wanted"
	mergeCommit = "do-not-merge/contains-merge-commits"
)
type ghc struct {
	*testing.T
	labels    map[github.Label]bool
	files     map[string][]byte
	prChanges []github.PullRequestChange

	addLabelErr, removeLabelErr, getIssueLabelsErr,
	getFileErr, getPullRequestChangesErr error
}

func TestHandlePR(t *testing.T) {
	var testcases = []struct {
		name string
		pullRequestEvent github.PullRequestEvent
		commits          []github.RepositoryCommit
		initialLabels  []string
		addedLabel string
	}{
		{
			name: "should label with do-not-merge/contains-merge-commits when merge commits present",
			pullRequestEvent: github.PullRequestEvent{
				Action:      github.PullRequestActionEdited,
				PullRequest: github.PullRequest{Number: 3, Head: github.PullRequestBranch{SHA: "sha"}},
			},
			commits: []github.RepositoryCommit{
				{SHA: "sha", Commit: github.GitCommit{Message: "One Commit"}},
				{SHA: "sha2", Commit: github.GitCommit{Message: "Two commit"}, Parents: []github.Commit{
					{
						Added: []string{"three.commit"},
					},
					{
						Added: []string{"merge.commit"},
					},
				}},
			},
			initialLabels: []string{helpWanted},
			addedLabel: fmt.Sprintf("/#3:%s", mergeCommit),
		},
		{
			name: "should not label with do-not-merge/contains-merge-commits when there are no merge commits present",
		},
		{
			name: "should remove label with do-not-merge/contains-merge-commits when merge commits have been removed",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakegithub.FakeClient{
				CreatedStatuses: make(map[string][]github.Status),
				PullRequests:    map[int]*github.PullRequest{tc.pullRequestEvent.PullRequest.Number: &tc.pullRequestEvent.PullRequest},
				IssueComments:   make(map[int][]github.IssueComment),
				CommitMap: map[string][]github.RepositoryCommit{
					"/#3": tc.commits,
				},
			}
			if err := handlePR(fc,logrus.WithField("plugin", pluginName),tc.pullRequestEvent); err != nil {
				t.Errorf("For case %s, didn't expect error from mergecommitblocker	 plugin: %v", tc.name, err)
			}
			ok := tc.addedLabel == ""
			if !ok {
				for _, label := range fc.IssueLabelsAdded {
					if reflect.DeepEqual(tc.addedLabel, label) {
						ok = true
						break
					}
				}
			}
			if !ok {
				t.Errorf("Expected to add: %#v, Got %#v in case %s.", tc.addedLabel, fc.IssueLabelsAdded, tc.name)
			}
		})
	}
}
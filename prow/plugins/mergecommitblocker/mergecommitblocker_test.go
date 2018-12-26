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
	"k8s.io/test-infra/prow/github"
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

func testHandlePR(t * testing.T) {
	var testcases = []struct {
		name string
		pullRequestEvent github.PullRequestEvent
		commits          []github.RepositoryCommit
		initialLabels  []string

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

		})
	}
}
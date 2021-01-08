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

package help

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
)

type fakePruner struct{}

func (fp *fakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}

func formatLabels(labels ...string) []string {
	r := []string{}
	for _, l := range labels {
		r = append(r, fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 1, l))
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

func TestLabel(t *testing.T) {
	type testCase struct {
		name                  string
		isPR                  bool
		issueState            string
		action                github.GenericCommentEventAction
		body                  string
		expectedNewLabels     []string
		expectedRemovedLabels []string
		issueLabels           []string
	}
	testcases := []testCase{
		{
			name:                  "Ignore irrelevant comment",
			body:                  "irrelelvant",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Ignore a PR",
			isPR:                  true,
			body:                  "/help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Ignore a closed issue",
			issueState:            "closed",
			body:                  "/help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Ignore a non-created comment",
			action:                github.GenericCommentActionEdited,
			body:                  "/help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Want helpLabel",
			body:                  "/help",
			expectedNewLabels:     formatLabels(labels.Help),
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Want helpLabel, already have it.",
			body:                  "/help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{labels.Help},
		},
		{
			name:                  "Want to remove helpLabel, have it",
			body:                  "/remove-help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels(labels.Help),
			issueLabels:           []string{labels.Help},
		},
		{
			name:                  "Want to remove helpLabel, don't have it",
			body:                  "/remove-help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Want to remove helpLabel and goodFirstIssueLabel, have helpLabel and goodFirstIssueLabel",
			body:                  "/remove-help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels(labels.Help, labels.GoodFirstIssue),
			issueLabels:           []string{labels.Help, labels.GoodFirstIssue},
		},
		{
			name:                  "Want to add goodFirstIssueLabel and helpLabel, don't have both",
			body:                  "/good-first-issue",
			expectedNewLabels:     formatLabels(labels.Help, labels.GoodFirstIssue),
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Want to add goodFirstIssueLabel and helpLabel, don't have goodFirstIssueLabel but have helpLabel",
			body:                  "/good-first-issue",
			expectedNewLabels:     formatLabels(labels.GoodFirstIssue),
			expectedRemovedLabels: []string{},
			issueLabels:           []string{labels.Help},
		},
		{
			name:                  "Want to add goodFirstIssueLabel and helpLabel, have both",
			body:                  "/good-first-issue",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{labels.Help, labels.GoodFirstIssue},
		},
		{
			name:                  "Want to remove goodFirstIssueLabel, have helpLabel and goodFirstIssueLabel",
			body:                  "/remove-good-first-issue",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels(labels.GoodFirstIssue),
			issueLabels:           []string{labels.Help, labels.GoodFirstIssue},
		},
		{
			name:                  "Want to remove goodFirstIssueLabel, have goodFirstIssueLabel",
			body:                  "/remove-good-first-issue",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels(labels.GoodFirstIssue),
			issueLabels:           []string{labels.GoodFirstIssue},
		},
		{
			name:                  "Want to remove goodFirstIssueLabel, have helpLabel but don't have goodFirstIssueLabel",
			body:                  "/remove-good-first-issue",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{labels.Help},
		},
		{
			name:                  "Want to remove goodFirstIssueLabel, but don't have it",
			body:                  "/remove-good-first-issue",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
	}

	helpGuidelinesURL := "https://git.k8s.io/community/contributors/guide/help-wanted.md"

	for _, tc := range testcases {
		sort.Strings(tc.expectedNewLabels)
		fakeClient := &fakegithub.FakeClient{
			Issues:             make(map[int]*github.Issue),
			IssueComments:      make(map[int][]github.IssueComment),
			RepoLabelsExisting: []string{labels.Help, labels.GoodFirstIssue},
			IssueLabelsAdded:   []string{},
			IssueLabelsRemoved: []string{},
		}
		// Add initial labels
		for _, label := range tc.issueLabels {
			fakeClient.AddLabel("org", "repo", 1, label)
		}

		if len(tc.issueState) == 0 {
			tc.issueState = "open"
		}
		if len(tc.action) == 0 {
			tc.action = github.GenericCommentActionCreated
		}

		e := &github.GenericCommentEvent{
			IsPR:       tc.isPR,
			IssueState: tc.issueState,
			Action:     tc.action,
			Body:       tc.body,
			Number:     1,
			Repo:       github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			User:       github.User{Login: "Alice"},
		}
		err := handle(fakeClient, logrus.WithField("plugin", pluginName), &fakePruner{}, e, helpGuidelinesURL)
		if err != nil {
			t.Errorf("For case %s, didn't expect error from label test: %v", tc.name, err)
			continue
		}

		// Check that all the correct labels (and only the correct labels) were added.
		expectLabels := append(formatLabels(tc.issueLabels...), tc.expectedNewLabels...)
		if expectLabels == nil {
			expectLabels = []string{}
		}
		sort.Strings(expectLabels)
		sort.Strings(fakeClient.IssueLabelsAdded)
		if !reflect.DeepEqual(expectLabels, fakeClient.IssueLabelsAdded) {
			t.Errorf("(%s): Expected the labels %q to be added, but %q were added.", tc.name, expectLabels, fakeClient.IssueLabelsAdded)
		}

		sort.Strings(tc.expectedRemovedLabels)
		sort.Strings(fakeClient.IssueLabelsRemoved)
		if !reflect.DeepEqual(tc.expectedRemovedLabels, fakeClient.IssueLabelsRemoved) {
			t.Errorf("(%s): Expected the labels %q to be removed, but %q were removed.", tc.name, tc.expectedRemovedLabels, fakeClient.IssueLabelsRemoved)
		}
	}
}

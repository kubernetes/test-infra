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

package plugin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

func testKey(org, repo string, num int) string {
	return fmt.Sprintf("%s/%s#%d", org, repo, num)
}

type fghc struct {
	allPRs []struct {
		PullRequest pullRequest `graphql:"... on PullRequest"`
	}
	pr *github.PullRequest

	initialLabels []github.Label
	mergeable     bool

	// The following are maps are keyed using 'testKey'
	commentCreated, commentDeleted       map[string]bool
	IssueLabelsAdded, IssueLabelsRemoved map[string][]string
}

func newFakeClient(prs []pullRequest, initialLabels []string, mergeable bool, pr *github.PullRequest) *fghc {
	f := &fghc{
		mergeable:          mergeable,
		commentCreated:     make(map[string]bool),
		commentDeleted:     make(map[string]bool),
		IssueLabelsAdded:   make(map[string][]string),
		IssueLabelsRemoved: make(map[string][]string),
		pr:                 pr,
	}
	for _, pr := range prs {
		s := struct {
			PullRequest pullRequest `graphql:"... on PullRequest"`
		}{pr}
		f.allPRs = append(f.allPRs, s)
	}
	for _, label := range initialLabels {
		f.initialLabels = append(f.initialLabels, github.Label{Name: label})
	}
	return f
}

func (f *fghc) GetIssueLabels(org, repo string, number int) ([]github.Label, error) {
	return f.initialLabels, nil
}

func (f *fghc) CreateComment(org, repo string, number int, comment string) error {
	f.commentCreated[testKey(org, repo, number)] = true
	return nil
}

func (f *fghc) BotUserChecker() (func(candidate string) bool, error) {
	return func(candidate string) bool { return candidate == "k8s-ci-robot" }, nil
}

func (f *fghc) AddLabel(org, repo string, number int, label string) error {
	key := testKey(org, repo, number)
	f.IssueLabelsAdded[key] = append(f.IssueLabelsAdded[key], label)
	return nil
}

func (f *fghc) RemoveLabel(org, repo string, number int, label string) error {
	key := testKey(org, repo, number)
	f.IssueLabelsRemoved[key] = append(f.IssueLabelsRemoved[key], label)
	return nil
}

func (f *fghc) IsMergeable(org, repo string, number int, sha string) (bool, error) {
	return f.mergeable, nil
}

func (f *fghc) DeleteStaleComments(org, repo string, number int, comments []github.IssueComment, isStale func(github.IssueComment) bool) error {
	f.commentDeleted[testKey(org, repo, number)] = true
	return nil
}

func (f *fghc) Query(_ context.Context, q interface{}, _ map[string]interface{}) error {
	query, ok := q.(*searchQuery)
	if !ok {
		return errors.New("invalid query format")
	}
	query.Search.Nodes = f.allPRs
	return nil
}

func (f *fghc) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	if f.pr != nil {
		return f.pr, nil
	}
	return nil, fmt.Errorf("didn't find pull request %s/%s#%d", org, repo, number)
}

func (f *fghc) compareExpected(t *testing.T, org, repo string, num int, expectedAdded []string, expectedRemoved []string, expectComment bool, expectDeletion bool) {
	key := testKey(org, repo, num)
	sort.Strings(expectedAdded)
	sort.Strings(expectedRemoved)
	sort.Strings(f.IssueLabelsAdded[key])
	sort.Strings(f.IssueLabelsRemoved[key])
	if !reflect.DeepEqual(expectedAdded, f.IssueLabelsAdded[key]) {
		t.Errorf("Expected the following labels to be added to %s: %q, but got %q.", key, expectedAdded, f.IssueLabelsAdded[key])
	}
	if !reflect.DeepEqual(expectedRemoved, f.IssueLabelsRemoved[key]) {
		t.Errorf("Expected the following labels to be removed from %s: %q, but got %q.", key, expectedRemoved, f.IssueLabelsRemoved[key])
	}
	if expectComment && !f.commentCreated[key] {
		t.Errorf("Expected a comment to be created on %s, but none was.", key)
	} else if !expectComment && f.commentCreated[key] {
		t.Errorf("Unexpected comment on %s.", key)
	}
	if expectDeletion && !f.commentDeleted[key] {
		t.Errorf("Expected a comment to be deleted from %s, but none was.", key)
	} else if !expectDeletion && f.commentDeleted[key] {
		t.Errorf("Unexpected comment deletion on %s.", key)
	}
}

func TestHandleIssueCommentEvent(t *testing.T) {

	pr := func() *github.PullRequest {
		pr := github.PullRequest{
			Base: github.PullRequestBranch{
				Repo: github.Repo{
					Name:  "repo",
					Owner: github.User{Login: "org"},
				},
			},
			Number: 5,
		}
		return &pr
	}

	oldSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = oldSleep }()

	testCases := []struct {
		name string
		pr   *github.PullRequest

		mergeable bool
		merged    bool
		labels    []string

		expectedAdded   []string
		expectedRemoved []string
		expectComment   bool
		expectDeletion  bool
	}{
		{
			name: "No pull request, ignoring",
		},
		{
			name:      "mergeable no-op",
			pr:        pr(),
			mergeable: true,
			labels:    []string{labels.LGTM, labels.Approved},
		},
		{
			name:      "unmergeable no-op",
			pr:        pr(),
			mergeable: false,
			labels:    []string{labels.LGTM, labels.Approved, labels.NeedsRebase},
		},
		{
			name:      "mergeable -> unmergeable",
			pr:        pr(),
			mergeable: false,
			labels:    []string{labels.LGTM, labels.Approved},

			expectedAdded: []string{labels.NeedsRebase},
			expectComment: true,
		},
		{
			name:      "unmergeable -> mergeable",
			pr:        pr(),
			mergeable: true,
			labels:    []string{labels.LGTM, labels.Approved, labels.NeedsRebase},

			expectedRemoved: []string{labels.NeedsRebase},
			expectDeletion:  true,
		},
		{
			name:   "merged pr is ignored",
			pr:     pr(),
			merged: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeClient(nil, tc.labels, tc.mergeable, tc.pr)
			ice := &github.IssueCommentEvent{}
			if tc.pr != nil {
				ice.Issue.PullRequest = &struct{}{}
				tc.pr.Merged = tc.merged
			}
			if err := HandleIssueCommentEvent(logrus.WithField("plugin", PluginName), fake, ice); err != nil {
				t.Fatalf("error handling issue comment event: %v", err)
			}
			fake.compareExpected(t, "org", "repo", 5, tc.expectedAdded, tc.expectedRemoved, tc.expectComment, tc.expectDeletion)
		})
	}
}

func TestHandlePullRequestEvent(t *testing.T) {
	oldSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = oldSleep }()

	testCases := []struct {
		name string

		mergeable bool
		merged    bool
		labels    []string

		expectedAdded   []string
		expectedRemoved []string
		expectComment   bool
		expectDeletion  bool
	}{
		{
			name:      "mergeable no-op",
			mergeable: true,
			labels:    []string{labels.LGTM, labels.Approved},
		},
		{
			name:      "unmergeable no-op",
			mergeable: false,
			labels:    []string{labels.LGTM, labels.Approved, labels.NeedsRebase},
		},
		{
			name:      "mergeable -> unmergeable",
			mergeable: false,
			labels:    []string{labels.LGTM, labels.Approved},

			expectedAdded: []string{labels.NeedsRebase},
			expectComment: true,
		},
		{
			name:      "unmergeable -> mergeable",
			mergeable: true,
			labels:    []string{labels.LGTM, labels.Approved, labels.NeedsRebase},

			expectedRemoved: []string{labels.NeedsRebase},
			expectDeletion:  true,
		},
		{
			name:   "merged pr is ignored",
			merged: true,
		},
	}

	for _, tc := range testCases {
		fake := newFakeClient(nil, tc.labels, tc.mergeable, nil)
		pre := &github.PullRequestEvent{
			Action: github.PullRequestActionSynchronize,
			PullRequest: github.PullRequest{
				Base: github.PullRequestBranch{
					Repo: github.Repo{
						Name:  "repo",
						Owner: github.User{Login: "org"},
					},
				},
				Merged: tc.merged,
				Number: 5,
			},
		}
		t.Logf("Running test scenario: %q", tc.name)
		if err := HandlePullRequestEvent(logrus.WithField("plugin", PluginName), fake, pre); err != nil {
			t.Fatalf("Unexpected error handling event: %v.", err)
		}
		fake.compareExpected(t, "org", "repo", 5, tc.expectedAdded, tc.expectedRemoved, tc.expectComment, tc.expectDeletion)
	}
}

func TestHandleAll(t *testing.T) {
	testPRs := []struct {
		labels    []string
		mergeable bool

		expectedAdded, expectedRemoved []string
		expectComment, expectDeletion  bool
	}{
		{
			mergeable: true,
			labels:    []string{labels.LGTM, labels.Approved},
		},
		{
			mergeable: false,
			labels:    []string{labels.LGTM, labels.Approved, labels.NeedsRebase},
		},
		{
			mergeable: false,
			labels:    []string{labels.LGTM, labels.Approved},

			expectedAdded: []string{labels.NeedsRebase},
			expectComment: true,
		},
		{
			mergeable: true,
			labels:    []string{labels.LGTM, labels.Approved, labels.NeedsRebase},

			expectedRemoved: []string{labels.NeedsRebase},
			expectDeletion:  true,
		},
	}

	prs := []pullRequest{}
	for i, testPR := range testPRs {
		pr := pullRequest{
			Number: githubql.Int(i),
		}
		if testPR.mergeable {
			pr.Mergeable = githubql.MergeableStateMergeable
		} else {
			pr.Mergeable = githubql.MergeableStateConflicting
		}
		for _, label := range testPR.labels {
			s := struct {
				Name githubql.String
			}{
				Name: githubql.String(label),
			}
			pr.Labels.Nodes = append(pr.Labels.Nodes, s)
		}
		prs = append(prs, pr)
	}
	fake := newFakeClient(prs, nil, false, nil)
	config := &plugins.Configuration{
		Plugins: map[string][]string{"/": {labels.LGTM, PluginName}},

		ExternalPlugins: map[string][]plugins.ExternalPlugin{"/": {{Name: PluginName}}},
	}

	if err := HandleAll(logrus.WithField("plugin", PluginName), fake, config); err != nil {
		t.Fatalf("Unexpected error handling all prs: %v.", err)
	}
	for i, pr := range testPRs {
		fake.compareExpected(t, "", "", i, pr.expectedAdded, pr.expectedRemoved, pr.expectComment, pr.expectDeletion)
	}
}

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

package blunderbuss

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

type fakeGithubClient struct {
	changes   []github.PullRequestChange
	requested []string
}

func newFakeGithubClient(filesChanged []string) *fakeGithubClient {
	changes := make([]github.PullRequestChange, 0, len(filesChanged))
	for _, name := range filesChanged {
		changes = append(changes, github.PullRequestChange{Filename: name})
	}
	return &fakeGithubClient{changes: changes}
}

func (c *fakeGithubClient) RequestReview(org, repo string, number int, logins []string) error {
	if org != "org" {
		return errors.New("org should be 'org'")
	}
	if repo != "repo" {
		return errors.New("repo should be 'repo'")
	}
	if number != 5 {
		return errors.New("number should be 5")
	}
	c.requested = append(c.requested, logins...)
	return nil
}

func (c *fakeGithubClient) GetPullRequestChanges(org, repo string, num int) ([]github.PullRequestChange, error) {
	if org != "org" {
		return nil, errors.New("org should be 'org'")
	}
	if repo != "repo" {
		return nil, errors.New("repo should be 'repo'")
	}
	if num != 5 {
		return nil, errors.New("number should be 5")
	}
	return c.changes, nil
}

type fakeOwnersClient struct {
	owners            map[string]string
	approvers         map[string]sets.String
	leafApprovers     map[string]sets.String
	reviewers         map[string]sets.String
	requiredReviewers map[string]sets.String
	leafReviewers     map[string]sets.String
}

func (foc *fakeOwnersClient) Approvers(path string) sets.String {
	return foc.approvers[path]
}

func (foc *fakeOwnersClient) LeafApprovers(path string) sets.String {
	return foc.leafApprovers[path]
}

func (foc *fakeOwnersClient) FindApproverOwnersForFile(path string) string {
	return foc.owners[path]
}

func (foc *fakeOwnersClient) Reviewers(path string) sets.String {
	return foc.reviewers[path]
}

func (foc *fakeOwnersClient) RequiredReviewers(path string) sets.String {
	return foc.requiredReviewers[path]
}

func (foc *fakeOwnersClient) LeafReviewers(path string) sets.String {
	return foc.leafReviewers[path]
}

func (foc *fakeOwnersClient) FindReviewersOwnersForFile(path string) string {
	return foc.owners[path]
}

var (
	owners = map[string]string{
		"a.go":  "1",
		"b.go":  "2",
		"bb.go": "3",
		"c.go":  "4",

		"e.go":  "5",
		"ee.go": "5",
	}
	reviewers = map[string]sets.String{
		"a.go": sets.NewString("al"),
		"b.go": sets.NewString("al"),
		"c.go": sets.NewString("charles"),

		"e.go":  sets.NewString("erick", "evan"),
		"ee.go": sets.NewString("erick", "evan"),
	}
	requiredReviewers = map[string]sets.String{
		"a.go": sets.NewString("ben"),

		"ee.go": sets.NewString("chris", "charles"),
	}
	leafReviewers = map[string]sets.String{
		"a.go":  sets.NewString("alice"),
		"b.go":  sets.NewString("bob"),
		"bb.go": sets.NewString("bob", "ben"),
		"c.go":  sets.NewString("cole", "carl", "chad"),

		"e.go":  sets.NewString("erick", "ellen"),
		"ee.go": sets.NewString("erick", "ellen"),
	}
	testcases = []struct {
		name                       string
		filesChanged               []string
		reviewerCount              int
		maxReviewerCount           int
		expectedRequested          []string
		alternateExpectedRequested []string
	}{
		{
			name:              "one file, 3 leaf reviewers, 1 parent, request 3",
			filesChanged:      []string{"c.go"},
			reviewerCount:     3,
			expectedRequested: []string{"cole", "carl", "chad"},
		},
		{
			name:              "one file, 3 leaf reviewers, 1 parent reviewer, request 4",
			filesChanged:      []string{"c.go"},
			reviewerCount:     4,
			expectedRequested: []string{"cole", "carl", "chad", "charles"},
		},
		{
			name:              "two files, 2 leaf reviewers, 1 common parent, request 2",
			filesChanged:      []string{"a.go", "b.go"},
			reviewerCount:     2,
			expectedRequested: []string{"alice", "ben", "bob"},
		},
		{
			name:              "two files, 2 leaf reviewers, 1 common parent, request 3",
			filesChanged:      []string{"a.go", "b.go"},
			reviewerCount:     3,
			expectedRequested: []string{"alice", "ben", "bob", "al"},
		},
		{
			name:              "one files, 1 leaf reviewers, request 1",
			filesChanged:      []string{"a.go"},
			reviewerCount:     1,
			maxReviewerCount:  1,
			expectedRequested: []string{"alice", "ben"},
		},
		{
			name:              "one file, 2 leaf reviewer, 2 parent reviewers (1 dup), request 3",
			filesChanged:      []string{"e.go"},
			reviewerCount:     3,
			expectedRequested: []string{"erick", "ellen", "evan"},
		},
		{
			name:                       "two files, 2 leaf reviewer, 2 parent reviewers (1 dup), request 1",
			filesChanged:               []string{"e.go"},
			reviewerCount:              1,
			expectedRequested:          []string{"erick"},
			alternateExpectedRequested: []string{"ellen"},
		},
		{
			name:              "two files, 1 common leaf reviewer, one additional leaf, one parent, request 1",
			filesChanged:      []string{"b.go", "bb.go"},
			reviewerCount:     1,
			expectedRequested: []string{"bob", "ben"},
		},
		{
			name:              "two files, 2 leaf reviewers, 1 common parent, request 1",
			filesChanged:      []string{"a.go", "b.go"},
			reviewerCount:     1,
			expectedRequested: []string{"alice", "ben", "bob"},
		},
		{
			name:                       "two files, 2 leaf reviewers, 1 common parent, request 1, limit 2",
			filesChanged:               []string{"a.go", "b.go"},
			reviewerCount:              1,
			maxReviewerCount:           1,
			expectedRequested:          []string{"alice", "ben"},
			alternateExpectedRequested: []string{"ben", "bob"},
		},
	}
)

func TestShouldAssignReviewersWorksAsExpected(t *testing.T) {
	for _, test := range []struct{
		shouldAssign bool
		initial bool
		title string
		body string
	}{
		{
			shouldAssign: false,
			initial:      true,
			title:        "WIP PR",
			body:         "",
		},
		{
			shouldAssign: false,
			initial:      false,
			title:        "Test",
			body:         "",
		},
		{
			shouldAssign: true,
			initial:      false,
			title:        "WIP PR",
			body:         "/assign-reviewers\ntest",
		},
		{
			shouldAssign: false,
			initial:      true,
			title:        "Normal PR with cc",
			body:         "/cc @ausername",
		},
		{
			shouldAssign: false,
			initial:      true,
			title:        "Normal PR with no-assign",
			body:         "/no-assign-reviewers",
		},
		{
			shouldAssign: true,
			initial:      true,
			title:        "WIP test",
			body:         "/cc @test\n/assign-reviewers",
		},
		{
			shouldAssign: false,
			initial:      false,
			title:        "Testing PR",
			body:         "Normal followup message",
		},
	}{
		if shouldAssignReviewers(test.initial, test.title, test.body) != test.shouldAssign {
			t.Errorf("")
		}
	}
}

// TestHandleWithExcludeApprovers tests that the handle function requests
// reviews from the correct number of unique users when ExcludeApprovers is
// true.
func TestHandleWithExcludeApproversOnlyReviewers(t *testing.T) {
	foc := &fakeOwnersClient{
		owners:            owners,
		reviewers:         reviewers,
		requiredReviewers: requiredReviewers,
		leafReviewers:     leafReviewers,
	}

	for _, tc := range testcases {
		fghc := newFakeGithubClient(tc.filesChanged)
		pre := &github.PullRequestEvent{
			Number:      5,
			PullRequest: github.PullRequest{Number: 5, User: github.User{Login: "author"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if err := handle(fghc, foc, logrus.WithField("plugin", pluginName), &tc.reviewerCount, nil, tc.maxReviewerCount, true, &pre.Repo, &pre.PullRequest); err != nil {
			t.Errorf("[%s] unexpected error from handle: %v", tc.name, err)
			continue
		}

		sort.Strings(fghc.requested)
		sort.Strings(tc.expectedRequested)
		sort.Strings(tc.alternateExpectedRequested)
		if !reflect.DeepEqual(fghc.requested, tc.expectedRequested) {
			if len(tc.alternateExpectedRequested) > 0 {
				if !reflect.DeepEqual(fghc.requested, tc.alternateExpectedRequested) {
					t.Errorf("[%s] expected the requested reviewers to be %q or %q, but got %q.", tc.name, tc.expectedRequested, tc.alternateExpectedRequested, fghc.requested)
				}
				continue
			}
			t.Errorf("[%s] expected the requested reviewers to be %q, but got %q.", tc.name, tc.expectedRequested, fghc.requested)
		}
	}
}

// TestHandleWithoutExcludeApprovers verifies that behavior is the same
// when ExcludeApprovers is false and only approvers exist in the OWNERS files.
// The owners fixture and test cases should always be the same as the ones in
// TestHandleWithExcludeApprovers.
func TestHandleWithoutExcludeApproversNoReviewers(t *testing.T) {
	foc := &fakeOwnersClient{
		owners:            owners,
		approvers:         reviewers,
		leafApprovers:     leafReviewers,
		requiredReviewers: requiredReviewers,
	}

	for _, tc := range testcases {
		fghc := newFakeGithubClient(tc.filesChanged)
		pre := &github.PullRequestEvent{
			Number:      5,
			PullRequest: github.PullRequest{Number: 5, User: github.User{Login: "author"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if err := handle(fghc, foc, logrus.WithField("plugin", pluginName), &tc.reviewerCount, nil, tc.maxReviewerCount, false, &pre.Repo, &pre.PullRequest); err != nil {
			t.Errorf("[%s] unexpected error from handle: %v", tc.name, err)
			continue
		}

		sort.Strings(fghc.requested)
		sort.Strings(tc.expectedRequested)
		sort.Strings(tc.alternateExpectedRequested)
		if !reflect.DeepEqual(fghc.requested, tc.expectedRequested) {
			if len(tc.alternateExpectedRequested) > 0 {
				if !reflect.DeepEqual(fghc.requested, tc.alternateExpectedRequested) {
					t.Errorf("[%s] expected the requested reviewers to be %q or %q, but got %q.", tc.name, tc.expectedRequested, tc.alternateExpectedRequested, fghc.requested)
				}
				continue
			}
			t.Errorf("[%s] expected the requested reviewers to be %q, but got %q.", tc.name, tc.expectedRequested, fghc.requested)
		}
	}
}

func TestHandleWithoutExcludeApproversMixed(t *testing.T) {
	foc := &fakeOwnersClient{
		owners: map[string]string{
			"a.go":  "1",
			"b.go":  "2",
			"bb.go": "3",
			"c.go":  "4",

			"e.go":  "5",
			"ee.go": "5",
		},
		approvers: map[string]sets.String{
			"a.go": sets.NewString("al"),
			"b.go": sets.NewString("jeff"),
			"c.go": sets.NewString("jeff"),

			"e.go":  sets.NewString(),
			"ee.go": sets.NewString("larry"),
		},
		leafApprovers: map[string]sets.String{
			"a.go": sets.NewString("alice"),
			"b.go": sets.NewString("brad"),
			"c.go": sets.NewString("evan"),

			"e.go":  sets.NewString("erick", "evan"),
			"ee.go": sets.NewString("erick", "evan"),
		},
		reviewers: map[string]sets.String{
			"a.go": sets.NewString("al"),
			"b.go": sets.NewString(),
			"c.go": sets.NewString("charles"),

			"e.go":  sets.NewString("erick", "evan"),
			"ee.go": sets.NewString("erick", "evan"),
		},
		leafReviewers: map[string]sets.String{
			"a.go":  sets.NewString("alice"),
			"b.go":  sets.NewString("bob"),
			"bb.go": sets.NewString("bob", "ben"),
			"c.go":  sets.NewString("cole", "carl", "chad"),

			"e.go":  sets.NewString("erick", "ellen"),
			"ee.go": sets.NewString("erick", "ellen"),
		},
	}

	var testcases = []struct {
		name                       string
		filesChanged               []string
		reviewerCount              int
		maxReviewerCount           int
		expectedRequested          []string
		alternateExpectedRequested []string
	}{
		{
			name:              "1 file, 1 leaf reviewer, 1 leaf approver, 1 approver, request 3",
			filesChanged:      []string{"b.go"},
			reviewerCount:     3,
			expectedRequested: []string{"bob", "brad", "jeff"},
		},
		{
			name:              "1 file, 1 leaf reviewer, 1 leaf approver, 1 approver, request 1, limit 1",
			filesChanged:      []string{"b.go"},
			reviewerCount:     1,
			expectedRequested: []string{"bob"},
		},
		{
			name:              "2 file, 2 leaf reviewers, 1 parent reviewers, 1 leaf approver, 1 approver, request 5",
			filesChanged:      []string{"a.go", "b.go"},
			reviewerCount:     5,
			expectedRequested: []string{"alice", "bob", "al", "brad", "jeff"},
		},
		{
			name:              "1 file, 1 leaf reviewer+approver, 1 reviewer+approver, request 3",
			filesChanged:      []string{"a.go"},
			reviewerCount:     3,
			expectedRequested: []string{"alice", "al"},
		},
		{
			name:              "1 file, 2 leaf reviewers, request 2",
			filesChanged:      []string{"e.go"},
			reviewerCount:     2,
			expectedRequested: []string{"erick", "ellen"},
		},
		{
			name:              "2 files, 2 leaf+parent reviewers, 1 parent reviewer, 1 parent approver, request 4",
			filesChanged:      []string{"e.go", "ee.go"},
			reviewerCount:     4,
			expectedRequested: []string{"erick", "ellen", "evan", "larry"},
		},
	}
	for _, tc := range testcases {
		fghc := newFakeGithubClient(tc.filesChanged)
		pre := &github.PullRequestEvent{
			Number:      5,
			PullRequest: github.PullRequest{Number: 5, User: github.User{Login: "author"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if err := handle(fghc, foc, logrus.WithField("plugin", pluginName), &tc.reviewerCount, nil, tc.maxReviewerCount, false, &pre.Repo, &pre.PullRequest); err != nil {
			t.Errorf("[%s] unexpected error from handle: %v", tc.name, err)
			continue
		}

		sort.Strings(fghc.requested)
		sort.Strings(tc.expectedRequested)
		sort.Strings(tc.alternateExpectedRequested)
		if !reflect.DeepEqual(fghc.requested, tc.expectedRequested) {
			if len(tc.alternateExpectedRequested) > 0 {
				if !reflect.DeepEqual(fghc.requested, tc.alternateExpectedRequested) {
					t.Errorf("[%s] expected the requested reviewers to be %q or %q, but got %q.", tc.name, tc.expectedRequested, tc.alternateExpectedRequested, fghc.requested)
				}
				continue
			}
			t.Errorf("[%s] expected the requested reviewers to be %q, but got %q.", tc.name, tc.expectedRequested, fghc.requested)
		}
	}
}

func TestHandleOld(t *testing.T) {
	foc := &fakeOwnersClient{
		reviewers: map[string]sets.String{
			"c.go": sets.NewString("charles"),
			"d.go": sets.NewString("dan"),
			"e.go": sets.NewString("erick", "evan"),
		},
		leafReviewers: map[string]sets.String{
			"a.go": sets.NewString("alice"),
			"b.go": sets.NewString("bob"),
			"c.go": sets.NewString("cole", "carl", "chad"),
			"e.go": sets.NewString("erick"),
		},
	}

	var testcases = []struct {
		name              string
		filesChanged      []string
		reviewerCount     int
		expectedRequested []string
	}{
		{
			name:              "one file, 3 leaf reviewers, request 3",
			filesChanged:      []string{"c.go"},
			reviewerCount:     3,
			expectedRequested: []string{"cole", "carl", "chad"},
		},
		{
			name:              "one file, 3 leaf reviewers, 1 parent reviewer, request 4",
			filesChanged:      []string{"c.go"},
			reviewerCount:     4,
			expectedRequested: []string{"cole", "carl", "chad", "charles"},
		},
		{
			name:              "two files, 2 leaf reviewers, request 2",
			filesChanged:      []string{"a.go", "b.go"},
			reviewerCount:     2,
			expectedRequested: []string{"alice", "bob"},
		},
		{
			name:              "one files, 1 leaf reviewers, request 1",
			filesChanged:      []string{"a.go"},
			reviewerCount:     1,
			expectedRequested: []string{"alice"},
		},
		{
			name:              "one file, 0 leaf reviewers, 1 parent reviewer, request 1",
			filesChanged:      []string{"d.go"},
			reviewerCount:     1,
			expectedRequested: []string{"dan"},
		},
		{
			name:              "one file, 0 leaf reviewers, 1 parent reviewer, request 2",
			filesChanged:      []string{"d.go"},
			reviewerCount:     2,
			expectedRequested: []string{"dan"},
		},
		{
			name:              "one file, 1 leaf reviewers, 2 parent reviewers (1 dup), request 2",
			filesChanged:      []string{"e.go"},
			reviewerCount:     2,
			expectedRequested: []string{"erick", "evan"},
		},
	}
	for _, tc := range testcases {
		fghc := newFakeGithubClient(tc.filesChanged)
		pre := &github.PullRequestEvent{
			Number:      5,
			PullRequest: github.PullRequest{Number: 5, User: github.User{Login: "author"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if err := handle(fghc, foc, logrus.WithField("plugin", pluginName), nil, &tc.reviewerCount, 0, false, &pre.Repo, &pre.PullRequest); err != nil {
			t.Errorf("[%s] unexpected error from handle: %v", tc.name, err)
			continue
		}

		sort.Strings(fghc.requested)
		sort.Strings(tc.expectedRequested)
		if !reflect.DeepEqual(fghc.requested, tc.expectedRequested) {
			t.Errorf("[%s] expected the requested reviewers to be %q, but got %q.", tc.name, tc.expectedRequested, fghc.requested)
		}
	}
}

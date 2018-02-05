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
	owners        map[string]string
	reviewers     map[string]sets.String
	leafReviewers map[string]sets.String
}

func (foc *fakeOwnersClient) Reviewers(path string) sets.String {
	return foc.reviewers[path]
}

func (foc *fakeOwnersClient) LeafReviewers(path string) sets.String {
	return foc.leafReviewers[path]
}

func (foc *fakeOwnersClient) FindReviewersOwnersForFile(path string) string {
	return foc.owners[path]
}

// TestHandle tests that the handle function requests reviews from the correct number of unique users.
func TestHandle(t *testing.T) {
	foc := &fakeOwnersClient{
		owners: map[string]string{
			"a.go":  "1",
			"b.go":  "2",
			"bb.go": "3",
			"c.go":  "4",

			"e.go":  "5",
			"ee.go": "5",
		},
		reviewers: map[string]sets.String{
			"a.go": sets.NewString("al"),
			"b.go": sets.NewString("al"),
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
			expectedRequested: []string{"alice", "bob"},
		},
		{
			name:              "two files, 2 leaf reviewers, 1 common parent, request 3",
			filesChanged:      []string{"a.go", "b.go"},
			reviewerCount:     3,
			expectedRequested: []string{"alice", "bob", "al"},
		},
		{
			name:              "one files, 1 leaf reviewers, request 1",
			filesChanged:      []string{"a.go"},
			reviewerCount:     1,
			expectedRequested: []string{"alice"},
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
	}
	for _, tc := range testcases {
		fghc := newFakeGithubClient(tc.filesChanged)
		pre := &github.PullRequestEvent{
			Number:      5,
			PullRequest: github.PullRequest{User: github.User{Login: "author"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if err := handle(fghc, foc, logrus.WithField("plugin", pluginName), &tc.reviewerCount, nil, pre); err != nil {
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
			PullRequest: github.PullRequest{User: github.User{Login: "author"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if err := handle(fghc, foc, logrus.WithField("plugin", pluginName), nil, &tc.reviewerCount, pre); err != nil {
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

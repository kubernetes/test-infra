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

package welcome

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

// TODO(bentheelder): these tests are a bit lame.
// There has to be a better way to write tests like this.

const (
	testWelcomeTemplate = "Welcome human! 🤖 {{.AuthorName}} {{.AuthorLogin}} {{.Repo}} {{.Org}}}"
)

type fakeClient struct {
	commentsAdded map[int][]string
	prs           map[string]sets.Int
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		commentsAdded: make(map[int][]string),
		prs:           make(map[string]sets.Int),
	}
}

// CreateComment adds and tracks a comment in the client
func (fc *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	fc.commentsAdded[number] = append(fc.commentsAdded[number], comment)
	return nil
}

// ClearComments removes all comments in the client
func (fc *fakeClient) ClearComments() {
	fc.commentsAdded = map[int][]string{}
}

// NumComments counts the number of tracked comments
func (fc *fakeClient) NumComments() int {
	n := 0
	for _, comments := range fc.commentsAdded {
		n += len(comments)
	}
	return n
}

var (
	expectedQueryRegex = regexp.MustCompile(`is:pr repo:(.+)/(.+) author:(.+)`)
)

// AddPR records an PR in the client
func (fc *fakeClient) AddPR(owner, repo, author string, number int) {
	key := fmt.Sprintf("%s,%s,%s", owner, repo, author)
	if _, ok := fc.prs[key]; !ok {
		fc.prs[key] = sets.Int{}
	}
	fc.prs[key].Insert(number)
}

// ClearPRs removes all PRs from the client
func (fc *fakeClient) ClearPRs() {
	fc.prs = make(map[string]sets.Int)
}

// FindIssues fails if the query does not match the expected query regex and
// looks up issues based on parsing the expected query format
func (fc *fakeClient) FindIssues(query, sort string, asc bool) ([]github.Issue, error) {
	fields := expectedQueryRegex.FindStringSubmatch(query)
	if fields == nil || len(fields) != 4 {
		return nil, fmt.Errorf("invalid query: `%s` does not match expected regex `%s`", query, expectedQueryRegex.String())
	}
	// "find" results
	owner, repo, author := fields[1], fields[2], fields[3]
	key := fmt.Sprintf("%s,%s,%s", owner, repo, author)

	issues := []github.Issue{}
	for _, number := range fc.prs[key].List() {
		issues = append(issues, github.Issue{
			Number: number,
		})
	}
	return issues, nil
}

func makeFakePullRequestEvent(owner, repo, author string, number int, action github.PullRequestEventAction) github.PullRequestEvent {
	return github.PullRequestEvent{
		Action: action,
		Number: number,
		PullRequest: github.PullRequest{
			Base: github.PullRequestBranch{
				Repo: github.Repo{
					Owner: github.User{
						Login: owner,
					},
					Name: repo,
				},
			},
			User: github.User{
				Login: author,
				Name:  author + "fullname",
			},
		},
	}
}

func TestHandlePR(t *testing.T) {
	fc := newFakeClient()
	// old PRs
	fc.AddPR("kubernetes", "test-infra", "contributorA", 1)
	fc.AddPR("kubernetes", "test-infra", "contributorB", 2)
	fc.AddPR("kubernetes", "test-infra", "contributorB", 3)

	testCases := []struct {
		name          string
		repoOwner     string
		repoName      string
		author        string
		prNumber      int
		prAction      github.PullRequestEventAction
		expectComment bool
	}{
		{
			name:          "existing contributorA",
			repoOwner:     "kubernetes",
			repoName:      "test-infra",
			author:        "contributorA",
			prNumber:      20,
			prAction:      github.PullRequestActionOpened,
			expectComment: false,
		},
		{
			name:          "existing contributorB",
			repoOwner:     "kubernetes",
			repoName:      "test-infra",
			author:        "contributorB",
			prNumber:      40,
			prAction:      github.PullRequestActionOpened,
			expectComment: false,
		},
		{
			name:          "new contributor",
			repoOwner:     "kubernetes",
			repoName:      "test-infra",
			author:        "newContributor",
			prAction:      github.PullRequestActionOpened,
			prNumber:      50,
			expectComment: true,
		},
		{
			name:          "new contributor, not PR open event",
			repoOwner:     "kubernetes",
			repoName:      "test-infra",
			author:        "newContributor",
			prAction:      github.PullRequestActionEdited,
			prNumber:      50,
			expectComment: false,
		},
	}

	c := client{
		GitHubClient: fc,
		Logger:       &logrus.Entry{},
	}
	for _, tc := range testCases {
		// clear out comments from the last test case
		fc.ClearComments()

		event := makeFakePullRequestEvent(tc.repoOwner, tc.repoName, tc.author, tc.prNumber, tc.prAction)
		// make sure the PR in the event is recorded
		fc.AddPR(tc.repoOwner, tc.repoName, tc.author, tc.prNumber)

		// try handling it
		if err := handlePR(c, event, testWelcomeTemplate); err != nil {
			t.Fatalf("did not expect error handling PR for case '%s': %v", tc.name, err)
		}

		// verify that comments were made
		numComments := fc.NumComments()
		if numComments > 1 {
			t.Fatalf("did not expect multiple comments for any test case and got %d comments", numComments)
		}
		if numComments == 0 && tc.expectComment {
			t.Fatalf("expected a comment for case '%s' and got none", tc.name)
		} else if numComments > 0 && !tc.expectComment {
			t.Fatalf("did not expect comments for case '%s' and got %d comments", tc.name, numComments)
		}
	}
}

/*
Copyright 2019 The Kubernetes Authors.

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

package issue_gatherer

import (
	"fmt"
	"net/http"
	"os"
	"testing"
)

// Mocks and Fakes

type fakeGitHub struct{}

func (fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	testReqs := map[string]struct {
		fileLoc    string
		linkHeader string
	}{
		"/repos/kubernetes/pageless/issues": {
			"./testdata/issues.json",
			"",
		},
		"/repos/kubernetes/paged/issues": {
			"./testdata/issues.json",
			`<https://api.github.com/repos/kubernetes/paged/issues?page=2>; rel="next", <https://api.github.com/repos/kubernetes/paged/issues?page=2>; rel="last"`,
		},
		"/repos/kubernetes/paged/issues?page=2": {
			"./testdata/issues2.json",
			`<https://api.github.com/repos/kubernetes/paged/issues?page=1>; rel="prev", <https://api.github.com/repos/kubernetes/paged/issues?page=1>; rel="first"`,
		},
		"/repos/kubernetes/example/issues/11111/comments": {
			"./testdata/Comments.json",
			"",
		},
		"/repos/kubernetes/example/issues/11115/comments": {
			"./testdata/NoComments.json",
			"",
		},
		"/repos/kubernetes/example/issues/20000/comments": {
			"./testdata/NoComments.json",
			"",
		},
	}

	testEntry := testReqs[req.URL.RequestURI()]

	file, err := os.Open(testEntry.fileLoc)
	if err != nil {
		return nil, err
	}

	hdr := make(http.Header, 0)
	hdr.Add("Link", testEntry.linkHeader)

	return &http.Response{
		Status: "200 OK",
		Body:   file,
		Header: hdr,
	}, nil
}

// Tests

func TestGitHubIssueScraper_GetIssues(t *testing.T) {
	testCases := []struct {
		repo     string
		expected int
	}{
		{"kubernetes/pageless", 2},
		{"kubernetes/paged", 3},
	}

	for _, test := range testCases {
		g := NewIssueScraper(test.repo)
		g.githubRoundTripper = fakeGitHub{}
		result := g.GetIssues()

		for _, issue := range result {
			if issue.IssueId == 0 || issue.Title == "" || issue.CommentsUrl == "" {
				t.Errorf("Issue Error: Num: %d, Title %s, Body %s", issue.IssueId, issue.Title, issue.Body)
			}
		}

		if len(result) != test.expected {
			t.Errorf("Wrong number of issues; got %d, expected %d", len(result), test.expected)
		}
	}
}

func TestGitHubIssueScraper_GetIssues_AssertComments(t *testing.T) {
	testCases := map[string][]int{
		"kubernetes/pageless": {2, 0},
		"kubernetes/paged":    {2, 0, 0},
	}

	for repo, expected := range testCases {
		g := NewIssueScraper(repo)
		g.githubRoundTripper = fakeGitHub{}
		result := g.GetIssues()

		for i, issue := range result {
			if expected[i] != len(issue.Comments) {
				t.Errorf("Wrong number of comments; got %d, expected %d", len(issue.Comments), expected[i])
			}
		}
	}
}

func TestGitHubIssueScraper_NewIssueScraper(t *testing.T) {
	testCases := []string{
		"kubernetes/test-infra",
		"foo",
		"",
	}

	for _, repo := range testCases {
		g := NewIssueScraper(repo)
		if g.RepoInfix != repo {
			t.Errorf("Scraper Constructor Fails: expected %s, got %s", repo, g.RepoInfix)
		}
	}
}

// Examples

func ExampleGitHubIssueScraper_GetIssues() {
	g := NewIssueScraper("kubernetes/test-infra")
	result := g.GetIssues()

	fmt.Printf("kubernetes/test-infra has %d open issues", len(result))
}

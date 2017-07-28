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

package githubutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-github/github"
)

// fakeGithub is a fake go-github client that doubles as a test instance representation. This fake
// client keeps track of the number of API calls made in order to test retry behavior and also allows
// the number of pages of results to be configured.
type fakeGithub struct {
	// name is a string representation of this fakeGithub instance.
	name string
	// hits is a count of the number of API calls made to fakeGithub.
	hits int
	// hitsBeforeResponse is the number of hits that should be received before fakeGithub responds without error.
	hitsBeforeResponse int
	// shouldSucceed indicates if the githubutil client should get a valid response.
	shouldSucceed bool
	// pages is the number of pages to return for paginated data. (Should be 1 for non-paginated data)
	pages int
}

// checkHits verifies that the githubutil client made the correct number of retries before returning.
func (f *fakeGithub) checkHits() bool {
	if f.shouldSucceed {
		return f.hits-f.pages+1 == f.hitsBeforeResponse
	}
	return f.hitsBeforeResponse > f.hits
}

// newTestClient creates a new githubutil client from a fakeGithub test instance.
func newTestClient(f *fakeGithub) *Client {
	return &Client{
		prService:           f,
		repoService:         f,
		retries:             5,
		retryInitialBackoff: time.Nanosecond,
		tokenReserve:        50,
		dryRun:              false,
	}
}

func (f *fakeGithub) CreateStatus(ctx context.Context, org, repo, ref string, status *github.RepoStatus) (*github.RepoStatus, *github.Response, error) {
	f.hits++
	if f.hits >= f.hitsBeforeResponse {
		return status, &github.Response{Rate: github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}}}, nil
	}
	return nil, nil, fmt.Errorf("some error that forces a retry")
}

func (f *fakeGithub) GetCombinedStatus(ctx context.Context, org, repo, ref string, opts *github.ListOptions) (*github.CombinedStatus, *github.Response, error) {
	f.hits++
	context := fmt.Sprintf("context %d", f.hits-f.hitsBeforeResponse+1)
	combStatus := &github.CombinedStatus{Statuses: []github.RepoStatus{{Context: &context}}}
	if f.hits >= f.hitsBeforeResponse {
		return combStatus, &github.Response{Rate: github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}}, LastPage: f.pages}, nil
	}
	return nil, nil, fmt.Errorf("some error that forces a retry")
}

func (f *fakeGithub) List(ctx context.Context, org, repo string, opts *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error) {
	f.hits++
	title := fmt.Sprintf("pr %d", f.hits-f.hitsBeforeResponse+1)
	list := []*github.PullRequest{{Title: &title}}
	if f.hits >= f.hitsBeforeResponse {
		return list, &github.Response{Rate: github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}}, LastPage: f.pages}, nil
	}
	return nil, nil, fmt.Errorf("some error that forces a retry")
}

// TestCreateStatus tests the CreateStatus function to ensure that it does retries and returns the correct result.
func TestCreateStatus(t *testing.T) {
	contextStr := "context"
	stateStr := "some state"
	descStr := "descriptive description"
	urlStr := "link"
	sampleStatus := &github.RepoStatus{
		Context:     &contextStr,
		State:       &stateStr,
		Description: &descStr,
		TargetURL:   &urlStr,
	}
	tests := []*fakeGithub{
		{name: "no retries", hitsBeforeResponse: 1, shouldSucceed: true, pages: 1},
		{name: "max retries", hitsBeforeResponse: 6, shouldSucceed: true, pages: 1},
		{name: "1 too many retries needed", hitsBeforeResponse: 7, shouldSucceed: false, pages: 1},
		{name: "3 too many retries needed", hitsBeforeResponse: 10, shouldSucceed: false, pages: 1},
	}
	for _, test := range tests {
		client := newTestClient(test)
		one := 1
		ref := "fake-ref"
		pr := &github.PullRequest{Number: &one, Head: &github.PullRequestBranch{SHA: &ref}}
		status, _, err := client.CreateStatus("org", "repo", pr, sampleStatus)
		if (err == nil) != test.shouldSucceed {
			t.Errorf("CreateStatus test '%s' failed because the error value was unexpected: %v", test.name, err)
		}
		if !test.checkHits() {
			t.Errorf("CreateStatus test '%s' failed with the wrong number of hits: %d", test.name, test.hits)
		}
		if test.shouldSucceed {
			if status == nil {
				t.Fatalf("CreateStatus test '%s' failed because the returned status was nil", test.name)
			}
			if status.Context == nil || *status.Context != contextStr {
				t.Errorf("CreateStatus test '%s' failed because the returned RepoStatus had a context of '%v' instead of '%s'", test.name, *status.Context, contextStr)
			}
			if status.State == nil || *status.State != stateStr {
				t.Errorf("CreateStatus test '%s' failed because the returned RepoStatus had a state of '%v' instead of '%s'", test.name, *status.State, stateStr)
			}
			if status.Description == nil || *status.Description != descStr {
				t.Errorf("CreateStatus test '%s' failed because the returned RepoStatus had a description of '%v' instead of '%s'", test.name, *status.Description, descStr)
			}
			if status.TargetURL == nil || *status.TargetURL != urlStr {
				t.Errorf("CreateStatus test '%s' failed because the returned RepoStatus had a target URL of '%v' instead of '%s'", test.name, *status.TargetURL, urlStr)
			}
		}
	}
}

// TestGetCombinedStatus tests the GetCombinedStatus function to ensure it does retries, handles pagination,
// and returns the correct result.
func TestGetCombinedStatus(t *testing.T) {
	tests := []*fakeGithub{
		{name: "no retries", hitsBeforeResponse: 1, shouldSucceed: true, pages: 1},
		{name: "max retries", hitsBeforeResponse: 6, shouldSucceed: true, pages: 1},
		{name: "1 too many retries needed", hitsBeforeResponse: 7, shouldSucceed: false, pages: 1},
		{name: "3 too many retries needed", hitsBeforeResponse: 10, shouldSucceed: false, pages: 1},
		{name: "2 pages", hitsBeforeResponse: 1, shouldSucceed: true, pages: 2},
		{name: "10 pages", hitsBeforeResponse: 1, shouldSucceed: true, pages: 10},
		{name: "2 pages 2 retries", hitsBeforeResponse: 3, shouldSucceed: true, pages: 2},
		{name: "10 pages max retries", hitsBeforeResponse: 6, shouldSucceed: true, pages: 10},
		{name: "10 pages one too many retries", hitsBeforeResponse: 7, shouldSucceed: false, pages: 10},
	}

	for _, test := range tests {
		client := newTestClient(test)
		combStatus, _, err := client.GetCombinedStatus("org", "repo", "ref")
		if (err == nil) != test.shouldSucceed {
			t.Errorf("GetCombinedStatus test '%s' failed because the error value was unexpected: %v", test.name, err)
		}
		if !test.checkHits() {
			t.Errorf("GetCombinedStatus test '%s' failed with the wrong number of hits: %d", test.name, test.hits)
		}
		if test.shouldSucceed {
			if combStatus == nil {
				t.Fatalf("GetCombinedStatus test '%s' failed because the combined status was nil.", test.name)
			}
			if len(combStatus.Statuses) != test.pages {
				t.Errorf("GetCombinedStatus test '%s' failed because the number of pages returned was %d instead of %d.", test.name, len(combStatus.Statuses), test.pages)
			}
			for index, status := range combStatus.Statuses {
				if status.Context == nil {
					t.Fatalf("GetCombinedStatus test '%s' failed because the status at index %d had a nil Context field.", test.name, index)
				}
				expectedContext := fmt.Sprintf("context %d", index+1)
				if *status.Context != expectedContext {
					t.Errorf("GetCombinedStatus test '%s' failed because the status at index %d had a context of '%s' instead of '%s'.", test.name, index, *status.Context, expectedContext)
				}
			}
		}
	}
}

// TestForEachPR tests the ForEachPR function to ensure it does retries properly, can handle pagination,
// and returns the correct result.
func TestForEachPR(t *testing.T) {
	tests := []*fakeGithub{
		{name: "no retries", hitsBeforeResponse: 1, shouldSucceed: true, pages: 1},
		{name: "max retries", hitsBeforeResponse: 6, shouldSucceed: true, pages: 1},
		{name: "1 too many retries needed", hitsBeforeResponse: 7, shouldSucceed: false, pages: 1},
		{name: "3 too many retries needed", hitsBeforeResponse: 10, shouldSucceed: false, pages: 1},
		{name: "2 pages", hitsBeforeResponse: 1, shouldSucceed: true, pages: 2},
		{name: "10 pages", hitsBeforeResponse: 1, shouldSucceed: true, pages: 10},
		{name: "2 pages 2 retries", hitsBeforeResponse: 3, shouldSucceed: true, pages: 2},
		{name: "10 pages max retries", hitsBeforeResponse: 6, shouldSucceed: true, pages: 10},
		{name: "10 pages one too many retries", hitsBeforeResponse: 7, shouldSucceed: false, pages: 10},
		{name: "continue on final error", hitsBeforeResponse: 1, shouldSucceed: true, pages: 13},
		{name: "continue on error", hitsBeforeResponse: 1, shouldSucceed: true, pages: 16},
	}

	for _, test := range tests {
		client := newTestClient(test)

		processed := 0
		process := func(pr *github.PullRequest) error {
			if pr == nil || pr.Title == nil {
				return fmt.Errorf("pr %d was invalid", processed+1)
			}
			expectedTitle := fmt.Sprintf("pr %d", processed+1)
			if *pr.Title != expectedTitle {
				return fmt.Errorf("expected pr title '%s' but got '%s'", expectedTitle, *pr.Title)
			}
			processed++
			if processed == 13 {
				// 13th PR processed returns an error.
				return fmt.Errorf("some munge error")
			}
			return nil
		}
		continueOnError := test.pages >= 13
		err := client.ForEachPR("org", "repo", &github.PullRequestListOptions{}, continueOnError, process)
		if (err == nil) != test.shouldSucceed {
			t.Errorf("ForEachPR test '%s' failed because the error value was unexpected: %v", test.name, err)
		}
		if !test.checkHits() {
			t.Errorf("ForEachPR test '%s' failed with the wrong number of hits: %d", test.name, test.hits)
		}
		if test.shouldSucceed && processed != test.pages {
			t.Errorf("ForEachPR test '%s' processed %d PRs, but there were only %d single entry pages.", test.name, processed, test.pages)
		}
	}
}

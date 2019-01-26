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

package ghclient

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-github/github"
)

func setForTest(client *Client) {
	client.retries = 5
	client.retryInitialBackoff = time.Nanosecond
	client.tokenReserve = 50
}

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
	// listOpts is the ListOptions that would be used in the call if the call were real.
	listOpts github.ListOptions
}

// checkHits verifies that the githubutil client made the correct number of retries before returning.
func (f *fakeGithub) checkHits() bool {
	if f.shouldSucceed {
		return f.hits-f.pages+1 == f.hitsBeforeResponse
	}
	return f.hitsBeforeResponse > f.hits
}

func (f *fakeGithub) call() ([]interface{}, *github.Response, error) {
	f.hits++
	if f.hits >= f.hitsBeforeResponse {
		return []interface{}{f.listOpts.Page},
			&github.Response{Rate: github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}}, LastPage: f.pages},
			nil
	}
	return nil, nil, fmt.Errorf("some error that forces a retry")
}

func TestRetryAndPagination(t *testing.T) {
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
		client := &Client{}
		setForTest(client)
		pages, err := client.depaginate("retry test", &test.listOpts, test.call)
		if (err == nil) != test.shouldSucceed {
			t.Errorf("Retry+Pagination test '%s' failed because the error value was unexpected: %v", test.name, err)
		}
		if !test.checkHits() {
			t.Errorf("Retry+Pagination test '%s' failed with the wrong number of hits: %d", test.name, test.hits)
		}
		if test.shouldSucceed && len(pages) != test.pages {
			t.Errorf("Retry+Pagination test '%s' failed because the number of pages returned was %d instead of %d. Pages returned: %#v", test.name, len(pages), test.pages, pages)
		}
	}
}

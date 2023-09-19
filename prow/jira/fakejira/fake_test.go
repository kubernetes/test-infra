/*
Copyright 2020 The Kubernetes Authors.

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

package fakejira

import (
	"context"
	"github.com/andygrunwald/go-jira"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"testing"
)

func TestFakeClient_SearchWithContext(t *testing.T) {
	s := make(map[SearchRequest]SearchResponse)
	issueList := []jira.Issue{
		{
			ID:     "123",
			Fields: &jira.IssueFields{Project: jira.Project{Name: "test"}},
		},
		{
			ID:     "1234",
			Fields: &jira.IssueFields{Project: jira.Project{Name: "test"}},
		},
		{
			ID:     "12345",
			Fields: &jira.IssueFields{Project: jira.Project{Name: "test"}},
		},
	}
	searchOptions := &jira.SearchOptions{MaxResults: 50, StartAt: 0}

	s[SearchRequest{query: "project=test", options: searchOptions}] = SearchResponse{
		issues:   issueList,
		response: &jira.Response{StartAt: 0, MaxResults: 3, Total: 3},
		error:    nil,
	}
	fakeClient := &FakeClient{SearchResponses: s}

	r, v, err := fakeClient.SearchWithContext(context.Background(), "project=test", searchOptions)
	if err != nil {
		t.Fatalf("unexpected error from search: %s", err)
	}
	cmpOption := cmpopts.IgnoreUnexported(jira.Date{})
	if diff := cmp.Diff(r, issueList, cmpOption); diff != "" {
		t.Fatalf("incorrect issues from search: %v", diff)
	}
	if diff := cmp.Diff(&jira.Response{StartAt: 0, MaxResults: 3, Total: 3}, v, cmpOption); diff != "" {
		t.Fatalf("incorrect metadata from search: %v", diff)
	}

	r, v, err = fakeClient.SearchWithContext(context.Background(), "unknown_query=fail", searchOptions)
	if r != nil {
		t.Fatalf("expected empty result for an invalid query, but got: %v", r)
	}
	if r != nil {
		t.Fatalf("expected no metadata for an invalid query, but got: %v", v)
	}
	if err == nil {
		t.Fatal("expected invalid query to fail, but got no error")
	}
}

func TestFakeClient_GetProjectVersions(t *testing.T) {
	fakeClient := &FakeClient{
		ProjectVersions: map[string][]*jira.Version{
			"ABC": {
				{
					Name: "Version1",
				},
				{
					Name: "Version2",
				},
				{
					Name: "Version3",
				},
			},
		},
	}

	for _, project := range []string{"ABC", "FOO"} {
		versions, err := fakeClient.GetProjectVersions(project)
		if len(versions) != len(fakeClient.ProjectVersions[project]) {
			t.Fatalf("expected: %d results, but got: %d", len(fakeClient.ProjectVersions[project]), len(versions))
		}
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
	}
}

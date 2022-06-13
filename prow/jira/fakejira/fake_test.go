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
	"reflect"
	"testing"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/trivago/tgo/tcontainer"
	jiraclient "k8s.io/test-infra/prow/jira"
)

var allowJiraDate = cmp.AllowUnexported(jira.Date{})

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

func TestCreateIssue(t *testing.T) {
	testCases := []struct {
		name             string
		issue            jira.Issue
		existingIssues   []jira.Issue
		expectedIssue    *jira.Issue
		createIssueError *jiraclient.CreateIssueError
	}{{
		name: "create single issue",
		issue: jira.Issue{
			Fields: &jira.IssueFields{
				Project: jira.Project{
					Name: "ABC",
				},
			},
		},
		expectedIssue: &jira.Issue{
			ID:  "1",
			Key: "ABC-1",
			Fields: &jira.IssueFields{
				Project: jira.Project{
					Name: "ABC",
				},
			},
		},
	}, {
		name: "create issue with other issues in other projects",
		issue: jira.Issue{
			Fields: &jira.IssueFields{
				Project: jira.Project{
					Name: "ABC",
				},
			},
		},
		existingIssues: []jira.Issue{
			{
				ID:  "22",
				Key: "ABC-41",
				Fields: &jira.IssueFields{
					Project: jira.Project{
						Name: "ABC",
					},
				},
			},
			{
				ID:  "52",
				Key: "DEF-16",
				Fields: &jira.IssueFields{
					Project: jira.Project{
						Name: "DEF",
					},
				},
			},
		},
		expectedIssue: &jira.Issue{
			ID:  "53",
			Key: "ABC-42",
			Fields: &jira.IssueFields{
				Project: jira.Project{
					Name: "ABC",
				},
			},
		},
	}, {
		name: "create issue with comments and Status set",
		issue: jira.Issue{
			Fields: &jira.IssueFields{
				Project: jira.Project{
					Name: "ABC",
				},
				Status: &jira.Status{
					Name: "NEW",
				},
				Comments: &jira.Comments{
					Comments: []*jira.Comment{{Body: "Hello"}},
				},
			},
		},
		createIssueError: &jiraclient.CreateIssueError{
			Errors: map[string]string{
				"comment": "this field cannot be set",
				"status":  "this field cannot be set",
			},
		},
	}}
	for _, tc := range testCases {
		ptrIssues := []*jira.Issue{}
		for index := range tc.existingIssues {
			ptrIssues = append(ptrIssues, &tc.existingIssues[index])
		}
		fakeClient := &FakeClient{Issues: ptrIssues}
		newIssue, err := fakeClient.CreateIssue(&tc.issue)
		if err != nil && tc.createIssueError == nil {
			t.Fatalf("%s: received error where none was expected: %v", tc.name, err)
		}
		if err == nil && tc.createIssueError != nil {
			t.Fatalf("%s: received no error where one was expected", tc.name)
		}
		if tc.expectedIssue != nil {
			if !reflect.DeepEqual(newIssue, tc.expectedIssue) {
				t.Errorf("%s: got incorrect issue after clone: %s", tc.name, cmp.Diff(newIssue, tc.expectedIssue, allowJiraDate))
			}
		}
	}
}

func TestCloneIssue(t *testing.T) {
	testCases := []struct {
		name              string
		issue             jira.Issue
		existingIssues    []jira.Issue
		expectedIssue     *jira.Issue
		expectedIssueLink *jira.IssueLink
	}{{
		name: "clone a basic issue with only project, description, and status set",
		issue: jira.Issue{
			ID:  "22",
			Key: "ABC-41",
			Fields: &jira.IssueFields{
				Description: "This is a test issue",
				Status: &jira.Status{
					Name: "POST",
				},
				Project: jira.Project{
					Name: "ABC",
				},
			},
		},
		existingIssues: []jira.Issue{
			{
				ID:  "22",
				Key: "ABC-41",
				Fields: &jira.IssueFields{
					Description: "This is a test issue",
					Status: &jira.Status{
						Name: "POST",
					},
					Project: jira.Project{
						Name: "ABC",
					},
				},
			},
		},
		expectedIssue: &jira.Issue{
			ID:  "23",
			Key: "ABC-42",
			Fields: &jira.IssueFields{
				Description: "This is a clone of issue ABC-41. The following is the description of the original issue: \n---\nThis is a test issue",
				Project: jira.Project{
					Name: "ABC",
				},
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "22"},
				}},
				Unknowns: tcontainer.MarshalMap{},
			},
		},
		expectedIssueLink: &jira.IssueLink{
			Type: jira.IssueLinkType{
				Name:    "Cloners",
				Inward:  "is cloned by",
				Outward: "clones",
			},
			OutwardIssue: &jira.Issue{ID: "22"},
			InwardIssue:  &jira.Issue{ID: "23"},
		},
	}}
	for _, tc := range testCases {
		ptrIssues := []*jira.Issue{}
		for index := range tc.existingIssues {
			ptrIssues = append(ptrIssues, &tc.existingIssues[index])
		}
		fakeClient := &FakeClient{Issues: ptrIssues}
		newIssue, err := fakeClient.CloneIssue(&tc.issue)
		if err != nil {
			t.Fatalf("%s: received error where none was expected: %v", tc.name, err)
		}
		if tc.expectedIssue != nil {
			if !reflect.DeepEqual(newIssue, tc.expectedIssue) {
				t.Errorf("%s: got incorrect issue after clone: %s", tc.name, cmp.Diff(newIssue, tc.expectedIssue, allowJiraDate))
			}
		}
		if tc.expectedIssueLink != nil {
			if len(fakeClient.IssueLinks) != 1 {
				t.Fatalf("%s: expected 1 issue link, got %d", tc.name, len(fakeClient.IssueLinks))
			}
			if !reflect.DeepEqual(newIssue, tc.expectedIssue) {
				t.Errorf("%s: got incorrect issue link: %s", tc.name, cmp.Diff(fakeClient.IssueLinks[0], tc.expectedIssueLink, allowJiraDate))
			}
		} else if len(fakeClient.IssueLinks) != 0 {
			t.Fatalf("%s: got %d issue links when none were expected", tc.name, len(fakeClient.IssueLinks))
		}
	}
}

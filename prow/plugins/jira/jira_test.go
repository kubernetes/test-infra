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

package jira

import (
	"fmt"
	"testing"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	jiraclient "k8s.io/test-infra/prow/jira"
	"k8s.io/test-infra/prow/plugins"
)

func TestRegex(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple",
			input:    "issue-123",
			expected: []string{"issue-123"},
		},
		{
			name:     "Simple with leading space",
			input:    " issue-123",
			expected: []string{"issue-123"},
		},
		{
			name:     "Simple with trailing space",
			input:    "issue-123 ",
			expected: []string{"issue-123"},
		},
		{
			name:     "Simple with leading newline",
			input:    "\nissue-123",
			expected: []string{"issue-123"},
		},
		{
			name:     "Simple with trailing newline",
			input:    "issue-123\n",
			expected: []string{"issue-123"},
		},
		{
			name:     "Simple with trailing colon",
			input:    "issue-123:",
			expected: []string{"issue-123"},
		},
		{
			name:     "Multiple matches",
			input:    "issue-123\nissue-456",
			expected: []string{"issue-123", "issue-456"},
		},
		{
			name:  "Trailing character, no match",
			input: "issue-123a",
		},
		{
			name:     "Issue from url",
			input:    "https://my-jira.com/browse/ABC-123",
			expected: []string{"ABC-123"},
		},
		{
			name:  "Trailing special characters, no match",
			input: "rehearse-15676-pull",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractCandidatesFromText(tc.input)
			if diff := cmp.Diff(tc.expected, result); diff != "" {
				t.Errorf("expected differs from actual: %s", diff)
			}
		})
	}
}

type fakeJiraClient struct {
	existingIssues []jira.Issue
	existingLinks  map[string][]jira.RemoteLink
	newLinks       []jira.RemoteLink
}

func (f *fakeJiraClient) GetIssue(id string) (*jira.Issue, error) {
	for _, existingIssue := range f.existingIssues {
		if existingIssue.ID == id {
			return &existingIssue, nil
		}
	}
	return nil, jiraclient.NewNotFoundError(fmt.Errorf("No issue %s found", id))
}

func (f *fakeJiraClient) GetRemoteLinks(id string) ([]jira.RemoteLink, error) {
	return f.existingLinks[id], nil
}

func (f *fakeJiraClient) AddRemoteLink(id string, link *jira.RemoteLink) error {
	if _, err := f.GetIssue(id); err != nil {
		return err
	}
	f.newLinks = append(f.newLinks, *link)
	return nil
}

func (f *fakeJiraClient) JiraClient() *jira.Client {
	panic("not implemented")
}

const fakeJiraUrl = "https://my-jira.com"

func (f *fakeJiraClient) JiraURL() string {
	return fakeJiraUrl
}

type fakeGitHubClient struct {
	editedComments map[string]string
}

func (f *fakeGitHubClient) EditComment(org, repo string, id int, body string) error {
	if f.editedComments == nil {
		f.editedComments = map[string]string{}
	}
	f.editedComments[fmt.Sprintf("%s/%s:%d", org, repo, id)] = body
	return nil
}

func (f *fakeGitHubClient) GetIssue(org, repo string, number int) (*github.Issue, error) {
	return nil, nil
}

func (f *fakeGitHubClient) EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error) {
	return nil, nil
}

func TestHandle(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                   string
		event                  github.GenericCommentEvent
		cfg                    *plugins.Jira
		existingIssues         []jira.Issue
		existingLinks          map[string][]jira.RemoteLink
		expectedNewLinks       []jira.RemoteLink
		expectedCommentUpdates map[string]string
	}{
		{
			name: "No issue referenced, nothing to do",
		},
		{
			name: "Link is created based on body",
			event: github.GenericCommentEvent{
				CommentID:  intPtr(1),
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "Some issue",
				Body:       "Some text and also ABC-123",
				Repo:       github.Repo{FullName: "org/repo", Owner: github.User{Login: "org"}, Name: "repo"},
				Number:     3,
			},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/issues/3",
				Title: "org/repo#3: Some issue",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			},
			}},
			expectedCommentUpdates: map[string]string{"org/repo:1": "Some text and also [ABC-123](https://my-jira.com/browse/ABC-123)"},
		},
		{
			name: "Link is created based on body with pasted link",
			event: github.GenericCommentEvent{
				CommentID:  intPtr(1),
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "Some issue",
				Body:       "Some text and also https://my-jira.com/browse/ABC-123",
				Repo:       github.Repo{FullName: "org/repo", Owner: github.User{Login: "org"}, Name: "repo"},
				Number:     3,
			},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/issues/3",
				Title: "org/repo#3: Some issue",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			},
			}},
		},
		{
			name: "Link is created based on body and issuecomment suffix is removed from url",
			event: github.GenericCommentEvent{
				CommentID:  intPtr(1),
				HTMLURL:    "https://github.com/org/repo/issues/3#issuecomment-705743977",
				IssueTitle: "Some issue",
				Body:       "Some text and also ABC-123",
				Repo:       github.Repo{FullName: "org/repo", Owner: github.User{Login: "org"}, Name: "repo"},
				Number:     3,
			},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/issues/3",
				Title: "org/repo#3: Some issue",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			},
			}},
			expectedCommentUpdates: map[string]string{"org/repo:1": "Some text and also [ABC-123](https://my-jira.com/browse/ABC-123)"},
		},
		{
			name: "Link is created based on title",
			event: github.GenericCommentEvent{
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "ABC-123: Some issue",
				Body:       "Some text",
				Repo:       github.Repo{FullName: "org/repo"},
				Number:     3,
			},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/issues/3",
				Title: "org/repo#3: ABC-123: Some issue",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			},
			}},
		},
		{
			name: "Multiple references for issue, one link is created",
			event: github.GenericCommentEvent{
				CommentID:  intPtr(1),
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "Some issue",
				Body:       "Some text and also ABC-123 and again ABC-123",
				Repo:       github.Repo{FullName: "org/repo", Owner: github.User{Login: "org"}, Name: "repo"},
				Number:     3,
			},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/issues/3",
				Title: "org/repo#3: Some issue",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			},
			}},
			expectedCommentUpdates: map[string]string{"org/repo:1": "Some text and also [ABC-123](https://my-jira.com/browse/ABC-123) and again [ABC-123](https://my-jira.com/browse/ABC-123)"},
		},
		{
			name: "Referenced issue doesn't exist, nothing to do",
			event: github.GenericCommentEvent{
				HTMLURL:    "https://github.com/org/repo/issues/3#issuecomment-705743977",
				IssueTitle: "Some issue",
				Body:       "Some text and also ABC-123",
				Repo:       github.Repo{FullName: "org/repo"},
				Number:     3,
			},
		},
		{
			name: "Link already exists, nothing to do",
			event: github.GenericCommentEvent{
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "Some issue",
				Body:       "Some text and also [ABC-123](https://my-jira.com/browse/ABC-123)",
				Repo:       github.Repo{FullName: "org/repo"},
				Number:     3,
			},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			existingLinks:  map[string][]jira.RemoteLink{"ABC-123": {{Object: &jira.RemoteLinkObject{URL: "https://github.com/org/repo/issues/3"}}}},
		},
		{
			name: "Valid issue in disabled project, case insensitive matching and no link",
			event: github.GenericCommentEvent{
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "Some issue",
				Body:       "Some text and also ENTERPRISE-4",
				Repo:       github.Repo{FullName: "org/repo"},
				Number:     3,
			},
			cfg:            &plugins.Jira{DisabledJiraProjects: []string{"Enterprise"}},
			existingIssues: []jira.Issue{{ID: "ENTERPRISE-4"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jiraClient := &fakeJiraClient{
				existingIssues: tc.existingIssues,
				existingLinks:  tc.existingLinks,
			}
			githubClient := &fakeGitHubClient{}

			if err := handle(jiraClient, githubClient, tc.cfg, logrus.NewEntry(logrus.New()), &tc.event); err != nil {
				t.Fatalf("handle failed: %v", err)
			}

			if diff := cmp.Diff(jiraClient.newLinks, tc.expectedNewLinks); diff != "" {
				t.Errorf("new links differs from expected new links: %s", diff)
			}

			if diff := cmp.Diff(githubClient.editedComments, tc.expectedCommentUpdates); diff != "" {
				t.Errorf("comment updates differ from expected: %s", diff)
			}
		})
	}

}

func intPtr(i int) *int {
	return &i
}

func TestInsertLinksIntoComment(t *testing.T) {
	t.Parallel()
	const issueName = "ABC-123"
	testCases := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name: "Multiline body starting with issue name",
			body: `ABC-123: Fix problems:
* First problem
* Second problem`,
			expected: `[ABC-123](https://my-jira.com/browse/ABC-123): Fix problems:
* First problem
* Second problem`,
		},
		{
			name: "Multiline body starting with already replaced issue name",
			body: `[ABC-123](https://my-jira.com/browse/ABC-123): Fix problems:
* First problem
* Second problem`,
			expected: `[ABC-123](https://my-jira.com/browse/ABC-123): Fix problems:
* First problem
* Second problem`,
		},
		{
			name: "Multiline body with multiple occurrence in the middle",
			body: `This change:
* Does stuff related to ABC-123
* And even more stuff related to ABC-123
* But also something else`,
			expected: `This change:
* Does stuff related to [ABC-123](https://my-jira.com/browse/ABC-123)
* And even more stuff related to [ABC-123](https://my-jira.com/browse/ABC-123)
* But also something else`,
		},
		{
			name: "Multiline body with multiple occurrence in the middle, some already replaced",
			body: `This change:
* Does stuff related to [ABC-123](https://my-jira.com/browse/ABC-123)
* And even more stuff related to ABC-123
* But also something else`,
			expected: `This change:
* Does stuff related to [ABC-123](https://my-jira.com/browse/ABC-123)
* And even more stuff related to [ABC-123](https://my-jira.com/browse/ABC-123)
* But also something else`,
		},
		{
			name: "Multiline body with issue name at the end",
			body: `This change:
is very important
because of ABC-123`,
			expected: `This change:
is very important
because of [ABC-123](https://my-jira.com/browse/ABC-123)`,
		},
		{
			name: "Multiline body with already replaced issue name at the end",
			body: `This change:
is very important
because of [ABC-123](https://my-jira.com/browse/ABC-123)`,
			expected: `This change:
is very important
because of [ABC-123](https://my-jira.com/browse/ABC-123)`,
		},
		{
			name:     "Pasted links are not replaced, as they are already clickable",
			body:     "https://my-jira.com/browse/ABC-123",
			expected: "https://my-jira.com/browse/ABC-123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(insertLinksIntoComment(tc.body, []string{issueName}, fakeJiraUrl), tc.expected); diff != "" {
				t.Errorf("actual result differs from expected result: %s", diff)
			}
		})
	}
}

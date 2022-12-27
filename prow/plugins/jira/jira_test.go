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
	"errors"
	"reflect"
	"testing"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	jiraclient "k8s.io/test-infra/prow/jira"
	"k8s.io/test-infra/prow/jira/fakejira"
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
		{
			name:     "Included in markdown link",
			input:    "[Jira Bug ABC-123](https://my-jira.com/browse/ABC-123)",
			expected: []string{"ABC-123", "ABC-123"},
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

func TestHandle(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                   string
		event                  github.GenericCommentEvent
		cfg                    *plugins.Jira
		projectCache           *threadsafeSet
		getIssueClientError    map[string]error
		existingIssues         []jira.Issue
		existingLinks          map[string][]jira.RemoteLink
		expectedNewLinks       []jira.RemoteLink
		expectedCommentUpdates []string
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
			projectCache:   &threadsafeSet{data: sets.NewString("abc")},
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
			expectedCommentUpdates: []string{"org/repo#1:Some text and also [ABC-123](https://my-jira.com/browse/ABC-123)"},
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
			projectCache:   &threadsafeSet{data: sets.NewString("abc")},
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
			projectCache:   &threadsafeSet{data: sets.NewString("abc")},
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
			expectedCommentUpdates: []string{"org/repo#1:Some text and also [ABC-123](https://my-jira.com/browse/ABC-123)"},
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
			projectCache:   &threadsafeSet{data: sets.NewString("abc")},
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
			projectCache:   &threadsafeSet{data: sets.NewString("abc")},
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
			expectedCommentUpdates: []string{"org/repo#1:Some text and also [ABC-123](https://my-jira.com/browse/ABC-123) and again [ABC-123](https://my-jira.com/browse/ABC-123)"},
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
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
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
			projectCache:   &threadsafeSet{data: sets.NewString("abc")},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			existingLinks:  map[string][]jira.RemoteLink{"ABC-123": {{Object: &jira.RemoteLinkObject{URL: "https://github.com/org/repo/issues/3", Title: "org/repo#3: Some issue"}}}},
		},
		{
			name: "Link exists but title is different, replacing it",
			event: github.GenericCommentEvent{
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "Some issue NEW",
				Body:       "Some text and also [ABC-123:](https://my-jira.com/browse/ABC-123)",
				Repo:       github.Repo{FullName: "org/repo"},
				Number:     3,
			},
			projectCache:   &threadsafeSet{data: sets.NewString("abc")},
			existingIssues: []jira.Issue{{ID: "ABC-123"}},
			existingLinks: map[string][]jira.RemoteLink{
				"ABC-123": {
					{
						Object: &jira.RemoteLinkObject{
							URL:   "https://github.com/org/repo/issues/3",
							Title: "org/repo#3: Some issue",
							Icon:  &jira.RemoteLinkIcon{Url16x16: "https://github.com/favicon.ico", Title: "GitHub"},
						},
					},
				},
			},
			expectedNewLinks: []jira.RemoteLink{
				{
					Object: &jira.RemoteLinkObject{
						URL:   "https://github.com/org/repo/issues/3",
						Title: "org/repo#3: Some issue NEW",
						Icon:  &jira.RemoteLinkIcon{Url16x16: "https://github.com/favicon.ico", Title: "GitHub"},
					},
				},
			},
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
			projectCache:   &threadsafeSet{data: sets.NewString("enterprise")},
			cfg:            &plugins.Jira{DisabledJiraProjects: []string{"Enterprise"}},
			existingIssues: []jira.Issue{{ID: "ENTERPRISE-4"}},
		},
		{
			name: "Valid issue in disabled project, multiple references, with markdown link, case insensitive matching, nothing to do",
			event: github.GenericCommentEvent{
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "ABC-123: Fixes Some issue",
				Body:       "Some text and also [ABC-123](https://my-jira.com/browse/ABC-123)",
				Repo:       github.Repo{FullName: "org/repo"},
				Number:     3,
			},
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			cfg:          &plugins.Jira{DisabledJiraProjects: []string{"abc"}},
		},
		{
			name: "Project 404 gets served from cache, nothing happens",
			event: github.GenericCommentEvent{
				HTMLURL:    "https://github.com/org/repo/issues/3",
				IssueTitle: "Some issue",
				Body:       "ABC-123",
				Repo:       github.Repo{FullName: "org/repo"},
				Number:     3,
			},
			projectCache:        &threadsafeSet{},
			getIssueClientError: map[string]error{"ABC-123": errors.New("error: didn't serve 404 from cache")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// convert []jira.Issue to []*jira.Issue
			var ptrIssues []*jira.Issue
			for index := range tc.existingIssues {
				ptrIssues = append(ptrIssues, &tc.existingIssues[index])
			}
			jiraClient := &fakejira.FakeClient{
				Issues:        ptrIssues,
				ExistingLinks: tc.existingLinks,
				GetIssueError: tc.getIssueClientError,
			}
			githubClient := fakegithub.NewFakeClient()

			if err := handleWithProjectCache(jiraClient, githubClient, tc.cfg, logrus.NewEntry(logrus.New()), &tc.event, tc.projectCache); err != nil {
				t.Fatalf("handle failed: %v", err)
			}

			if diff := cmp.Diff(jiraClient.NewLinks, tc.expectedNewLinks); diff != "" {
				t.Errorf("new links differs from expected new links: %s", diff)
			}

			if diff := cmp.Diff(githubClient.IssueCommentsEdited, tc.expectedCommentUpdates); diff != "" {
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
		{
			name: "code section is not replaced",
			body: `This change:
is very important` + "\n```bash\n" +
				`ABC-123` +
				"\n```\n" + `ABC-123
`,
			expected: `This change:
is very important` + "\n```bash\n" +
				`ABC-123` +
				"\n```\n" + `[ABC-123](https://my-jira.com/browse/ABC-123)
`,
		},
		{
			name: "inline code is not replaced",
			body: `This change:
is very important` + "\n``ABC-123`` and `ABC-123` shouldn't be replaced, as well as ``ABC-123: text text``. " +
				`ABC-123 should be replaced.
`,
			expected: `This change:
is very important` + "\n``ABC-123`` and `ABC-123` shouldn't be replaced, as well as ``ABC-123: text text``. " +
				`[ABC-123](https://my-jira.com/browse/ABC-123) should be replaced.
`,
		},
		{
			name:     "Multiline codeblock that is denoted through four leading spaces",
			body:     "I meant to do this test:\r\n\r\n    operator_test.go:1914: failed to read output from pod unique-id-header-test-1: container \"curl\" in pod \"unique-id-header-ABC-123\" is waiting to start: ContainerCreating\r\n\r\n",
			expected: "I meant to do this test:\r\n\r\n    operator_test.go:1914: failed to read output from pod unique-id-header-test-1: container \"curl\" in pod \"unique-id-header-ABC-123\" is waiting to start: ContainerCreating\r\n\r\n",
		},
		{
			name:     "parts of words starting with a dash are not replaced",
			body:     "this shouldn't be replaced: whatever-ABC-123 and also inline `whatever-ABC-123`",
			expected: "this shouldn't be replaced: whatever-ABC-123 and also inline `whatever-ABC-123`",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(insertLinksIntoComment(tc.body, []string{issueName}, fakejira.FakeJiraUrl), tc.expected); diff != "" {
				t.Errorf("actual result differs from expected result: %s", diff)
			}
		})
	}
}

func TestProjectCachingJiraClient(t *testing.T) {
	t.Parallel()
	lowerCaseIssue := jira.Issue{ID: "issue-123"}
	upperCaseIssue := jira.Issue{ID: "ISSUE-123"}
	testCases := []struct {
		name           string
		client         jiraclient.Client
		issueToRequest string
		cache          *threadsafeSet
		expectedError  error
	}{
		{
			name:           "404 gets served from cache",
			client:         &fakejira.FakeClient{},
			issueToRequest: "issue-123",
			cache:          &threadsafeSet{data: sets.String{}},
			expectedError:  jiraclient.NewNotFoundError(errors.New("404 from cache")),
		},
		{
			name:           "Success",
			client:         &fakejira.FakeClient{Issues: []*jira.Issue{&lowerCaseIssue}},
			issueToRequest: "issue-123",
			cache:          &threadsafeSet{data: sets.NewString("issue")},
		},
		{
			name:           "Success case-insensitive",
			client:         &fakejira.FakeClient{Issues: []*jira.Issue{&upperCaseIssue}},
			issueToRequest: "ISSUE-123",
			cache:          &threadsafeSet{data: sets.NewString("issue")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cachingClient := &projectCachingJiraClient{
				Client: tc.client,
				cache:  tc.cache,
			}

			_, err := cachingClient.GetIssue(tc.issueToRequest)
			if diff := cmp.Diff(tc.expectedError, err, cmp.Exporter(func(_ reflect.Type) bool { return true })); diff != "" {
				t.Fatalf("expected error differs from expected: %s", diff)
			}
		})
	}
}

func TestFilterOutDisabledJiraProjects(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		candidates     []string
		jiraConfig     *plugins.Jira
		expectedOutput []string
	}{{
		name:           "empty jira config",
		candidates:     []string{"ABC-123", "DEF-567"},
		jiraConfig:     nil,
		expectedOutput: []string{"ABC-123", "DEF-567"},
	}, {
		name:           "upper case disabled list",
		candidates:     []string{"ABC-123", "DEF-567"},
		jiraConfig:     &plugins.Jira{DisabledJiraProjects: []string{"ABC"}},
		expectedOutput: []string{"DEF-567"},
	}, {
		name:           "lower case disabled list",
		candidates:     []string{"ABC-123", "DEF-567"},
		jiraConfig:     &plugins.Jira{DisabledJiraProjects: []string{"abc"}},
		expectedOutput: []string{"DEF-567"},
	}, {
		name:           "multiple disabled projects",
		candidates:     []string{"ABC-123", "DEF-567"},
		jiraConfig:     &plugins.Jira{DisabledJiraProjects: []string{"abc", "def"}},
		expectedOutput: []string{},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := filterOutDisabledJiraProjects(tc.candidates, tc.jiraConfig)
			if diff := cmp.Diff(tc.expectedOutput, output); diff != "" {
				t.Fatalf("actual output differes from expected output: %s", diff)
			}
		})
	}
}

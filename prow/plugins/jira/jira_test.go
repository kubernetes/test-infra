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
	"fmt"
	"reflect"
	"testing"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"github.com/trivago/tgo/tcontainer"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	prowconfig "k8s.io/test-infra/prow/config"
	cherrypicker "k8s.io/test-infra/prow/external-plugins/cherrypicker/lib"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	jiraclient "k8s.io/test-infra/prow/jira"
	"k8s.io/test-infra/prow/jira/fakejira"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var allowEventAndDate = cmp.AllowUnexported(event{}, jira.Date{})

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
	return &github.Issue{}, nil
}

func (f *fakeGitHubClient) EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error) {
	return issue, nil
}

func TestHandle(t *testing.T) {
	t.Parallel()
	yes := true
	open := true
	v1Str := "v1"
	v2Str := "v2"
	v1 := []*jira.Version{{Name: v1Str}}
	v2 := []*jira.Version{{Name: v2Str}}
	v3 := []*jira.Version{{Name: "v3"}}
	updated := plugins.JiraBugState{Status: "UPDATED"}
	modified := plugins.JiraBugState{Status: "MODIFIED"}
	verified := []plugins.JiraBugState{{Status: "VERIFIED"}}
	jiraTransitions := []jira.Transition{
		{
			ID:   "1",
			Name: "NEW",
			To: jira.Status{
				Name: "NEW",
			},
		},
		{
			ID:   "2",
			Name: "MODIFIED",
			To: jira.Status{
				Name: "MODIFIED",
			},
		},
		{
			ID:   "3",
			Name: "UPDATED",
			To: jira.Status{
				Name: "UPDATED",
			},
		},
		{
			ID:   "4",
			Name: "VERIFIED",
			To: jira.Status{
				Name: "VERIFIED",
			},
		},
		{
			ID:   "5",
			Name: "CLOSED",
			To: jira.Status{
				Name: "CLOSED",
			},
		},
	}
	severityCritical := struct {
		Value string
	}{Value: "<img alt=\"\" src=\"/images/icons/priorities/critical.svg\" width=\"16\" height=\"16\"> Critical"}
	severityImportant := struct {
		Value string
	}{Value: "<img alt=\"\" src=\"/images/icons/priorities/important.svg\" width=\"16\" height=\"16\"> Important"}
	severityModerate := struct {
		Value string
	}{Value: "<img alt=\"\" src=\"/images/icons/priorities/moderate.svg\" width=\"16\" height=\"16\"> Moderate"}
	severityLow := struct {
		Value string
	}{Value: "<img alt=\"\" src=\"/images/icons/priorities/low.svg\" width=\"16\" height=\"16\"> Low"}
	fieldLinkTo123 := jira.IssueLink{
		Type: jira.IssueLinkType{
			Name:    "Cloners",
			Inward:  "is cloned by",
			Outward: "clones",
		},
		OutwardIssue: &jira.Issue{ID: "1"},
	}
	fieldLinkTo124 := jira.IssueLink{
		Type: jira.IssueLinkType{
			Name:    "Cloners",
			Inward:  "is cloned by",
			Outward: "clone",
		},
		InwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
	}
	linkBetween123to124 := jira.IssueLink{
		Type: jira.IssueLinkType{
			Name:    "Cloners",
			Inward:  "is cloned by",
			Outward: "clones",
		},
		InwardIssue:  &jira.Issue{ID: "2"},
		OutwardIssue: &jira.Issue{ID: "1"},
	}
	base := &event{
		org: "org", repo: "repo", baseRef: "branch", number: 1, key: "OCPBUGS-123", body: "This PR fixes OCPBUGS-123", title: "OCPBUGS-123: fixed it!", htmlUrl: "https://github.com/org/repo/pull/1", login: "user",
	}
	var testCases = []struct {
		name                       string
		labels                     []string
		humanLabelled              bool
		missing                    bool
		merged                     bool
		closed                     bool
		opened                     bool
		cherryPick                 bool
		cherryPickFromPRNum        int
		cherryPickTo               string
		body                       string
		title                      string
		remoteLinks                map[string][]jira.RemoteLink
		prs                        []github.PullRequest
		issues                     []jira.Issue
		issueGetErrors             map[string]error
		issueCreateErrors          map[string]error
		issueUpdateErrors          map[string]error
		options                    plugins.JiraBranchOptions
		expectedLabels             []string
		expectedComment            string
		expectedIssue              *jira.Issue
		expectedNewRemoteLinks     []jira.RemoteLink
		expectedRemovedRemoteLinks []jira.RemoteLink
		projectCache               *threadsafeSet
		isComment                  bool
		existingIssueLinks         []*jira.IssueLink
		// most of the tests can be handled by a single event struct with small modifications; for tests with more extensive differences, allow override
		overrideEvent          *event
		disabledProjects       []string
		expectedCommentUpdates []string
	}{
		{
			name:          "No issue referenced, nothing to do",
			overrideEvent: &event{org: "org", repo: "repo", baseRef: "branch", number: 1, title: "Not a jira bugfix", htmlUrl: "http.com", login: "user"},
		},
		{
			name:         "Link is created based on body",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				commentID: intPtr(1),
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "Some issue",
				body:      "Some text and also ABC-123",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewRemoteLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
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
			name:         "Link is created based on body with pasted link",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				commentID: intPtr(1),
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "Some issue",
				body:      "Some text and also https://my-jira.com/browse/ABC-123",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewRemoteLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
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
			name:         "Link is created based on body and issuecomment suffix is removed from url",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				commentID: intPtr(1),
				htmlUrl:   "https://github.com/org/repo/issues/3#issuecomment-705743977",
				title:     "Some issue",
				body:      "Some text and also ABC-123",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewRemoteLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
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
			name:         "Link is created based on title",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "ABC-123: Some issue",
				body:      "Some text",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewRemoteLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
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
			name:         "Multiple references for issue, one link is created",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				commentID: intPtr(1),
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "Some issue",
				body:      "Some text and also ABC-123 and again ABC-123",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issues: []jira.Issue{{ID: "ABC-123"}},
			expectedNewRemoteLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
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
			name:         "Referenced issue doesn't exist, nothing to do",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				htmlUrl:   "https://github.com/org/repo/issues/3#issuecomment-705743977",
				title:     "Some issue",
				body:      "Some text and also ABC-123",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
		},
		{
			name:         "Link already exists, nothing to do",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "Some issue",
				body:      "Some text and also [ABC-123](https://my-jira.com/browse/ABC-123)",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issues:      []jira.Issue{{ID: "ABC-123"}},
			remoteLinks: map[string][]jira.RemoteLink{"ABC-123": {{ID: 1, Object: &jira.RemoteLinkObject{URL: "https://github.com/org/repo/issues/3", Title: "Some issue"}}}},
		},
		{
			name:         "Link exists but title is different, replacing it",
			projectCache: &threadsafeSet{data: sets.NewString("abc")},
			overrideEvent: &event{
				isComment: true,
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "Some issue NEW",
				body:      "Some text and also [ABC-123:](https://my-jira.com/browse/ABC-123)",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issues: []jira.Issue{{ID: "ABC-123"}},
			remoteLinks: map[string][]jira.RemoteLink{
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
			expectedNewRemoteLinks: []jira.RemoteLink{
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
			name:         "Valid issue in disabled project, case insensitive matching and no link",
			projectCache: &threadsafeSet{data: sets.NewString("enterprise")},
			overrideEvent: &event{
				isComment: true,
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "Some issue",
				body:      "Some text and also ENTERPRISE-4",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			disabledProjects: []string{"Enterprise"},
			issues:           []jira.Issue{{ID: "ENTERPRISE-4"}},
		},
		{
			name:         "Project 404 gets served from cache, nothing happens",
			projectCache: &threadsafeSet{},
			overrideEvent: &event{
				isComment: true,
				htmlUrl:   "https://github.com/org/repo/issues/3",
				title:     "Some issue",
				body:      "ABC-123",
				org:       "org",
				repo:      "repo",
				number:    3,
			},
			issueGetErrors: map[string]error{"ABC-123": errors.New("error: didn't serve 404 from cache")},
		},
		{
			name:         "no bug found leaves a comment",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			expectedComment: `org/repo#1:@user: No Jira issue with key OCPBUGS-123 exists in the tracker at https://my-jira.com.
Once a valid bug is referenced in the title of this pull request, request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "error fetching bug leaves a comment",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issueGetErrors: map[string]error{"OCPBUGS-123": errors.New("injected error getting bug")},
			expectedComment: `org/repo#1:@user: An error was encountered searching for bug OCPBUGS-123 on the Jira server at https://my-jira.com. No known errors were detected, please see the full error message for details.

<details><summary>Full error message.</summary>

<code>
injected error getting bug
</code>

</details>

Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "valid bug removes invalid label, adds valid/severity labels and comments",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"customfield_12316142": severityCritical}}}},
			options:        plugins.JiraBranchOptions{}, // no requirements --> always valid
			labels:         []string{"jira/invalid-bug"},
			expectedLabels: []string{"jira/valid-bug", "jira/severity-critical"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "invalid bug adds invalid label, removes valid label and comments",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"customfield_12316142": severityImportant}}}},
			options:        plugins.JiraBranchOptions{IsOpen: &open},
			labels:         []string{"jira/valid-bug", "jira/severity-critical"},
			expectedLabels: []string{"jira/invalid-bug", "jira/severity-important"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is invalid:
 - expected the bug to be open, but it isn't

Comment <code>/jira refresh</code> to re-evaluate validity if changes to the Jira bug are made, or edit the title of this pull request to link to a different bug.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "invalid bug adds keeps human-added valid bug label",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"customfield_12316142": severityImportant}}}},
			options:        plugins.JiraBranchOptions{IsOpen: &open},
			humanLabelled:  true,
			labels:         []string{"jira/valid-bug", "jira/severity-critical"},
			expectedLabels: []string{"jira/valid-bug", "jira/severity-important"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is invalid:
 - expected the bug to be open, but it isn't

Comment <code>/jira refresh</code> to re-evaluate validity if changes to the Jira bug are made, or edit the title of this pull request to link to a different bug.

Retaining the jira/valid-bug label as it was manually added.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:         "no bug removes all labels and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			missing:      true,
			labels:       []string{"jira/valid-bug", "jira/invalid-bug"},
			expectedComment: `org/repo#1:@user: No Jira bug is referenced in the title of this pull request.
To reference a bug, add 'OCPBUGS-XXX:' to the title of this pull request and request another bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:         "valid bug with status update removes invalid label, adds valid label, comments and updates status",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"customfield_12316142": severityModerate}}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			options:        plugins.JiraBranchOptions{StateAfterValidation: &updated}, // no requirements --> always valid
			labels:         []string{"jira/invalid-bug"},
			expectedLabels: []string{"jira/valid-bug", "jira/severity-moderate"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid. The bug has been moved to the UPDATED state.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status:   &jira.Status{Name: "UPDATED"},
				Unknowns: tcontainer.MarshalMap{"customfield_12316142": severityModerate},
			}},
		},
		{
			name:           "valid bug with status update removes invalid label, adds valid label, comments and updates status with resolution",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"customfield_12316142": severityLow}}}},
			options:        plugins.JiraBranchOptions{StateAfterValidation: &plugins.JiraBugState{Status: "CLOSED", Resolution: "VALIDATED"}}, // no requirements --> always valid
			labels:         []string{"jira/invalid-bug"},
			expectedLabels: []string{"jira/valid-bug", "jira/severity-low"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid. The bug has been moved to the CLOSED (VALIDATED) state.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{
					Name: "CLOSED",
				},
				Resolution: &jira.Resolution{
					Name: "VALIDATED",
				},
				// due to the way `Unknowns` works, this has to be provided as a map[string]interface{}
				Unknowns: tcontainer.MarshalMap{"customfield_12316142": map[string]interface{}{"Value": string(`<img alt="" src="/images/icons/priorities/low.svg" width="16" height="16"> Low`)}},
			},
			},
		},
		{
			name:           "valid bug with status update removes invalid label, adds valid label, comments and does not update status when it is already correct",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Status: &jira.Status{Name: "UPDATED"}}}},
			options:        plugins.JiraBranchOptions{StateAfterValidation: &updated}, // no requirements --> always valid
			labels:         []string{"jira/invalid-bug"},
			expectedLabels: []string{"jira/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Status: &jira.Status{Name: "UPDATED"}}},
		},
		{
			name:           "valid bug with external link removes invalid label, adds valid label, comments, makes an external bug link",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123"}},
			options:        plugins.JiraBranchOptions{AddExternalLink: &yes}, // no requirements --> always valid
			labels:         []string{"jira/invalid-bug"},
			expectedLabels: []string{"jira/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid. The bug has been updated to refer to the pull request using the external bug tracker.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123"},
			expectedNewRemoteLinks: []jira.RemoteLink{{Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			},
			}},
		},
		{
			name:         "valid bug with already existing external link removes invalid label, adds valid label, comments to say nothing changed",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123"}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			options:        plugins.JiraBranchOptions{AddExternalLink: &yes}, // no requirements --> always valid
			labels:         []string{"jira/invalid-bug"},
			expectedLabels: []string{"jira/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123"},
		},
		{
			name:         "failure to fetch dependent bug results in a comment",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				IssueLinks: []*jira.IssueLink{&fieldLinkTo124},
			}}},
			existingIssueLinks: []*jira.IssueLink{&linkBetween123to124},
			issueGetErrors:     map[string]error{"OCPBUGS-124": errors.New("injected error getting bug")},
			options:            plugins.JiraBranchOptions{DependentBugStates: &verified},
			expectedComment: `org/repo#1:@user: An error was encountered searching for dependent bug OCPBUGS-124 for bug OCPBUGS-123 on the Jira server at https://my-jira.com. No known errors were detected, please see the full error message for details.

<details><summary>Full error message.</summary>

<code>
injected error getting bug
</code>

</details>

Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:         "valid bug with dependent bugs removes invalid label, adds valid label, comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "MODIFIED"},
				IssueLinks: []*jira.IssueLink{&fieldLinkTo124},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &v1,
				},
			},
			}, {ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "VERIFIED"},
				IssueLinks: []*jira.IssueLink{&fieldLinkTo123},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &v2,
				},
			}}},
			existingIssueLinks: []*jira.IssueLink{&linkBetween123to124},
			options:            plugins.JiraBranchOptions{IsOpen: &yes, TargetVersion: &v1Str, DependentBugStates: &verified, DependentBugTargetVersions: &[]string{v2Str}},
			labels:             []string{"jira/invalid-bug"},
			expectedLabels:     []string{"jira/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid.

<details><summary>5 validation(s) were run on this bug</summary>

* bug is open, matching expected state (open)
* bug target version (v1) matches configured target version for branch (v1)
* dependent bug [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) is in the state VERIFIED, which is one of the valid states (VERIFIED)
* dependent [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) targets the "v2" version, which is one of the valid target versions: v2
* bug has dependents</details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:         "valid bug on merged PR with one external link migrates to new state with resolution and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "MODIFIED"},
			}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.JiraBranchOptions{StateAfterMerge: &plugins.JiraBugState{Status: "CLOSED", Resolution: "MERGED"}}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: All pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)

[Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) has been moved to the CLOSED (MERGED) state.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "MERGED"},
				Unknowns:   tcontainer.MarshalMap{},
			}},
		},
		{
			name:         "valid bug on merged PR with one external link migrates to new state and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.JiraBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: All pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)

[Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) has been moved to the MODIFIED state.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}},
		},
		{
			name:         "valid bug on merged PR with many external links migrates to new state and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{
				ID: 1,
				Object: &jira.RemoteLinkObject{
					URL:   "https://github.com/org/repo/pull/1",
					Title: "org/repo#1: OCPBUGS-123: fixed it!",
					Icon: &jira.RemoteLinkIcon{
						Url16x16: "https://github.com/favicon.ico",
						Title:    "GitHub",
					},
				},
			}, {
				ID: 2,
				Object: &jira.RemoteLinkObject{
					URL:   "https://github.com/org/repo/pull/22",
					Title: "org/repo#22: OCPBUGS-123: fixed it!",
					Icon: &jira.RemoteLinkIcon{
						Url16x16: "https://github.com/favicon.ico",
						Title:    "GitHub",
					},
				},
			},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}, {Number: 22, Merged: true}},
			options: plugins.JiraBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: All pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)
 * [org/repo#22](https://github.com/org/repo/pull/22)

[Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) has been moved to the MODIFIED state.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}},
		},
		{
			name:         "valid bug on merged PR with unmerged external links does nothing",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{
				ID: 1,
				Object: &jira.RemoteLinkObject{
					URL:   "https://github.com/org/repo/pull/1",
					Title: "org/repo#1: OCPBUGS-123: fixed it!",
					Icon: &jira.RemoteLinkIcon{
						Url16x16: "https://github.com/favicon.ico",
						Title:    "GitHub",
					},
				},
			}, {
				ID: 2,
				Object: &jira.RemoteLinkObject{
					URL:   "https://github.com/org/repo/pull/22",
					Title: "org/repo#22: OCPBUGS-123: fixed it!",
					Icon: &jira.RemoteLinkIcon{
						Url16x16: "https://github.com/favicon.ico",
						Title:    "GitHub",
					},
				},
			},
			}},
			prs:           []github.PullRequest{{Number: base.number, Merged: true}, {Number: 22, Merged: false, State: "open"}},
			options:       plugins.JiraBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{}},
			expectedComment: `org/repo#1:@user: Some pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)

The following pull requests linked via external trackers have not merged:
 * [org/repo#22](https://github.com/org/repo/pull/22) is open

These pull request must merge or be unlinked from the Jira bug in order for it to move to the next state. Once unlinked, request a bug refresh with <code>/jira refresh</code>.

[Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) has not been moved to the MODIFIED state.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:         "External bug on rep that is not in our config is ignored, bug gets set to MODIFIED",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/unreferenced/repo/pull/22",
				Title: "unreferenced/repo#22: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:           []github.PullRequest{{Number: 22, Merged: false, State: "open"}},
			options:       plugins.JiraBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}},
			expectedComment: `org/repo#1:@user: All pull requests linked via external trackers have merged:


[Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) has been moved to the MODIFIED state.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:         "valid bug on merged PR with one external link but no status after merge configured does nothing",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123"}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:           []github.PullRequest{{Number: base.number, Merged: true}},
			options:       plugins.JiraBranchOptions{}, // no requirements --> always valid
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123"},
		},
		{
			name:         "valid bug on merged PR with one external link but no referenced bug in the title does nothing",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			missing:      true,
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123"}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:           []github.PullRequest{{Number: base.number, Merged: true}},
			options:       plugins.JiraBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123"},
		},
		{
			name:           "valid bug on merged PR with one external link fails to update bug and comments",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:         true,
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123"}},
			issueGetErrors: map[string]error{"OCPBUGS-123": errors.New("injected error getting bug")},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.JiraBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: An error was encountered searching for bug OCPBUGS-123 on the Jira server at https://my-jira.com. No known errors were detected, please see the full error message for details.

<details><summary>Full error message.</summary>

<code>
injected error getting bug
</code>

</details>

Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123"},
		},
		{
			name:         "valid bug on merged PR with merged external links but unknown status does not migrate to new state and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       true,
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.JiraBranchOptions{StateAfterValidation: &updated, StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) is in an unrecognized state (CLOSED) and will not be moved to the MODIFIED state.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,

			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}},
		},
		{
			name:         "closed PR removes link and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       false,
			closed:       true,
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: false}},
			options: plugins.JiraBranchOptions{AddExternalLink: &yes},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123). The bug has been updated to no longer refer to the pull request using the external bug tracker.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}},
			expectedRemovedRemoteLinks: []jira.RemoteLink{{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			},
		},
		{
			name:         "closed PR without a link does nothing",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       false,
			closed:       true,
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}}},
			prs:     []github.PullRequest{{Number: base.number, Merged: false}},
			options: plugins.JiraBranchOptions{AddExternalLink: &yes},
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}},
		},
		{
			name:         "closed PR removes link, changes bug state, and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       false,
			closed:       true,
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Status: &jira.Status{Name: "POST"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: false}},
			options: plugins.JiraBranchOptions{AddExternalLink: &yes, StateAfterClose: &plugins.JiraBugState{Status: "NEW"}},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123). The bug has been updated to no longer refer to the pull request using the external bug tracker. All external bug links have been closed. The bug has been moved to the NEW state.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "NEW"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}, {
					Body:       "Bug status changed to NEW as previous linked PR https://github.com/org/repo/pull/1 has been closed",
					Visibility: PrivateVisibility,
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}},
			expectedRemovedRemoteLinks: []jira.RemoteLink{{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			},
		},
		{
			name:         "closed PR with missing bug does nothing",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       false,
			closed:       true,
			missing:      true,
			issues:       []jira.Issue{},
			prs:          []github.PullRequest{{Number: base.number, Merged: false}},
			options:      plugins.JiraBranchOptions{AddExternalLink: &yes, StateAfterClose: &plugins.JiraBugState{Status: "NEW"}},
		},
		{
			name:         "closed PR with multiple external links removes link, does not change bug state, and comments",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			merged:       false,
			closed:       true,
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "POST"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}}},
			remoteLinks: map[string][]jira.RemoteLink{"OCPBUGS-123": {{
				ID: 1,
				Object: &jira.RemoteLinkObject{
					URL:   "https://github.com/org/repo/pull/1",
					Title: "org/repo#1: OCPBUGS-123: fixed it!",
					Icon: &jira.RemoteLinkIcon{
						Url16x16: "https://github.com/favicon.ico",
						Title:    "GitHub",
					},
				},
			}, {
				ID: 2,
				Object: &jira.RemoteLinkObject{
					URL:   "https://github.com/org/repo/issues/42",
					Title: "org/repo#42: OCPBUGS-123: fixed it!",
					Icon: &jira.RemoteLinkIcon{
						Url16x16: "https://github.com/favicon.ico",
						Title:    "GitHub",
					},
				}},
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: false}},
			options: plugins.JiraBranchOptions{AddExternalLink: &yes, StateAfterClose: &plugins.JiraBugState{Status: "NEW"}},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123). The bug has been updated to no longer refer to the pull request using the external bug tracker.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "POST"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
				},
			}},
			expectedRemovedRemoteLinks: []jira.RemoteLink{{ID: 1, Object: &jira.RemoteLinkObject{
				URL:   "https://github.com/org/repo/pull/1",
				Title: "org/repo#1: OCPBUGS-123: fixed it!",
				Icon: &jira.RemoteLinkIcon{
					Url16x16: "https://github.com/favicon.ico",
					Title:    "GitHub",
				},
			}},
			},
		},
		{
			name:         "Cherrypick PR results in cloned bug creation",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v2,
				},
			}}},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.title}},
			title:               "[v1] " + base.title,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.JiraBranchOptions{TargetVersion: &v1Str},
			expectedComment: `org/repo#1:@user: [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) has been cloned as [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124). Retitling PR to link against new bug.
/retitle [v1] OCPBUGS-124: fixed it!

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Description: "This is a clone of issue OCPBUGS-123. The following is the description of the original issue: \n---\n",
				Status:      &jira.Status{Name: "CLOSED"}, // during a clone on a real jira server, this field would get unset/reset; the fake client copies
				IssueLinks:  []*jira.IssueLink{&fieldLinkTo123},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": map[string]interface{}{"Value": `<img alt="" src="/images/icons/priorities/critical.svg" width="16" height="16"> Critical`},
					"customfield_12319940": []interface{}{map[string]interface{}{"name": v1Str}},
				},
			}},
		},
		{
			name:         "parent PR of cherrypick not existing results in error",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v2,
				},
			}}},
			prs:                 []github.PullRequest{{Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.title}},
			title:               "[v1] " + base.title,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.JiraBranchOptions{TargetVersion: &v1Str},
			expectedComment: `org/repo#1:@user: Error creating a cherry-pick bug in Jira: failed to check the state of cherrypicked pull request at https://github.com/org/repo/pull/1: pull request number 1 does not exist.
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:         "failure to obtain parent bug for cherrypick results in error",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v2,
				},
			}}},
			issueGetErrors:      map[string]error{"OCPBUGS-123": errors.New("injected error getting bug")},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.title}},
			title:               "[v1] " + base.title,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.JiraBranchOptions{TargetVersion: &v1Str},
			expectedComment: `org/repo#1:@user: An error was encountered searching for bug OCPBUGS-123 on the Jira server at https://my-jira.com. No known errors were detected, please see the full error message for details.

<details><summary>Full error message.</summary>

<code>
injected error getting bug
</code>

</details>

Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:         "failure to clone bug for cherrypick results in error",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v2,
				},
			}}},
			issueCreateErrors:   map[string]error{"OCPBUGS-124": errors.New("injected error creating OCPBUGS-124")},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.title}},
			title:               "[v1] " + base.title,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.JiraBranchOptions{TargetVersion: &v1Str},
			expectedComment: `org/repo#1:@user: An error was encountered cloning bug for cherrypick for bug OCPBUGS-123 on the Jira server at https://my-jira.com. No known errors were detected, please see the full error message for details.

<details><summary>Full error message.</summary>

<code>
injected error creating OCPBUGS-124
</code>

</details>

Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:         "failure to update bug for results in error",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v2,
				},
			}}},
			issueUpdateErrors:   map[string]error{"2": errors.New("injected error updating bug")},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.title}},
			title:               "[v1] " + base.title,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.JiraBranchOptions{TargetVersion: &v1Str},
			expectedComment: `org/repo#1:@user: An error was encountered updating cherry-pick bug in Jira: Created cherrypick [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124), but encountered error updating target version for bug OCPBUGS-124 on the Jira server at https://my-jira.com. No known errors were detected, please see the full error message for details.

<details><summary>Full error message.</summary>

<code>
injected error updating bug
</code>

</details>

Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:         "If bug clone with correct target version already exists, just retitle PR",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				IssueLinks: []*jira.IssueLink{&fieldLinkTo124},
				Status:     &jira.Status{Name: "CLOSED"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v2,
				},
			}}, {ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				IssueLinks: []*jira.IssueLink{&fieldLinkTo123},
				Status:     &jira.Status{Name: "NEW"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v1,
				},
			}},
			},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.title}},
			title:               "[v1] " + base.title,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.JiraBranchOptions{TargetVersion: &v1Str},
			expectedComment: `org/repo#1:@user: Detected clone of [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) with correct target version. Retitling PR to link to clone:
/retitle [v1] OCPBUGS-124: fixed it!

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:         "Clone for different version does not block creation of new clone",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				Comments: &jira.Comments{Comments: []*jira.Comment{{
					Body: "This is a bug",
				}}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v2,
				},
			}}, {ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "NEW"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": severityCritical,
					"customfield_12319940": &v3,
				},
			}},
			},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.title}},
			title:               "[v1] " + base.title,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.JiraBranchOptions{TargetVersion: &v1Str},
			expectedComment: `org/repo#1:@user: [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) has been cloned as [Jira Issue OCPBUGS-125](https://my-jira.com/browse/OCPBUGS-125). Retitling PR to link against new bug.
/retitle [v1] OCPBUGS-125: fixed it!

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "3", Key: "OCPBUGS-125", Fields: &jira.IssueFields{
				Description: "This is a clone of issue OCPBUGS-123. The following is the description of the original issue: \n---\n",
				Status:      &jira.Status{Name: "CLOSED"}, // during a clone on a real jira server, this field would get unset/reset; the fake client copies
				IssueLinks:  []*jira.IssueLink{&fieldLinkTo123},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12316142": map[string]interface{}{"Value": `<img alt="" src="/images/icons/priorities/critical.svg" width="16" height="16"> Critical`},
					"customfield_12319940": []interface{}{map[string]interface{}{"name": v1Str}},
				},
			}},
		}, {
			name:         "Bug with non-allowed security level is ignored",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"security": jiraclient.SecurityLevel{Name: "security"}}}}},
			options:      plugins.JiraBranchOptions{AllowedSecurityLevels: []string{"internal"}},
			prs:          []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}},
			// there should be no comment returned in this test case
		}, {
			name:           "Bug with security level on repo with no allowed security levels results in comment on /jira refresh",
			projectCache:   &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:         []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"security": jiraclient.SecurityLevel{Name: "security"}}}}},
			prs:            []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}},
			body:           "/jira refresh",
			isComment:      true,
			expectedLabels: []string{"jira/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>/jira refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:         "Bug with non-allowed security level results in comment on /jira refresh",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"security": jiraclient.SecurityLevel{Name: "security"}}}}},
			prs:          []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}},
			body:         "/jira refresh",
			isComment:    true,
			options:      plugins.JiraBranchOptions{AllowedSecurityLevels: []string{"internal"}},
			expectedComment: `org/repo#1:@user: [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) is in a security level that is not in the allowed security levels for this repo.
Allowed security levels for this repo are:
- internal

<details>

In response to [this](https://github.com/org/repo/pull/1):

>/jira refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:         "Bug with non-allowed security level results in comment on PR creation",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues:       []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{"security": jiraclient.SecurityLevel{Name: "security"}}}}},
			prs:          []github.PullRequest{{Number: base.number, Body: base.body, Title: base.title}},
			body:         "/jira refresh",
			isComment:    true,
			opened:       true,
			options:      plugins.JiraBranchOptions{AllowedSecurityLevels: []string{"internal"}},
			expectedComment: `org/repo#1:@user: [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) is in a security level that is not in the allowed security levels for this repo.
Allowed security levels for this repo are:
- internal

<details>

In response to [this](https://github.com/org/repo/pull/1):

>/jira refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:         "Bug with allowed group is properly handled",
			projectCache: &threadsafeSet{data: sets.NewString("ocpbugs")},
			issues: []jira.Issue{{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{Unknowns: tcontainer.MarshalMap{
				"security":             jiraclient.SecurityLevel{Name: "security"},
				"customfield_12316142": severityModerate,
			}}}},
			options:        plugins.JiraBranchOptions{StateAfterValidation: &updated, AllowedSecurityLevels: []string{"security"}},
			labels:         []string{"jira/invalid-bug"},
			expectedLabels: []string{"jira/valid-bug", "jira/severity-moderate"},
			expectedComment: `org/repo#1:@user: This pull request references [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123), which is valid. The bug has been moved to the UPDATED state.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](https://github.com/org/repo/pull/1):

>This PR fixes OCPBUGS-123


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedIssue: &jira.Issue{ID: "1", Key: "OCPBUGS-123", Fields: &jira.IssueFields{
				Unknowns: tcontainer.MarshalMap{
					"security":             jiraclient.SecurityLevel{Name: "security"},
					"customfield_12316142": severityModerate,
				}, Status: &jira.Status{Name: "UPDATED"},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var ptrIssues []*jira.Issue
			for index := range tc.issues {
				ptrIssues = append(ptrIssues, &tc.issues[index])
			}
			jiraClient := &fakejira.FakeClient{
				Issues:           ptrIssues,
				ExistingLinks:    tc.remoteLinks,
				GetIssueError:    tc.issueGetErrors,
				CreateIssueError: tc.issueCreateErrors,
				UpdateIssueError: tc.issueUpdateErrors,
				Transitions:      jiraTransitions,
			}
			var testEvent event // copy so parallel tests don't collide
			if tc.overrideEvent != nil {
				testEvent = *tc.overrideEvent
			} else {
				testEvent = *base // copy so parallel tests don't collide
			}
			testEvent.missing = tc.missing
			testEvent.merged = tc.merged
			testEvent.closed = tc.closed || tc.merged
			testEvent.opened = tc.opened
			testEvent.cherrypick = tc.cherryPick
			testEvent.cherrypickFromPRNum = tc.cherryPickFromPRNum
			testEvent.cherrypickTo = tc.cherryPickTo
			if tc.body != "" {
				testEvent.body = tc.body
			}
			if tc.title != "" {
				testEvent.title = tc.title
			}
			gc := fakegithub.NewFakeClient()
			gc.IssueLabelsExisting = []string{}
			gc.IssueComments = map[int][]github.IssueComment{}
			gc.PullRequests = map[int]*github.PullRequest{}
			gc.WasLabelAddedByHumanVal = tc.humanLabelled
			for _, label := range tc.labels {
				gc.IssueLabelsExisting = append(gc.IssueLabelsExisting, fmt.Sprintf("%s/%s#%d:%s", testEvent.org, testEvent.repo, testEvent.number, label))
			}
			for _, pr := range tc.prs {
				pr := pr
				gc.PullRequests[pr.Number] = &pr
			}
			if err := handleWithProjectCache(jiraClient, gc, tc.disabledProjects, tc.options, logrus.WithField("testCase", tc.name), testEvent, sets.NewString("org/repo"), tc.projectCache); err != nil {
				t.Fatalf("handle failed: %v", err)
			}

			if diff := cmp.Diff(jiraClient.NewLinks, tc.expectedNewRemoteLinks); diff != "" {
				t.Errorf("new links differs from expected new links: %s", diff)
			}

			if diff := cmp.Diff(jiraClient.RemovedLinks, tc.expectedRemovedRemoteLinks); diff != "" {
				t.Errorf("removed links differs from expected removed links: %s", diff)
			}

			if diff := cmp.Diff(gc.IssueCommentsEdited, tc.expectedCommentUpdates); diff != "" {
				t.Errorf("comment updates differ from expected: %s", diff)
			}

			checkComments(gc, tc.name, tc.expectedComment, t)

			expected := sets.NewString()
			for _, label := range tc.expectedLabels {
				expected.Insert(fmt.Sprintf("%s/%s#%d:%s", testEvent.org, testEvent.repo, testEvent.number, label))
			}

			actual := sets.NewString(gc.IssueLabelsExisting...)
			actual.Insert(gc.IssueLabelsAdded...)
			actual.Delete(gc.IssueLabelsRemoved...)

			if missing := expected.Difference(actual); missing.Len() > 0 {
				t.Errorf("%s: missing expected labels: %v", tc.name, missing.List())
			}
			if extra := actual.Difference(expected); extra.Len() > 0 {
				t.Errorf("%s: unexpected labels: %v", tc.name, extra.List())
			}

			// reset errors on client for verification
			jiraClient.CreateIssueError = nil
			jiraClient.GetIssueError = nil
			if tc.expectedIssue != nil {
				actual, err := jiraClient.GetIssue(tc.expectedIssue.ID)
				if err != nil {
					t.Errorf("%s: failed to get expected bug during test: %v", tc.name, err)
				}
				if !reflect.DeepEqual(actual, tc.expectedIssue) {
					t.Errorf("%s: got incorrect bug after update: %s", tc.name, cmp.Diff(actual, tc.expectedIssue, allowEventAndDate))
				}
			}
		})
	}
}

func checkComments(client *fakegithub.FakeClient, name, expectedComment string, t *testing.T) {
	wantedComments := 0
	if expectedComment != "" {
		wantedComments = 1
	}
	if len(client.IssueCommentsAdded) != wantedComments {
		t.Errorf("%s: wanted %d comment, got %d: %v", name, wantedComments, len(client.IssueCommentsAdded), client.IssueCommentsAdded)
	}

	if expectedComment != "" && len(client.IssueCommentsAdded) == 1 {
		if expectedComment != client.IssueCommentsAdded[0] {
			t.Errorf("%s: got incorrect comment: %v", name, cmp.Diff(expectedComment, client.IssueCommentsAdded[0]))
		}
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
			expectedError:  jiraclient.NewNotFoundError(errors.New("404 from cache for key issue-123, project name issue")),
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

func TestHelpProvider(t *testing.T) {
	rawConfig := `disabled_jira_projects:
- "private-project"
default:
  "*":
    target_version: global-default
  "global-branch":
    is_open: false
    target_version: global-branch-default
orgs:
  my-org:
    default:
      "*":
        is_open: true
        target_version: my-org-default
        state_after_validation:
          status: "PRE"
      "my-org-branch":
        target_version: my-org-branch-default
        state_after_validation:
          status: "POST"
        add_external_link: true
    repos:
      my-repo:
        branches:
          "*":
            is_open: false
            target_version: my-repo-default
            valid_states:
            - status: VALIDATED
          "my-repo-branch":
            target_version: my-repo-branch
            valid_states:
            - status: MODIFIED
            add_external_link: true
            state_after_merge:
              status: MODIFIED
            allowed_security_levels:
            - default
          "branch-that-likes-closed-bugs":
            valid_states:
            - status: VERIFIED
            - status: CLOSED
              resolution: ERRATA
            dependent_bug_states:
            - status: CLOSED
              resolution: ERRATA
            state_after_merge:
              status: CLOSED
              resolution: FIXED
            state_after_validation:
              status: CLOSED
              resolution: VALIDATED`

	var config plugins.Jira
	if err := yaml.Unmarshal([]byte(rawConfig), &config); err != nil {
		t.Fatalf("couldn't unmarshal config: %v", err)
	}

	pc := &plugins.Configuration{Jira: config}
	enabledRepos := []prowconfig.OrgRepo{
		{Org: "some-org", Repo: "some-repo"},
		{Org: "my-org", Repo: "some-repo"},
		{Org: "my-org", Repo: "my-repo"},
	}
	help, err := helpProvider(pc, enabledRepos)
	if err != nil {
		t.Fatalf("unexpected error creating help provider: %v", err)
	}
	// don't check snippet
	help.Snippet = ""

	expected := &pluginhelp.PluginHelp{
		Description: "The jira plugin ensures that pull requests reference a valid Jira bug in their title.",
		Config: map[string]string{
			"some-org/some-repo": `The plugin has the following configuration:<ul>
<li>by default, valid bugs must target the "global-default" version.</li>
<li>on the "global-branch" branch, valid bugs must be closed and target the "global-branch-default" version.</li>
</ul>`,
			"my-org/some-repo": `The plugin has the following configuration:<ul>
<li>by default, valid bugs must be open and target the "my-org-default" version. After being linked to a pull request, bugs will be moved to the PRE state.</li>
<li>on the "my-org-branch" branch, valid bugs must be open and target the "my-org-branch-default" version. After being linked to a pull request, bugs will be moved to the POST state and updated to refer to the pull request using the external bug tracker.</li>
</ul>`,
			"my-org/my-repo": `The plugin has the following configuration:<ul>
<li>by default, valid bugs must be closed, target the "my-repo-default" version, and be in one of the following states: VALIDATED. After being linked to a pull request, bugs will be moved to the PRE state.</li>
<li>on the "branch-that-likes-closed-bugs" branch, valid bugs must be closed, target the "my-repo-default" version, be in one of the following states: VERIFIED, CLOSED (ERRATA), depend on at least one other bug, and have all dependent bugs in one of the following states: CLOSED (ERRATA). After being linked to a pull request, bugs will be moved to the CLOSED (VALIDATED) state and moved to the CLOSED (FIXED) state when all linked pull requests are merged.</li>
<li>on the "my-org-branch" branch, valid bugs must be closed, target the "my-repo-default" version, and be in one of the following states: VALIDATED. After being linked to a pull request, bugs will be moved to the POST state and updated to refer to the pull request using the external bug tracker.</li>
<li>on the "my-repo-branch" branch, valid bugs must be closed, target the "my-repo-branch" version, and be in one of the following states: MODIFIED. After being linked to a pull request, bugs will be moved to the PRE state, updated to refer to the pull request using the external bug tracker, and moved to the MODIFIED state when all linked pull requests are merged.</li>
</ul>`,
		},
		Commands: []pluginhelp.Command{
			{
				Usage:       "/jira refresh",
				Description: "Check Jira for a valid bug referenced in the PR title",
				Featured:    false,
				WhoCanUse:   "Anyone",
				Examples:    []string{"/jira refresh"},
			}, {
				Usage:       "/jira cc-qa",
				Description: "Request PR review from QA contact specified in Jira",
				Featured:    false,
				WhoCanUse:   "Anyone",
				Examples:    []string{"/jira cc-qa"},
			},
		},
	}

	if actual := help; !reflect.DeepEqual(actual, expected) {
		t.Errorf("resolved incorrect plugin help: %v", cmp.Diff(actual, expected, allowEventAndDate))
	}
}

func TestDigestPR(t *testing.T) {
	yes := true
	var testCases = []struct {
		name              string
		pre               github.PullRequestEvent
		validateByDefault *bool
		expected          *event
		expectedErr       bool
	}{
		{
			name: "unrelated event gets ignored",
			pre: github.PullRequestEvent{
				Action: github.PullRequestFileAdded,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number: 1,
					Title:  "OCPBUGS-123: fixed it!",
					State:  "open",
				},
			},
		},
		{
			name: "unrelated title gets ignored",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number: 1,
					Title:  "fixing a typo",
					State:  "open",
				},
			},
		},
		{
			name: "unrelated title gets handled when validating by default",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "fixing a typo",
					State:   "open",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			validateByDefault: &yes,
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, state: "open", missing: true, opened: true, key: "", title: "fixing a typo", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "title referencing bug gets an event",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "OCPBUGS-123: fixed it!",
					State:   "open",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, state: "open", opened: true, key: "OCPBUGS-123", title: "OCPBUGS-123: fixed it!", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "title referencing bug gets an event on PR merge",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionClosed,
				PullRequest: github.PullRequest{
					Merged: true,
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "OCPBUGS-123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, merged: true, closed: true, key: "OCPBUGS-123", title: "OCPBUGS-123: fixed it!", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "title referencing bug gets an event on PR close",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionClosed,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "OCPBUGS-123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, merged: false, closed: true, key: "OCPBUGS-123", title: "OCPBUGS-123: fixed it!", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "non-jira cherrypick PR sets e.missing to true",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "release-4.4",
					},
					Number:  3,
					Title:   "[release-4.4] fixing a typo",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
					Body: `This is an automated cherry-pick of #2

/assign user`,
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "release-4.4", number: 3, opened: true, body: "This is an automated cherry-pick of #2\n\n/assign user", title: "[release-4.4] fixing a typo", htmlUrl: "http.com", login: "user", cherrypick: true, cherrypickFromPRNum: 2, cherrypickTo: "release-4.4", missing: true,
			},
		},
		{
			name: "cherrypicked PR gets cherrypick event",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "release-4.4",
					},
					Number:  3,
					Title:   "[release-4.4] OCPBUGS-123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
					Body: `This is an automated cherry-pick of #2

/assign user`,
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "release-4.4", number: 3, opened: true, body: "This is an automated cherry-pick of #2\n\n/assign user", title: "[release-4.4] OCPBUGS-123: fixed it!", htmlUrl: "http.com", login: "user", cherrypick: true, cherrypickFromPRNum: 2, cherrypickTo: "release-4.4", key: "OCPBUGS-123",
			},
		},
		{
			name: "edited cherrypicked PR gets normal event",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionEdited,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "release-4.4",
					},
					Number:  3,
					Title:   "[release-4.4] OCPBUGS-123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
					Body: `This is an automated cherry-pick of #2

/assign user`,
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "release-4.4", number: 3, key: "OCPBUGS-123", body: "This is an automated cherry-pick of #2\n\n/assign user", title: "[release-4.4] OCPBUGS-123: fixed it!", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "title change referencing same bug gets no event",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "OCPBUGS-123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
				Changes: []byte(`{"title":{"from":"OCPBUGS-123: fixed it! (WIP)"}}`),
			},
		},
		{
			name: "title change referencing new bug gets event",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "OCPBUGS-123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
				Changes: []byte(`{"title":{"from":"fixed it! (WIP)"}}`),
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, opened: true, key: "OCPBUGS-123", title: "OCPBUGS-123: fixed it!", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "title change dereferencing bug gets event",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
				Changes: []byte(`{"title":{"from":"OCPBUGS-123: fixed it! (WIP)"}}`),
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, opened: true, missing: true, title: "fixed it!", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "title change to no bug with unrelated changes gets no event",
			pre: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				PullRequest: github.PullRequest{
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "org",
							},
							Name: "repo",
						},
						Ref: "branch",
					},
					Number:  1,
					Title:   "fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
				Changes: []byte(`{"oops":{"doops":"payload"}}`),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			event, err := digestPR(logrus.WithField("testCase", testCase.name), testCase.pre, testCase.validateByDefault)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			if actual, expected := event, testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: did not get correct event: %v", testCase.name, cmp.Diff(actual, expected, allowEventAndDate))
			}
		})
	}
}

func TestDigestComment(t *testing.T) {
	var testCases = []struct {
		name            string
		e               github.GenericCommentEvent
		title           string
		merged          bool
		expected        *event
		expectedComment string
		expectedErr     bool
	}{
		{
			name: "unrelated event gets ignored",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionDeleted,
				IsPR:   true,
				Body:   "/jira refresh",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
			},
			title: "OCPBUGS-123: oopsie doopsie",
		},
		{
			name: "unrelated title gets an event saying so",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/jira refresh",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
				User: github.User{
					Login: "user",
				},
				HTMLURL: "www.com",
			},
			title: "cole, please review this typo fix",
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, missing: true, body: "/jira refresh", htmlUrl: "www.com", login: "user", cc: false, isComment: true,
			},
		},
		{
			name: "comment on issue gets no event but a comment",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   false,
				Body:   "/jira refresh",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
			},
			title: "someone misspelled words in this repo",
			expectedComment: `org/repo#1:@: Jira bug referencing is only supported for Pull Requests, not issues.

<details>

In response to [this]():

>/jira refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name: "title referencing bug gets an event",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/jira refresh",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
				User: github.User{
					Login: "user",
				},
				HTMLURL: "www.com",
			},
			title: "OCPBUGS-123: oopsie doopsie",
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, key: "OCPBUGS-123", body: "/jira refresh", htmlUrl: "www.com", login: "user", cc: false, isComment: true,
			},
		},
		{
			name: "title referencing bug in a merged PR gets an event",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/jira refresh",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
				User: github.User{
					Login: "user",
				},
				HTMLURL: "www.com",
			},
			title:  "OCPBUGS-123: oopsie doopsie",
			merged: true,
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, key: "OCPBUGS-123", merged: true, body: "/jira refresh", htmlUrl: "www.com", login: "user", cc: false, isComment: true,
			},
		},
		{
			name: "cc-qa comment event has cc bool set to true",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/jira cc-qa",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
				User: github.User{
					Login: "user",
				},
				HTMLURL: "www.com",
			},
			title: "OCPBUGS-123: oopsie doopsie",
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, key: "OCPBUGS-123", body: "/jira cc-qa", htmlUrl: "www.com", login: "user", cc: true, isComment: true,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := fakegithub.NewFakeClient()
			client.PullRequests = map[int]*github.PullRequest{
				1: {Base: github.PullRequestBranch{Ref: "branch"}, Title: testCase.title, Merged: testCase.merged},
			}
			event, err := digestComment(client, logrus.WithField("testCase", testCase.name), testCase.e)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			if actual, expected := event, testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: did not get correct event: %v", testCase.name, cmp.Diff(actual, expected, allowEventAndDate))
			}

			checkComments(client, testCase.name, testCase.expectedComment, t)
		})
	}
}

func TestBugKeyFromTitle(t *testing.T) {
	var testCases = []struct {
		title            string
		expectedKey      string
		expectedNotFound bool
	}{
		{
			title:            "no match",
			expectedKey:      "",
			expectedNotFound: true,
		},
		{
			title:       "OCPBUGS-12: Canonical",
			expectedKey: "OCPBUGS-12",
		},
		{
			title:            "OCPBUGS-12 : Space before colon",
			expectedKey:      "",
			expectedNotFound: true,
		},
		{
			title:       "[rebase release-1.0] OCPBUGS-12: Prefix",
			expectedKey: "OCPBUGS-12",
		},
		{
			title:       "Revert: \"OCPBUGS-12: Revert default\"",
			expectedKey: "OCPBUGS-12",
		},
		{
			title:       "OCPBUGS-34: Revert: \"OCPBUGS-12: Revert default\"",
			expectedKey: "OCPBUGS-34",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.title, func(t *testing.T) {
			key, notFound, err := bugKeyFromTitle(testCase.title)
			if err != nil {
				t.Errorf("%s: Unexpected error: %v", testCase.title, err)
			}
			if key != testCase.expectedKey {
				t.Errorf("%s: unexpected %s != %s", testCase.title, key, testCase.expectedKey)
			}
			if notFound != testCase.expectedNotFound {
				t.Errorf("%s: unexpected %t != %t", testCase.title, notFound, testCase.expectedNotFound)
			}
		})
	}
}

func TestValidateBug(t *testing.T) {
	open, closed := true, false
	oneStr, twoStr := "v1", "v2"
	one := []*jira.Version{{Name: "v1"}}
	two := []*jira.Version{{Name: "v2"}}
	verified := []plugins.JiraBugState{{Status: "VERIFIED"}}
	modified := []plugins.JiraBugState{{Status: "MODIFIED"}}
	updated := plugins.JiraBugState{Status: "UPDATED"}
	var testCases = []struct {
		name        string
		issue       *jira.Issue
		dependents  []*jira.Issue
		options     plugins.JiraBranchOptions
		valid       bool
		validations []string
		why         []string
	}{
		{
			name:    "no requirements means a valid bug",
			issue:   &jira.Issue{Fields: &jira.IssueFields{}},
			options: plugins.JiraBranchOptions{},
			valid:   true,
		},
		{
			name:        "matching open requirement means a valid bug",
			issue:       &jira.Issue{Fields: &jira.IssueFields{Status: &jira.Status{Name: "NEW"}}},
			options:     plugins.JiraBranchOptions{IsOpen: &open},
			valid:       true,
			validations: []string{"bug is open, matching expected state (open)"},
		},
		{
			name:        "matching closed requirement means a valid bug",
			issue:       &jira.Issue{Fields: &jira.IssueFields{Status: &jira.Status{Name: "CLOSED"}}},
			options:     plugins.JiraBranchOptions{IsOpen: &closed},
			valid:       true,
			validations: []string{"bug isn't open, matching expected state (not open)"},
		},
		{
			name:    "not matching open requirement means an invalid bug",
			issue:   &jira.Issue{Fields: &jira.IssueFields{Status: &jira.Status{Name: "CLOSED"}}},
			options: plugins.JiraBranchOptions{IsOpen: &open},
			valid:   false,
			why:     []string{"expected the bug to be open, but it isn't"},
		},
		{
			name:    "not matching closed requirement means an invalid bug",
			issue:   &jira.Issue{Fields: &jira.IssueFields{Status: &jira.Status{Name: "NEW"}}},
			options: plugins.JiraBranchOptions{IsOpen: &closed},
			valid:   false,
			why:     []string{"expected the bug to not be open, but it is"},
		},
		{
			name: "matching target version requirement means a valid bug",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &one,
				},
			}},
			options:     plugins.JiraBranchOptions{TargetVersion: &oneStr},
			valid:       true,
			validations: []string{"bug target version (v1) matches configured target version for branch (v1)"},
		},
		{
			name: "not matching target version requirement means an invalid bug",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &two,
				},
			}},
			options: plugins.JiraBranchOptions{TargetVersion: &oneStr},
			valid:   false,
			why:     []string{"expected the bug to target the \"v1\" version, but it targets \"v2\" instead"},
		},
		{
			name:    "not setting target version requirement means an invalid bug",
			issue:   &jira.Issue{Fields: &jira.IssueFields{}},
			options: plugins.JiraBranchOptions{TargetVersion: &oneStr},
			valid:   false,
			why:     []string{"expected the bug to target the \"v1\" version, but no target version was set"},
		},
		{
			name:        "matching status requirement means a valid bug",
			issue:       &jira.Issue{Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}},
			options:     plugins.JiraBranchOptions{ValidStates: &modified},
			valid:       true,
			validations: []string{"bug is in the state MODIFIED, which is one of the valid states (MODIFIED)"},
		},
		{
			name:        "matching status requirement by being in the migrated state means a valid bug",
			issue:       &jira.Issue{Fields: &jira.IssueFields{Status: &jira.Status{Name: "UPDATED"}}},
			options:     plugins.JiraBranchOptions{ValidStates: &modified, StateAfterValidation: &updated},
			valid:       true,
			validations: []string{"bug is in the state UPDATED, which is one of the valid states (MODIFIED, UPDATED)"},
		},
		{
			name:    "not matching status requirement means an invalid bug",
			issue:   &jira.Issue{Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}},
			options: plugins.JiraBranchOptions{ValidStates: &verified},
			valid:   false,
			why:     []string{"expected the bug to be in one of the following states: VERIFIED, but it is MODIFIED instead"},
		},
		{
			name:    "dependent status requirement with no dependent bugs means a valid bug",
			issue:   &jira.Issue{Key: "OCPBUGS-123", Fields: &jira.IssueFields{}},
			options: plugins.JiraBranchOptions{DependentBugStates: &verified},
			valid:   false,
			why:     []string{"expected [Jira Issue OCPBUGS-123](https://my-jira.com/browse/OCPBUGS-123) to depend on a bug in one of the following states: VERIFIED, but no dependents were found"},
		},
		{
			name: "not matching dependent bug status requirement means an invalid bug",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
			}},
			dependents:  []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}}},
			options:     plugins.JiraBranchOptions{DependentBugStates: &verified},
			valid:       false,
			validations: []string{"bug has dependents"},
			why:         []string{"expected dependent [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) to be in one of the following states: VERIFIED, but it is MODIFIED instead"},
		},
		{
			name: "not matching dependent bug target version requirement means an invalid bug",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
			}},
			dependents: []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "MODIFIED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &two,
				},
			}}},
			options:     plugins.JiraBranchOptions{DependentBugTargetVersions: &[]string{oneStr}},
			valid:       false,
			validations: []string{"bug has dependents"},
			why:         []string{"expected dependent [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) to target a version in v1, but it targets \"v2\" instead"},
		},
		{
			name: "not having a dependent bug target version means an invalid bug",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
			}},
			dependents:  []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}}},
			options:     plugins.JiraBranchOptions{DependentBugTargetVersions: &[]string{oneStr}},
			valid:       false,
			validations: []string{"bug has dependents"},
			why:         []string{"expected dependent [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) to target a version in v1, but no target version was set"},
		},
		{
			name: "matching all requirements means a valid bug",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "MODIFIED"},
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &one,
				},
			}},
			dependents: []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "MODIFIED"},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &two,
				},
			}}},
			options: plugins.JiraBranchOptions{IsOpen: &open, TargetVersion: &oneStr, ValidStates: &modified, DependentBugStates: &modified, DependentBugTargetVersions: &[]string{twoStr}},
			validations: []string{`bug is open, matching expected state (open)`,
				`bug target version (v1) matches configured target version for branch (v1)`,
				"bug is in the state MODIFIED, which is one of the valid states (MODIFIED)",
				"dependent bug [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) is in the state MODIFIED, which is one of the valid states (MODIFIED)",
				`dependent [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) targets the "v2" version, which is one of the valid target versions: v2`,
				"bug has dependents"},
			valid: true,
		},
		{
			name: "matching no requirements means an invalid bug",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status: &jira.Status{Name: "CLOSED"},
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
				Unknowns: tcontainer.MarshalMap{
					"customfield_12319940": &one,
				},
			}},
			dependents:  []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{Status: &jira.Status{Name: "MODIFIED"}}}},
			options:     plugins.JiraBranchOptions{IsOpen: &open, TargetVersion: &twoStr, ValidStates: &verified, DependentBugStates: &verified},
			valid:       false,
			validations: []string{"bug has dependents"},
			why: []string{"expected the bug to be open, but it isn't",
				"expected the bug to target the \"v2\" version, but it targets \"v1\" instead",
				"expected the bug to be in one of the following states: VERIFIED, but it is CLOSED instead",
				"expected dependent [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) to be in one of the following states: VERIFIED, but it is MODIFIED instead",
			},
		},
		{
			name: "matching status means a valid bug when resolution is not required",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "LOL_GO_AWAY"},
			}},
			options:     plugins.JiraBranchOptions{ValidStates: &[]plugins.JiraBugState{{Status: "CLOSED"}}},
			valid:       true,
			validations: []string{"bug is in the state CLOSED (LOL_GO_AWAY), which is one of the valid states (CLOSED)"},
		},
		{
			name: "matching just status means an invalid bug when resolution does not match",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "LOL_GO_AWAY"},
			}},
			options: plugins.JiraBranchOptions{ValidStates: &[]plugins.JiraBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:   false,
			why: []string{
				"expected the bug to be in one of the following states: CLOSED (ERRATA), but it is CLOSED (LOL_GO_AWAY) instead",
			},
		},
		{
			name: "matching status and resolution means a valid bug when both are required",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "ERRATA"},
			}},
			options:     plugins.JiraBranchOptions{ValidStates: &[]plugins.JiraBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:       true,
			validations: []string{"bug is in the state CLOSED (ERRATA), which is one of the valid states (CLOSED (ERRATA))"},
		},
		{
			name: "matching resolution means a valid bug when status is not required",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "ERRATA"},
			}},
			options:     plugins.JiraBranchOptions{ValidStates: &[]plugins.JiraBugState{{Resolution: "ERRATA"}}},
			valid:       true,
			validations: []string{"bug is in the state CLOSED (ERRATA), which is one of the valid states (any status with resolution ERRATA)"},
		},
		{
			name: "matching just resolution means an invalid bug when status does not match",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "ERRATA"},
			}},
			options: plugins.JiraBranchOptions{ValidStates: &[]plugins.JiraBugState{{Status: "RESOLVED", Resolution: "ERRATA"}}},
			valid:   false,
			why: []string{
				"expected the bug to be in one of the following states: RESOLVED (ERRATA), but it is CLOSED (ERRATA) instead",
			},
		},
		{
			name: "matching status on dependent bug means a valid bug when resolution is not required",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "LOL_GO_AWAY"},
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
			}},
			dependents: []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "LOL_GO_AWAY"},
			}}},
			options:     plugins.JiraBranchOptions{DependentBugStates: &[]plugins.JiraBugState{{Status: "CLOSED"}}},
			valid:       true,
			validations: []string{"dependent bug [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) is in the state CLOSED (LOL_GO_AWAY), which is one of the valid states (CLOSED)", "bug has dependents"},
		},
		{
			name: "matching just status on dependent bug means an invalid bug when resolution does not match",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "LOL_GO_AWAY"},
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
			}},
			dependents: []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "LOL_GO_AWAY"},
			}}},
			options:     plugins.JiraBranchOptions{DependentBugStates: &[]plugins.JiraBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:       false,
			validations: []string{"bug has dependents"},
			why: []string{
				"expected dependent [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) to be in one of the following states: CLOSED (ERRATA), but it is CLOSED (LOL_GO_AWAY) instead",
			},
		},
		{
			name: "matching status and resolution on dependent bug means a valid bug when both are required",
			issue: &jira.Issue{Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "ERRATA"},
				IssueLinks: []*jira.IssueLink{{
					Type: jira.IssueLinkType{
						Name:    "Cloners",
						Inward:  "is cloned by",
						Outward: "clones",
					},
					OutwardIssue: &jira.Issue{ID: "2", Key: "OCPBUGS-124"},
				}},
			}},
			dependents: []*jira.Issue{{ID: "2", Key: "OCPBUGS-124", Fields: &jira.IssueFields{
				Status:     &jira.Status{Name: "CLOSED"},
				Resolution: &jira.Resolution{Name: "ERRATA"},
			}}},
			options:     plugins.JiraBranchOptions{DependentBugStates: &[]plugins.JiraBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:       true,
			validations: []string{"dependent bug [Jira Issue OCPBUGS-124](https://my-jira.com/browse/OCPBUGS-124) is in the state CLOSED (ERRATA), which is one of the valid states (CLOSED (ERRATA))", "bug has dependents"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			valid, validations, why := validateBug(testCase.issue, testCase.dependents, testCase.options, "https://my-jira.com")
			if valid != testCase.valid {
				t.Errorf("%s: didn't validate bug correctly, expected %t got %t", testCase.name, testCase.valid, valid)
			}
			if !reflect.DeepEqual(validations, testCase.validations) {
				t.Errorf("%s: didn't get correct validations: %v", testCase.name, cmp.Diff(testCase.validations, validations, allowEventAndDate))
			}
			if !reflect.DeepEqual(why, testCase.why) {
				t.Errorf("%s: didn't get correct reasons why: %v", testCase.name, cmp.Diff(testCase.why, why, allowEventAndDate))
			}
		})
	}
}

func TestProcessQuery(t *testing.T) {
	var testCases = []struct {
		name     string
		query    emailToLoginQuery
		email    string
		expected string
	}{
		{
			name: "single login returns cc",
			query: emailToLoginQuery{
				Search: querySearch{
					Edges: []queryEdge{{
						Node: queryNode{
							User: queryUser{
								Login: "ValidLogin",
							},
						},
					}},
				},
			},
			email:    "qa_tester@example.com",
			expected: "Requesting review from QA contact:\n/cc @ValidLogin",
		}, {
			name: "no login returns not found error",
			query: emailToLoginQuery{
				Search: querySearch{
					Edges: []queryEdge{},
				},
			},
			email:    "qa_tester@example.com",
			expected: "No GitHub users were found matching the public email listed for the QA contact in Jira (qa_tester@example.com), skipping review request.",
		}, {
			name: "multiple logins returns multiple results error",
			query: emailToLoginQuery{
				Search: querySearch{
					Edges: []queryEdge{{
						Node: queryNode{
							User: queryUser{
								Login: "Login1",
							},
						},
					}, {
						Node: queryNode{
							User: queryUser{
								Login: "Login2",
							},
						},
					}},
				},
			},
			email:    "qa_tester@example.com",
			expected: "Multiple GitHub users were found matching the public email listed for the QA contact in Jira (qa_tester@example.com), skipping review request. List of users with matching email:\n\t- Login1\n\t- Login2",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := processQuery(&testCase.query, testCase.email, logrus.WithField("testCase", testCase.name))
			if response != testCase.expected {
				t.Errorf("%s: Expected \"%s\", got \"%s\"", testCase.name, testCase.expected, response)
			}
		})
	}
}

func TestGetCherrypickPRMatch(t *testing.T) {
	var prNum = 123
	var branch = "v2"
	var testCases = []struct {
		name      string
		requestor string
		note      string
	}{{
		name: "No requestor or string",
	}, {
		name:      "Include requestor",
		requestor: "user",
	}, {
		name: "Include note",
		note: "this is a test",
	}, {
		name:      "Include requestor and note",
		requestor: "user",
		note:      "this is a test",
	}}
	var pr = &github.PullRequestEvent{
		PullRequest: github.PullRequest{
			Base: github.PullRequestBranch{
				Ref: branch,
			},
		},
	}
	for _, testCase := range testCases {
		testPR := *pr
		testPR.PullRequest.Body = cherrypicker.CreateCherrypickBody(prNum, testCase.requestor, testCase.note)
		cherrypick, cherrypickOfPRNum, cherrypickTo, err := getCherryPickMatch(testPR)
		if err != nil {
			t.Fatalf("%s: Got error but did not expect one: %v", testCase.name, err)
		}
		if !cherrypick {
			t.Errorf("%s: Expected cherrypick to be true, but got false", testCase.name)
		}
		if cherrypickOfPRNum != prNum {
			t.Errorf("%s: Got incorrect PR num: Expected %d, got %d", testCase.name, prNum, cherrypickOfPRNum)
		}
		if cherrypickTo != "v2" {
			t.Errorf("%s: Got incorrect cherrypick to branch: Expected %s, got %s", testCase.name, branch, cherrypickTo)
		}
	}
}

func TestIsBugAllowed(t *testing.T) {
	testCases := []struct {
		name           string
		bug            *jira.Issue
		securityLevels []string
		expected       bool
	}{
		{
			name:           "no groups configured means always allowed",
			securityLevels: []string{},
			expected:       true,
		},
		{
			name: "matching one level is allowed",
			bug: &jira.Issue{Fields: &jira.IssueFields{
				Unknowns: tcontainer.MarshalMap{
					"security": jiraclient.SecurityLevel{Name: "whoa"},
				},
			}},
			securityLevels: []string{"whoa", "really", "cool"},
			expected:       true,
		},
		{
			name: "no levels matching is not allowed",
			bug: &jira.Issue{Fields: &jira.IssueFields{
				Unknowns: tcontainer.MarshalMap{
					"security": jiraclient.SecurityLevel{Name: "whoa"},
				},
			}},
			securityLevels: []string{"other"},
			expected:       false,
		},
		{
			name:           "no level set in bug is equal to level default",
			bug:            &jira.Issue{Fields: &jira.IssueFields{}},
			securityLevels: []string{"default"},
			expected:       true,
		},
		{
			name:           "default level is not set",
			bug:            &jira.Issue{Fields: &jira.IssueFields{}},
			securityLevels: []string{"internal"},
			expected:       false,
		},
	}
	for _, testCase := range testCases {
		actual, err := isBugAllowed(testCase.bug, testCase.securityLevels)
		if err != nil {
			// this error should never occur when run against a real jira server, so no need to test error handling
			t.Fatalf("%s: unexpected error: %v", testCase.name, err)
		}
		if actual != testCase.expected {
			t.Errorf("%s: isBugAllowed returned %v incorrectly", testCase.name, actual)
		}
	}
}

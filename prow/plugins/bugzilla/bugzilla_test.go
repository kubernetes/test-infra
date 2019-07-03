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

package bugzilla

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/bugzilla"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/pluginhelp"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/plugins"
)

func TestHelpProvider(t *testing.T) {
	rawConfig := `default:
  "*":
    target_release: global-default
  "global-branch":
    is_open: false
    target_release: global-branch-default
orgs:
  my-org:
    default:
      "*":
        is_open: true
        target_release: my-org-default
        status_after_validation: "PRE"
      "my-org-branch":
        target_release: my-org-branch-default
        status_after_validation: "POST"
        add_external_link: true
    repos:
      my-repo:
        branches:
          "*":
            is_open: false
            target_release: my-repo-default
            statuses:
            - VALIDATED
          "my-repo-branch":
            target_release: my-repo-branch
            statuses:
            - MODIFIED
            add_external_link: true`
	var config plugins.Bugzilla
	if err := yaml.Unmarshal([]byte(rawConfig), &config); err != nil {
		t.Fatalf("couldn't unmarshal config: %v", err)
	}

	pc := &plugins.Configuration{Bugzilla: config}
	help, err := helpProvider(pc, []string{"some-org/some-repo", "my-org/some-repo", "my-org/my-repo"})
	if err != nil {
		t.Fatalf("unexpected error creating help provider: %v", err)
	}

	expected := &pluginhelp.PluginHelp{
		Description: "The bugzilla plugin ensures that pull requests reference a valid Bugzilla bug in their title.",
		Config: map[string]string{
			"some-org/some-repo": `The plugin has the following configuration:<ul>
<li>by default, valid bugs must target the "global-default" release.</li>
<li>on the "global-branch" branch, valid bugs must be closed and target the "global-branch-default" release.</li>
</ul>`,
			"my-org/some-repo": `The plugin has the following configuration:<ul>
<li>by default, valid bugs must be open and target the "my-org-default" release. After being linked to a pull request, bugs will be moved to the "PRE" state.</li>
<li>on the "my-org-branch" branch, valid bugs must be open and target the "my-org-branch-default" release. After being linked to a pull request, bugs will be moved to the "POST" state and updated to refer to the pull request using the external bug tracker.</li>
</ul>`,
			"my-org/my-repo": `The plugin has the following configuration:<ul>
<li>by default, valid bugs must be closed, target the "my-repo-default" release, and be in one of the following states: VALIDATED. After being linked to a pull request, bugs will be moved to the "PRE" state.</li>
<li>on the "my-org-branch" branch, valid bugs must be closed, target the "my-repo-default" release, and be in one of the following states: VALIDATED. After being linked to a pull request, bugs will be moved to the "POST" state and updated to refer to the pull request using the external bug tracker.</li>
<li>on the "my-repo-branch" branch, valid bugs must be closed, target the "my-repo-branch" release, and be in one of the following states: MODIFIED. After being linked to a pull request, bugs will be moved to the "PRE" state and updated to refer to the pull request using the external bug tracker.</li>
</ul>`,
		},
		Commands: []pluginhelp.Command{{
			Usage:       "/bugzilla refresh",
			Description: "Check Bugzilla for a valid bug referenced in the PR title",
			Featured:    false,
			WhoCanUse:   "Anyone",
			Examples:    []string{"/bugzilla refresh"},
		}},
	}

	if actual := help; !reflect.DeepEqual(actual, expected) {
		t.Errorf("resolved incorrect plugin help: %v", diff.ObjectReflectDiff(actual, expected))
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
					Title:  "Bug 123: fixed it!",
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
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			validateByDefault: &yes,
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, missing: true, bugId: 0, body: "fixing a typo", htmlUrl: "http.com", login: "user",
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
					Title:   "Bug 123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
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
					Title:   "Bug 123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
				Changes: []byte(`{"title":{"from":"Bug 123: fixed it! (WIP)"}}`),
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
					Title:   "Bug 123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
				Changes: []byte(`{"title":{"from":"fixed it! (WIP)"}}`),
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
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
				Changes: []byte(`{"title":{"from":"Bug 123: fixed it! (WIP)"}}`),
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, missing: true, body: "fixed it!", htmlUrl: "http.com", login: "user",
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
				t.Errorf("%s: did not get correct event: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestDigestComment(t *testing.T) {
	var testCases = []struct {
		name            string
		e               github.GenericCommentEvent
		title           string
		expected        *event
		expectedComment string
		expectedErr     bool
	}{
		{
			name: "unrelated event gets ignored",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionDeleted,
				IsPR:   true,
				Body:   "/bugzilla refresh",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
			},
			title: "Bug 123: oopsie doopsie",
		},
		{
			name: "unrelated title gets an event saying so",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/bugzilla refresh",
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, missing: true, body: "/bugzilla refresh", htmlUrl: "www.com", login: "user",
			},
		},
		{
			name: "comment on issue gets no event but a comment",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   false,
				Body:   "/bugzilla refresh",
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				Number: 1,
			},
			title: "someone misspelled words in this repo",
			expectedComment: `org/repo#1:@: Bugzilla bug referencing is only supported for Pull Requests, not issues.

<details>

In response to [this]():

>/bugzilla refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name: "title referencing bug gets an event",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/bugzilla refresh",
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
			title: "Bug 123: oopsie doopsie",
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "/bugzilla refresh", htmlUrl: "www.com", login: "user",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := fakegithub.FakeClient{
				PullRequests: map[int]*github.PullRequest{
					1: {Base: github.PullRequestBranch{Ref: "branch"}, Title: testCase.title},
				},
				IssueComments: map[int][]github.IssueComment{},
			}
			event, err := digestComment(&client, logrus.WithField("testCase", testCase.name), testCase.e)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			if actual, expected := event, testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: did not get correct event: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}

			checkComments(client, testCase.name, testCase.expectedComment, t)
		})
	}
}

func TestHandle(t *testing.T) {
	yes := true
	open := true
	updated := "UPDATED"
	verified := []string{"VERIFIED"}
	e := event{
		org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
	}
	var testCases = []struct {
		name                 string
		labels               []string
		missing              bool
		externalBugExists    bool
		bugs                 []bugzilla.Bug
		bugErrors            []int
		options              plugins.BugzillaBranchOptions
		expectedLabels       []string
		expectedComment      string
		expectedBug          *bugzilla.Bug
		expectedExternalBugs []bugzilla.ExternalBug
	}{
		{
			name: "no bug found leaves a comment",
			expectedComment: `org/repo#1:@user: No Bugzilla bug with ID 123 exists in the tracker at www.bugzilla.
Once a valid bug is referenced in the title of this pull request, request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:      "error fetching bug leaves a comment",
			bugErrors: []int{123},
			expectedComment: `org/repo#1:@user: An error was encountered searching the Bugzilla server at www.bugzilla for bug 123:
> injected error getting bug
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "valid bug removes invalid label, adds valid label and comments",
			bugs:           []bugzilla.Bug{{ID: 123}},
			options:        plugins.BugzillaBranchOptions{}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references a valid [Bugzilla bug](www.bugzilla/show_bug.cgi?id=123).

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "invalid bug adds invalid label, removes valid label and comments",
			bugs:           []bugzilla.Bug{{ID: 123}},
			options:        plugins.BugzillaBranchOptions{IsOpen: &open},
			labels:         []string{"bugzilla/valid-bug"},
			expectedLabels: []string{"bugzilla/invalid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references an invalid [Bugzilla bug](www.bugzilla/show_bug.cgi?id=123):
 - expected the bug to be open, but it isn't

Comment <code>/bugzilla refresh</code> to re-evaluate validity if changes to the Bugzilla bug are made, or edit the title of this pull request to link to a different bug.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:    "no bug removes all labels and comments",
			missing: true,
			labels:  []string{"bugzilla/valid-bug", "bugzilla/invalid-bug"},
			expectedComment: `org/repo#1:@user: No Bugzilla bug is referenced in the title of this pull request.
To reference a bug, add 'Bug XXX:' to the title of this pull request and request another bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "valid bug with status update removes invalid label, adds valid label, comments and updates status",
			bugs:           []bugzilla.Bug{{ID: 123}},
			options:        plugins.BugzillaBranchOptions{StatusAfterValidation: &updated}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references a valid [Bugzilla bug](www.bugzilla/show_bug.cgi?id=123). The bug has been moved to the UPDATED state.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "UPDATED"},
		},
		{
			name:           "valid bug with status update removes invalid label, adds valid label, comments and does not update status when it is already correct",
			bugs:           []bugzilla.Bug{{ID: 123, Status: updated}},
			options:        plugins.BugzillaBranchOptions{StatusAfterValidation: &updated}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references a valid [Bugzilla bug](www.bugzilla/show_bug.cgi?id=123).

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "UPDATED"},
		},
		{
			name:           "valid bug with external link removes invalid label, adds valid label, comments, makes an external bug link",
			bugs:           []bugzilla.Bug{{ID: 123}},
			options:        plugins.BugzillaBranchOptions{AddExternalLink: &yes}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references a valid [Bugzilla bug](www.bugzilla/show_bug.cgi?id=123). The bug has been updated to refer to the pull request using the external bug tracker.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1"}},
		},
		{
			name:              "valid bug with already existing external link removes invalid label, adds valid label, comments to say nothing changed",
			bugs:              []bugzilla.Bug{{ID: 123}},
			externalBugExists: true,
			options:           plugins.BugzillaBranchOptions{AddExternalLink: &yes}, // no requirements --> always valid
			labels:            []string{"bugzilla/invalid-bug"},
			expectedLabels:    []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references a valid [Bugzilla bug](www.bugzilla/show_bug.cgi?id=123).

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1"}},
		},
		{
			name:      "failure to fetch dependent bug results in a comment",
			bugs:      []bugzilla.Bug{{ID: 123, DependsOn: []int{124}}},
			bugErrors: []int{124},
			options:   plugins.BugzillaBranchOptions{DependentBugStatuses: &verified},
			expectedComment: `org/repo#1:@user: An error was encountered searching the Bugzilla server at www.bugzilla for dependent bug 124:
> injected error getting bug
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "valid bug with dependent bugs removes invalid label, adds valid label, comments",
			bugs:           []bugzilla.Bug{{ID: 123, DependsOn: []int{124}}, {ID: 124, Status: "VERIFIED"}},
			options:        plugins.BugzillaBranchOptions{DependentBugStatuses: &verified},
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references a valid [Bugzilla bug](www.bugzilla/show_bug.cgi?id=123).

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gc := fakegithub.FakeClient{
				IssueLabelsExisting: []string{},
				IssueComments:       map[int][]github.IssueComment{},
			}
			for _, label := range testCase.labels {
				gc.IssueLabelsExisting = append(gc.IssueLabelsExisting, fmt.Sprintf("%s/%s#%d:%s", e.org, e.repo, e.number, label))
			}
			bc := bugzilla.Fake{
				EndpointString: "www.bugzilla",
				Bugs:           map[int]bugzilla.Bug{},
				BugErrors:      sets.NewInt(),
				ExternalBugs:   map[int][]bugzilla.ExternalBug{},
			}
			for _, bug := range testCase.bugs {
				bc.Bugs[bug.ID] = bug
			}
			bc.BugErrors.Insert(testCase.bugErrors...)
			if testCase.externalBugExists {
				bc.ExternalBugs[e.bugId] = []bugzilla.ExternalBug{{
					BugzillaBugID: e.bugId,
					ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", e.org, e.repo, e.number),
				}}
			}
			e.missing = testCase.missing
			err := handle(e, &gc, &bc, testCase.options, logrus.WithField("testCase", testCase.name))
			if err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			expected := sets.NewString()
			for _, label := range testCase.expectedLabels {
				expected.Insert(fmt.Sprintf("%s/%s#%d:%s", e.org, e.repo, e.number, label))
			}

			actual := sets.NewString(gc.IssueLabelsExisting...)
			actual.Insert(gc.IssueLabelsAdded...)
			actual.Delete(gc.IssueLabelsRemoved...)

			if missing := expected.Difference(actual); missing.Len() > 0 {
				t.Errorf("%s: missing expected labels: %v", testCase.name, missing.List())
			}
			if extra := actual.Difference(expected); extra.Len() > 0 {
				t.Errorf("%s: unexpected labels: %v", testCase.name, extra.List())
			}

			checkComments(gc, testCase.name, testCase.expectedComment, t)

			if testCase.expectedBug != nil {
				if actual, expected := bc.Bugs[testCase.expectedBug.ID], *testCase.expectedBug; !reflect.DeepEqual(actual, expected) {
					t.Errorf("%s: got incorrect bug after update: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
				}
			}
			if len(testCase.expectedExternalBugs) > 0 {
				if actual, expected := bc.ExternalBugs[testCase.expectedBug.ID], testCase.expectedExternalBugs; !reflect.DeepEqual(actual, expected) {
					t.Errorf("%s: got incorrect external bugs after update: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
				}
			}
		})
	}
}

func checkComments(client fakegithub.FakeClient, name, expectedComment string, t *testing.T) {
	wantedComments := 0
	if expectedComment != "" {
		wantedComments = 1
	}
	if len(client.IssueCommentsAdded) != wantedComments {
		t.Errorf("%s: wanted %d comment, got %d: %v", name, wantedComments, len(client.IssueCommentsAdded), client.IssueCommentsAdded)
	}

	if expectedComment != "" && len(client.IssueCommentsAdded) == 1 {
		if expectedComment != client.IssueCommentsAdded[0] {
			t.Errorf("%s: got incorrect comment: %v", name, diff.StringDiff(expectedComment, client.IssueCommentsAdded[0]))
		}
	}
}

func TestTitleMatch(t *testing.T) {
	var testCases = []struct {
		title    string
		expected int
	}{
		{
			title:    "no match",
			expected: -1,
		},
		{
			title:    "Bug 12: Canonical",
			expected: 12,
		},
		{
			title:    "bug 12: Lowercase",
			expected: 12,
		},
		{
			title:    "Bug 12 : Space before colon",
			expected: -1,
		},
		{
			title:    "[rebase release-1.0] Bug 12: Prefix",
			expected: 12,
		},
		{
			title:    "Revert: \"Bug 12: Revert default\"",
			expected: 12,
		},
		{
			title:    "Bug 34: Revert: \"Bug 12: Revert default\"",
			expected: 34,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.title, func(t *testing.T) {
			actual := -1
			match := titleMatch.FindStringSubmatch(testCase.title)
			if match != nil {
				id, err := strconv.Atoi(match[1])
				if err != nil {
					t.Fatal(err)
				}

				actual = id
			}

			if actual != testCase.expected {
				t.Errorf("unexpected %d != %d", actual, testCase.expected)
			}
		})
	}
}

func TestValidateBug(t *testing.T) {
	open, closed := true, false
	one, two := "v1", "v2"
	verified, modified := []string{"VERIFIED"}, []string{"MODIFIED"}
	updated := "UPDATED"
	var testCases = []struct {
		name       string
		bug        bugzilla.Bug
		dependents []bugzilla.Bug
		options    plugins.BugzillaBranchOptions
		valid      bool
		why        []string
	}{
		{
			name:    "no requirements means a valid bug",
			bug:     bugzilla.Bug{},
			options: plugins.BugzillaBranchOptions{},
			valid:   true,
		},
		{
			name:    "matching open requirement means a valid bug",
			bug:     bugzilla.Bug{IsOpen: true},
			options: plugins.BugzillaBranchOptions{IsOpen: &open},
			valid:   true,
		},
		{
			name:    "matching closed requirement means a valid bug",
			bug:     bugzilla.Bug{IsOpen: false},
			options: plugins.BugzillaBranchOptions{IsOpen: &closed},
			valid:   true,
		},
		{
			name:    "not matching open requirement means an invalid bug",
			bug:     bugzilla.Bug{IsOpen: false},
			options: plugins.BugzillaBranchOptions{IsOpen: &open},
			valid:   false,
			why:     []string{"expected the bug to be open, but it isn't"},
		},
		{
			name:    "not matching closed requirement means an invalid bug",
			bug:     bugzilla.Bug{IsOpen: true},
			options: plugins.BugzillaBranchOptions{IsOpen: &closed},
			valid:   false,
			why:     []string{"expected the bug to not be open, but it is"},
		},
		{
			name:    "matching target release requirement means a valid bug",
			bug:     bugzilla.Bug{TargetRelease: []string{"v1"}},
			options: plugins.BugzillaBranchOptions{TargetRelease: &one},
			valid:   true,
		},
		{
			name:    "not matching target release requirement means an invalid bug",
			bug:     bugzilla.Bug{TargetRelease: []string{"v2"}},
			options: plugins.BugzillaBranchOptions{TargetRelease: &one},
			valid:   false,
			why:     []string{"expected the bug to target the \"v1\" release, but it targets \"v2\" instead"},
		},
		{
			name:    "not setting target release requirement means an invalid bug",
			bug:     bugzilla.Bug{},
			options: plugins.BugzillaBranchOptions{TargetRelease: &one},
			valid:   false,
			why:     []string{"expected the bug to target the \"v1\" release, but no target release was set"},
		},
		{
			name:    "matching status requirement means a valid bug",
			bug:     bugzilla.Bug{Status: "MODIFIED"},
			options: plugins.BugzillaBranchOptions{Statuses: &modified},
			valid:   true,
		},
		{
			name:    "matching status requirement by being in the migrated state means a valid bug",
			bug:     bugzilla.Bug{Status: "UPDATED"},
			options: plugins.BugzillaBranchOptions{Statuses: &modified, StatusAfterValidation: &updated},
			valid:   true,
		},
		{
			name:    "not matching status requirement means an invalid bug",
			bug:     bugzilla.Bug{Status: "MODIFIED"},
			options: plugins.BugzillaBranchOptions{Statuses: &verified},
			valid:   false,
			why:     []string{"expected the bug to be in one of the following states: VERIFIED, but it is MODIFIED instead"},
		},
		{
			name:    "dependent status requirement with no dependent bugs means a valid bug",
			bug:     bugzilla.Bug{DependsOn: []int{}},
			options: plugins.BugzillaBranchOptions{DependentBugStatuses: &verified},
			valid:   true,
		},
		{
			name:       "not matching dependent bug status requirement means an invalid bug",
			bug:        bugzilla.Bug{DependsOn: []int{1}},
			dependents: []bugzilla.Bug{{ID: 1, Status: "MODIFIED"}},
			options:    plugins.BugzillaBranchOptions{DependentBugStatuses: &verified},
			valid:      false,
			why:        []string{"expected dependent [Bugzilla bug](bugzilla.com/show_bug.cgi?id=1) to be in one of the following states: VERIFIED, but it is MODIFIED instead"},
		},
		{
			name:       "matching all requirements means a valid bug",
			bug:        bugzilla.Bug{IsOpen: false, TargetRelease: []string{"v1"}, Status: "MODIFIED", DependsOn: []int{1}},
			dependents: []bugzilla.Bug{{ID: 1, Status: "MODIFIED"}},
			options:    plugins.BugzillaBranchOptions{IsOpen: &closed, TargetRelease: &one, Statuses: &modified, DependentBugStatuses: &modified},
			valid:      true,
		},
		{
			name:       "matching no requirements means an invalid bug",
			bug:        bugzilla.Bug{IsOpen: false, TargetRelease: []string{"v1"}, Status: "MODIFIED", DependsOn: []int{1}},
			dependents: []bugzilla.Bug{{ID: 1, Status: "MODIFIED"}},
			options:    plugins.BugzillaBranchOptions{IsOpen: &open, TargetRelease: &two, Statuses: &verified, DependentBugStatuses: &verified},
			valid:      false,
			why: []string{
				"expected the bug to be open, but it isn't",
				"expected the bug to target the \"v2\" release, but it targets \"v1\" instead",
				"expected the bug to be in one of the following states: VERIFIED, but it is MODIFIED instead",
				"expected dependent [Bugzilla bug](bugzilla.com/show_bug.cgi?id=1) to be in one of the following states: VERIFIED, but it is MODIFIED instead",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			valid, why := validateBug(testCase.bug, testCase.dependents, testCase.options, "bugzilla.com")
			if valid != testCase.valid {
				t.Errorf("%s: didn't validate bug correctly, expected %t got %t", testCase.name, testCase.valid, valid)
			}
			if !reflect.DeepEqual(why, testCase.why) {
				t.Errorf("%s: didn't get correct reasons why: %v", testCase.name, diff.ObjectReflectDiff(testCase.why, why))
			}
		})
	}
}

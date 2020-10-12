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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/bugzilla"
	prowconfig "k8s.io/test-infra/prow/config"
	cherrypicker "k8s.io/test-infra/prow/external-plugins/cherrypicker/lib"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var allowEvent = cmp.AllowUnexported(event{})

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
        state_after_validation:
          status: "PRE"
      "my-org-branch":
        target_release: my-org-branch-default
        state_after_validation:
          status: "POST"
        add_external_link: true
    repos:
      my-repo:
        branches:
          "*":
            is_open: false
            target_release: my-repo-default
            valid_states:
            - status: VALIDATED
          "my-repo-branch":
            target_release: my-repo-branch
            valid_states:
            - status: MODIFIED
            add_external_link: true
            state_after_merge:
              status: MODIFIED
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

	var config plugins.Bugzilla
	if err := yaml.Unmarshal([]byte(rawConfig), &config); err != nil {
		t.Fatalf("couldn't unmarshal config: %v", err)
	}

	pc := &plugins.Configuration{Bugzilla: config}
	enabledRepos := []prowconfig.OrgRepo{
		{Org: "some-org", Repo: "some-repo"},
		{Org: "my-org", Repo: "some-repo"},
		{Org: "my-org", Repo: "my-repo"},
	}
	help, err := helpProvider(pc, enabledRepos)
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
<li>by default, valid bugs must be open and target the "my-org-default" release. After being linked to a pull request, bugs will be moved to the PRE state.</li>
<li>on the "my-org-branch" branch, valid bugs must be open and target the "my-org-branch-default" release. After being linked to a pull request, bugs will be moved to the POST state and updated to refer to the pull request using the external bug tracker.</li>
</ul>`,
			"my-org/my-repo": `The plugin has the following configuration:<ul>
<li>by default, valid bugs must be closed, target the "my-repo-default" release, and be in one of the following states: VALIDATED. After being linked to a pull request, bugs will be moved to the PRE state.</li>
<li>on the "branch-that-likes-closed-bugs" branch, valid bugs must be closed, target the "my-repo-default" release, be in one of the following states: VERIFIED, CLOSED (ERRATA), depend on at least one other bug, and have all dependent bugs in one of the following states: CLOSED (ERRATA). After being linked to a pull request, bugs will be moved to the CLOSED (VALIDATED) state and moved to the CLOSED (FIXED) state when all linked pull requests are merged.</li>
<li>on the "my-org-branch" branch, valid bugs must be closed, target the "my-repo-default" release, and be in one of the following states: VALIDATED. After being linked to a pull request, bugs will be moved to the POST state and updated to refer to the pull request using the external bug tracker.</li>
<li>on the "my-repo-branch" branch, valid bugs must be closed, target the "my-repo-branch" release, and be in one of the following states: MODIFIED. After being linked to a pull request, bugs will be moved to the PRE state, updated to refer to the pull request using the external bug tracker, and moved to the MODIFIED state when all linked pull requests are merged.</li>
</ul>`,
		},
		Commands: []pluginhelp.Command{
			{
				Usage:       "/bugzilla refresh",
				Description: "Check Bugzilla for a valid bug referenced in the PR title",
				Featured:    false,
				WhoCanUse:   "Anyone",
				Examples:    []string{"/bugzilla refresh"},
			}, {
				Usage:       "/bugzilla assign-qa",
				Description: "(DEPRECATED) Assign PR to QA contact specified in Bugzilla",
				Featured:    false,
				WhoCanUse:   "Anyone",
				Examples:    []string{"/bugzilla assign-qa"},
			}, {
				Usage:       "/bugzilla cc-qa",
				Description: "Request PR review from QA contact specified in Bugzilla",
				Featured:    false,
				WhoCanUse:   "Anyone",
				Examples:    []string{"/bugzilla cc-qa"},
			},
		},
	}

	if actual := help; !reflect.DeepEqual(actual, expected) {
		t.Errorf("resolved incorrect plugin help: %v", cmp.Diff(actual, expected, allowEvent))
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, state: "open", missing: true, opened: true, bugId: 0, body: "fixing a typo", htmlUrl: "http.com", login: "user",
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
					State:   "open",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, state: "open", opened: true, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
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
					Title:   "Bug 123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, merged: true, closed: true, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
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
					Title:   "Bug 123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, merged: false, closed: true, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
			},
		},
		{
			name: "non-bugzilla cherrypick PR sets e.missing to true",
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
				org: "org", repo: "repo", baseRef: "release-4.4", number: 3, opened: true, body: "[release-4.4] fixing a typo", htmlUrl: "http.com", login: "user", cherrypick: true, cherrypickFromPRNum: 2, cherrypickTo: "release-4.4", missing: true,
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
					Title:   "[release-4.4] Bug 123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
					Body: `This is an automated cherry-pick of #2

/assign user`,
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "release-4.4", number: 3, opened: true, body: "[release-4.4] Bug 123: fixed it!", htmlUrl: "http.com", login: "user", cherrypick: true, cherrypickFromPRNum: 2, cherrypickTo: "release-4.4", bugId: 123,
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
					Title:   "[release-4.4] Bug 123: fixed it!",
					HTMLURL: "http.com",
					User: github.User{
						Login: "user",
					},
					Body: `This is an automated cherry-pick of #2

/assign user`,
				},
			},
			expected: &event{
				org: "org", repo: "repo", baseRef: "release-4.4", number: 3, bugId: 123, body: "[release-4.4] Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, opened: true, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, opened: true, missing: true, body: "fixed it!", htmlUrl: "http.com", login: "user",
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
				t.Errorf("%s: did not get correct event: %v", testCase.name, cmp.Diff(actual, expected, allowEvent))
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, missing: true, body: "/bugzilla refresh", htmlUrl: "www.com", login: "user", assign: false, cc: false,
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "/bugzilla refresh", htmlUrl: "www.com", login: "user", assign: false, cc: false,
			},
		},
		{
			name: "title referencing bug in a merged PR gets an event",
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
			title:  "Bug 123: oopsie doopsie",
			merged: true,
			expected: &event{
				org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, merged: true, body: "/bugzilla refresh", htmlUrl: "www.com", login: "user", assign: false, cc: false,
			},
		},
		{
			name: "assign-qa comment event has assign bool set to true",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/bugzilla assign-qa",
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "/bugzilla assign-qa", htmlUrl: "www.com", login: "user", assign: true, cc: false,
			},
		},
		{
			name: "cc-qa comment event has cc bool set to true",
			e: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				IsPR:   true,
				Body:   "/bugzilla cc-qa",
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
				org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "/bugzilla cc-qa", htmlUrl: "www.com", login: "user", assign: false, cc: true,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := fakegithub.FakeClient{
				PullRequests: map[int]*github.PullRequest{
					1: {Base: github.PullRequestBranch{Ref: "branch"}, Title: testCase.title, Merged: testCase.merged},
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
				t.Errorf("%s: did not get correct event: %v", testCase.name, cmp.Diff(actual, expected, allowEvent))
			}

			checkComments(client, testCase.name, testCase.expectedComment, t)
		})
	}
}

func TestHandle(t *testing.T) {
	yes := true
	open := true
	v1 := "v1"
	v2 := "v2"
	updated := plugins.BugzillaBugState{Status: "UPDATED"}
	modified := plugins.BugzillaBugState{Status: "MODIFIED"}
	verified := []plugins.BugzillaBugState{{Status: "VERIFIED"}}
	base := &event{
		org: "org", repo: "repo", baseRef: "branch", number: 1, bugId: 123, body: "Bug 123: fixed it!", htmlUrl: "http.com", login: "user",
	}
	var testCases = []struct {
		name                string
		labels              []string
		missing             bool
		merged              bool
		closed              bool
		opened              bool
		cherryPick          bool
		cherryPickFromPRNum int
		cherryPickTo        string
		// the "e.body" for PRs is the PR title; this field can be used to replace the "body" for PR handles for cases where the body != description
		body                  string
		externalBugs          []bugzilla.ExternalBug
		prs                   []github.PullRequest
		bugs                  []bugzilla.Bug
		bugComments           map[int][]bugzilla.Comment
		bugErrors             []int
		bugCreateErrors       []string
		subComponents         map[int]map[string][]string
		options               plugins.BugzillaBranchOptions
		expectedLabels        []string
		expectedComment       string
		expectedBug           *bugzilla.Bug
		expectedExternalBugs  []bugzilla.ExternalBug
		expectedSubComponents map[int]map[string][]string
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
			expectedComment: `org/repo#1:@user: An error was encountered searching for bug 123 on the Bugzilla server at www.bugzilla:
> injected error getting bug
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "valid bug removes invalid label, adds valid/severity labels and comments",
			bugs:           []bugzilla.Bug{{ID: 123, Severity: "urgent"}},
			options:        plugins.BugzillaBranchOptions{}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug", "bugzilla/severity-urgent"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:           "invalid bug adds invalid label, removes valid label and comments",
			bugs:           []bugzilla.Bug{{ID: 123, Severity: "high"}},
			options:        plugins.BugzillaBranchOptions{IsOpen: &open},
			labels:         []string{"bugzilla/valid-bug", "bugzilla/severity-urgent"},
			expectedLabels: []string{"bugzilla/invalid-bug", "bugzilla/severity-high"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is invalid:
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
			bugs:           []bugzilla.Bug{{ID: 123, Severity: "medium"}},
			options:        plugins.BugzillaBranchOptions{StateAfterValidation: &updated}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug", "bugzilla/severity-medium"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid. The bug has been moved to the UPDATED state.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "UPDATED", Severity: "medium"},
		},
		{
			name:           "valid bug with status update removes invalid label, adds valid label, comments and updates status with resolution",
			bugs:           []bugzilla.Bug{{ID: 123, Status: "MODIFIED", Severity: "low"}},
			options:        plugins.BugzillaBranchOptions{StateAfterValidation: &plugins.BugzillaBugState{Status: "CLOSED", Resolution: "VALIDATED"}}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug", "bugzilla/severity-low"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid. The bug has been moved to the CLOSED (VALIDATED) state.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "CLOSED", Resolution: "VALIDATED", Severity: "low"},
		},
		{
			name:           "valid bug with status update removes invalid label, adds valid label, comments and does not update status when it is already correct",
			bugs:           []bugzilla.Bug{{ID: 123, Status: "UPDATED", Severity: "unspecified"}},
			options:        plugins.BugzillaBranchOptions{StateAfterValidation: &updated}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug", "bugzilla/severity-unspecified"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "UPDATED", Severity: "unspecified"},
		},
		{
			name:           "valid bug with external link removes invalid label, adds valid label, comments, makes an external bug link",
			bugs:           []bugzilla.Bug{{ID: 123}},
			options:        plugins.BugzillaBranchOptions{AddExternalLink: &yes}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid. The bug has been updated to refer to the pull request using the external bug tracker.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1"}},
		},
		{
			name: "valid bug with already existing external link removes invalid label, adds valid label, comments to say nothing changed",
			bugs: []bugzilla.Bug{{ID: 123}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
			}},
			options:        plugins.BugzillaBranchOptions{AddExternalLink: &yes}, // no requirements --> always valid
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

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
			options:   plugins.BugzillaBranchOptions{DependentBugStates: &verified},
			expectedComment: `org/repo#1:@user: An error was encountered searching for dependent bug 124 for bug 123 on the Bugzilla server at www.bugzilla:
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
			bugs:           []bugzilla.Bug{{IsOpen: true, ID: 123, DependsOn: []int{124}, TargetRelease: []string{v1}}, {ID: 124, Status: "VERIFIED", TargetRelease: []string{v2}}},
			options:        plugins.BugzillaBranchOptions{IsOpen: &yes, TargetRelease: &v1, DependentBugStates: &verified, DependentBugTargetReleases: &[]string{v2}},
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid.

<details><summary>5 validation(s) were run on this bug</summary>

* bug is open, matching expected state (open)
* bug target release (v1) matches configured target release for branch (v1)
* dependent bug [Bugzilla bug 124](www.bugzilla/show_bug.cgi?id=124) is in the state VERIFIED, which is one of the valid states (VERIFIED)
* dependent [Bugzilla bug 124](www.bugzilla/show_bug.cgi?id=124) targets the "v2" release, which is one of the valid target releases: v2
* bug has dependents</details>

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:   "valid bug on merged PR with one external link migrates to new state with resolution and comments",
			merged: true,
			bugs:   []bugzilla.Bug{{ID: 123, Status: "MODIFIED"}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.BugzillaBranchOptions{StateAfterMerge: &plugins.BugzillaBugState{Status: "CLOSED", Resolution: "MERGED"}}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: All pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)

[Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) has been moved to the CLOSED (MERGED) state.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123, Status: "CLOSED", Resolution: "MERGED"},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}},
		},
		{
			name:   "valid bug on merged PR with one external link migrates to new state and comments",
			merged: true,
			bugs:   []bugzilla.Bug{{ID: 123}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.BugzillaBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: All pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)

[Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) has been moved to the MODIFIED state.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123, Status: "MODIFIED"},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}},
		},
		{
			name:   "valid bug on merged PR with many external links migrates to new state and comments",
			merged: true,
			bugs:   []bugzilla.Bug{{ID: 123}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}, {
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/22", base.org, base.repo),
				Org:           base.org, Repo: base.repo, Num: 22,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}, {Number: 22, Merged: true}},
			options: plugins.BugzillaBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: All pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)
 * [org/repo#22](https://github.com/org/repo/pull/22)

[Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) has been moved to the MODIFIED state.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "MODIFIED"},
			expectedExternalBugs: []bugzilla.ExternalBug{
				{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1},
				{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/22", Org: "org", Repo: "repo", Num: 22},
			},
		},
		{
			name:   "valid bug on merged PR with unmerged external links does nothing",
			merged: true,
			bugs:   []bugzilla.Bug{{ID: 123}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}, {
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/22", base.org, base.repo),
				Org:           base.org, Repo: base.repo, Num: 22,
			}},
			prs:         []github.PullRequest{{Number: base.number, Merged: true}, {Number: 22, Merged: false, State: "open"}},
			options:     plugins.BugzillaBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedBug: &bugzilla.Bug{ID: 123},
			expectedExternalBugs: []bugzilla.ExternalBug{
				{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1},
				{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/22", Org: "org", Repo: "repo", Num: 22},
			},
			expectedComment: `org/repo#1:@user: Some pull requests linked via external trackers have merged:
 * [org/repo#1](https://github.com/org/repo/pull/1)

The following pull requests linked via external trackers have not merged:
 * [org/repo#22](https://github.com/org/repo/pull/22) is open

These pull request must merge or be unlinked from the Bugzilla bug in order for it to move to the next state.

[Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) has not been moved to the MODIFIED state.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:   "valid bug on merged PR with one external link but no status after merge configured does nothing",
			merged: true,
			bugs:   []bugzilla.Bug{{ID: 123}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:                  []github.PullRequest{{Number: base.number, Merged: true}},
			options:              plugins.BugzillaBranchOptions{}, // no requirements --> always valid
			expectedBug:          &bugzilla.Bug{ID: 123},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}},
		},
		{
			name:    "valid bug on merged PR with one external link but no referenced bug in the title does nothing",
			merged:  true,
			missing: true,
			bugs:    []bugzilla.Bug{{ID: 123}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:                  []github.PullRequest{{Number: base.number, Merged: true}},
			options:              plugins.BugzillaBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedBug:          &bugzilla.Bug{ID: 123},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}},
		},
		{
			name:      "valid bug on merged PR with one external link fails to update bug and comments",
			merged:    true,
			bugs:      []bugzilla.Bug{{ID: 123}},
			bugErrors: []int{123},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.BugzillaBranchOptions{StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: An error was encountered searching for bug 123 on the Bugzilla server at www.bugzilla:
> injected error getting bug
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}},
		},
		{
			name:   "valid bug on merged PR with merged external links but unknown status does not migrate to new state and comments",
			merged: true,
			bugs:   []bugzilla.Bug{{ID: 123, Status: "CLOSED", Severity: "urgent"}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: true}},
			options: plugins.BugzillaBranchOptions{StateAfterValidation: &updated, StateAfterMerge: &modified}, // no requirements --> always valid
			expectedComment: `org/repo#1:@user: [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) is in an unrecognized state (CLOSED) and will not be moved to the MODIFIED state.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123, Status: "CLOSED", Severity: "urgent"},
			expectedExternalBugs: []bugzilla.ExternalBug{{BugzillaBugID: 123, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}},
		},
		{
			name:   "closed PR removes link and comments",
			merged: false,
			closed: true,
			bugs:   []bugzilla.Bug{{ID: 123, Status: "CLOSED", Severity: "urgent"}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: false}},
			options: plugins.BugzillaBranchOptions{AddExternalLink: &yes},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123). The bug has been updated to no longer refer to the pull request using the external bug tracker.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123, Status: "CLOSED", Severity: "urgent"},
			expectedExternalBugs: []bugzilla.ExternalBug{},
		},
		{
			name:        "closed PR without a link does nothing",
			merged:      false,
			closed:      true,
			bugs:        []bugzilla.Bug{{ID: 123, Status: "CLOSED", Severity: "urgent"}},
			prs:         []github.PullRequest{{Number: base.number, Merged: false}},
			options:     plugins.BugzillaBranchOptions{AddExternalLink: &yes},
			expectedBug: &bugzilla.Bug{ID: 123, Status: "CLOSED", Severity: "urgent"},
		},
		{
			name:   "closed PR removes link, changes bug state, and comments",
			merged: false,
			closed: true,
			bugs:   []bugzilla.Bug{{ID: 123, Status: "POST", Severity: "urgent"}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: false}},
			options: plugins.BugzillaBranchOptions{AddExternalLink: &yes, StateAfterClose: &plugins.BugzillaBugState{Status: "NEW"}},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123). The bug has been updated to no longer refer to the pull request using the external bug tracker. All external bug links have been closed. The bug has been moved to the NEW state.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug:          &bugzilla.Bug{ID: 123, Status: "NEW", Severity: "urgent"},
			expectedExternalBugs: []bugzilla.ExternalBug{},
		},
		{
			name:        "closed PR with missing bug does nothing",
			merged:      false,
			closed:      true,
			missing:     true,
			bugs:        []bugzilla.Bug{},
			prs:         []github.PullRequest{{Number: base.number, Merged: false}},
			options:     plugins.BugzillaBranchOptions{AddExternalLink: &yes, StateAfterClose: &plugins.BugzillaBugState{Status: "NEW"}},
			expectedBug: &bugzilla.Bug{},
		},
		{
			name:   "closed PR with multiple exernal links removes link, does not change bug state, and comments",
			merged: false,
			closed: true,
			bugs:   []bugzilla.Bug{{ID: 123, Status: "POST", Severity: "urgent"}},
			externalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, base.number),
				Org:           base.org, Repo: base.repo, Num: base.number,
			}, {
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, 42),
				Org:           base.org, Repo: base.repo, Num: 42,
			}},
			prs:     []github.PullRequest{{Number: base.number, Merged: false}},
			options: plugins.BugzillaBranchOptions{AddExternalLink: &yes, StateAfterClose: &plugins.BugzillaBugState{Status: "NEW"}},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123). The bug has been updated to no longer refer to the pull request using the external bug tracker.

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "POST", Severity: "urgent"},
			expectedExternalBugs: []bugzilla.ExternalBug{{
				BugzillaBugID: base.bugId,
				ExternalBugID: fmt.Sprintf("%s/%s/pull/%d", base.org, base.repo, 42),
				Org:           base.org, Repo: base.repo, Num: 42,
			}},
		},
		{
			name:                "Cherrypick PR results in cloned bug creation",
			bugs:                []bugzilla.Bug{{Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v2"}, ID: 123, Status: "CLOSED", Severity: "urgent"}},
			bugComments:         map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedComment: `org/repo#1:@user: [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) has been cloned as [Bugzilla bug 124](www.bugzilla/show_bug.cgi?id=124). Retitling PR to link against new bug.
/retitle [v1] Bug 124: fixed it!

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v1"}, ID: 124, DependsOn: []int{123}, Severity: "urgent"},
		},
		{
			name:                "parent PR of cherrypick not existing results in error",
			bugs:                []bugzilla.Bug{{Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v2"}, ID: 123, Status: "CLOSED", Severity: "urgent"}},
			bugComments:         map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			prs:                 []github.PullRequest{{Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedComment: `org/repo#1:@user: Error creating a cherry-pick bug in Bugzilla: failed to check the state of cherrypicked pull request at https://github.com/org/repo/pull/1: pull request number 1 does not exist.
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:                "failure to obtain parent bug for cherrypick results in error",
			bugs:                []bugzilla.Bug{{Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v2"}, ID: 123, Status: "CLOSED", Severity: "urgent"}},
			bugComments:         map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			bugErrors:           []int{123},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedComment: `org/repo#1:@user: An error was encountered searching for bug 123 on the Bugzilla server at www.bugzilla:
> injected error getting bug
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:                "failure to clone bug for cherrypick results in error",
			bugs:                []bugzilla.Bug{{Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v2"}, ID: 123, Status: "CLOSED", Severity: "urgent"}},
			bugComments:         map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			bugCreateErrors:     []string{"+++ This bug was initially created as a clone of Bug #123 +++\n\nThis is a bug"},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedComment: `org/repo#1:@user: An error was encountered cloning bug for cherrypick for bug 123 on the Bugzilla server at www.bugzilla:
> injected error creating new bug
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			// Since the clone does an update operation as part of the clone, this error still occurs in the call to `CloneBug`.
			// We cannot easily test the error handling of the target release update call, as that happens after the DependsOn update done during cloning
			name:                "failure to update bug for results in error",
			bugs:                []bugzilla.Bug{{Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v2"}, ID: 123, Status: "CLOSED", Severity: "urgent"}},
			bugComments:         map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			bugErrors:           []int{124},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedComment: `org/repo#1:@user: An error was encountered cloning bug for cherrypick for bug 123 on the Bugzilla server at www.bugzilla:
> injected error updating bug
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name: "If bug clone with correct target version already exists, just retitle PR",
			bugs: []bugzilla.Bug{
				{Summary: "This is a test bug", Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v2"}, ID: 123, Status: "CLOSED", Severity: "urgent", Blocks: []int{124}},
				{Summary: "This is a test bug", Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v1"}, ID: 124, Status: "NEW", Severity: "urgent", DependsOn: []int{123}},
			},
			bugComments:         map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedComment: `org/repo#1:@user: Detected clone of [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) with correct target release. Retitling PR to link to clone:
/retitle [v1] Bug 124: fixed it!

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name: "Clone for different release does not block creation of new clone",
			bugs: []bugzilla.Bug{
				{Summary: "This is a test bug", Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v2"}, ID: 123, Status: "CLOSED", Severity: "urgent", Blocks: []int{124}},
				{Summary: "This is a test bug", Product: "Test", Component: []string{"TestComponent"}, TargetRelease: []string{"v3"}, ID: 124, Status: "NEW", Severity: "urgent", DependsOn: []int{123}},
			},
			bugComments:         map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedComment: `org/repo#1:@user: [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) has been cloned as [Bugzilla bug 125](www.bugzilla/show_bug.cgi?id=125). Retitling PR to link against new bug.
/retitle [v1] Bug 125: fixed it!

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:        "Bug with SubComponents creates bug with correct subcomponents",
			bugs:        []bugzilla.Bug{{Product: "Test", Component: []string{"TestComponent"}, ID: 123, Status: "CLOSED", Severity: "urgent"}},
			bugComments: map[int][]bugzilla.Comment{123: {{BugID: 123, Count: 0, Text: "This is a bug"}}},
			subComponents: map[int]map[string][]string{
				123: {
					"TestComponent": {
						"TestSubComponent",
					},
				},
			},
			prs:                 []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}, {Number: 2, Body: "This is an automated cherry-pick of #1.\n\n/assign user", Title: "[v1] " + base.body}},
			body:                "[v1] " + base.body,
			cherryPick:          true,
			cherryPickFromPRNum: 1,
			cherryPickTo:        "v1",
			options:             plugins.BugzillaBranchOptions{TargetRelease: &v1},
			expectedSubComponents: map[int]map[string][]string{
				123: {
					"TestComponent": {
						"TestSubComponent",
					},
				},
				124: {
					"TestComponent": {
						"TestSubComponent",
					},
				},
			},
			expectedComment: `org/repo#1:@user: [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) has been cloned as [Bugzilla bug 124](www.bugzilla/show_bug.cgi?id=124). Retitling PR to link against new bug.
/retitle [v1] Bug 124: fixed it!

<details>

In response to [this](http.com):

>[v1] Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:    "Bug with non-allowed group is ignored",
			bugs:    []bugzilla.Bug{{ID: 123, Groups: []string{"security"}}},
			options: plugins.BugzillaBranchOptions{AllowedGroups: []string{"internal"}},
			prs:     []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}},
			// there should be no comment returned in this test case
		}, {
			name:           "Bug with non-allowed group on repo with no allowed groups results in comment on /bugzilla refresh",
			bugs:           []bugzilla.Bug{{ID: 123, Groups: []string{"security"}}},
			prs:            []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}},
			body:           "/bugzilla refresh",
			expectedLabels: []string{"bugzilla/valid-bug"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](http.com):

>/bugzilla refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:    "Bug with non-allowed group on repo with different allowed groups results in comment on /bugzilla refresh",
			bugs:    []bugzilla.Bug{{ID: 123, Groups: []string{"security"}}},
			prs:     []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}},
			body:    "/bugzilla refresh",
			options: plugins.BugzillaBranchOptions{AllowedGroups: []string{"internal"}},
			expectedComment: `org/repo#1:@user: [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) is in a bug group that is not in the allowed groups for this repo.
Allowed groups for this repo are:
- internal

<details>

In response to [this](http.com):

>/bugzilla refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:    "Bug with non-allowed group on repo with different allowed groups results in comment on PR creation",
			bugs:    []bugzilla.Bug{{ID: 123, Groups: []string{"security"}}},
			prs:     []github.PullRequest{{Number: base.number, Body: base.body, Title: base.body}},
			body:    "/bugzilla refresh",
			opened:  true,
			options: plugins.BugzillaBranchOptions{AllowedGroups: []string{"internal"}},
			expectedComment: `org/repo#1:@user: [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123) is in a bug group that is not in the allowed groups for this repo.
Allowed groups for this repo are:
- internal

<details>

In response to [this](http.com):

>/bugzilla refresh


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		}, {
			name:           "Bug with allowed group is properly handled",
			bugs:           []bugzilla.Bug{{ID: 123, Severity: "medium", Groups: []string{"security"}}},
			options:        plugins.BugzillaBranchOptions{StateAfterValidation: &updated, AllowedGroups: []string{"security"}},
			labels:         []string{"bugzilla/invalid-bug"},
			expectedLabels: []string{"bugzilla/valid-bug", "bugzilla/severity-medium"},
			expectedComment: `org/repo#1:@user: This pull request references [Bugzilla bug 123](www.bugzilla/show_bug.cgi?id=123), which is valid. The bug has been moved to the UPDATED state.

<details><summary>No validations were run on this bug</summary></details>

<details>

In response to [this](http.com):

>Bug 123: fixed it!


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
			expectedBug: &bugzilla.Bug{ID: 123, Status: "UPDATED", Severity: "medium", Groups: []string{"security"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := *base // copy so parallel tests don't collide
			gc := fakegithub.FakeClient{
				IssueLabelsExisting: []string{},
				IssueComments:       map[int][]github.IssueComment{},
				PullRequests:        map[int]*github.PullRequest{},
			}
			for _, label := range testCase.labels {
				gc.IssueLabelsExisting = append(gc.IssueLabelsExisting, fmt.Sprintf("%s/%s#%d:%s", e.org, e.repo, e.number, label))
			}
			for _, pr := range testCase.prs {
				gc.PullRequests[pr.Number] = &pr
			}
			bc := bugzilla.Fake{
				EndpointString:  "www.bugzilla",
				Bugs:            map[int]bugzilla.Bug{},
				SubComponents:   map[int]map[string][]string{},
				BugComments:     testCase.bugComments,
				BugErrors:       sets.NewInt(),
				BugCreateErrors: sets.NewString(),
				ExternalBugs:    map[int][]bugzilla.ExternalBug{},
			}
			for _, bug := range testCase.bugs {
				bc.Bugs[bug.ID] = bug
			}
			bc.BugErrors.Insert(testCase.bugErrors...)
			bc.BugCreateErrors.Insert(testCase.bugCreateErrors...)
			for _, externalBug := range testCase.externalBugs {
				bc.ExternalBugs[externalBug.BugzillaBugID] = append(bc.ExternalBugs[externalBug.BugzillaBugID], externalBug)
			}
			for id, subComponent := range testCase.subComponents {
				bc.SubComponents[id] = subComponent
			}
			e.missing = testCase.missing
			e.merged = testCase.merged
			e.closed = testCase.closed || testCase.merged
			e.opened = testCase.opened
			e.cherrypick = testCase.cherryPick
			e.cherrypickFromPRNum = testCase.cherryPickFromPRNum
			e.cherrypickTo = testCase.cherryPickTo
			if testCase.body != "" {
				e.body = testCase.body
			}
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
					t.Errorf("%s: got incorrect bug after update: %s", testCase.name, cmp.Diff(actual, expected, allowEvent))
				}
				if actual, expected := bc.ExternalBugs[testCase.expectedBug.ID], testCase.expectedExternalBugs; !reflect.DeepEqual(actual, expected) {
					t.Errorf("%s: got incorrect external bugs after update: %s", testCase.name, cmp.Diff(actual, expected, allowEvent))
				}
			}
			if testCase.expectedSubComponents != nil && !reflect.DeepEqual(bc.SubComponents, testCase.expectedSubComponents) {
				t.Errorf("%s: got incorrect subcomponents after update: %s", testCase.name, cmp.Diff(actual, expected))
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

func TestBugIDFromTitle(t *testing.T) {
	var testCases = []struct {
		title            string
		expectedNum      int
		expectedNotFound bool
	}{
		{
			title:            "no match",
			expectedNum:      0,
			expectedNotFound: true,
		},
		{
			title:       "Bug 12: Canonical",
			expectedNum: 12,
		},
		{
			title:       "bug 12: Lowercase",
			expectedNum: 12,
		},
		{
			title:            "Bug 12 : Space before colon",
			expectedNum:      0,
			expectedNotFound: true,
		},
		{
			title:       "[rebase release-1.0] Bug 12: Prefix",
			expectedNum: 12,
		},
		{
			title:       "Revert: \"Bug 12: Revert default\"",
			expectedNum: 12,
		},
		{
			title:       "Bug 34: Revert: \"Bug 12: Revert default\"",
			expectedNum: 34,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.title, func(t *testing.T) {
			num, notFound, err := bugIDFromTitle(testCase.title)
			if err != nil {
				t.Errorf("%s: Unexpected error: %v", testCase.title, err)
			}
			if num != testCase.expectedNum {
				t.Errorf("%s: unexpected %d != %d", testCase.title, num, testCase.expectedNum)
			}
			if notFound != testCase.expectedNotFound {
				t.Errorf("%s: unexpected %t != %t", testCase.title, notFound, testCase.expectedNotFound)
			}
		})
	}
}

func TestValidateBug(t *testing.T) {
	open, closed := true, false
	one, two := "v1", "v2"
	verified := []plugins.BugzillaBugState{{Status: "VERIFIED"}}
	modified := []plugins.BugzillaBugState{{Status: "MODIFIED"}}
	updated := plugins.BugzillaBugState{Status: "UPDATED"}
	var testCases = []struct {
		name        string
		bug         bugzilla.Bug
		dependents  []bugzilla.Bug
		options     plugins.BugzillaBranchOptions
		valid       bool
		validations []string
		why         []string
	}{
		{
			name:    "no requirements means a valid bug",
			bug:     bugzilla.Bug{},
			options: plugins.BugzillaBranchOptions{},
			valid:   true,
		},
		{
			name:        "matching open requirement means a valid bug",
			bug:         bugzilla.Bug{IsOpen: true},
			options:     plugins.BugzillaBranchOptions{IsOpen: &open},
			valid:       true,
			validations: []string{"bug is open, matching expected state (open)"},
		},
		{
			name:        "matching closed requirement means a valid bug",
			bug:         bugzilla.Bug{IsOpen: false},
			options:     plugins.BugzillaBranchOptions{IsOpen: &closed},
			valid:       true,
			validations: []string{"bug isn't open, matching expected state (not open)"},
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
			name:        "matching target release requirement means a valid bug",
			bug:         bugzilla.Bug{TargetRelease: []string{"v1"}},
			options:     plugins.BugzillaBranchOptions{TargetRelease: &one},
			valid:       true,
			validations: []string{"bug target release (v1) matches configured target release for branch (v1)"},
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
			name:        "matching status requirement means a valid bug",
			bug:         bugzilla.Bug{Status: "MODIFIED"},
			options:     plugins.BugzillaBranchOptions{ValidStates: &modified},
			valid:       true,
			validations: []string{"bug is in the state MODIFIED, which is one of the valid states (MODIFIED)"},
		},
		{
			name:        "matching status requirement by being in the migrated state means a valid bug",
			bug:         bugzilla.Bug{Status: "UPDATED"},
			options:     plugins.BugzillaBranchOptions{ValidStates: &modified, StateAfterValidation: &updated},
			valid:       true,
			validations: []string{"bug is in the state UPDATED, which is one of the valid states (MODIFIED, UPDATED)"},
		},
		{
			name:    "not matching status requirement means an invalid bug",
			bug:     bugzilla.Bug{Status: "MODIFIED"},
			options: plugins.BugzillaBranchOptions{ValidStates: &verified},
			valid:   false,
			why:     []string{"expected the bug to be in one of the following states: VERIFIED, but it is MODIFIED instead"},
		},
		{
			name:    "dependent status requirement with no dependent bugs means a valid bug",
			bug:     bugzilla.Bug{DependsOn: []int{}},
			options: plugins.BugzillaBranchOptions{DependentBugStates: &verified},
			valid:   false,
			why:     []string{"expected [Bugzilla bug 0](bugzilla.com/show_bug.cgi?id=0) to depend on a bug in one of the following states: VERIFIED, but no dependents were found"},
		},
		{
			name:        "not matching dependent bug status requirement means an invalid bug",
			bug:         bugzilla.Bug{DependsOn: []int{1}},
			dependents:  []bugzilla.Bug{{ID: 1, Status: "MODIFIED"}},
			options:     plugins.BugzillaBranchOptions{DependentBugStates: &verified},
			valid:       false,
			validations: []string{"bug has dependents"},
			why:         []string{"expected dependent [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) to be in one of the following states: VERIFIED, but it is MODIFIED instead"},
		},
		{
			name:        "not matching dependent bug target release requirement means an invalid bug",
			bug:         bugzilla.Bug{DependsOn: []int{1}},
			dependents:  []bugzilla.Bug{{ID: 1, TargetRelease: []string{"v2"}}},
			options:     plugins.BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one}},
			valid:       false,
			validations: []string{"bug has dependents"},
			why:         []string{"expected dependent [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) to target a release in v1, but it targets \"v2\" instead"},
		},
		{
			name:        "not having a dependent bug target release means an invalid bug",
			bug:         bugzilla.Bug{DependsOn: []int{1}},
			dependents:  []bugzilla.Bug{{ID: 1, TargetRelease: []string{}}},
			options:     plugins.BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one}},
			valid:       false,
			validations: []string{"bug has dependents"},
			why:         []string{"expected dependent [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) to target a release in v1, but no target release was set"},
		},
		{
			name:       "matching all requirements means a valid bug",
			bug:        bugzilla.Bug{IsOpen: false, TargetRelease: []string{"v1"}, Status: "MODIFIED", DependsOn: []int{1}},
			dependents: []bugzilla.Bug{{ID: 1, Status: "MODIFIED", TargetRelease: []string{"v2"}}},
			options:    plugins.BugzillaBranchOptions{IsOpen: &closed, TargetRelease: &one, ValidStates: &modified, DependentBugStates: &modified, DependentBugTargetReleases: &[]string{two}},
			validations: []string{"bug isn't open, matching expected state (not open)",
				`bug target release (v1) matches configured target release for branch (v1)`,
				"bug is in the state MODIFIED, which is one of the valid states (MODIFIED)",
				"dependent bug [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) is in the state MODIFIED, which is one of the valid states (MODIFIED)",
				`dependent [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) targets the "v2" release, which is one of the valid target releases: v2`,
				"bug has dependents"},
			valid: true,
		},
		{
			name:        "matching no requirements means an invalid bug",
			bug:         bugzilla.Bug{IsOpen: false, TargetRelease: []string{"v1"}, Status: "MODIFIED", DependsOn: []int{1}},
			dependents:  []bugzilla.Bug{{ID: 1, Status: "MODIFIED"}},
			options:     plugins.BugzillaBranchOptions{IsOpen: &open, TargetRelease: &two, ValidStates: &verified, DependentBugStates: &verified},
			valid:       false,
			validations: []string{"bug has dependents"},
			why: []string{
				"expected the bug to be open, but it isn't",
				"expected the bug to target the \"v2\" release, but it targets \"v1\" instead",
				"expected the bug to be in one of the following states: VERIFIED, but it is MODIFIED instead",
				"expected dependent [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) to be in one of the following states: VERIFIED, but it is MODIFIED instead",
			},
		},
		{
			name:        "matching status means a valid bug when resolution is not required",
			bug:         bugzilla.Bug{Status: "CLOSED", Resolution: "LOL_GO_AWAY"},
			options:     plugins.BugzillaBranchOptions{ValidStates: &[]plugins.BugzillaBugState{{Status: "CLOSED"}}},
			valid:       true,
			validations: []string{"bug is in the state CLOSED (LOL_GO_AWAY), which is one of the valid states (CLOSED)"},
		},
		{
			name:    "matching just status means an invalid bug when resolution does not match",
			bug:     bugzilla.Bug{Status: "CLOSED", Resolution: "LOL_GO_AWAY"},
			options: plugins.BugzillaBranchOptions{ValidStates: &[]plugins.BugzillaBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:   false,
			why: []string{
				"expected the bug to be in one of the following states: CLOSED (ERRATA), but it is CLOSED (LOL_GO_AWAY) instead",
			},
		},
		{
			name:        "matching status and resolution means a valid bug when both are required",
			bug:         bugzilla.Bug{Status: "CLOSED", Resolution: "ERRATA"},
			options:     plugins.BugzillaBranchOptions{ValidStates: &[]plugins.BugzillaBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:       true,
			validations: []string{"bug is in the state CLOSED (ERRATA), which is one of the valid states (CLOSED (ERRATA))"},
		},
		{
			name:        "matching resolution means a valid bug when status is not required",
			bug:         bugzilla.Bug{Status: "CLOSED", Resolution: "ERRATA"},
			options:     plugins.BugzillaBranchOptions{ValidStates: &[]plugins.BugzillaBugState{{Resolution: "ERRATA"}}},
			valid:       true,
			validations: []string{"bug is in the state CLOSED (ERRATA), which is one of the valid states (any status with resolution ERRATA)"},
		},
		{
			name:    "matching just resolution means an invalid bug when status does not match",
			bug:     bugzilla.Bug{Status: "CLOSED", Resolution: "ERRATA"},
			options: plugins.BugzillaBranchOptions{ValidStates: &[]plugins.BugzillaBugState{{Status: "RESOLVED", Resolution: "ERRATA"}}},
			valid:   false,
			why: []string{
				"expected the bug to be in one of the following states: RESOLVED (ERRATA), but it is CLOSED (ERRATA) instead",
			},
		},
		{
			name:        "matching status on dependent bug means a valid bug when resolution is not required",
			bug:         bugzilla.Bug{Status: "CLOSED", Resolution: "LOL_GO_AWAY"},
			dependents:  []bugzilla.Bug{{ID: 1, Status: "CLOSED", Resolution: "LOL_GO_AWAY"}},
			options:     plugins.BugzillaBranchOptions{DependentBugStates: &[]plugins.BugzillaBugState{{Status: "CLOSED"}}},
			valid:       true,
			validations: []string{"dependent bug [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) is in the state CLOSED (LOL_GO_AWAY), which is one of the valid states (CLOSED)", "bug has dependents"},
		},
		{
			name:        "matching just status on dependent bug means an invalid bug when resolution does not match",
			bug:         bugzilla.Bug{Status: "CLOSED", Resolution: "LOL_GO_AWAY"},
			dependents:  []bugzilla.Bug{{ID: 1, Status: "CLOSED", Resolution: "LOL_GO_AWAY"}},
			options:     plugins.BugzillaBranchOptions{DependentBugStates: &[]plugins.BugzillaBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:       false,
			validations: []string{"bug has dependents"},
			why: []string{
				"expected dependent [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) to be in one of the following states: CLOSED (ERRATA), but it is CLOSED (LOL_GO_AWAY) instead",
			},
		},
		{
			name:        "matching status and resolution on dependent bug means a valid bug when both are required",
			bug:         bugzilla.Bug{Status: "CLOSED", Resolution: "ERRATA"},
			dependents:  []bugzilla.Bug{{ID: 1, Status: "CLOSED", Resolution: "ERRATA"}},
			options:     plugins.BugzillaBranchOptions{DependentBugStates: &[]plugins.BugzillaBugState{{Status: "CLOSED", Resolution: "ERRATA"}}},
			valid:       true,
			validations: []string{"dependent bug [Bugzilla bug 1](bugzilla.com/show_bug.cgi?id=1) is in the state CLOSED (ERRATA), which is one of the valid states (CLOSED (ERRATA))", "bug has dependents"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			valid, validations, why := validateBug(testCase.bug, testCase.dependents, testCase.options, "bugzilla.com")
			if valid != testCase.valid {
				t.Errorf("%s: didn't validate bug correctly, expected %t got %t", testCase.name, testCase.valid, valid)
			}
			if !reflect.DeepEqual(validations, testCase.validations) {
				t.Errorf("%s: didn't get correct validations: %v", testCase.name, cmp.Diff(testCase.validations, validations, allowEvent))
			}
			if !reflect.DeepEqual(why, testCase.why) {
				t.Errorf("%s: didn't get correct reasons why: %v", testCase.name, cmp.Diff(testCase.why, why, allowEvent))
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
			expected: "No GitHub users were found matching the public email listed for the QA contact in Bugzilla (qa_tester@example.com), skipping review request.",
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
			expected: "Multiple GitHub users were found matching the public email listed for the QA contact in Bugzilla (qa_tester@example.com), skipping review request. List of users with matching email:\n\t- Login1\n\t- Login2",
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

func TestUpdateTitleBugID(t *testing.T) {
	testCases := []struct {
		name     string
		title    string
		expected string
	}{{
		name:     "handle `Bug`",
		title:    "Bug 123: Fix segfault",
		expected: "Bug 124: Fix segfault",
	}, {
		name:     "handle `BUG`",
		title:    "BUG 123: Fix segfault",
		expected: "BUG 124: Fix segfault",
	}, {
		name:     "handle `[release-4.5] BUG`",
		title:    "[release-4.5] BUG 123: Fix segfault",
		expected: "[release-4.5] BUG 124: Fix segfault",
	}}
	for _, testCase := range testCases {
		newTitle, err := updateTitleBugID(testCase.title, 123, 124)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", testCase.name, err)
		}
		if newTitle != testCase.expected {
			t.Errorf("%s: Expected `%s`, got `%s`", testCase.name, testCase.expected, newTitle)
		}
	}
}

func TestIsBugAllowed(t *testing.T) {
	testCases := []struct {
		name     string
		bug      *bugzilla.Bug
		groups   []string
		expected bool
	}{
		{
			name:     "no groups configured means always allowed",
			groups:   []string{},
			expected: true,
		},
		{
			name: "all groups matching is allowed",
			bug: &bugzilla.Bug{
				Groups: []string{"whoa", "really", "cool"},
			},
			groups:   []string{"whoa", "really", "cool"},
			expected: true,
		},
		{
			name: "some but not all groups matching is not allowed",
			bug: &bugzilla.Bug{
				Groups: []string{"whoa", "really", "cool"},
			},
			groups:   []string{"whoa", "really"},
			expected: false,
		},
		{
			name: "no groups matching is not allowed",
			bug: &bugzilla.Bug{
				Groups: []string{"whoa", "really", "cool"},
			},
			groups:   []string{"other"},
			expected: false,
		},
	}
	for _, testCase := range testCases {
		if actual, expected := isBugAllowed(testCase.bug, testCase.groups), testCase.expected; actual != expected {
			t.Errorf("%s: isBugAllowed returned %v incorrectly", testCase.name, actual)
		}
	}
}

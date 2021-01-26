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

package retitle

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestHandleGenericComment(t *testing.T) {
	var testCases = []struct {
		name              string
		state             string
		allowClosedIssues bool
		action            github.GenericCommentEventAction
		isPr              bool
		body              string
		trusted           func(string) (bool, error)
		expectedTitle     string
		expectedErr       bool
		expectedComment   string
	}{
		{
			name:              "when closed issues are not allowed, comment on closed issue is ignored",
			state:             "closed",
			allowClosedIssues: false,
			action:            github.GenericCommentActionCreated,
			body:              "/retitle foobar",
		},
		{
			name:              "when closed issues are allowed, trusted user edits PR title on closed issue",
			state:             "closed",
			allowClosedIssues: true,
			action:            github.GenericCommentActionCreated,
			body:              "/retitle foobar",
			isPr:              true,
			trusted: func(user string) (bool, error) {
				return true, nil
			},
			expectedTitle: "foobar",
		},
		{
			name:   "edited comment on open issue is ignored",
			state:  "open",
			action: github.GenericCommentActionEdited,
			body:   "/retitle foobar",
		},
		{
			name:   "new unrelated comment on open issue is ignored",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "whatever else",
		},
		{
			name:   "new comment on open issue returns error when failing to check trusted",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle foobar",
			trusted: func(user string) (bool, error) {
				return false, errors.New("oops")
			},
			expectedErr: true,
		},
		{
			name:   "new comment on open issue comments when user is not trusted",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle foobar",
			trusted: func(user string) (bool, error) {
				return false, nil
			},
			expectedComment: `org/repo#1:@user: Re-titling can only be requested by trusted users, like repository collaborators.

<details>

In response to [this]():

>/retitle foobar


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:   "new comment on open issue comments with no title",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle     ",
			trusted: func(user string) (bool, error) {
				return true, nil
			},
			expectedComment: `org/repo#1:@user: Titles may not be empty.

<details>

In response to [this]():

>/retitle     


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:   "new comment on open issue comments with a title with @mention",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle Add @mention to OWNERS",
			trusted: func(user string) (bool, error) {
				return true, nil
			},
			expectedComment: `org/repo#1:@user: Titles may not contain [keywords](https://help.github.com/articles/closing-issues-using-keywords) which can automatically close issues and at(@) mentions.

<details>

In response to [this]():

>/retitle Add @mention to OWNERS


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:   "new comment on open issue comments with a title with invalid keyword",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle Fixes #9999",
			trusted: func(user string) (bool, error) {
				return true, nil
			},
			expectedComment: `org/repo#1:@user: Titles may not contain [keywords](https://help.github.com/articles/closing-issues-using-keywords) which can automatically close issues and at(@) mentions.

<details>

In response to [this]():

>/retitle Fixes #9999


Instructions for interacting with me using PR comments are available [here](https://git.k8s.io/community/contributors/guide/pull-requests.md).  If you have questions or suggestions related to my behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.
</details>`,
		},
		{
			name:   "trusted user edits PR title",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle foobar",
			isPr:   true,
			trusted: func(user string) (bool, error) {
				return true, nil
			},
			expectedTitle: "foobar",
		},
		{
			name:   "trusted user edits issue title",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle foobar",
			isPr:   false,
			trusted: func(user string) (bool, error) {
				return true, nil
			},
			expectedTitle: "foobar",
		},
		{
			name:   "carriage return is stripped",
			state:  "open",
			action: github.GenericCommentActionCreated,
			body:   "/retitle foobar\r",
			isPr:   false,
			trusted: func(user string) (bool, error) {
				return true, nil
			},
			expectedTitle: "foobar",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gce := github.GenericCommentEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: "org",
					},
					Name: "repo",
				},
				User: github.User{
					Login: "user",
				},
				Number:     1,
				IssueState: testCase.state,
				Action:     testCase.action,
				IsPR:       testCase.isPr,
				Body:       testCase.body,
			}
			gc := fakegithub.FakeClient{
				Issues:        map[int]*github.Issue{1: {Title: "Old"}},
				PullRequests:  map[int]*github.PullRequest{1: {Title: "Old"}},
				IssueComments: map[int][]github.IssueComment{},
			}

			err := handleGenericComment(&gc, testCase.trusted, testCase.allowClosedIssues, logrus.WithField("test-case", testCase.name), gce)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			var actual string
			if testCase.isPr {
				actual = gc.PullRequests[gce.Number].Title
			} else {
				actual = gc.Issues[gce.Number].Title
			}

			if testCase.expectedTitle != "" && actual != testCase.expectedTitle {
				t.Errorf("%s: expected title %q, got %q", testCase.name, testCase.expectedTitle, actual)
			}

			wantedComments := 0
			if testCase.expectedComment != "" {
				wantedComments = 1
			}
			if len(gc.IssueCommentsAdded) != wantedComments {
				t.Errorf("%s: wanted %d comment, got %d: %v", testCase.name, wantedComments, len(gc.IssueCommentsAdded), gc.IssueCommentsAdded)
			}

			if testCase.expectedComment != "" && len(gc.IssueCommentsAdded) == 1 {
				if testCase.expectedComment != gc.IssueCommentsAdded[0] {
					t.Errorf("%s: got incorrect comment: %v", testCase.name, diff.StringDiff(testCase.expectedComment, gc.IssueCommentsAdded[0]))
				}
			}
		})
	}
}

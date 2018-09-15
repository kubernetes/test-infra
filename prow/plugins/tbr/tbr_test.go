/*
Copyright 2016 The Kubernetes Authors.

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

package tbr

import (
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
)

func fakeLabels(fc *fakegithub.FakeClient, org, repo string, number int) []string {
	githubLabels, _ := fc.GetIssueLabels(org, repo, number)
	ret := []string{}
	for _, label := range githubLabels {
		ret = append(ret, label.Name)
	}
	return ret
}

func TestHandleGenericComment(t *testing.T) {
	cases := []struct {
		name        string
		event       github.GenericCommentEvent
		issueLabels []string

		expectLabels     []string
		expectNoComments bool
	}{
		{
			name: "should tbr",
			event: github.GenericCommentEvent{
				Action:      github.GenericCommentActionCreated,
				IssueState:  "open",
				IsPR:        true,
				Body:        "/tbr\n",
				User:        github.User{Login: "author"},
				IssueAuthor: github.User{Login: "author"},
				Number:      5,
				Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				HTMLURL:     "<url>",
			},
			issueLabels:      []string{"approved"},
			expectLabels:     []string{"approved", LGTMLabel, TBRLabel},
			expectNoComments: false,
		},
		{
			name: "not approved",
			event: github.GenericCommentEvent{
				Action:      github.GenericCommentActionCreated,
				IssueState:  "open",
				IsPR:        true,
				Body:        "/tbr\n",
				User:        github.User{Login: "author"},
				IssueAuthor: github.User{Login: "author"},
				Number:      5,
				Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				HTMLURL:     "<url>",
			},
			issueLabels:      []string{},
			expectLabels:     []string{},
			expectNoComments: false,
		},
		{
			name: "not author",
			event: github.GenericCommentEvent{
				Action:      github.GenericCommentActionCreated,
				IssueState:  "open",
				IsPR:        true,
				Body:        "/tbr\n",
				User:        github.User{Login: "not-author"},
				IssueAuthor: github.User{Login: "author"},
				Number:      5,
				Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				HTMLURL:     "<url>",
			},
			issueLabels:      []string{"approved"},
			expectLabels:     []string{"approved"},
			expectNoComments: false,
		},
		{
			name: "not matching",
			event: github.GenericCommentEvent{
				Action:      github.GenericCommentActionCreated,
				IssueState:  "open",
				IsPR:        true,
				Body:        "/tbr-not-matching\n",
				User:        github.User{Login: "author"},
				IssueAuthor: github.User{Login: "author"},
				Number:      5,
				Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				HTMLURL:     "<url>",
			},
			issueLabels:      []string{},
			expectLabels:     []string{},
			expectNoComments: true,
		},
	}
	fpc := &plugins.Configuration{}
	fle := logrus.WithField("plugin", PluginName)
	for _, tc := range cases {
		fgc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		org := tc.event.Repo.Owner.Login
		repo := tc.event.Repo.Name
		number := tc.event.Number
		for _, label := range tc.issueLabels {
			fgc.AddLabel(org, repo, number, label)
		}
		err := handleGenericComment(fgc, fpc, fle, tc.event)
		if err != nil {
			t.Errorf("case: '%s' Unexpected error: %v", tc.name, err)
		}
		nComments := len(fgc.IssueCommentsAdded)
		if nComments > 0 && tc.expectNoComments {
			t.Errorf("case: '%s' Unexpected number of comments: %v != 0", tc.name, nComments)
		} else if nComments == 0 && !tc.expectNoComments {
			t.Errorf("case: '%s' Unexpected number of comments: %v > 0", tc.name, nComments)
		}
		labels := fakeLabels(fgc, org, repo, number)
		// sort so we can compare ...
		sort.Strings(labels)
		sort.Strings(tc.expectLabels)
		if !reflect.DeepEqual(labels, tc.expectLabels) {
			t.Errorf(
				"case: '%s' labels did not match expected: %v != %v",
				tc.name, labels, tc.expectLabels,
			)
		}
	}
}

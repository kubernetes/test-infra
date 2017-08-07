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

package help

import (
	"fmt"
	"sort"
	"testing"

	"github.com/Sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

const (
	fakeRepoOrg  = "fakeOrg"
	fakeRepoName = "fakeName"
	prNumber     = 1
)

func formatLabels(labels ...string) []string {
	r := []string{}
	for _, l := range labels {
		r = append(r, fmt.Sprintf("%s/%s#%d:%s", fakeRepoOrg, fakeRepoName, prNumber, l))
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

func getFakeRepoIssueComment(commentBody string, issueLabels []string, assignees []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		IssueComments: make(map[int][]github.IssueComment),
	}

	startingLabels := []github.Label{}
	for _, label := range issueLabels {
		startingLabels = append(startingLabels, github.Label{Name: label})
	}

	assignedUsers := []github.User{}
	for _, user := range assignees {
		assignedUsers = append(assignedUsers, github.User{Name: user})
	}

	ice := github.IssueCommentEvent{
		Repo: github.Repo{
			Owner: github.User{Login: fakeRepoOrg},
			Name:  fakeRepoName,
		},
		Comment: github.IssueComment{
			Body: commentBody,
			User: github.User{Login: "b"},
		},
		Issue: github.Issue{
			User:        github.User{Login: "a"},
			Assignees:   assignedUsers,
			Number:      prNumber,
			PullRequest: &struct{}{},
			Labels:      startingLabels,
		},
		Action: "created",
	}

	ae := assignEvent{
		action:    ice.Action,
		body:      ice.Comment.Body,
		login:     ice.Comment.User.Login,
		org:       ice.Repo.Owner.Login,
		repo:      ice.Repo.Name,
		url:       ice.Comment.HTMLURL,
		number:    ice.Issue.Number,
		issue:     ice.Issue,
		assignees: ice.Issue.Assignees,
		hasLabel:  func(label string) (bool, error) { return ice.Issue.HasLabel(label), nil },
	}

	return fakeCli, ae
}

func getFakeRepoIssue(commentBody string, issueLabels []string, assignees []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		Issues:        make([]github.Issue, 1),
		IssueComments: make(map[int][]github.IssueComment),
	}

	startingLabels := []github.Label{}
	for _, label := range issueLabels {
		startingLabels = append(startingLabels, github.Label{Name: label})
	}

	assignedUsers := []github.User{}
	for _, user := range assignees {
		assignedUsers = append(assignedUsers, github.User{Name: user})
	}

	ie := github.IssueEvent{
		Repo: github.Repo{
			Owner: github.User{Login: fakeRepoOrg},
			Name:  fakeRepoName,
		},
		Issue: github.Issue{
			User:        github.User{Login: "a"},
			Assignees:   assignedUsers,
			Number:      prNumber,
			PullRequest: &struct{}{},
			Body:        commentBody,
			Labels:      startingLabels,
		},
		Action: "opened",
	}

	ae := assignEvent{
		action:    ie.Action,
		body:      ie.Issue.Body,
		login:     ie.Issue.User.Login,
		org:       ie.Repo.Owner.Login,
		repo:      ie.Repo.Name,
		url:       ie.Issue.HTMLURL,
		number:    ie.Issue.Number,
		issue:     ie.Issue,
		assignees: ie.Issue.Assignees,
		hasLabel:  func(label string) (bool, error) { return ie.Issue.HasLabel(label), nil },
	}

	return fakeCli, ae
}

func TestHelp(t *testing.T) {
	type testCase struct {
		name                  string
		body                  string
		expectedNewLabels     []string
		expectedRemovedLabels []string
		issueLabels           []string
		assignees             []string
	}
	testcases := []testCase{
		{
			name:                  "Irrelevant comment",
			body:                  "irrelelvant",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
			assignees:             []string{},
		},
		{
			name:                  "Want help, no assignee",
			body:                  "/help",
			expectedNewLabels:     formatLabels("help-wanted"),
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
			assignees:             []string{},
		},
		{
			name:                  "Want help, already assigned",
			body:                  "/help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
			assignees:             []string{"Alice"},
		},
		{
			name:                  "Has help label, is now assigned",
			body:                  "irrelelvant",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("help-wanted"),
			issueLabels:           []string{"help-wanted"},
			assignees:             []string{"Alice"},
		},
	}

	fakeRepoFunctions := []func(string, []string, []string) (*fakegithub.FakeClient, assignEvent){
		getFakeRepoIssueComment,
		getFakeRepoIssue,
	}

	for _, tc := range testcases {
		sort.Strings(tc.expectedNewLabels)

		for i := 0; i < len(fakeRepoFunctions); i++ {
			fakeClient, ae := fakeRepoFunctions[i](tc.body, tc.issueLabels, tc.assignees)

			if err := handle(fakeClient, logrus.WithField("plugin", pluginName), ae); err != nil {
				t.Errorf("For case %s, didn't expect error from test: %v", tc.name, err)
				return
			}

			if len(tc.expectedNewLabels) != len(fakeClient.LabelsAdded) {
				t.Errorf("For test %v,\n\tExpected %+v \n\tFound %+v", tc.name, tc.expectedNewLabels, fakeClient.LabelsAdded)
				return
			}

			sort.Strings(fakeClient.LabelsAdded)

			for i := range tc.expectedNewLabels {
				if tc.expectedNewLabels[i] != fakeClient.LabelsAdded[i] {
					t.Errorf("For test %v,\n\tExpected %+v \n\tFound %+v", tc.name, tc.expectedNewLabels, fakeClient.LabelsAdded)
					break
				}
			}

			if len(tc.expectedRemovedLabels) != len(fakeClient.LabelsRemoved) {
				t.Errorf("For test %v,\n\tExpected Removed %+v \n\tFound %+v", tc.name, tc.expectedRemovedLabels, fakeClient.LabelsRemoved)
				return
			}

			for i := range tc.expectedRemovedLabels {
				if tc.expectedRemovedLabels[i] != fakeClient.LabelsRemoved[i] {
					t.Errorf("For test %v,\n\tExpected %+v \n\tFound %+v", tc.name, tc.expectedRemovedLabels, fakeClient.LabelsRemoved)
					break
				}
			}
		}
	}
}

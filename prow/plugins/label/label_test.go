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

package label

import (
	"fmt"
	"testing"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

const (
	fakeRepoOrg  = "fakeOrg"
	fakeRepoName = "fakeName"
	orgMember    = "Alice"
	nonOrgMember = "Bob"
	prNumber     = 1
)

type testCase struct {
	name              string
	body              string
	commenter         string
	expectedNewLabels []string
	repoLabels        []string
	isPr              bool
}

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

func getFakeRepoIssueComment(commentBody, commenter string, repoLabels []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		IssueComments:  make(map[int][]github.IssueComment),
		ExistingLabels: repoLabels,
		OrgMembers:     []string{orgMember},
	}
	startingLabels := []github.Label{}

	ice := github.IssueCommentEvent{
		Repo: github.Repo{
			Owner: github.User{Login: fakeRepoOrg},
			Name:  fakeRepoName,
		},
		Comment: github.IssueComment{
			Body: commentBody,
			User: github.User{Login: commenter},
		},
		Issue: github.Issue{
			User:        github.User{Login: "a"},
			Number:      prNumber,
			PullRequest: &struct{}{},
			Labels:      startingLabels,
		},
		Action: "created",
	}

	ae := assignEvent{
		action:  ice.Action,
		body:    ice.Comment.Body,
		login:   ice.Comment.User.Login,
		org:     ice.Repo.Owner.Login,
		repo:    ice.Repo.Name,
		url:     ice.Comment.HTMLURL,
		number:  ice.Issue.Number,
		issue:   ice.Issue,
		comment: ice.Comment,
	}

	return fakeCli, ae
}

func getFakeRepoIssue(commentBody, commenter string, repoLabels []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		Issues:         make([]github.Issue, 1),
		IssueComments:  make(map[int][]github.IssueComment),
		ExistingLabels: repoLabels,
		OrgMembers:     []string{orgMember},
	}
	startingLabels := []github.Label{}

	ie := github.IssueEvent{
		Repo: github.Repo{
			Owner: github.User{Login: fakeRepoOrg},
			Name:  fakeRepoName,
		},
		Issue: github.Issue{
			User:        github.User{Login: "a"},
			Number:      prNumber,
			PullRequest: &struct{}{},
			Body:        commentBody,
			Labels:      startingLabels,
		},
		Action: "created",
	}

	ae := assignEvent{
		action: ie.Action,
		body:   ie.Issue.Body,
		login:  ie.Issue.User.Login,
		org:    ie.Repo.Owner.Login,
		repo:   ie.Repo.Name,
		url:    ie.Issue.HTMLURL,
		number: ie.Issue.Number,
		issue:  ie.Issue,
		comment: github.IssueComment{
			Body: ie.Issue.Body,
			User: ie.Issue.User,
		},
	}

	return fakeCli, ae
}

func getFakeRepoPullRequest(commentBody, commenter string, repoLabels []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		PullRequests:   make(map[int]*github.PullRequest),
		IssueComments:  make(map[int][]github.IssueComment),
		ExistingLabels: repoLabels,
		OrgMembers:     []string{orgMember},
	}
	startingLabel := github.Label{}

	pre := github.PullRequestEvent{
		PullRequest: github.PullRequest{
			User:   github.User{Login: commenter},
			Number: prNumber,
			Base: github.PullRequestBranch{
				Repo: github.Repo{
					Owner: github.User{Login: fakeRepoOrg},
					Name:  fakeRepoName,
				},
			},
			Body: commentBody,
			Head: github.PullRequestBranch{
				Repo: github.Repo{
					Owner: github.User{Login: fakeRepoOrg},
					Name:  fakeRepoName,
				},
			},
		},
		Label:  startingLabel,
		Number: prNumber,
		Action: "created",
	}

	ae := assignEvent{
		action: pre.Action,
		body:   pre.PullRequest.Body,
		login:  pre.PullRequest.User.Login,
		org:    pre.PullRequest.Base.Repo.Owner.Login,
		repo:   pre.PullRequest.Base.Repo.Name,
		url:    pre.PullRequest.HTMLURL,
		number: pre.Number,
		comment: github.IssueComment{
			Body: pre.PullRequest.Body,
			User: pre.PullRequest.User,
		},
	}

	return fakeCli, ae
}

func examineTestCase(t *testing.T, fakeClient *fakegithub.FakeClient, ae assignEvent, tc testCase) {
	if err := handle(fakeClient, logrus.WithField("plugin", pluginName), ae); err != nil {
		t.Errorf("For case %s, didn't expect error from label test: %v", tc.name, err)
		return
	}

	// pass if expected and actual are both empty arrays
	if len(tc.expectedNewLabels) == 0 && len(fakeClient.LabelsAdded) == 0 {
		return
	}

	if len(tc.expectedNewLabels) != len(fakeClient.LabelsAdded) {
		t.Errorf("For test %v,\n\tExpected %+v \n\tFound %+v", tc.name, tc.expectedNewLabels, fakeClient.LabelsAdded)
		return
	}

	for i := range tc.expectedNewLabels {
		if tc.expectedNewLabels[i] != fakeClient.LabelsAdded[i] {
			t.Errorf("For test %v,\n\tExpected %+v \n\tFound %+v", tc.name, tc.expectedNewLabels, fakeClient.LabelsAdded)
			break
		}
	}
}

func TestLabel(t *testing.T) {
	// "a" is the author, "a", "r1", and "r2" are reviewers.

	testcases := []testCase{
		{
			name:              "Irrelevant comment",
			body:              "irrelelvant",
			expectedNewLabels: []string{},
			repoLabels:        []string{},
			commenter:         orgMember,
		},
		{
			name:              "Empty Area",
			body:              "/area",
			expectedNewLabels: []string{},
			repoLabels:        []string{"area/infra"},
			commenter:         orgMember,
		},
		{
			name:              "Add Single Area Label",
			body:              "/area infra",
			repoLabels:        []string{"area/infra"},
			expectedNewLabels: formatLabels("area/infra"),
			commenter:         orgMember,
		},
		{
			name:              "Add Single Priority Label",
			body:              "/priority critical",
			repoLabels:        []string{"area/infra", "priority/critical"},
			expectedNewLabels: formatLabels("priority/critical"),
			commenter:         orgMember,
		},
		{
			name:              "Add Single Kind Label",
			body:              "/kind bug",
			repoLabels:        []string{"area/infra", "priority/critical", "kind/bug"},
			expectedNewLabels: formatLabels("kind/bug"),
			commenter:         orgMember,
		},
		{
			name:              "Adding Labels is Case Insensitive",
			body:              "/kind BuG",
			repoLabels:        []string{"area/infra", "priority/critical", "kind/bug"},
			expectedNewLabels: formatLabels("kind/bug"),
			commenter:         orgMember,
		},
		{
			name:              "Adding Labels is Case Insensitive",
			body:              "/kind bug",
			repoLabels:        []string{"area/infra", "priority/critical", "kind/BUG"},
			expectedNewLabels: formatLabels("kind/BUG"),
			commenter:         orgMember,
		},
		{
			name:              "Can't Add Non Existent Label",
			body:              "/priority critical",
			repoLabels:        []string{"area/infra"},
			expectedNewLabels: formatLabels(),
			commenter:         orgMember,
		},
		{
			name:              "Non Org Member Can't Add",
			body:              "/area infra",
			repoLabels:        []string{"area/infra", "priority/critical", "kind/bug"},
			expectedNewLabels: formatLabels("area/infra"),
			commenter:         nonOrgMember,
		},
		{
			name:              "Command must start at the beginning of the line",
			body:              "  /area infra",
			repoLabels:        []string{"area/infra", "area/api", "priority/critical", "priority/urgent", "priority/important", "kind/bug"},
			expectedNewLabels: formatLabels(),
			commenter:         orgMember,
		},
		{
			name:              "Can't Add Labels Non Existing Labels",
			body:              "/area lgtm",
			repoLabels:        []string{"area/infra", "area/api", "priority/critical"},
			expectedNewLabels: formatLabels(),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Area Labels",
			body:              "/area api infra",
			repoLabels:        []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			expectedNewLabels: formatLabels("area/api", "area/infra"),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Priority Labels",
			body:              "/priority critical important",
			repoLabels:        []string{"priority/critical", "priority/important"},
			expectedNewLabels: formatLabels("priority/critical", "priority/important"),
			commenter:         orgMember,
		},
		{
			name:              "Label Prefix Must Match Command (Area-Priority Mismatch)",
			body:              "/area urgent",
			repoLabels:        []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			expectedNewLabels: formatLabels(),
			commenter:         orgMember,
		},
		{
			name:              "Label Prefix Must Match Command (Priority-Area Mismatch)",
			body:              "/priority infra",
			repoLabels:        []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			expectedNewLabels: formatLabels(),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Labels (Some Valid)",
			body:              "/area lgtm infra",
			repoLabels:        []string{"area/infra", "area/api"},
			expectedNewLabels: formatLabels("area/infra"),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Types of Labels Different Lines",
			body:              "/priority urgent\n/area infra",
			repoLabels:        []string{"area/infra", "priority/urgent"},
			expectedNewLabels: formatLabels("priority/urgent", "area/infra"),
			commenter:         orgMember,
		},
		{
			name:              "Infer One Sig Label",
			body:              "@sig-node-misc",
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node"},
			expectedNewLabels: formatLabels("sig/node"),
			commenter:         orgMember,
		},
		{
			name:              "Infer Multiple Sig Labels One Line",
			body:              "@sig-node-misc @sig-api-machinery-bugs",
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			expectedNewLabels: formatLabels("sig/node", "sig/api-machinery"),
			commenter:         orgMember,
		},
		{
			name:              "Infer Multiple Sig Labels Different Lines",
			body:              "@sig-node-misc\n@sig-api-machinery-bugs",
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			expectedNewLabels: formatLabels("sig/node", "sig/api-machinery"),
			commenter:         orgMember,
		},
		{
			name:              "Infer Multiple Sig Labels Different Lines With Other Text",
			body:              "Code Comment.  Design Review\n@sig-node-misc\ncc @sig-api-machinery-bugs",
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			expectedNewLabels: formatLabels("sig/node", "sig/api-machinery"),
			commenter:         orgMember,
		},
		{
			name:              "Add Area, Priority Labels and CC a Sig",
			body:              "/area infra\n/priority urgent Design Review\n@sig-node-misc\ncc @sig-api-machinery-bugs",
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			expectedNewLabels: formatLabels("area/infra", "priority/urgent", "sig/node", "sig/api-machinery"),
			commenter:         orgMember,
		},
	}

	for _, tc := range testcases {
		fakeClient, ice := getFakeRepoIssueComment(tc.body, tc.commenter, tc.repoLabels)
		examineTestCase(t, fakeClient, ice, tc)

		fakeClient, ie := getFakeRepoIssue(tc.body, tc.commenter, tc.repoLabels)
		examineTestCase(t, fakeClient, ie, tc)

		fakeClient, pr := getFakeRepoPullRequest(tc.body, tc.commenter, tc.repoLabels)
		examineTestCase(t, fakeClient, pr, tc)
	}
}

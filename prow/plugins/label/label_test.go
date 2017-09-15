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
	"sort"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/slack/fakeslack"
)

const (
	fakeRepoOrg  = "fakeOrg"
	fakeRepoName = "fakeName"
	orgMember    = "Alice"
	nonOrgMember = "Bob"
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

func getFakeRepoIssueComment(commentBody, commenter string, repoLabels, issueLabels []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		IssueComments:  make(map[int][]github.IssueComment),
		ExistingLabels: repoLabels,
		OrgMembers:     []string{orgMember},
	}

	startingLabels := []github.Label{}
	for _, label := range issueLabels {
		startingLabels = append(startingLabels, github.Label{Name: label})
	}

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
	}

	ae := assignEvent{
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

func getFakeRepoIssue(commentBody, creator string, repoLabels, issueLabels []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		Issues:         make([]github.Issue, 1),
		IssueComments:  make(map[int][]github.IssueComment),
		ExistingLabels: repoLabels,
		OrgMembers:     []string{orgMember},
	}

	startingLabels := []github.Label{}
	for _, label := range issueLabels {
		startingLabels = append(startingLabels, github.Label{Name: label})
	}

	ie := github.IssueEvent{
		Repo: github.Repo{
			Owner: github.User{Login: fakeRepoOrg},
			Name:  fakeRepoName,
		},
		Issue: github.Issue{
			User:        github.User{Login: creator},
			Number:      prNumber,
			PullRequest: &struct{}{},
			Body:        commentBody,
			Labels:      startingLabels,
		},
	}

	ae := assignEvent{
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

func getFakeRepoPullRequest(commentBody, commenter string, repoLabels, issueLabels []string) (*fakegithub.FakeClient, assignEvent) {
	fakeCli := &fakegithub.FakeClient{
		PullRequests:   make(map[int]*github.PullRequest),
		IssueComments:  make(map[int][]github.IssueComment),
		ExistingLabels: repoLabels,
		OrgMembers:     []string{orgMember},
	}

	startingLabels := []github.Label{}
	for _, label := range issueLabels {
		startingLabels = append(startingLabels, github.Label{Name: label})
	}

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
		Number: prNumber,
	}

	ae := assignEvent{
		body:   pre.PullRequest.Body,
		login:  pre.PullRequest.User.Login,
		org:    pre.PullRequest.Base.Repo.Owner.Login,
		repo:   pre.PullRequest.Base.Repo.Name,
		url:    pre.PullRequest.HTMLURL,
		number: pre.Number,
		issue: github.Issue{
			User:   pre.PullRequest.User,
			Number: pre.Number,
			Body:   pre.PullRequest.Body,
			Labels: startingLabels,
		},
		comment: github.IssueComment{
			Body: pre.PullRequest.Body,
			User: pre.PullRequest.User,
		},
	}

	return fakeCli, ae
}

func TestLabel(t *testing.T) {
	// "a" is the author, "a", "r1", and "r2" are reviewers.

	type testCase struct {
		name                  string
		body                  string
		commenter             string
		expectedNewLabels     []string
		expectedRemovedLabels []string
		repoLabels            []string
		issueLabels           []string
	}
	testcases := []testCase{
		{
			name:                  "Irrelevant comment",
			body:                  "irrelelvant",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			repoLabels:            []string{},
			issueLabels:           []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Empty Area",
			body:                  "/area",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			repoLabels:            []string{},
			issueLabels:           []string{"area/infra"},
			commenter:             orgMember,
		},
		{
			name:                  "Add Single Area Label",
			body:                  "/area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Status Approved Label",
			body:                  "/status approved-for-milestone ",
			repoLabels:            []string{"status/approved-for-milestone"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("status/approved-for-milestone"),
			expectedRemovedLabels: []string{},
			commenter:             "sig-lead",
		},
		{
			name:                  "Add Status In Progress Label",
			body:                  "/status in-progress",
			repoLabels:            []string{"status/in-progress"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("status/in-progress"),
			expectedRemovedLabels: []string{},
			commenter:             "sig-lead",
		},
		{
			name:                  "Add Status In Review Label",
			body:                  "/status in-review",
			repoLabels:            []string{"status/in-review"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("status/in-review"),
			expectedRemovedLabels: []string{},
			commenter:             "sig-lead",
		},
		{
			name:                  "Non sig lead can't add status/accepted-for-milestone",
			body:                  "/status accepted-for-milestone",
			repoLabels:            []string{"status/accepted-for-milestone"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             "invalidLead",
		},
		{
			name:                  "Non sig lead can't add status/in-review",
			body:                  "/status in-review",
			repoLabels:            []string{"status/in-review"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             "invalidLead",
		},
		{
			name:                  "Invalid Status Label Attempt",
			body:                  "/status not-a-real-status",
			repoLabels:            []string{"status/in-review"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             "sig-lead",
		},
		{
			name:                  "Add Single Area Label when already present on Issue",
			body:                  "/area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Single Priority Label",
			body:                  "/priority critical",
			repoLabels:            []string{"area/infra", "priority/critical"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("priority/critical"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Single Kind Label",
			body:                  "/kind bug",
			repoLabels:            []string{"area/infra", "priority/critical", "kind/bug"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("kind/bug"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Adding Labels is Case Insensitive",
			body:                  "/kind BuG",
			repoLabels:            []string{"area/infra", "priority/critical", "kind/bug"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("kind/bug"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Adding Labels is Case Insensitive",
			body:                  "/kind bug",
			repoLabels:            []string{"area/infra", "priority/critical", "kind/BUG"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("kind/BUG"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Can't Add Non Existent Label",
			body:                  "/priority critical",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Non Org Member Can't Add",
			body:                  "/area infra",
			repoLabels:            []string{"area/infra", "priority/critical", "kind/bug"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             nonOrgMember,
		},
		{
			name:                  "Command must start at the beginning of the line",
			body:                  "  /area infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent", "priority/important", "kind/bug"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Can't Add Labels Non Existing Labels",
			body:                  "/area lgtm",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Multiple Area Labels",
			body:                  "/area api infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("area/api", "area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Multiple Area Labels one already present on Issue",
			body:                  "/area api infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{"area/api"},
			expectedNewLabels:     formatLabels("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Multiple Priority Labels",
			body:                  "/priority critical important",
			repoLabels:            []string{"priority/critical", "priority/important"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("priority/critical", "priority/important"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Label Prefix Must Match Command (Area-Priority Mismatch)",
			body:                  "/area urgent",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Label Prefix Must Match Command (Priority-Area Mismatch)",
			body:                  "/priority infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Multiple Labels (Some Valid)",
			body:                  "/area lgtm infra",
			repoLabels:            []string{"area/infra", "area/api"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Multiple Types of Labels Different Lines",
			body:                  "/priority urgent\n/area infra",
			repoLabels:            []string{"area/infra", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("priority/urgent", "area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Infer One Sig Label",
			body:                  "@kubernetes/sig-node-misc",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Infer Multiple Sig Labels One Line",
			body:                  "@kubernetes/sig-node-misc @kubernetes/sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Infer Multiple Sig Labels Different Lines",
			body:                  "@kubernetes/sig-node-misc\n@kubernetes/sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Infer Multiple Sig Labels Different Lines With Other Text",
			body:                  "Code Comment.  Design Review\n@kubernetes/sig-node-misc\ncc @kubernetes/sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Area, Priority Labels and CC a Sig",
			body:                  "/area infra\n/priority urgent Design Review\n@kubernetes/sig-node-misc\ncc @kubernetes/sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("area/infra", "priority/urgent", "sig/node", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add SIG Label and CC a Sig",
			body:                  "/sig testing\ncc @kubernetes/sig-api-machinery-misc\n",
			repoLabels:            []string{"area/infra", "sig/testing", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/testing", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Remove Area Label when no such Label on Repo",
			body:                  "/remove-area infra",
			repoLabels:            []string{},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Remove Area Label when no such Label on Issue",
			body:                  "/remove-area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Remove Area Label",
			body:                  "/remove-area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("area/infra"),
			commenter:             orgMember,
		},
		{
			name:                  "Remove Kind Label",
			body:                  "/remove-kind api-server",
			repoLabels:            []string{"area/infra", "priority/high", "kind/api-server"},
			issueLabels:           []string{"area/infra", "priority/high", "kind/api-server"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("kind/api-server"),
			commenter:             orgMember,
		},
		{
			name:                  "Remove Priority Label",
			body:                  "/remove-priority high",
			repoLabels:            []string{"area/infra", "priority/high"},
			issueLabels:           []string{"area/infra", "priority/high"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("priority/high"),
			commenter:             orgMember,
		},
		{
			name:                  "Remove SIG Label",
			body:                  "/remove-sig testing",
			repoLabels:            []string{"area/infra", "sig/testing"},
			issueLabels:           []string{"area/infra", "sig/testing"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("sig/testing"),
			commenter:             orgMember,
		},
		{
			name:                  "Remove Multiple Labels",
			body:                  "/remove-priority low high\n/remove-kind api-server\n/remove-area  infra",
			repoLabels:            []string{"area/infra", "priority/high", "priority/low", "kind/api-server"},
			issueLabels:           []string{"area/infra", "priority/high", "priority/low", "kind/api-server"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("priority/low", "priority/high", "kind/api-server", "area/infra"),
			commenter:             orgMember,
		},
		{
			name:                  "Add and Remove Label at the same time",
			body:                  "/remove-area infra\n/area test",
			repoLabels:            []string{"area/infra", "area/test"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     formatLabels("area/test"),
			expectedRemovedLabels: formatLabels("area/infra"),
			commenter:             orgMember,
		},
		{
			name:                  "Add and Remove the same Label",
			body:                  "/remove-area infra\n/area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("area/infra"),
			commenter:             orgMember,
		},
		{
			name:                  "Multiple Add and Delete Labels",
			body:                  "/remove-area ruby\n/remove-kind srv\n/remove-priority l m\n/area go\n/kind cli\n/priority h",
			repoLabels:            []string{"area/go", "area/ruby", "kind/cli", "kind/srv", "priority/h", "priority/m", "priority/l"},
			issueLabels:           []string{"area/ruby", "kind/srv", "priority/l", "priority/m"},
			expectedNewLabels:     formatLabels("area/go", "kind/cli", "priority/h"),
			expectedRemovedLabels: formatLabels("area/ruby", "kind/srv", "priority/l", "priority/m"),
			commenter:             orgMember,
		},
	}

	fakeRepoFunctions := []func(string, string, []string, []string) (*fakegithub.FakeClient, assignEvent){
		getFakeRepoIssueComment,
		getFakeRepoIssue,
		getFakeRepoPullRequest,
	}

	for _, tc := range testcases {
		sort.Strings(tc.expectedNewLabels)

		for i := 0; i < len(fakeRepoFunctions); i++ {
			fakeClient, ae := fakeRepoFunctions[i](tc.body, tc.commenter, tc.repoLabels, tc.issueLabels)
			fakeMilestoneId := 123456
			fakeSlackClient := &fakeslack.FakeClient{
				SentMessages: make(map[string][]string),
			}
			err := handle(fakeClient, logrus.WithField("plugin", pluginName), ae, fakeSlackClient, fakeMilestoneId)
			if err != nil {
				t.Errorf("For case %s, didn't expect error from label test: %v", tc.name, err)
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

//Make sure we are repeating sig mentions for non org members (and only non org members)
func TestRepeat(t *testing.T) {

	type testCase struct {
		name                   string
		body                   string
		commenter              string
		expectedRepeatedLabels []string
		issueLabels            []string
		repoLabels             []string
	}
	testcases := []testCase{
		{
			name: "Dont repeat when org member adds one sig label",
			body: "@kubernetes/sig-node-misc",
			expectedRepeatedLabels: []string{},
			repoLabels:             []string{"area/infra", "priority/urgent", "sig/node"},
			issueLabels:            []string{},
			commenter:              orgMember,
		},
		{
			name: "Repeat when non org adds one sig label",
			body: "@kubernetes/sig-node-misc",
			expectedRepeatedLabels: []string{"@kubernetes/sig-node-misc"},
			repoLabels:             []string{"area/infra", "priority/urgent", "sig/node", "sig/node"},
			issueLabels:            []string{},
			commenter:              nonOrgMember,
		},
		{
			name: "Don't repeat non existent labels",
			body: "@kubernetes/sig-node-misc @kubernetes/sig-api-machinery-bugs",
			expectedRepeatedLabels: []string{},
			repoLabels:             []string{},
			issueLabels:            []string{},
			commenter:              nonOrgMember,
		},
		{
			name: "Dont repeat multiple if org member",
			body: "@kubernetes/sig-node-misc @kubernetes/sig-api-machinery-bugs",
			expectedRepeatedLabels: []string{},
			repoLabels:             []string{"sig/node", "sig/api-machinery"},
			issueLabels:            []string{},
			commenter:              orgMember,
		},
		{
			name: "Repeat multiple valid labels from non org member",
			body: "@kubernetes/sig-node-misc @kubernetes/sig-api-machinery-bugs",
			expectedRepeatedLabels: []string{"@kubernetes/sig-node-misc", "@kubernetes/sig-api-machinery-bugs"},
			repoLabels:             []string{"sig/node", "sig/api-machinery"},
			issueLabels:            []string{},
			commenter:              nonOrgMember,
		},
		{
			name: "Repeat multiple valid labels with a line break from non org member.",
			body: "@kubernetes/sig-node-misc\n@kubernetes/sig-api-machinery-bugs",
			expectedRepeatedLabels: []string{"@kubernetes/sig-node-misc", "@kubernetes/sig-api-machinery-bugs"},
			repoLabels:             []string{"sig/node", "sig/api-machinery"},
			issueLabels:            []string{},
			commenter:              nonOrgMember,
		},
		{
			name: "Repeat Multiple Sig Labels Different Lines With Other Text",
			body: "Code Comment.  Design Review\n@kubernetes/sig-node-misc\ncc @kubernetes/sig-api-machinery-bugs",
			expectedRepeatedLabels: []string{"@kubernetes/sig-node-misc", "@kubernetes/sig-api-machinery-bugs"},
			repoLabels:             []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:            []string{},
			commenter:              nonOrgMember,
		},
		{
			name: "Repeat when multiple label adding commands",
			body: "/area infra\n/priority urgent Design Review\n@kubernetes/sig-node-misc\ncc @kubernetes/sig-api-machinery-bugs",
			expectedRepeatedLabels: []string{"@kubernetes/sig-node-misc", "@kubernetes/sig-api-machinery-bugs"},
			repoLabels:             []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:            []string{},
			commenter:              nonOrgMember,
		},
	}

	//Test that this functionality works on comments, newly opened issues and newly opened PRs
	fakeRepoFunctions := []func(string, string, []string, []string) (*fakegithub.FakeClient, assignEvent){
		getFakeRepoIssueComment,
		getFakeRepoIssue,
		getFakeRepoPullRequest,
	}

	for _, tc := range testcases {
		sort.Strings(tc.expectedRepeatedLabels)

		for i := 0; i < len(fakeRepoFunctions); i++ {
			fakeClient, ae := fakeRepoFunctions[i](tc.body, tc.commenter, tc.repoLabels, tc.issueLabels)
			m := map[string]string{}
			for _, l := range tc.repoLabels {
				m[l] = ""
			}

			member, _ := fakeClient.IsMember(ae.org, ae.login)
			toRepeat := []string{}
			if !member {
				toRepeat = ae.getRepeats(sigMatcher.FindAllStringSubmatch(tc.body, -1), m)
			}

			sort.Strings(toRepeat)
			if len(tc.expectedRepeatedLabels) != len(toRepeat) {
				t.Errorf("For test %v and case %v,\n\tExpected %+v \n\tFound %+v", tc.name, i, tc.expectedRepeatedLabels, toRepeat)
				continue
			}

			for i := range tc.expectedRepeatedLabels {
				if tc.expectedRepeatedLabels[i] != toRepeat[i] {
					t.Errorf("For test %v,\n\tExpected %+v \n\tFound %+v", tc.name, tc.expectedRepeatedLabels, toRepeat)
					break
				}
			}

		}
	}
}

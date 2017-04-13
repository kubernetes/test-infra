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
	"reflect"
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

func getFakeRepo(commentBody, commenter string, repoLabels, issueLabels []string) (*fakegithub.FakeClient, github.IssueCommentEvent) {
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
		Action: "created",
	}
	return fakeCli, ice
}

func TestLabel(t *testing.T) {
	// "a" is the author, "a", "r1", and "r2" are reviewers.

	var testcases = []struct {
		name                  string
		body                  string
		commenter             string
		expectedNewLabels     []string
		expectedRemovedLabels []string
		repoLabels            []string
		issueLabels           []string
		isPr                  bool
	}{
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
			body:                  "@sig-node-misc",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Infer Multiple Sig Labels One Line",
			body:                  "@sig-node-misc @sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Infer Multiple Sig Labels Different Lines",
			body:                  "@sig-node-misc\n@sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Infer Multiple Sig Labels Different Lines With Other Text",
			body:                  "Code Comment.  Design Review\n@sig-node-misc\ncc @sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("sig/node", "sig/api-machinery"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Area, Priority Labels and CC a Sig",
			body:                  "/area infra\n/priority urgent Design Review\n@sig-node-misc\ncc @sig-api-machinery-bugs",
			repoLabels:            []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("area/infra", "priority/urgent", "sig/node", "sig/api-machinery"),
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
			expectedRemovedLabels: []string{},
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

	for _, tc := range testcases {
		fakeClient, ice := getFakeRepo(tc.body, tc.commenter, tc.repoLabels, tc.issueLabels)
		if err := handle(fakeClient, logrus.WithField("plugin", pluginName), ice); err != nil {
			t.Errorf("For case %s, didn't expect error from label test: %v", tc.name, err)
			continue
		}

		if len(tc.expectedNewLabels) > 0 && !reflect.DeepEqual(tc.expectedNewLabels, fakeClient.LabelsAdded) {
			t.Errorf("For test %v,\n\tExpected Added %+v \n\tFound %+v", tc.name, tc.expectedNewLabels, fakeClient.LabelsAdded)
		}

		if len(tc.expectedRemovedLabels) > 0 && !reflect.DeepEqual(tc.expectedRemovedLabels, fakeClient.LabelsRemoved) {
			t.Errorf("For test %v,\n\tExpected Removed %+v \n\tFound %+v", tc.name, tc.expectedRemovedLabels, fakeClient.LabelsRemoved)
		}
	}
}

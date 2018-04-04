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
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

const (
	orgMember    = "Alice"
	nonOrgMember = "Bob"
)

func formatLabels(labels ...string) []string {
	r := []string{}
	for _, l := range labels {
		r = append(r, fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 1, l))
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

func TestLabel(t *testing.T) {
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
			repoLabels:            []string{"area/infra"},
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
			name:                  "Add Single Triage Label",
			body:                  "/triage needs-information",
			repoLabels:            []string{"area/infra", "triage/needs-information"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     formatLabels("triage/needs-information"),
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
			name:                  "Add Multiple Area Labels (Some Valid)",
			body:                  "/area lgtm infra",
			repoLabels:            []string{"area/infra", "area/api"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
		},
		{
			name:                  "Add Multiple Committee Labels (Some Valid)",
			body:                  "/committee steering calamity",
			repoLabels:            []string{"committee/conduct", "committee/steering"},
			issueLabels:           []string{},
			expectedNewLabels:     formatLabels("committee/steering"),
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
			name:                  "Remove Committee Label",
			body:                  "/remove-committee infinite-monkeys",
			repoLabels:            []string{"area/infra", "sig/testing", "committee/infinite-monkeys"},
			issueLabels:           []string{"area/infra", "sig/testing", "committee/infinite-monkeys"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("committee/infinite-monkeys"),
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
			name:                  "Remove Triage Label",
			body:                  "/remove-triage needs-information",
			repoLabels:            []string{"area/infra", "triage/needs-information"},
			issueLabels:           []string{"area/infra", "triage/needs-information"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels("triage/needs-information"),
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

	for _, tc := range testcases {
		sort.Strings(tc.expectedNewLabels)
		fakeClient := &fakegithub.FakeClient{
			Issues:         make([]github.Issue, 1),
			IssueComments:  make(map[int][]github.IssueComment),
			ExistingLabels: tc.repoLabels,
			OrgMembers:     map[string][]string{"org": {orgMember}},
			LabelsAdded:    []string{},
			LabelsRemoved:  []string{},
		}
		// Add initial labels
		for _, label := range tc.issueLabels {
			fakeClient.AddLabel("org", "repo", 1, label)
		}
		e := &github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			Body:   tc.body,
			Number: 1,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			User:   github.User{Login: tc.commenter},
		}
		err := handle(fakeClient, logrus.WithField("plugin", pluginName), e)
		if err != nil {
			t.Errorf("For case %s, didn't expect error from label test: %v", tc.name, err)
			continue
		}

		// Check that all the correct labels (and only the correct labels) were added.
		expectLabels := append(formatLabels(tc.issueLabels...), tc.expectedNewLabels...)
		if expectLabels == nil {
			expectLabels = []string{}
		}
		sort.Strings(expectLabels)
		sort.Strings(fakeClient.LabelsAdded)
		if !reflect.DeepEqual(expectLabels, fakeClient.LabelsAdded) {
			t.Errorf("(%s): Expected the labels %q to be added, but %q were added.", tc.name, expectLabels, fakeClient.LabelsAdded)
		}

		sort.Strings(tc.expectedRemovedLabels)
		sort.Strings(fakeClient.LabelsRemoved)
		if !reflect.DeepEqual(tc.expectedRemovedLabels, fakeClient.LabelsRemoved) {
			t.Errorf("(%s): Expected the labels %q to be removed, but %q were removed.", tc.name, tc.expectedRemovedLabels, fakeClient.LabelsRemoved)
		}
	}
}

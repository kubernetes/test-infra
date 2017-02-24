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

	"k8s.io/kubernetes/pkg/util/sets"
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

var areaLabels = []string{"area/api", "area/infra"}
var priorityLabels = []string{"priority/important", "priority/critical"}

func formatLabels(labels sets.String) sets.String {
	r := sets.NewString()
	for l := range labels {
		r.Insert(fmt.Sprintf("%s/%s#%d:%s", fakeRepoOrg, fakeRepoName, prNumber, l))
	}
	return r
}

func getFakeRepo(commentBody, commenter string, initial_labels []string) (*fakegithub.FakeClient, github.IssueCommentEvent) {
	fc := &fakegithub.FakeClient{
		IssueComments: make(map[int][]github.IssueComment),
		OrgMembers:    []string{orgMember},
	}
	startingLabels := []github.Label{}
	for _, l := range initial_labels {
		startingLabels = append(startingLabels, github.Label{Name: l})
	}

	ice := github.IssueCommentEvent{
		Repo: github.Repo{
			Owner: github.User{Name: fakeRepoOrg},
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
	return fc, ice
}

func TestLabel(t *testing.T) {
	// "a" is the author, "a", "r1", and "r2" are reviewers.
	var testcases = []struct {
		name              string
		body              string
		commenter         string
		expectedNewLabels sets.String
		removedLabels     []string
		initialLabels     []string
		isPr              bool
	}{
		{
			name:              "Irrelevant comment",
			body:              "irrelelvant",
			expectedNewLabels: sets.NewString(),
			commenter:         orgMember,
		},
		{
			name:              "Empty Area",
			body:              "/area",
			expectedNewLabels: formatLabels(sets.NewString()),
			commenter:         orgMember,
		},
		{
			name:              "Add Single Area Label",
			body:              "/area area/infra",
			expectedNewLabels: formatLabels(sets.NewString("area/infra")),
			commenter:         orgMember,
		},
		{
			name:              "Add Single Priority Label",
			body:              "/priority priority/critical",
			expectedNewLabels: formatLabels(sets.NewString("priority/critical")),
			commenter:         orgMember,
		},
		{
			name:              "Non Org Member Can't Add",
			body:              "/area area/infra",
			expectedNewLabels: formatLabels(sets.NewString()),
			commenter:         nonOrgMember,
		},
		{
			name:              "Command must start at the beginning of the line",
			body:              "  /area lgtm",
			expectedNewLabels: formatLabels(sets.NewString()),
			commenter:         orgMember,
		},
		{
			name:              "Can't Add Labels With Wrong Prefixes",
			body:              "/area lgtm",
			expectedNewLabels: formatLabels(sets.NewString()),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Area Labels",
			body:              "/area area/api area/infra",
			expectedNewLabels: formatLabels(sets.NewString("area/api", "area/infra")),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Priority Labels",
			body:              "/priority priority/critical priority/urgent",
			expectedNewLabels: formatLabels(sets.NewString("priority/critical", "priority/urgent")),
			commenter:         orgMember,
		},
		{
			name:              "Label Prefix Must Match Command (Area-Priority Mismatch)",
			body:              "/area priority/urgent",
			expectedNewLabels: formatLabels(sets.NewString()),
			commenter:         orgMember,
		},
		{
			name:              "Label Prefix Must Match Command (Priority-Area Mismatch)",
			body:              "/priority area/infra",
			expectedNewLabels: formatLabels(sets.NewString()),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Labels (Some Valid)",
			body:              "/area lgtm area/infra",
			expectedNewLabels: formatLabels(sets.NewString("area/infra")),
			commenter:         orgMember,
		},
		{
			name:              "Add Multiple Types of Labels Different Lines",
			body:              "/priority priority-urgent\n/area area/infra",
			expectedNewLabels: formatLabels(sets.NewString("priority-urgent", "area/infra")),
			commenter:         orgMember,
		},
	}
	for _, tc := range testcases {
		fc, ice := getFakeRepo(tc.body, tc.commenter, tc.initialLabels)
		if err := handle(fc, ice); err != nil {
			t.Errorf("For case %s, didn't expect error from label test: %v", tc.name, err)
			continue
		}
		if !tc.expectedNewLabels.Equal(sets.NewString(fc.LabelsAdded...)) {
			t.Errorf("For test %v,\n\tExpected %+v \n\tFound %+v", tc.name, tc.expectedNewLabels, sets.NewString(fc.LabelsAdded...))
		}
	}
}

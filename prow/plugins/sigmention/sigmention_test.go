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

package sigmention

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func formatLabels(labels []string) []string {
	r := []string{}
	for _, l := range labels {
		r = append(r, fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 1, l))
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

func TestSigMention(t *testing.T) {
	orgMember := "cjwagner"
	nonOrgMember := "john"
	bot := "k8s-ci-robot"
	type testCase struct {
		name              string
		body              string
		commenter         string
		expectedRepeats   []string
		expectedNewLabels []string
		issueLabels       []string
		repoLabels        []string
		regexp            string
	}
	testcases := []testCase{
		{
			name:              "Dont repeat when org member mentions sig",
			body:              "@kubernetes/sig-node-misc",
			expectedRepeats:   []string{},
			expectedNewLabels: []string{"sig/node"},
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node"},
			issueLabels:       []string{},
			commenter:         orgMember,
		},
		{
			name:              "Dont repeat or label when bot adds mentions sig",
			body:              "@kubernetes/sig-node-misc",
			expectedRepeats:   []string{},
			expectedNewLabels: []string{},
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node"},
			issueLabels:       []string{},
			commenter:         bot,
		},
		{
			name:              "Repeat when non org adds one sig label (sig label already present)",
			body:              "@kubernetes/sig-node-bugs",
			expectedRepeats:   []string{"@kubernetes/sig-node-bugs"},
			expectedNewLabels: []string{"kind/bug"},
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node", "kind/bug"},
			issueLabels:       []string{"sig/node"},
			commenter:         nonOrgMember,
		},
		{
			name:              "Don't repeat non existent labels",
			body:              "@kubernetes/sig-node-misc @kubernetes/sig-api-machinery-bugs",
			expectedRepeats:   []string{},
			expectedNewLabels: []string{},
			repoLabels:        []string{},
			issueLabels:       []string{},
			commenter:         nonOrgMember,
		},
		{
			name:              "Dont repeat multiple if org member (all labels present).",
			body:              "@kubernetes/sig-node-misc @kubernetes/sig-api-machinery-bugs",
			expectedRepeats:   []string{},
			expectedNewLabels: []string{},
			repoLabels:        []string{"sig/node", "sig/api-machinery", "kind/bug"},
			issueLabels:       []string{"sig/node", "sig/api-machinery", "kind/bug"},
			commenter:         orgMember,
		},
		{
			name:              "Repeat multiple valid labels from non org member",
			body:              "@kubernetes/sig-node-misc @kubernetes/sig-api-machinery-bugs",
			expectedRepeats:   []string{"@kubernetes/sig-node-misc", "@kubernetes/sig-api-machinery-bugs"},
			expectedNewLabels: []string{"sig/node", "sig/api-machinery", "kind/bug"},
			repoLabels:        []string{"sig/node", "sig/api-machinery", "kind/bug"},
			issueLabels:       []string{},
			commenter:         nonOrgMember,
		},
		{
			name:              "Repeat multiple valid labels with a line break from non org member.",
			body:              "@kubernetes/sig-node-misc\n@kubernetes/sig-api-machinery-bugs",
			expectedRepeats:   []string{"@kubernetes/sig-node-misc", "@kubernetes/sig-api-machinery-bugs"},
			expectedNewLabels: []string{"sig/node", "sig/api-machinery", "kind/bug"},
			repoLabels:        []string{"sig/node", "sig/api-machinery", "kind/bug"},
			issueLabels:       []string{},
			commenter:         nonOrgMember,
		},
		{
			name:              "Repeat Multiple Sig Labels Different Lines With Other Text",
			body:              "Code Comment.  Design Review\n@kubernetes/sig-node-proposals\ncc @kubernetes/sig-api-machinery-bugs",
			expectedRepeats:   []string{"@kubernetes/sig-node-proposals", "@kubernetes/sig-api-machinery-bugs"},
			expectedNewLabels: []string{"sig/node", "sig/api-machinery", "kind/bug"},
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery", "kind/bug"},
			issueLabels:       []string{},
			commenter:         nonOrgMember,
		},
		{
			name:              "Repeat when multiple label adding commands (sig labels present)",
			body:              "/area infra\n/priority urgent Design Review\n@kubernetes/sig-node-misc\ncc @kubernetes/sig-api-machinery-bugs",
			expectedRepeats:   []string{"@kubernetes/sig-node-misc", "@kubernetes/sig-api-machinery-bugs"},
			expectedNewLabels: []string{"sig/node", "kind/bug"},
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node", "sig/api-machinery", "sig/testing", "kind/bug"},
			issueLabels:       []string{"sig/api-machinery", "sig/testing"},
			commenter:         nonOrgMember,
		},
		{
			name:              "Works for non-specialized teams",
			body:              "@openshift/sig-node",
			expectedRepeats:   []string{},
			expectedNewLabels: []string{"sig/node"},
			repoLabels:        []string{"area/infra", "priority/urgent", "sig/node"},
			issueLabels:       []string{},
			commenter:         orgMember,
			regexp:            `(?m)@openshift/sig-([\w-]*)`,
		},
	}

	for _, tc := range testcases {
		fakeClient := &fakegithub.FakeClient{
			OrgMembers:     map[string][]string{"org": {orgMember, bot}},
			ExistingLabels: tc.repoLabels,
			IssueComments:  make(map[int][]github.IssueComment),
		}
		// Add initial labels to issue.
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

		testRe := tc.regexp
		if testRe == "" {
			testRe = `(?m)@kubernetes/sig-([\w-]*)-(misc|test-failures|bugs|feature-requests|proposals|pr-reviews|api-reviews)`
		}
		re, err := regexp.Compile(testRe)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}

		if err := handle(fakeClient, logrus.WithField("plugin", pluginName), e, re); err != nil {
			t.Errorf("(%s): Unexpected error from handle: %v.", tc.name, err)
			continue
		}

		// Check that all the correct labels (and only the correct labels) were added.
		expectLabels := append(formatLabels(tc.expectedNewLabels), formatLabels(tc.issueLabels)...)
		sort.Strings(expectLabels)
		sort.Strings(fakeClient.LabelsAdded)
		if !reflect.DeepEqual(expectLabels, fakeClient.LabelsAdded) {
			t.Errorf("(%s): Expected issue to end with labels %q, but ended with %q.", tc.name, expectLabels, fakeClient.LabelsAdded)
		}

		// Check that the comment contains the correct sig mentions repeats if it exists.
		comments := fakeClient.IssueComments[1]
		if len(tc.expectedRepeats) == 0 {
			if len(comments) > 0 {
				t.Errorf("(%s): No sig mentions should have been repeated, but a comment was still made.", tc.name)
			}
			continue
		}
		if len(comments) != 1 {
			t.Errorf(
				"(%s): Expected sig mentions to be repeated in 1 comment, but %d comments were created!",
				tc.name,
				len(comments),
			)
			continue
		}
		for _, repeat := range tc.expectedRepeats {
			if !strings.Contains(comments[0].Body, repeat) {
				t.Errorf("(%s): Comment body does not contain sig mention %q.", tc.name, repeat)
			}
		}
	}
}

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

package milestonestatus

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
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

func TestMilestoneStatus(t *testing.T) {
	type testCase struct {
		name              string
		body              string
		commenter         string
		expectedNewLabels []string
		shouldComment     bool
	}
	testcases := []testCase{
		{
			name:              "Dont label when non sig-lead user approves",
			body:              "/status approved-for-milestone",
			expectedNewLabels: []string{},
			commenter:         "sig-follow",
			shouldComment:     true,
		},
		{
			name:              "Dont label when non sig-lead user marks in progress",
			body:              "/status in-progress",
			expectedNewLabels: []string{},
			commenter:         "sig-follow",
			shouldComment:     true,
		},
		{
			name:              "Dont label when non sig-lead user marks in review",
			body:              "/status in-review",
			expectedNewLabels: []string{},
			commenter:         "sig-follow",
			shouldComment:     true,
		},
		{
			name:              "Label when sig-lead user approves",
			body:              "/status approved-for-milestone",
			expectedNewLabels: []string{"status/approved-for-milestone"},
			commenter:         "sig-lead",
			shouldComment:     false,
		},
		{
			name:              "Label when sig-lead user marks in progress",
			body:              "/status in-progress",
			expectedNewLabels: []string{"status/in-progress"},
			commenter:         "sig-lead",
			shouldComment:     false,
		},
		{
			name:              "Label when sig-lead user marks in review",
			body:              "/status in-review",
			expectedNewLabels: []string{"status/in-review"},
			commenter:         "sig-lead",
			shouldComment:     false,
		},
		{
			name:              "Dont label when sig-lead user marks invalid status",
			body:              "/status in-valid",
			expectedNewLabels: []string{},
			commenter:         "sig-lead",
			shouldComment:     false,
		},
		{
			name:              "Dont label when sig-lead user marks empty status",
			body:              "/status ",
			expectedNewLabels: []string{},
			commenter:         "sig-lead",
			shouldComment:     false,
		},
	}

	for _, tc := range testcases {
		fakeClient := &fakegithub.FakeClient{IssueComments: make(map[int][]github.IssueComment)}
		e := &github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			Body:   tc.body,
			Number: 1,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			User:   github.User{Login: tc.commenter},
		}

		if err := handle(fakeClient, logrus.WithField("plugin", pluginName), e, 42); err != nil {
			t.Errorf("(%s): Unexpected error from handle: %v.", tc.name, err)
			continue
		}

		// Check that the correct labels were added.
		expectLabels := formatLabels(tc.expectedNewLabels...)
		sort.Strings(expectLabels)
		sort.Strings(fakeClient.LabelsAdded)
		if !reflect.DeepEqual(expectLabels, fakeClient.LabelsAdded) {
			t.Errorf("(%s): Expected issue to end with labels %q, but ended with %q.", tc.name, expectLabels, fakeClient.LabelsAdded)
		}

		// Check that a comment was left iff one should have been left.
		comments := len(fakeClient.IssueComments[1])
		if tc.shouldComment && comments != 1 {
			t.Errorf("(%s): 1 comment should have been made, but %d comments were made.", tc.name, comments)
		} else if !tc.shouldComment && comments != 0 {
			t.Errorf("(%s): No comment should have been made, but %d comments were made.", tc.name, comments)
		}
	}
}

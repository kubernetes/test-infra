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

package milestone

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
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
		previousMilestone int
		expectedMilestone int
		noRepoMaintainer  bool
	}
	var milestonesMap = map[string]int{"v1.0": 1}
	testcases := []testCase{
		{
			name:              "Update the milestone when a sig-lead uses the command",
			body:              "/milestone v1.0",
			commenter:         "sig-lead",
			previousMilestone: 0,
			expectedMilestone: 1,
		},
		{
			name:              "Don't update the milestone if a sig-lead enters an invalid milestone",
			body:              "/milestone v2.0",
			commenter:         "sig-lead",
			previousMilestone: 0,
			expectedMilestone: 0,
		},
		{
			name:              "Don't update the milestone when a sig-lead uses the command with an invalid milestone",
			body:              "/milestone abc",
			commenter:         "sig-lead",
			previousMilestone: 0,
			expectedMilestone: 0,
		},
		{
			name:              "Don't update the milestone if a sig-follow enters a valid milestone",
			body:              "/milestone v1.0",
			commenter:         "sig-follow",
			previousMilestone: 0,
			expectedMilestone: 0,
		},
		{
			name:              "Clear the milestone if a sig lead tries to clear",
			body:              "/milestone clear",
			commenter:         "sig-lead",
			previousMilestone: 1,
			expectedMilestone: 0,
		},
		{
			name:              "Don't clear the milestone if a sig follow tries to clear",
			body:              "/milestone clear",
			commenter:         "sig-follow",
			previousMilestone: 10,
			expectedMilestone: 10,
		},
		{
			name:              "Multiline comment",
			body:              "Foo\n/milestone v1.0\r\n/priority critical-urgent",
			commenter:         "sig-lead",
			previousMilestone: 10,
			expectedMilestone: 1,
		},
		{
			name:              "Use default maintainer team when none is specified",
			body:              "Foo\n/milestone v1.0\r\n/priority critical-urgent",
			commenter:         "default-sig-lead",
			previousMilestone: 10,
			expectedMilestone: 1,
			noRepoMaintainer:  true,
		},
		{
			name:              "Don't use default maintainer team when one is specified",
			body:              "Foo\n/milestone v1.0\r\n/priority critical-urgent",
			commenter:         "default-sig-lead",
			previousMilestone: 10,
			expectedMilestone: 10,
			noRepoMaintainer:  false,
		},
	}

	for _, tc := range testcases {
		fakeClient := fakegithub.NewFakeClient()
		fakeClient.MilestoneMap = milestonesMap
		fakeClient.Milestone = tc.previousMilestone

		maintainersID := 42
		maintainersName := "fake-maintainers-team"
		e := &github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			Body:   tc.body,
			Number: 1,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			User:   github.User{Login: tc.commenter},
		}

		repoMilestone := map[string]plugins.Milestone{"": {MaintainersID: 0, MaintainersTeam: maintainersName}}

		if !tc.noRepoMaintainer {
			repoMilestone["org/repo"] = plugins.Milestone{MaintainersID: maintainersID, MaintainersTeam: maintainersName}
		}

		if err := handle(fakeClient, logrus.WithField("plugin", pluginName), e, repoMilestone); err != nil {
			t.Errorf("(%s): Unexpected error from handle: %v.", tc.name, err)
			continue
		}

		// Check that the milestone was set if it was supposed to be set
		if fakeClient.Milestone != tc.expectedMilestone {
			t.Errorf("Expected the milestone to be updated for the issue for %s.  Expected Milestone %v, Actual Milestone %v.", tc.name, tc.expectedMilestone, fakeClient.Milestone)
		}
	}
}

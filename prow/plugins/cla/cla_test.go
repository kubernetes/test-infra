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

package cla

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestCLALabels(t *testing.T) {
	var testcases = []struct {
		name          string
		context       string
		state         string
		statusSHA     string
		issues        []github.Issue
		pullRequests  []github.PullRequest
		labels        []string
		addedLabels   []string
		removedLabels []string
	}{
		{
			name:          "unrecognized status context has no effect",
			context:       "unknown",
			state:         "success",
			addedLabels:   nil,
			removedLabels: nil,
		},
		{
			name:          "cla/linuxfoundation status pending has no effect",
			context:       "cla/linuxfoundation",
			state:         "pending",
			addedLabels:   nil,
			removedLabels: nil,
		},
		{
			name: "cla/linuxfoundation status success does not add/remove labels " +
				"when not the head commit in a PR",
			context:   "cla/linuxfoundation",
			state:     "success",
			statusSHA: "a",
			issues: []github.Issue{
				{Number: 3, State: "open", Labels: []github.Label{}},
			},
			pullRequests: []github.PullRequest{
				{Number: 3, Head: github.PullRequestBranch{SHA: "b"}},
			},
			addedLabels:   nil,
			removedLabels: nil,
		},
		{
			name: "cla/linuxfoundation status failure does not add/remove labels " +
				"when not the head commit in a PR",
			context:   "cla/linuxfoundation",
			state:     "failure",
			statusSHA: "a",
			issues: []github.Issue{
				{Number: 3, State: "open", Labels: []github.Label{{Name: claYesLabel}}},
			},
			pullRequests: []github.PullRequest{
				{Number: 3, Head: github.PullRequestBranch{SHA: "b"}},
			},
			addedLabels:   nil,
			removedLabels: nil,
		},
		{
			name:      "cla/linuxfoundation status on head commit of PR adds the cla-yes label when its state is \"success\"",
			context:   "cla/linuxfoundation",
			state:     "success",
			statusSHA: "a",
			issues: []github.Issue{
				{Number: 3, State: "open", Labels: []github.Label{}},
			},
			pullRequests: []github.PullRequest{
				{Number: 3, Head: github.PullRequestBranch{SHA: "a"}},
			},
			addedLabels:   []string{fmt.Sprintf("/#3:%s", claYesLabel)},
			removedLabels: nil,
		},
		{
			name:      "cla/linuxfoundation status on head commit of PR does nothing when pending",
			context:   "cla/linuxfoundation",
			state:     "pending",
			statusSHA: "a",
			issues: []github.Issue{
				{Number: 3, State: "open", Labels: []github.Label{}},
			},
			pullRequests: []github.PullRequest{
				{Number: 3, Head: github.PullRequestBranch{SHA: "a"}},
			},
			addedLabels:   nil,
			removedLabels: nil,
		},
		{
			name:      "cla/linuxfoundation status success removes \"cncf-cla: no\" label",
			context:   "cla/linuxfoundation",
			state:     "success",
			statusSHA: "a",
			issues: []github.Issue{
				{Number: 3, State: "open", Labels: []github.Label{{Name: claNoLabel}}},
			},
			pullRequests: []github.PullRequest{
				{Number: 3, Head: github.PullRequestBranch{SHA: "a"}},
			},
			addedLabels:   []string{fmt.Sprintf("/#3:%s", claYesLabel)},
			removedLabels: []string{fmt.Sprintf("/#3:%s", claNoLabel)},
		},
		{
			name:      "cla/linuxfoundation status failure removes \"cncf-cla: yes\" label",
			context:   "cla/linuxfoundation",
			state:     "failure",
			statusSHA: "a",
			issues: []github.Issue{
				{Number: 3, State: "open", Labels: []github.Label{{Name: claYesLabel}}},
			},
			pullRequests: []github.PullRequest{
				{Number: 3, Head: github.PullRequestBranch{SHA: "a"}},
			},
			addedLabels:   []string{fmt.Sprintf("/#3:%s", claNoLabel)},
			removedLabels: []string{fmt.Sprintf("/#3:%s", claYesLabel)},
		},
	}
	for _, tc := range testcases {
		pullRequests := make(map[int]*github.PullRequest)
		for _, pr := range tc.pullRequests {
			pullRequests[pr.Number] = &pr
		}

		fc := &fakegithub.FakeClient{
			PullRequests:  pullRequests,
			Issues:        tc.issues,
			IssueComments: make(map[int][]github.IssueComment),
		}
		se := github.StatusEvent{
			Context: tc.context,
			SHA:     tc.statusSHA,
			State:   tc.state,
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), se); err != nil {
			t.Errorf("For case %s, didn't expect error from cla plugin: %v", tc.name, err)
			continue
		}

		if !reflect.DeepEqual(fc.LabelsAdded, tc.addedLabels) {
			t.Errorf("Expected: %#v, Got %#v in case %s.", tc.addedLabels, fc.LabelsAdded, tc.name)
		}

		if !reflect.DeepEqual(fc.LabelsRemoved, tc.removedLabels) {
			t.Errorf("Expected: %#v, Got %#v in case %s.", tc.removedLabels, fc.LabelsRemoved, tc.name)
		}
	}
}

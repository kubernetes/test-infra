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

package wip

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestWipLabel(t *testing.T) {
	var testcases = []struct {
		name          string
		hasLabel      bool
		needsLabel    bool
		shouldLabel   bool
		shouldUnlabel bool
	}{
		{
			name:          "nothing to do, need nothing",
			hasLabel:      false,
			needsLabel:    false,
			shouldLabel:   false,
			shouldUnlabel: false,
		},
		{
			name:          "needs label and comment",
			hasLabel:      false,
			needsLabel:    true,
			shouldLabel:   true,
			shouldUnlabel: false,
		},
		{
			name:          "unnecessary label should be removed",
			hasLabel:      true,
			needsLabel:    false,
			shouldLabel:   false,
			shouldUnlabel: true,
		},
		{
			name:          "nothing to do, have everything",
			hasLabel:      true,
			needsLabel:    true,
			shouldLabel:   false,
			shouldUnlabel: false,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			PullRequests:  make(map[int]*github.PullRequest),
			IssueComments: make(map[int][]github.IssueComment),
		}
		org, repo, number := "org", "repo", 5
		e := &event{
			org:        org,
			repo:       repo,
			number:     number,
			hasLabel:   tc.hasLabel,
			needsLabel: tc.needsLabel,
		}

		if err := handle(fc, logrus.WithField("plugin", pluginName), e); err != nil {
			t.Errorf("For case %s, didn't expect error from wip: %v", tc.name, err)
			continue
		}

		fakeLabel := fmt.Sprintf("%s/%s#%d:%s", org, repo, number, label)
		if tc.shouldLabel {
			if len(fc.LabelsAdded) != 1 || fc.LabelsAdded[0] != fakeLabel {
				t.Errorf("For case %s: expected to add %q label but instead added: %v", tc.name, label, fc.LabelsAdded)
			}
		} else if len(fc.LabelsAdded) > 0 {
			t.Errorf("For case %s, expected to not add %q label but added: %v", tc.name, label, fc.LabelsAdded)
		}
		if tc.shouldUnlabel {
			if len(fc.LabelsRemoved) != 1 || fc.LabelsRemoved[0] != fakeLabel {
				t.Errorf("For case %s: expected to remove %q label but instead removed: %v", tc.name, label, fc.LabelsRemoved)
			}
		} else if len(fc.LabelsRemoved) > 0 {
			t.Errorf("For case %s, expected to not remove %q label but removed: %v", tc.name, label, fc.LabelsRemoved)
		}
	}
}

func TestHasWipPrefix(t *testing.T) {
	var tests = []struct {
		title    string
		expected bool
	}{
		{
			title:    "dummy title",
			expected: false,
		},
		{
			title:    "WIP dummy title",
			expected: true,
		},
		{
			title:    "WIP: dummy title",
			expected: true,
		},
		{
			title:    "[WIP] dummy title",
			expected: true,
		},
		{
			title:    "(WIP) dummy title",
			expected: true,
		},
		{
			title:    "<WIP> dummy title",
			expected: true,
		},
		{
			title:    "wip dummy title",
			expected: true,
		},
		{
			title:    "[wip] dummy title",
			expected: true,
		},
		{
			title:    "(wip) dummy title",
			expected: true,
		},
		{
			title:    "<wip> dummy title",
			expected: true,
		},
		{
			title:    "Wipe out GCP project before reusing",
			expected: false,
		},
	}

	for _, test := range tests {
		if actual, expected := titleRegex.MatchString(test.title), test.expected; actual != expected {
			t.Errorf("for title %q, got WIP prefix match %v but got %v", test.title, actual, expected)
		}
	}
}

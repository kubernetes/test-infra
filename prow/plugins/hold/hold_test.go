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

package hold

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
)

func TestHandle(t *testing.T) {
	var tests = []struct {
		name          string
		body          string
		hasLabel      bool
		shouldLabel   bool
		shouldUnlabel bool
		isPR          bool
	}{
		{
			name:          "nothing to do",
			body:          "noise",
			hasLabel:      false,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "quoted hold in a reply",
			body:          "> /hold",
			hasLabel:      false,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested hold",
			body:          "/hold",
			hasLabel:      false,
			shouldLabel:   true,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested hold with reason",
			body:          "/hold for further review",
			hasLabel:      false,
			shouldLabel:   true,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested hold, Label already exists",
			body:          "/hold",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested hold with reason, Label already exists",
			body:          "/hold for further review",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested hold cancel",
			body:          "/hold cancel",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: true,
			isPR:          true,
		},
		{
			name:          "requested hold cancel with whitespace",
			body:          "/hold   cancel  ",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: true,
			isPR:          true,
		},
		{
			name:          "requested hold cancel, Label already gone",
			body:          "/hold cancel",
			hasLabel:      false,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested unhold",
			body:          "/unhold",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: true,
			isPR:          true,
		},
		{
			name:          "requested unhold with whitespace",
			body:          "/unhold    ",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: true,
			isPR:          true,
		},
		{
			name:          "requested unhold, Label already gone",
			body:          "/unhold",
			hasLabel:      false,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested hold for issues",
			body:          "/hold",
			hasLabel:      false,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          false,
		},
		{
			name:          "requested unhold for issues",
			body:          "/unhold",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          false,
		},
		{
			name:          "requested remove hold label",
			body:          "/remove-hold",
			hasLabel:      true,
			shouldLabel:   false,
			shouldUnlabel: true,
			isPR:          true,
		},
		{
			name:          "requested remove hold label with whitespaces in between",
			body:          "/remove -    hold",
			hasLabel:      false,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
		{
			name:          "requested remove hold label with no seperating hyphen",
			body:          "/removehold",
			hasLabel:      false,
			shouldLabel:   false,
			shouldUnlabel: false,
			isPR:          true,
		},
	}

	for _, tc := range tests {
		fc := fakegithub.NewFakeClient()
		fc.IssueComments = make(map[int][]github.IssueComment)

		e := &github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			Body:   tc.body,
			Number: 1,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			IsPR:   tc.isPR,
		}
		hasLabel := func(label string, issueLabels []github.Label) bool {
			return tc.hasLabel
		}

		if err := handle(fc, logrus.WithField("plugin", PluginName), e, hasLabel); err != nil {
			t.Errorf("For case %s, didn't expect error from hold: %v", tc.name, err)
			continue
		}

		fakeLabel := fmt.Sprintf("org/repo#1:%s", labels.Hold)
		if tc.shouldLabel {
			if len(fc.IssueLabelsAdded) != 1 || fc.IssueLabelsAdded[0] != fakeLabel {
				t.Errorf("For case %s: expected to add %q Label but instead added: %v", tc.name, labels.Hold, fc.IssueLabelsAdded)
			}
		} else if len(fc.IssueLabelsAdded) > 0 {
			t.Errorf("For case %s, expected to not add %q Label but added: %v", tc.name, labels.Hold, fc.IssueLabelsAdded)
		}
		if tc.shouldUnlabel {
			if len(fc.IssueLabelsRemoved) != 1 || fc.IssueLabelsRemoved[0] != fakeLabel {
				t.Errorf("For case %s: expected to remove %q Label but instead removed: %v", tc.name, labels.Hold, fc.IssueLabelsRemoved)
			}
		} else if len(fc.IssueLabelsRemoved) > 0 {
			t.Errorf("For case %s, expected to not remove %q Label but removed: %v", tc.name, labels.Hold, fc.IssueLabelsRemoved)
		}
	}
}

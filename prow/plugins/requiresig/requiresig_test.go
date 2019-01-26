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

package requiresig

import (
	"regexp"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
)

const (
	helpWanted          = "help-wanted"
	open                = "open"
	sigApps             = "sig/apps"
	committeeSteering   = "committee/steering"
	wgContainerIdentity = "wg/container-identity"
	username            = "Ali"
)

type fakePruner struct{}

func (fp *fakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}

func TestHandle(t *testing.T) {
	tests := []struct {
		name           string
		action         github.IssueEventAction
		isPR           bool
		body           string
		initialLabels  []string
		unrelatedLabel bool
		expectComment  bool
		expectedAdd    string
		expectedRemove string
	}{
		{
			name:          "ignore PRs",
			action:        github.IssueActionLabeled,
			isPR:          true,
			initialLabels: []string{helpWanted},
		},
		{
			name:          "issue closed action",
			action:        github.IssueActionClosed,
			initialLabels: []string{helpWanted},
		},
		{
			name:          "issue has sig/foo label, no needs-sig label",
			action:        github.IssueActionLabeled,
			initialLabels: []string{helpWanted, sigApps},
		},
		{
			name:          "issue has no sig/foo label, no needs-sig label",
			action:        github.IssueActionUnlabeled,
			initialLabels: []string{helpWanted},
			expectComment: true,
			expectedAdd:   labels.NeedsSig,
		},
		{
			name:          "issue has needs-sig label, no sig/foo label",
			action:        github.IssueActionLabeled,
			initialLabels: []string{helpWanted, labels.NeedsSig},
		},
		{
			name:           "issue has both needs-sig label and sig/foo label",
			action:         github.IssueActionLabeled,
			initialLabels:  []string{helpWanted, labels.NeedsSig, sigApps},
			expectedRemove: labels.NeedsSig,
		},
		{
			name:          "issue has committee/foo label, no needs-sig label",
			action:        github.IssueActionLabeled,
			initialLabels: []string{helpWanted, committeeSteering},
		},
		{
			name:           "issue has both needs-sig label and committee/foo label",
			action:         github.IssueActionLabeled,
			initialLabels:  []string{helpWanted, labels.NeedsSig, committeeSteering},
			expectedRemove: labels.NeedsSig,
		},
		{
			name:          "issue has wg/foo label, no needs-sig label",
			action:        github.IssueActionLabeled,
			initialLabels: []string{helpWanted, wgContainerIdentity},
		},
		{
			name:           "issue has both needs-sig label and wg/foo label",
			action:         github.IssueActionLabeled,
			initialLabels:  []string{helpWanted, labels.NeedsSig, wgContainerIdentity},
			expectedRemove: labels.NeedsSig,
		},
		{
			name:          "issue has no sig/foo label, no needs-sig label, body mentions sig",
			action:        github.IssueActionOpened,
			body:          "I am mentioning a sig @kubernetes/sig-testing-misc more stuff.",
			initialLabels: []string{helpWanted},
		},
		{
			name:          "issue has no sig/foo label, no needs-sig label, body uses /sig command",
			action:        github.IssueActionOpened,
			body:          "I am using a sig command.\n/sig testing",
			initialLabels: []string{helpWanted},
		},
		// Ignoring label events for labels other than sig labels prevents the
		// plugin from adding and then removing the needs-sig label when new
		// issues are created and include multiple label commands including a
		// `/sig` command. In this case a label event caused by adding a non-sig
		// label may occur before the `/sig` command is processed and the sig
		// label is added.
		{
			name:           "ignore non-sig label added events",
			action:         github.IssueActionLabeled,
			body:           "I am using a sig command.\n/kind bug\n/sig testing",
			initialLabels:  []string{helpWanted},
			unrelatedLabel: true,
		},
		{
			name:           "ignore non-sig label removed events",
			action:         github.IssueActionUnlabeled,
			body:           "I am using a sig command.\n/kind bug\n/sig testing",
			initialLabels:  []string{helpWanted},
			unrelatedLabel: true,
		},
	}

	mentionRe := regexp.MustCompile(`(?m)@kubernetes/sig-testing-misc`)
	for _, test := range tests {
		fghc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}

		var initLabels []github.Label
		for _, label := range test.initialLabels {
			initLabels = append(initLabels, github.Label{Name: label})
		}
		var pr *struct{}
		if test.isPR {
			pr = &struct{}{}
		}
		ie := &github.IssueEvent{
			Action: test.action,
			Issue: github.Issue{
				Labels:      initLabels,
				Number:      5,
				PullRequest: pr,
				Body:        test.body,
			},
		}
		if test.action == github.IssueActionUnlabeled || test.action == github.IssueActionLabeled {
			if test.unrelatedLabel {
				ie.Label.Name = labels.Bug
			} else {
				ie.Label.Name = "sig/awesome"
			}
		}
		if err := handle(logrus.WithField("plugin", "require-sig"), fghc, &fakePruner{}, ie, mentionRe); err != nil {
			t.Fatalf("[%s] Unexpected error from handle: %v.", test.name, err)
		}

		if got := len(fghc.IssueComments[5]); test.expectComment && got != 1 {
			t.Errorf("[%s] Expected 1 comment to be created but got %d.", test.name, got)
		} else if !test.expectComment && got != 0 {
			t.Errorf("[%s] Expected no comments to be created but got %d.", test.name, got)
		}

		if count := len(fghc.IssueLabelsAdded); test.expectedAdd == "" && count != 0 {
			t.Errorf("[%s] Unexpected labels added: %q.", test.name, fghc.IssueLabelsAdded)
		} else if test.expectedAdd != "" && count == 1 {
			if expected, got := "/#5:"+test.expectedAdd, fghc.IssueLabelsAdded[0]; got != expected {
				t.Errorf("[%s] Expected label %q to be added but got %q.", test.name, expected, got)
			}
		} else if test.expectedAdd != "" && count > 1 {
			t.Errorf("[%s] Expected label \"/#5:%s\" to be added but got %q.", test.name, test.expectedAdd, fghc.IssueLabelsAdded)
		}

		if count := len(fghc.IssueLabelsRemoved); test.expectedRemove == "" && count != 0 {
			t.Errorf("[%s] Unexpected labels removed: %q.", test.name, fghc.IssueLabelsRemoved)
		} else if test.expectedRemove != "" && count == 1 {
			if expected, got := "/#5:"+test.expectedRemove, fghc.IssueLabelsRemoved[0]; got != expected {
				t.Errorf("[%s] Expected label %q to be removed but got %q.", test.name, expected, got)
			}
		} else if test.expectedRemove != "" && count > 1 {
			t.Errorf("[%s] Expected label \"/#5:%s\" to be removed but got %q.", test.name, test.expectedRemove, fghc.IssueLabelsRemoved)
		}
	}
}

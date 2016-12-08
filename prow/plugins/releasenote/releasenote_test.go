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

package releasenote

import (
	"testing"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestReleaseNoteComment(t *testing.T) {
	var testcases = []struct {
		name          string
		action        string
		body          string
		isAuthor      bool
		isReviewer    bool
		currentLabels []string

		deletedLabels []string
		addedLabel    string
		shouldComment bool
	}{
		{
			name:          "unrelated comment",
			action:        "created",
			body:          "oh dear",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},
		},
		{
			name:          "author release-note-none",
			action:        "created",
			isAuthor:      true,
			body:          "/release-note-none",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "reviewer release-note",
			action:        "created",
			isReviewer:    true,
			body:          "/release-note",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded},
			addedLabel:    releaseNote,
		},
		{
			name:          "someone else release-note",
			action:        "created",
			body:          "/release-note",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			shouldComment: true,
		},
		{
			name:          "already has release-note",
			action:        "created",
			body:          "/release-note",
			currentLabels: []string{releaseNote, "other"},
		},
		{
			name:          "delete multiple labels",
			action:        "created",
			isReviewer:    true,
			body:          "/release-note",
			currentLabels: []string{releaseNote, releaseNoteLabelNeeded, releaseNoteNone, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded, releaseNoteNone},
		},
		{
			name:       "no label present",
			action:     "created",
			isReviewer: true,
			body:       "/release-note-none",

			addedLabel: releaseNoteNone,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		ice := github.IssueCommentEvent{
			Action: tc.action,
			Comment: github.IssueComment{
				Body: tc.body,
			},
			Issue: github.Issue{
				User:        github.User{Login: "a"},
				Number:      5,
				State:       "open",
				PullRequest: &struct{}{},
				Assignees:   []github.User{{Login: "r"}},
			},
		}
		if tc.isAuthor {
			ice.Comment.User.Login = "a"
		} else if tc.isReviewer {
			ice.Comment.User.Login = "r"
		}
		for _, l := range tc.currentLabels {
			ice.Issue.Labels = append(ice.Issue.Labels, github.Label{Name: l})
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), ice); err != nil {
			t.Errorf("For case %s, did not expect error: %v", tc.name, err)
		}
		if tc.shouldComment && len(fc.IssueComments[5]) == 0 {
			t.Errorf("For case %s, didn't comment but should have.", tc.name)
		}
		if len(fc.LabelsAdded) > 1 {
			t.Errorf("For case %s, added more than one label: %v", tc.name, fc.LabelsAdded)
		} else if len(fc.LabelsAdded) == 0 && tc.addedLabel != "" {
			t.Errorf("For case %s, should have added %s but didn't.", tc.name, tc.addedLabel)
		} else if len(fc.LabelsAdded) == 1 && fc.LabelsAdded[0] != "/#5:"+tc.addedLabel {
			t.Errorf("For case %s, added wrong label. Got %s, expected %s", tc.name, fc.LabelsAdded[0], tc.addedLabel)
		}
		for _, dl := range tc.deletedLabels {
			deleted := false
			for _, lr := range fc.LabelsRemoved {
				if lr == "/#5:"+dl {
					deleted = true
					break
				}
			}
			if !deleted {
				t.Errorf("For case %s, expected %s label deleted, but it wasn't.", tc.name, dl)
			}
		}
	}
}

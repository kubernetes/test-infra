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
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestReleaseNoteComment(t *testing.T) {
	var testcases = []struct {
		name          string
		action        github.IssueCommentEventAction
		commentBody   string
		issueBody     string
		isMember      bool
		isAuthor      bool
		currentLabels []string

		deletedLabels []string
		addedLabel    string
		shouldComment bool
	}{
		{
			name:          "unrelated comment",
			action:        github.IssueCommentActionCreated,
			commentBody:   "oh dear",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},
		},
		{
			name:          "author release-note-none with missing block",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none with empty block",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			issueBody:     "bologna ```release-note \n ```",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none with \"none\" block",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			issueBody:     "bologna ```release-note \nnone \n ```",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none, has deprecated label",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			currentLabels: []string{releaseNoteLabelNeeded, deprecatedReleaseNoteLabelNeeded, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded, deprecatedReleaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none, trailing space.",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none ",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none, no op.",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			currentLabels: []string{releaseNoteNone, "other"},
		},
		{
			name:          "member release-note",
			action:        github.IssueCommentActionCreated,
			isMember:      true,
			commentBody:   "/release-note",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},

			shouldComment: true,
		},
		{
			name:          "someone else release-note, trailing space.",
			action:        github.IssueCommentActionCreated,
			commentBody:   "/release-note \r",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},
			shouldComment: true,
		},
		{
			name:          "someone else release-note-none",
			action:        github.IssueCommentActionCreated,
			commentBody:   "/release-note-none",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},
			shouldComment: true,
		},
		{
			name:          "author release-note-action-required",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-action-required",
			currentLabels: []string{releaseNoteLabelNeeded, "other"},
			shouldComment: true,
		},
		{
			name:          "release-note-none, delete multiple labels",
			action:        github.IssueCommentActionCreated,
			isMember:      true,
			commentBody:   "/release-note-none",
			currentLabels: []string{releaseNote, releaseNoteLabelNeeded, releaseNoteActionRequired, releaseNoteNone, "other"},

			deletedLabels: []string{releaseNoteLabelNeeded, releaseNoteActionRequired, releaseNote},
		},
		{
			name:        "no label present",
			action:      github.IssueCommentActionCreated,
			isMember:    true,
			commentBody: "/release-note-none",

			addedLabel: releaseNoteNone,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
			OrgMembers:    map[string][]string{"": {"m"}},
		}
		ice := github.IssueCommentEvent{
			Action: tc.action,
			Comment: github.IssueComment{
				Body: tc.commentBody,
			},
			Issue: github.Issue{
				Body:        tc.issueBody,
				User:        github.User{Login: "a"},
				Number:      5,
				State:       "open",
				PullRequest: &struct{}{},
				Assignees:   []github.User{{Login: "r"}},
			},
		}
		if tc.isAuthor {
			ice.Comment.User.Login = "a"
		} else if tc.isMember {
			ice.Comment.User.Login = "m"
		}
		for _, l := range tc.currentLabels {
			ice.Issue.Labels = append(ice.Issue.Labels, github.Label{Name: l})
		}
		if err := handleComment(fc, logrus.WithField("plugin", pluginName), ice); err != nil {
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

		var expectedDeleted []string
		for _, expect := range tc.deletedLabels {
			expectedDeleted = append(expectedDeleted, "/#5:"+expect)
		}
		sort.Strings(expectedDeleted)
		sort.Strings(fc.LabelsRemoved)
		if !reflect.DeepEqual(expectedDeleted, fc.LabelsRemoved) {
			t.Errorf(
				"For case %s, expected %q labels to be deleted, but %q were deleted.",
				tc.name,
				expectedDeleted,
				fc.LabelsRemoved,
			)
		}
	}
}

const lgtmLabel = "lgtm"

func formatLabels(num int, labels ...string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, fmt.Sprintf("org/repo#%d:%s", num, l))
	}
	return out
}

func newFakeClient(body, branch string, initialLabels, comments []string, parentPRs map[int]string) (*fakegithub.FakeClient, *github.PullRequestEvent) {
	labels := formatLabels(1, initialLabels...)
	for parent, l := range parentPRs {
		labels = append(labels, formatLabels(parent, l)...)
	}
	var issueComments []github.IssueComment
	for _, comment := range comments {
		issueComments = append(issueComments, github.IssueComment{Body: comment})
	}
	return &fakegithub.FakeClient{
			IssueComments: map[int][]github.IssueComment{1: issueComments},
			ExistingLabels: []string{
				lgtmLabel,
				releaseNote,
				releaseNoteLabelNeeded,
				releaseNoteNone,
				releaseNoteActionRequired,
			},
			LabelsAdded:   labels,
			LabelsRemoved: []string{},
		},
		&github.PullRequestEvent{
			Action: github.PullRequestActionEdited,
			Number: 1,
			PullRequest: github.PullRequest{
				Base:   github.PullRequestBranch{Ref: branch},
				Number: 1,
				Body:   body,
				User:   github.User{Login: "cjwagner"},
			},
			Repo: github.Repo{
				Owner: github.User{Login: "org"},
				Name:  "repo",
			},
		}
}

func TestReleaseNotePR(t *testing.T) {
	tests := []struct {
		name          string
		initialLabels []string
		body          string
		branch        string // Defaults to master
		parentPRs     map[int]string
		issueComments []string
		labelsAdded   []string
		labelsRemoved []string
	}{
		{
			name:          "LGTM with release-note",
			initialLabels: []string{lgtmLabel, releaseNote},
			body:          "```release-note\n note note note.\n```",
		},
		{
			name:          "LGTM with release-note, arbitrary comment",
			initialLabels: []string{lgtmLabel, releaseNote},
			body:          "```release-note\n note note note.\n```",
			issueComments: []string{"Release notes are great fun."},
		},
		{
			name:          "LGTM with release-note-none",
			initialLabels: []string{lgtmLabel, releaseNoteNone},
			body:          "```release-note\nnone\n```",
		},
		{
			name:          "LGTM with release-note-none, /release-note-none comment, empty block",
			initialLabels: []string{lgtmLabel, releaseNoteNone},
			body:          "```release-note\n```",
			issueComments: []string{"/release-note-none "},
		},
		{
			name:          "LGTM with release-note-action-required",
			initialLabels: []string{lgtmLabel, releaseNoteActionRequired},
			body:          "```release-note\n Action required.\n```",
		},
		{
			name:          "LGTM with release-note-action-required, /release-note-none comment",
			initialLabels: []string{lgtmLabel, releaseNoteActionRequired},
			body:          "```release-note\n Action required.\n```",
			issueComments: []string{"Release notes are great fun.", "Especially \n/release-note-none"},
		},
		{
			name:          "LGTM with release-note-label-needed",
			initialLabels: []string{lgtmLabel, releaseNoteLabelNeeded},
		},
		{
			name:          "LGTM with release-note-label-needed, /release-note-none comment",
			initialLabels: []string{lgtmLabel, releaseNoteLabelNeeded},
			issueComments: []string{"Release notes are great fun.", "Especially \n/release-note-none"},
			labelsAdded:   []string{releaseNoteNone},
			labelsRemoved: []string{releaseNoteLabelNeeded},
		},
		{
			name:          "LGTM only",
			initialLabels: []string{lgtmLabel},
			labelsAdded:   []string{releaseNoteLabelNeeded},
		},
		{
			name:          "No labels",
			initialLabels: []string{},
			labelsAdded:   []string{releaseNoteLabelNeeded},
		},
		{
			name:          "release-note",
			initialLabels: []string{releaseNote},
			body:          "```release-note normal note.```",
		},
		{
			name:          "release-note, /release-note-none comment",
			initialLabels: []string{releaseNote},
			body:          "```release-note normal note.```",
			issueComments: []string{"/release-note-none "},
		},
		{
			name:          "release-note-none",
			initialLabels: []string{releaseNoteNone},
			body:          "```release-note\nnone\n```",
		},
		{
			name:          "release-note-action-required",
			initialLabels: []string{releaseNoteActionRequired},
			body:          "```release-note\n action required```",
		},
		{
			name:          "release-note and release-note-label-needed with no note",
			initialLabels: []string{releaseNote, releaseNoteLabelNeeded},
			labelsRemoved: []string{releaseNote},
		},
		{
			name:          "release-note and release-note-label-needed with note",
			initialLabels: []string{releaseNote, releaseNoteLabelNeeded},
			body:          "```release-note note  ```",
			labelsRemoved: []string{releaseNoteLabelNeeded},
		},
		{
			name:          "release-note-none and release-note-label-needed",
			initialLabels: []string{releaseNoteNone, releaseNoteLabelNeeded},
			body:          "```release-note\nnone\n```",
			labelsRemoved: []string{releaseNoteLabelNeeded},
		},
		{
			name:          "release-note-action-required and release-note-label-needed",
			initialLabels: []string{releaseNoteActionRequired, releaseNoteLabelNeeded},
			body:          "```release-note\nSomething something dark side. Something something ACTION REQUIRED.```",
			labelsRemoved: []string{releaseNoteLabelNeeded},
		},
		{
			name:          "do not add needs label when parent PR has releaseNote label",
			branch:        "release-1.2",
			initialLabels: []string{},
			body:          "Cherry pick of #2 on release-1.2.",
			parentPRs:     map[int]string{2: releaseNote},
		},
		{
			name:          "do not touch LGTM on non-master when parent PR has releaseNote label, but remove releaseNoteNeeded",
			branch:        "release-1.2",
			initialLabels: []string{lgtmLabel, releaseNoteLabelNeeded},
			body:          "Cherry pick of #2 on release-1.2.",
			parentPRs:     map[int]string{2: releaseNote},
			labelsRemoved: []string{releaseNoteLabelNeeded},
		},
		{
			name:          "do nothing when PR has releaseNoteActionRequired, but parent PR does not have releaseNote label",
			branch:        "release-1.2",
			initialLabels: []string{releaseNoteActionRequired},
			body:          "Cherry pick of #2 on release-1.2.\n```release-note note action required note\n```",
			parentPRs:     map[int]string{2: releaseNoteNone},
		},
		{
			name:          "add releaseNoteNeeded on non-master when parent PR has releaseNoteNone label",
			branch:        "release-1.2",
			initialLabels: []string{lgtmLabel},
			body:          "Cherry pick of #2 on release-1.2.",
			parentPRs:     map[int]string{2: releaseNoteNone},
			labelsAdded:   []string{releaseNoteLabelNeeded},
		},
		{
			name:          "add releaseNoteNeeded on non-master when 1 of 2 parent PRs has releaseNoteNone",
			branch:        "release-1.2",
			initialLabels: []string{lgtmLabel},
			body:          "Other text.\nCherry pick of #2 on release-1.2.\nCherry pick of #4 on release-1.2.\n",
			parentPRs:     map[int]string{2: releaseNote, 4: releaseNoteNone},
			labelsAdded:   []string{releaseNoteLabelNeeded},
		},
		{
			name:          "remove releaseNoteNeeded on non-master when both parent PRs have a release note",
			branch:        "release-1.2",
			initialLabels: []string{lgtmLabel, releaseNoteLabelNeeded},
			body:          "Other text.\nCherry pick of #2 on release-1.2.\nCherry pick of #4 on release-1.2.\n",
			parentPRs:     map[int]string{2: releaseNote, 4: releaseNoteActionRequired},
			labelsRemoved: []string{releaseNoteLabelNeeded},
		},
		{
			name:          "add releaseNoteActionRequired on non-master when body contains note even though both parent PRs have a release note (non-mandatory RN)",
			branch:        "release-1.2",
			initialLabels: []string{lgtmLabel, releaseNoteLabelNeeded},
			body:          "Other text.\nCherry pick of #2 on release-1.2.\nCherry pick of #4 on release-1.2.\n```release-note\nSome changes were made but there still is action required.\n```",
			parentPRs:     map[int]string{2: releaseNote, 4: releaseNoteActionRequired},
			labelsAdded:   []string{releaseNoteActionRequired},
			labelsRemoved: []string{releaseNoteLabelNeeded},
		},
		{
			name:          "add releaseNoteNeeded, remove release-note on non-master when release-note block is removed and parent PR has releaseNoteNone label",
			branch:        "release-1.2",
			initialLabels: []string{lgtmLabel, releaseNote},
			body:          "Cherry pick of #2 on release-1.2.\n```release-note\n```\n/cc @cjwagner",
			parentPRs:     map[int]string{2: releaseNoteNone},
			labelsAdded:   []string{releaseNoteLabelNeeded},
			labelsRemoved: []string{releaseNote},
		},
		{
			name:          "add releaseNoteLabelNeeded, remove release-note on non-master when release-note block is removed and parent PR has releaseNoteNone label",
			branch:        "release-1.2",
			initialLabels: []string{lgtmLabel, releaseNote},
			body:          "Cherry pick of #2 on release-1.2.\n```release-note\n```\n/cc @cjwagner",
			parentPRs:     map[int]string{2: releaseNoteNone},
			labelsAdded:   []string{releaseNoteLabelNeeded},
			labelsRemoved: []string{releaseNote},
		},
	}
	for _, test := range tests {
		if test.branch == "" {
			test.branch = "master"
		}
		fc, pr := newFakeClient(test.body, test.branch, test.initialLabels, test.issueComments, test.parentPRs)

		err := handlePR(fc, logrus.WithField("plugin", pluginName), pr)
		if err != nil {
			t.Fatalf("Unexpected error from handlePR: %v", err)
		}

		// Check that all the correct labels (and only the correct labels) were added.
		expectAdded := formatLabels(1, append(test.initialLabels, test.labelsAdded...)...)
		for parent, label := range test.parentPRs {
			expectAdded = append(expectAdded, formatLabels(parent, label)...)
		}
		sort.Strings(expectAdded)
		sort.Strings(fc.LabelsAdded)
		if !reflect.DeepEqual(expectAdded, fc.LabelsAdded) {
			t.Errorf("(%s): Expected labels to be added: %q, but got: %q.", test.name, expectAdded, fc.LabelsAdded)
		}
		expectRemoved := formatLabels(1, test.labelsRemoved...)
		sort.Strings(expectRemoved)
		sort.Strings(fc.LabelsRemoved)
		if !reflect.DeepEqual(expectRemoved, fc.LabelsRemoved) {
			t.Errorf("(%s): Expected labels to be removed: %q, but got %q.", test.name, expectRemoved, fc.LabelsRemoved)
		}
	}
}

func TestGetReleaseNote(t *testing.T) {
	tests := []struct {
		body                        string
		expectedReleaseNote         string
		expectedReleaseNoteVariable string
	}{
		{
			body:                        "**Release note**:  ```NONE```",
			expectedReleaseNote:         "NONE",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "**Release note**:\n\n ```\nNONE\n```",
			expectedReleaseNote:         "NONE",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "**Release note**:\n<!--  Steps to write your release note:\n...\n-->\n```NONE\n```",
			expectedReleaseNote:         "NONE",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "**Release note**:\n\n  ```This is a description of my feature```",
			expectedReleaseNote:         "This is a description of my feature",
			expectedReleaseNoteVariable: releaseNote,
		},
		{
			body:                        "**Release note**: ```This is my feature. There is some action required for my feature.```",
			expectedReleaseNote:         "This is my feature. There is some action required for my feature.",
			expectedReleaseNoteVariable: releaseNoteActionRequired,
		},
		{
			body:                        "```release-note\nsomething great.\n```",
			expectedReleaseNote:         "something great.",
			expectedReleaseNoteVariable: releaseNote,
		},
		{
			body:                        "```release-note\nNONE\n```",
			expectedReleaseNote:         "NONE",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "```release-note\n`NONE`\n```",
			expectedReleaseNote:         "`NONE`",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "```release-note\n`\"NONE\"`\n```",
			expectedReleaseNote:         "`\"NONE\"`",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "**Release note**:\n```release-note\nNONE\n```\n",
			expectedReleaseNote:         "NONE",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "",
			expectedReleaseNote:         "",
			expectedReleaseNoteVariable: releaseNoteLabelNeeded,
		},
	}

	for testNum, test := range tests {
		calculatedReleaseNote := getReleaseNote(test.body)
		if test.expectedReleaseNote != calculatedReleaseNote {
			t.Errorf("Test %v: Expected %v as the release note, got %v", testNum, test.expectedReleaseNote, calculatedReleaseNote)
		}
		calculatedLabel := determineReleaseNoteLabel(test.body)
		if test.expectedReleaseNoteVariable != calculatedLabel {
			t.Errorf("Test %v: Expected %v as the release note label, got %v", testNum, test.expectedReleaseNoteVariable, calculatedLabel)
		}
	}
}

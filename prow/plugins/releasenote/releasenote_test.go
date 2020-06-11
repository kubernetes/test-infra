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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
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
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},
		},
		{
			name:          "author release-note-none with missing block",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},

			deletedLabels: []string{ReleaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none with empty block",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			issueBody:     "bologna ```release-note \n ```",
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},

			deletedLabels: []string{ReleaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none with \"none\" block",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none",
			issueBody:     "bologna ```release-note \nnone \n ```",
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},

			deletedLabels: []string{ReleaseNoteLabelNeeded},
			addedLabel:    releaseNoteNone,
		},
		{
			name:          "author release-note-none, trailing space.",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-none ",
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},

			deletedLabels: []string{ReleaseNoteLabelNeeded},
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
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},

			shouldComment: true,
		},
		{
			name:          "someone else release-note, trailing space.",
			action:        github.IssueCommentActionCreated,
			commentBody:   "/release-note \r",
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},
			shouldComment: true,
		},
		{
			name:          "someone else release-note-none",
			action:        github.IssueCommentActionCreated,
			commentBody:   "/release-note-none",
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},
			shouldComment: true,
		},
		{
			name:          "author release-note-action-required",
			action:        github.IssueCommentActionCreated,
			isAuthor:      true,
			commentBody:   "/release-note-action-required",
			currentLabels: []string{ReleaseNoteLabelNeeded, "other"},
			shouldComment: true,
		},
		{
			name:          "release-note-none, delete multiple labels",
			action:        github.IssueCommentActionCreated,
			isMember:      true,
			commentBody:   "/release-note-none",
			currentLabels: []string{releaseNote, ReleaseNoteLabelNeeded, releaseNoteActionRequired, releaseNoteNone, "other"},

			deletedLabels: []string{ReleaseNoteLabelNeeded, releaseNoteActionRequired, releaseNote},
		},
		{
			name:        "no label present",
			action:      github.IssueCommentActionCreated,
			isMember:    true,
			commentBody: "/release-note-none",

			addedLabel: releaseNoteNone,
		},
		{
			name:          "member release-note-none, PR has kind/deprecation label",
			action:        github.IssueCommentActionCreated,
			isMember:      true,
			commentBody:   "/release-note-none",
			currentLabels: []string{deprecationLabel},
			shouldComment: true,
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
		if err := handleComment(fc, logrus.WithField("plugin", PluginName), ice); err != nil {
			t.Errorf("For case %s, did not expect error: %v", tc.name, err)
		}
		if tc.shouldComment && len(fc.IssueComments[5]) == 0 {
			t.Errorf("For case %s, didn't comment but should have.", tc.name)
		}
		if len(fc.IssueLabelsAdded) > 1 {
			t.Errorf("For case %s, added more than one label: %v", tc.name, fc.IssueLabelsAdded)
		} else if len(fc.IssueLabelsAdded) == 0 && tc.addedLabel != "" {
			t.Errorf("For case %s, should have added %s but didn't.", tc.name, tc.addedLabel)
		} else if len(fc.IssueLabelsAdded) == 1 && fc.IssueLabelsAdded[0] != "/#5:"+tc.addedLabel {
			t.Errorf("For case %s, added wrong label. Got %s, expected %s", tc.name, fc.IssueLabelsAdded[0], tc.addedLabel)
		}

		var expectedDeleted []string
		for _, expect := range tc.deletedLabels {
			expectedDeleted = append(expectedDeleted, "/#5:"+expect)
		}
		sort.Strings(expectedDeleted)
		sort.Strings(fc.IssueLabelsRemoved)
		if !reflect.DeepEqual(expectedDeleted, fc.IssueLabelsRemoved) {
			t.Errorf(
				"For case %s, expected %q labels to be deleted, but %q were deleted.",
				tc.name,
				expectedDeleted,
				fc.IssueLabelsRemoved,
			)
		}
	}
}

const lgtmLabel = labels.LGTM

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
			RepoLabelsExisting: []string{
				lgtmLabel,
				releaseNote,
				ReleaseNoteLabelNeeded,
				releaseNoteNone,
				releaseNoteActionRequired,
			},
			IssueLabelsAdded:   labels,
			IssueLabelsRemoved: []string{},
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
		name               string
		initialLabels      []string
		body               string
		branch             string // Defaults to master
		parentPRs          map[int]string
		issueComments      []string
		IssueLabelsAdded   []string
		IssueLabelsRemoved []string
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
			name:          "LGTM with do-not-merge/release-note-label-needed",
			initialLabels: []string{lgtmLabel, ReleaseNoteLabelNeeded},
		},
		{
			name:               "LGTM with do-not-merge/release-note-label-needed, /release-note-none comment",
			initialLabels:      []string{lgtmLabel, ReleaseNoteLabelNeeded},
			issueComments:      []string{"Release notes are great fun.", "Especially \n/release-note-none"},
			IssueLabelsAdded:   []string{releaseNoteNone},
			IssueLabelsRemoved: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:             "LGTM only",
			initialLabels:    []string{lgtmLabel},
			IssueLabelsAdded: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:             "No labels",
			initialLabels:    []string{},
			IssueLabelsAdded: []string{ReleaseNoteLabelNeeded},
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
			name:               "release-note and do-not-merge/release-note-label-needed with no note",
			initialLabels:      []string{releaseNote, ReleaseNoteLabelNeeded},
			IssueLabelsRemoved: []string{releaseNote},
		},
		{
			name:               "release-note and do-not-merge/release-note-label-needed with note",
			initialLabels:      []string{releaseNote, ReleaseNoteLabelNeeded},
			body:               "```release-note note  ```",
			IssueLabelsRemoved: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:               "release-note-none and do-not-merge/release-note-label-needed",
			initialLabels:      []string{releaseNoteNone, ReleaseNoteLabelNeeded},
			body:               "```release-note\nnone\n```",
			IssueLabelsRemoved: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:               "release-note-action-required and do-not-merge/release-note-label-needed",
			initialLabels:      []string{releaseNoteActionRequired, ReleaseNoteLabelNeeded},
			body:               "```release-note\nSomething something dark side. Something something ACTION REQUIRED.```",
			IssueLabelsRemoved: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:          "do not add needs label when parent PR has releaseNote label",
			branch:        "release-1.2",
			initialLabels: []string{},
			body:          "Cherry pick of #2 on release-1.2.",
			parentPRs:     map[int]string{2: releaseNote},
		},
		{
			name:               "do not touch LGTM on non-master when parent PR has releaseNote label, but remove releaseNoteNeeded",
			branch:             "release-1.2",
			initialLabels:      []string{lgtmLabel, ReleaseNoteLabelNeeded},
			body:               "Cherry pick of #2 on release-1.2.",
			parentPRs:          map[int]string{2: releaseNote},
			IssueLabelsRemoved: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:          "do nothing when PR has releaseNoteActionRequired, but parent PR does not have releaseNote label",
			branch:        "release-1.2",
			initialLabels: []string{releaseNoteActionRequired},
			body:          "Cherry pick of #2 on release-1.2.\n```release-note note action required note\n```",
			parentPRs:     map[int]string{2: releaseNoteNone},
		},
		{
			name:             "add releaseNoteNeeded on non-master when parent PR has releaseNoteNone label",
			branch:           "release-1.2",
			initialLabels:    []string{lgtmLabel},
			body:             "Cherry pick of #2 on release-1.2.",
			parentPRs:        map[int]string{2: releaseNoteNone},
			IssueLabelsAdded: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:             "add releaseNoteNeeded on non-master when 1 of 2 parent PRs has releaseNoteNone",
			branch:           "release-1.2",
			initialLabels:    []string{lgtmLabel},
			body:             "Other text.\nCherry pick of #2 on release-1.2.\nCherry pick of #4 on release-1.2.\n",
			parentPRs:        map[int]string{2: releaseNote, 4: releaseNoteNone},
			IssueLabelsAdded: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:               "remove releaseNoteNeeded on non-master when both parent PRs have a release note",
			branch:             "release-1.2",
			initialLabels:      []string{lgtmLabel, ReleaseNoteLabelNeeded},
			body:               "Other text.\nCherry pick of #2 on release-1.2.\nCherry pick of #4 on release-1.2.\n",
			parentPRs:          map[int]string{2: releaseNote, 4: releaseNoteActionRequired},
			IssueLabelsRemoved: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:               "add releaseNoteActionRequired on non-master when body contains note even though both parent PRs have a release note (non-mandatory RN)",
			branch:             "release-1.2",
			initialLabels:      []string{lgtmLabel, ReleaseNoteLabelNeeded},
			body:               "Other text.\nCherry pick of #2 on release-1.2.\nCherry pick of #4 on release-1.2.\n```release-note\nSome changes were made but there still is action required.\n```",
			parentPRs:          map[int]string{2: releaseNote, 4: releaseNoteActionRequired},
			IssueLabelsAdded:   []string{releaseNoteActionRequired},
			IssueLabelsRemoved: []string{ReleaseNoteLabelNeeded},
		},
		{
			name:               "add releaseNoteNeeded, remove release-note on non-master when release-note block is removed and parent PR has releaseNoteNone label",
			branch:             "release-1.2",
			initialLabels:      []string{lgtmLabel, releaseNote},
			body:               "Cherry pick of #2 on release-1.2.\n```release-note\n```\n/cc @cjwagner",
			parentPRs:          map[int]string{2: releaseNoteNone},
			IssueLabelsAdded:   []string{ReleaseNoteLabelNeeded},
			IssueLabelsRemoved: []string{releaseNote},
		},
		{
			name:               "add ReleaseNoteLabelNeeded, remove release-note on non-master when release-note block is removed and parent PR has releaseNoteNone label",
			branch:             "release-1.2",
			initialLabels:      []string{lgtmLabel, releaseNote},
			body:               "Cherry pick of #2 on release-1.2.\n```release-note\n```\n/cc @cjwagner",
			parentPRs:          map[int]string{2: releaseNoteNone},
			IssueLabelsAdded:   []string{ReleaseNoteLabelNeeded},
			IssueLabelsRemoved: []string{releaseNote},
		},
		{
			name:               "add ReleaseNoteLabelNeeded, remove ReleaseNoteNone when kind/deprecation label is added",
			initialLabels:      []string{deprecationLabel, releaseNoteNone},
			body:               "```release-note\nnone\n```",
			IssueLabelsAdded:   []string{ReleaseNoteLabelNeeded},
			IssueLabelsRemoved: []string{releaseNoteNone},
		},
		{
			name:             "release-note-none command cannot override deprecation label",
			issueComments:    []string{"/release-note-none "},
			initialLabels:    []string{deprecationLabel},
			body:             "",
			IssueLabelsAdded: []string{ReleaseNoteLabelNeeded},
		},
	}
	for _, test := range tests {
		if test.branch == "" {
			test.branch = "master"
		}
		fc, pr := newFakeClient(test.body, test.branch, test.initialLabels, test.issueComments, test.parentPRs)

		err := handlePR(fc, logrus.WithField("plugin", PluginName), pr)
		if err != nil {
			t.Fatalf("Unexpected error from handlePR: %v", err)
		}

		// Check that all the correct labels (and only the correct labels) were added.
		expectAdded := formatLabels(1, append(test.initialLabels, test.IssueLabelsAdded...)...)
		for parent, label := range test.parentPRs {
			expectAdded = append(expectAdded, formatLabels(parent, label)...)
		}
		sort.Strings(expectAdded)
		sort.Strings(fc.IssueLabelsAdded)
		if !reflect.DeepEqual(expectAdded, fc.IssueLabelsAdded) {
			t.Errorf("(%s): Expected labels to be added: %q, but got: %q.", test.name, expectAdded, fc.IssueLabelsAdded)
		}
		expectRemoved := formatLabels(1, test.IssueLabelsRemoved...)
		sort.Strings(expectRemoved)
		sort.Strings(fc.IssueLabelsRemoved)
		if !reflect.DeepEqual(expectRemoved, fc.IssueLabelsRemoved) {
			t.Errorf("(%s): Expected labels to be removed: %q, but got %q.", test.name, expectRemoved, fc.IssueLabelsRemoved)
		}
	}
}

func TestGetReleaseNote(t *testing.T) {
	tests := []struct {
		body                        string
		labels                      sets.String
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
			expectedReleaseNoteVariable: ReleaseNoteLabelNeeded,
		},
		{
			body:                        "",
			labels:                      sets.NewString(releaseNoteNone),
			expectedReleaseNote:         "",
			expectedReleaseNoteVariable: releaseNoteNone,
		},
		{
			body:                        "",
			labels:                      sets.NewString(deprecationLabel),
			expectedReleaseNote:         "",
			expectedReleaseNoteVariable: ReleaseNoteLabelNeeded,
		},
		{
			body:                        "",
			labels:                      sets.NewString(releaseNoteNone, deprecationLabel),
			expectedReleaseNote:         "",
			expectedReleaseNoteVariable: ReleaseNoteLabelNeeded,
		},
		{
			body:                        "```release-note\nNONE\n```",
			labels:                      sets.NewString(deprecationLabel),
			expectedReleaseNote:         "NONE",
			expectedReleaseNoteVariable: ReleaseNoteLabelNeeded,
		},
	}

	for testNum, test := range tests {
		calculatedReleaseNote := getReleaseNote(test.body)
		if test.expectedReleaseNote != calculatedReleaseNote {
			t.Errorf("Test %v: Expected %v as the release note, got %v", testNum, test.expectedReleaseNote, calculatedReleaseNote)
		}
		calculatedLabel := determineReleaseNoteLabel(test.body, test.labels)
		if test.expectedReleaseNoteVariable != calculatedLabel {
			t.Errorf("Test %v: Expected %v as the release note label, got %v", testNum, test.expectedReleaseNoteVariable, calculatedLabel)
		}
	}
}

func TestShouldHandlePR(t *testing.T) {
	tests := []struct {
		name           string
		action         github.PullRequestEventAction
		label          string
		expectedResult bool
	}{
		{
			name:           "Pull Request Action: Opened",
			action:         github.PullRequestActionOpened,
			label:          "",
			expectedResult: true,
		},
		{
			name:           "Pull Request Action: Edited",
			action:         github.PullRequestActionEdited,
			label:          "",
			expectedResult: true,
		},
		{
			name:           "Pull Request Action: Release Note label",
			action:         github.PullRequestActionLabeled,
			label:          ReleaseNoteLabelNeeded,
			expectedResult: true,
		},
		{
			name:           "Pull Request Action: Non Release Note label",
			action:         github.PullRequestActionLabeled,
			label:          "do-not-merge/cherry-pick-not-approved",
			expectedResult: false,
		},
	}

	for _, test := range tests {
		pr := github.PullRequestEvent{
			Action: test.action,
			Label: github.Label{
				Name: test.label,
			},
		}
		result := shouldHandlePR(&pr)

		if test.expectedResult != result {
			t.Errorf("(%s): Expected value to be: %t, but got %t.", test.name, test.expectedResult, result)
		}
	}
}

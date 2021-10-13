/*
Copyright 2018 The Kubernetes Authors.

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

package requirematchinglabel

import (
	"regexp"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

type fakeGitHub struct {
	labels                               sets.String
	IssueLabelsAdded, IssueLabelsRemoved sets.String
	commented                            bool
}

func newFakeGitHub(initialLabels ...string) *fakeGitHub {
	return &fakeGitHub{
		labels:             sets.NewString(initialLabels...),
		IssueLabelsAdded:   sets.NewString(),
		IssueLabelsRemoved: sets.NewString(),
	}
}

func (f *fakeGitHub) AddLabel(org, repo string, number int, label string) error {
	f.labels.Insert(label)
	f.IssueLabelsAdded.Insert(label)
	return nil
}

func (f *fakeGitHub) RemoveLabel(org, repo string, number int, label string) error {
	f.labels.Delete(label)
	f.IssueLabelsRemoved.Insert(label)
	return nil
}

func (f *fakeGitHub) CreateComment(org, repo string, number int, content string) error {
	f.commented = true
	return nil
}

func (f *fakeGitHub) GetIssueLabels(org, repo string, number int) ([]github.Label, error) {
	res := make([]github.Label, 0, len(f.labels))
	for label := range f.labels {
		res = append(res, github.Label{Name: label})
	}
	return res, nil
}

type fakePruner struct{}

func (fp *fakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}

func TestHandle(t *testing.T) {
	configs := []plugins.RequireMatchingLabel{
		// needs-sig over k8s org (issues)
		{
			Org:          "k8s",
			Issues:       true,
			Re:           regexp.MustCompile(`^(sig|wg|committee)/`),
			MissingLabel: "needs-sig",
		},

		// needs-kind over k8s/t-i repo (PRs)
		{
			Org:          "k8s",
			Repo:         "t-i",
			PRs:          true,
			Re:           regexp.MustCompile(`^kind/`),
			MissingLabel: "needs-kind",
		},
		// needs-cat over k8s/t-i:meow branch (issues and PRs) (will comment)
		{
			Org:            "k8s",
			Repo:           "t-i",
			Branch:         "meow",
			Issues:         true,
			PRs:            true,
			Re:             regexp.MustCompile(`^(cat|floof|loaf)$`),
			MissingLabel:   "needs-cat",
			MissingComment: "Meow?",
		},
	}

	tcs := []struct {
		name          string
		event         *event
		initialLabels []string

		expectComment   bool
		expectedAdded   sets.String
		expectedRemoved sets.String
	}{
		{
			name: "ignore PRs",
			event: &event{
				org:    "k8s",
				repo:   "k8s",
				branch: "foo",
			},
			initialLabels: []string{labels.LGTM},
		},
		{
			name: "ignore wrong org",
			event: &event{
				org:  "fejtaverse",
				repo: "repo",
			},
			initialLabels: []string{labels.LGTM},
		},
		{
			name: "ignore unrelated label change",
			event: &event{
				org:    "k8s",
				repo:   "t-i",
				branch: "master",
				label:  "unrelated",
			},
			initialLabels: []string{labels.LGTM},
		},
		{
			name: "add needs-kind label to PR",
			event: &event{
				org:    "k8s",
				repo:   "t-i",
				branch: "master",
			},
			initialLabels: []string{labels.LGTM},
			expectedAdded: sets.NewString("needs-kind"),
		},
		{
			name: "remove needs-kind label from PR based on label change",
			event: &event{
				org:    "k8s",
				repo:   "t-i",
				branch: "master",
				label:  "kind/best",
			},
			initialLabels:   []string{labels.LGTM, "needs-kind", "kind/best"},
			expectedRemoved: sets.NewString("needs-kind"),
		},
		{
			name: "don't remove needs-kind label from issue based on label change (ignore issues)",
			event: &event{
				org:   "k8s",
				repo:  "t-i",
				label: "kind/best",
			},
			initialLabels: []string{labels.LGTM, "needs-kind", "kind/best", "sig/cats"},
		},
		{
			name: "don't remove needs-kind label from PR already missing it",
			event: &event{
				org:    "k8s",
				repo:   "t-i",
				branch: "master",
				label:  "kind/best",
			},
			initialLabels: []string{labels.LGTM, "kind/best"},
		},
		{
			name: "add org scoped needs-sig to issue",
			event: &event{
				org:   "k8s",
				repo:  "k8s",
				label: "sig/bash",
			},
			initialLabels: []string{labels.LGTM, "kind/best"},
			expectedAdded: sets.NewString("needs-sig"),
		},
		{
			name: "don't add org scoped needs-sig to issue when another sig/* label remains",
			event: &event{
				org:   "k8s",
				repo:  "k8s",
				label: "sig/bash",
			},
			initialLabels: []string{labels.LGTM, "kind/best", "wg/foo"},
		},
		{
			name: "add branch scoped needs-cat to issue",
			event: &event{
				org:   "k8s",
				repo:  "t-i",
				label: "cat",
			},
			initialLabels: []string{labels.LGTM, "wg/foo"},
			expectedAdded: sets.NewString("needs-cat"),
			expectComment: true,
		},
		{
			name: "add branch scoped needs-cat to PR",
			event: &event{
				org:    "k8s",
				repo:   "t-i",
				branch: "meow",
			},
			initialLabels: []string{labels.LGTM, "kind/best"},
			expectedAdded: sets.NewString("needs-cat"),
			expectComment: true,
		},
		{
			name: "remove branch scoped needs-cat from PR, add repo scoped needs-kind",
			event: &event{
				org:    "k8s",
				repo:   "t-i",
				branch: "meow",
			},
			initialLabels:   []string{labels.LGTM, "needs-cat", "cat", "floof"},
			expectedAdded:   sets.NewString("needs-kind"),
			expectedRemoved: sets.NewString("needs-cat"),
		},
		{
			name: "add branch scoped needs-cat to issue, remove org scoped needs-sig",
			event: &event{
				org:  "k8s",
				repo: "t-i",
			},
			initialLabels:   []string{labels.LGTM, "needs-sig", "wg/foo"},
			expectedAdded:   sets.NewString("needs-cat"),
			expectedRemoved: sets.NewString("needs-sig"),
			expectComment:   true,
		},
	}

	for _, tc := range tcs {
		t.Logf("Running test case %q...", tc.name)
		log := logrus.WithField("plugin", "require-matching-label")
		fghc := newFakeGitHub(tc.initialLabels...)
		if err := handle(log, fghc, &fakePruner{}, configs, tc.event); err != nil {
			t.Fatalf("Unexpected error from handle: %v.", err)
		}

		if tc.expectComment && !fghc.commented {
			t.Error("Expected a comment, but didn't get one.")
		} else if !tc.expectComment && fghc.commented {
			t.Error("Expected no comments to be created but got one.")
		}

		if !tc.expectedAdded.Equal(fghc.IssueLabelsAdded) {
			t.Errorf("Expected the %q labels to be added, but got %q.", tc.expectedAdded.List(), fghc.IssueLabelsAdded.List())
		}

		if !tc.expectedRemoved.Equal(fghc.IssueLabelsRemoved) {
			t.Errorf("Expected the %q labels to be removed, but got %q.", tc.expectedRemoved.List(), fghc.IssueLabelsRemoved.List())
		}
	}
}

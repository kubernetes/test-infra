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

package blockade

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

var (
	// Sample changes:
	docFile    = github.PullRequestChange{Filename: "docs/documentation.md", BlobURL: "<URL1>"}
	docOwners  = github.PullRequestChange{Filename: "docs/OWNERS", BlobURL: "<URL2>"}
	docOwners2 = github.PullRequestChange{Filename: "docs/2/OWNERS", BlobURL: "<URL3>"}
	srcGo      = github.PullRequestChange{Filename: "src/code.go", BlobURL: "<URL4>"}
	srcSh      = github.PullRequestChange{Filename: "src/shell.sh", BlobURL: "<URL5>"}
	docSh      = github.PullRequestChange{Filename: "docs/shell.sh", BlobURL: "<URL6>"}

	// Sample blockades:
	blockDocs = plugins.Blockade{
		Repos:        []string{"org/repo"},
		BlockRegexps: []string{`docs/.*`},
		Explanation:  "1",
	}
	blockDocsExceptOwners = plugins.Blockade{
		Repos:            []string{"org/repo"},
		BlockRegexps:     []string{`docs/.*`},
		ExceptionRegexps: []string{`.*OWNERS`},
		Explanation:      "2",
	}
	blockShell = plugins.Blockade{
		Repos:        []string{"org/repo"},
		BlockRegexps: []string{`.*\.sh`},
		Explanation:  "3",
	}
	blockAllOrg = plugins.Blockade{
		Repos:        []string{"org"},
		BlockRegexps: []string{`.*`},
		Explanation:  "4",
	}
	blockAllOther = plugins.Blockade{
		Repos:        []string{"org2"},
		BlockRegexps: []string{`.*`},
		Explanation:  "5",
	}
)

// TestCalculateBlocks validates that changes are blocked or allowed correctly.
func TestCalculateBlocks(t *testing.T) {
	tcs := []struct {
		name            string
		changes         []github.PullRequestChange
		config          []plugins.Blockade
		expectedSummary summary
	}{
		{
			name:    "blocked by 1/1 blockade (no exceptions), extra file",
			config:  []plugins.Blockade{blockDocs},
			changes: []github.PullRequestChange{docFile, docOwners, srcGo},
			expectedSummary: summary{
				"1": []github.PullRequestChange{docFile, docOwners},
			},
		},
		{
			name:    "blocked by 1/1 blockade (1/2 files are exceptions), extra file",
			config:  []plugins.Blockade{blockDocsExceptOwners},
			changes: []github.PullRequestChange{docFile, docOwners, srcGo},
			expectedSummary: summary{
				"2": []github.PullRequestChange{docFile},
			},
		},
		{
			name:            "blocked by 0/1 blockades (2/2 exceptions), extra file",
			config:          []plugins.Blockade{blockDocsExceptOwners},
			changes:         []github.PullRequestChange{docOwners, docOwners2, srcGo},
			expectedSummary: summary{},
		},
		{
			name:            "blocked by 0/1 blockades (no exceptions), extra file",
			config:          []plugins.Blockade{blockDocsExceptOwners},
			changes:         []github.PullRequestChange{srcGo, srcSh},
			expectedSummary: summary{},
		},
		{
			name:    "blocked by 2/2 blockades (no exceptions), extra file",
			config:  []plugins.Blockade{blockDocsExceptOwners, blockShell},
			changes: []github.PullRequestChange{srcGo, srcSh, docFile},
			expectedSummary: summary{
				"2": []github.PullRequestChange{docFile},
				"3": []github.PullRequestChange{srcSh},
			},
		},
		{
			name:    "blocked by 2/2 blockades w/ single file",
			config:  []plugins.Blockade{blockDocsExceptOwners, blockShell},
			changes: []github.PullRequestChange{docSh},
			expectedSummary: summary{
				"2": []github.PullRequestChange{docSh},
				"3": []github.PullRequestChange{docSh},
			},
		},
		{
			name:    "blocked by 2/2 blockades w/ single file (1/2 exceptions)",
			config:  []plugins.Blockade{blockDocsExceptOwners, blockShell},
			changes: []github.PullRequestChange{docSh, docOwners},
			expectedSummary: summary{
				"2": []github.PullRequestChange{docSh},
				"3": []github.PullRequestChange{docSh},
			},
		},
		{
			name:    "blocked by 1/2 blockades (1/2 exceptions), extra file",
			config:  []plugins.Blockade{blockDocsExceptOwners, blockShell},
			changes: []github.PullRequestChange{srcSh, docOwners, srcGo},
			expectedSummary: summary{
				"3": []github.PullRequestChange{srcSh},
			},
		},
		{
			name:            "blocked by 0/2 blockades (1/2 exceptions), extra file",
			config:          []plugins.Blockade{blockDocsExceptOwners, blockShell},
			changes:         []github.PullRequestChange{docOwners, srcGo},
			expectedSummary: summary{},
		},
	}

	for _, tc := range tcs {
		blockades := compileApplicableBlockades("org", "repo", logrus.WithField("plugin", PluginName), tc.config)
		sum := calculateBlocks(tc.changes, blockades)
		if !reflect.DeepEqual(sum, tc.expectedSummary) {
			t.Errorf("[%s] Expected summary: %#v, actual summary: %#v.", tc.name, tc.expectedSummary, sum)
		}
	}
}

func TestSummaryString(t *testing.T) {
	// Just one example for now.
	tcs := []struct {
		name             string
		sum              summary
		expectedContents []string
	}{
		{
			name: "Simple example",
			sum: summary{
				"reason A": []github.PullRequestChange{docFile},
				"reason B": []github.PullRequestChange{srcGo, srcSh},
			},
			expectedContents: []string{
				"#### Reasons for blocking this PR:\n",
				"[reason A]\n- [docs/documentation.md](<URL1>)\n\n",
				"[reason B]\n- [src/code.go](<URL4>)\n\n- [src/shell.sh](<URL5>)\n\n",
			},
		},
	}

	for _, tc := range tcs {
		got := tc.sum.String()
		for _, expected := range tc.expectedContents {
			if !strings.Contains(got, expected) {
				t.Errorf("[%s] Expected summary %#v to contain %q, but got %q.", tc.name, tc.sum, expected, got)
			}
		}
	}
}

func formatLabel(label string) string {
	return fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 1, label)
}

type fakePruner struct{}

func (f *fakePruner) PruneComments(_ func(ic github.IssueComment) bool) {}

// TestHandle validates that:
// - The correct labels are added/removed.
// - A comment is created when needed.
// - Uninteresting events are ignored.
// - Blockades that don't apply to this repo are ignored.
func TestHandle(t *testing.T) {
	// Don't need to validate the following because they are validated by other tests:
	// - Block calculation. (Whether or not changes justify blocking the PR.)
	// - Comment contents, just existence.
	otherLabel := labels.LGTM

	tcs := []struct {
		name       string
		action     github.PullRequestEventAction
		config     []plugins.Blockade
		hasLabel   bool
		filesBlock bool // This is ignored if there are no applicable blockades for the repo.

		labelAdded     string
		labelRemoved   string
		commentCreated bool
	}{
		{
			name:       "Boring action",
			action:     github.PullRequestActionEdited,
			config:     []plugins.Blockade{blockDocsExceptOwners},
			hasLabel:   false,
			filesBlock: true,
		},
		{
			name:       "Basic block",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{blockDocsExceptOwners},
			hasLabel:   false,
			filesBlock: true,

			labelAdded:     labels.BlockedPaths,
			commentCreated: true,
		},
		{
			name:       "Basic block, already labeled",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{blockDocsExceptOwners},
			hasLabel:   true,
			filesBlock: true,
		},
		{
			name:       "Not blocked, not labeled",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{blockDocsExceptOwners},
			hasLabel:   false,
			filesBlock: false,
		},
		{
			name:       "Not blocked, has label",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{blockDocsExceptOwners},
			hasLabel:   true,
			filesBlock: false,

			labelRemoved: labels.BlockedPaths,
		},
		{
			name:       "No blockade, not labeled",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{},
			hasLabel:   false,
			filesBlock: true,
		},
		{
			name:       "No blockade, has label",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{},
			hasLabel:   true,
			filesBlock: true,

			labelRemoved: labels.BlockedPaths,
		},
		{
			name:       "Basic block (org scoped blockade)",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{blockAllOrg},
			hasLabel:   false,
			filesBlock: true,

			labelAdded:     labels.BlockedPaths,
			commentCreated: true,
		},
		{
			name:       "Would be blocked, but blockade is not applicable; not labeled",
			action:     github.PullRequestActionOpened,
			config:     []plugins.Blockade{blockAllOther},
			hasLabel:   false,
			filesBlock: true,
		},
	}

	for _, tc := range tcs {
		expectAdded := []string{}
		fakeClient := &fakegithub.FakeClient{
			RepoLabelsExisting: []string{labels.BlockedPaths, otherLabel},
			IssueComments:      make(map[int][]github.IssueComment),
			PullRequestChanges: make(map[int][]github.PullRequestChange),
			IssueLabelsAdded:   []string{},
			IssueLabelsRemoved: []string{},
		}
		if tc.hasLabel {
			label := formatLabel(labels.BlockedPaths)
			fakeClient.IssueLabelsAdded = append(fakeClient.IssueLabelsAdded, label)
			expectAdded = append(expectAdded, label)
		}
		calcF := func(_ []github.PullRequestChange, blockades []blockade) summary {
			if !tc.filesBlock {
				return nil
			}
			sum := make(summary)
			for _, b := range blockades {
				// For this test assume 'docFile' is blocked by every blockade that is applicable to the repo.
				sum[b.explanation] = []github.PullRequestChange{docFile}
			}
			return sum
		}
		pre := &github.PullRequestEvent{
			Action: tc.action,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			Number: 1,
		}
		if err := handle(fakeClient, logrus.WithField("plugin", PluginName), tc.config, &fakePruner{}, calcF, pre); err != nil {
			t.Errorf("[%s] Unexpected error from handle: %v.", tc.name, err)
			continue
		}

		if tc.labelAdded != "" {
			expectAdded = append(expectAdded, formatLabel(tc.labelAdded))
		}
		sort.Strings(expectAdded)
		sort.Strings(fakeClient.IssueLabelsAdded)
		if !reflect.DeepEqual(expectAdded, fakeClient.IssueLabelsAdded) {
			t.Errorf("[%s]: Expected labels to be added: %q, but got: %q.", tc.name, expectAdded, fakeClient.IssueLabelsAdded)
		}
		expectRemoved := []string{}
		if tc.labelRemoved != "" {
			expectRemoved = append(expectRemoved, formatLabel(tc.labelRemoved))
		}
		sort.Strings(expectRemoved)
		sort.Strings(fakeClient.IssueLabelsRemoved)
		if !reflect.DeepEqual(expectRemoved, fakeClient.IssueLabelsRemoved) {
			t.Errorf("[%s]: Expected labels to be removed: %q, but got: %q.", tc.name, expectRemoved, fakeClient.IssueLabelsRemoved)
		}

		if count := len(fakeClient.IssueComments[1]); count > 1 {
			t.Errorf("[%s] More than 1 comment created! (%d created).", tc.name, count)
		} else if (count == 1) != tc.commentCreated {
			t.Errorf("[%s] Expected comment created: %t, but got %t.", tc.name, tc.commentCreated, count == 1)
		}
	}
}

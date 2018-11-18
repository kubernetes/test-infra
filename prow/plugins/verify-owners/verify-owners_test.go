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

package verifyowners

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
)

var ownerFiles = map[string][]byte{
	"emptyApprovers": []byte(`approvers:
reviewers:
- alice
- bob
labels:
- label1
`),
	"emptyApproversFilters": []byte(`filters:
  ".*":
    approvers:
    reviewers:
    - alice
    - bob
    labels:
    - label1
`),
	"invalidSyntax": []byte(`approvers
- jdoe
reviewers:
- alice
- bob
labels:
- label1
`),
	"invalidSyntaxFilters": []byte(`filters:
  ".*":
    approvers
    - jdoe
    reviewers:
    - alice
    - bob
    labels:
    - label1
`),
	"invalidLabels": []byte(`approvers:
- jdoe
reviewers:
- alice
- bob
labels:
- lgtm
`),
	"invalidLabelsFilters": []byte(`filters:
  ".*":
    approvers:
    - jdoe
    reviewers:
    - alice
    - bob
    labels:
    - lgtm
`),
	"noApprovers": []byte(`reviewers:
- alice
- bob
labels:
- label1
`),
	"noApproversFilters": []byte(`filters:
  ".*":
    reviewers:
    - alice
    - bob
    labels:
    - label1
`),
	"valid": []byte(`approvers:
- jdoe
reviewers:
- alice
- bob
labels:
- label1
`),
	"validFilters": []byte(`filters:
  ".*":
    approvers:
    - jdoe
    reviewers:
    - alice
    - bob
    labels:
    - label1
`),
}

func IssueLabelsAddedContain(arr []string, str string) bool {
	for _, a := range arr {
		// IssueLabelsAdded format is owner/repo#number:label
		b := strings.Split(a, ":")
		if b[len(b)-1] == str {
			return true
		}
	}
	return false
}

func newFakeGithubClient(files []string, pr int) *fakegithub.FakeClient {
	var changes []github.PullRequestChange
	for _, file := range files {
		changes = append(changes, github.PullRequestChange{Filename: file})
	}
	return &fakegithub.FakeClient{
		PullRequestChanges: map[int][]github.PullRequestChange{pr: changes},
		Reviews:            map[int][]github.Review{},
	}
}

func TestHandle(t *testing.T) {
	var tests = []struct {
		name         string
		filesChanged []string
		ownersFile   string
		shouldLabel  bool
	}{
		{
			name:         "no OWNERS file",
			filesChanged: []string{"a.go", "b.go"},
			ownersFile:   "valid",
			shouldLabel:  false,
		},
		{
			name:         "no OWNERS file with filters",
			filesChanged: []string{"a.go", "b.go"},
			ownersFile:   "validFilters",
			shouldLabel:  false,
		},
		{
			name:         "good OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "valid",
			shouldLabel:  false,
		},
		{
			name:         "good OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "validFilters",
			shouldLabel:  false,
		},
		{
			name:         "invalid syntax OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntax",
			shouldLabel:  true,
		},
		{
			name:         "invalid syntax OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidSyntaxFilters",
			shouldLabel:  true,
		},
		{
			name:         "forbidden labels in OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidLabels",
			shouldLabel:  true,
		},
		{
			name:         "forbidden labels in OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "invalidLabelsFilters",
			shouldLabel:  true,
		},
		{
			name:         "empty approvers in OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "emptyApprovers",
			shouldLabel:  true,
		},
		{
			name:         "empty approvers in OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "emptyApproversFilters",
			shouldLabel:  true,
		},
		{
			name:         "no approvers in OWNERS file",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "noApprovers",
			shouldLabel:  true,
		},
		{
			name:         "no approvers in OWNERS file with filters",
			filesChanged: []string{"OWNERS", "b.go"},
			ownersFile:   "noApproversFilters",
			shouldLabel:  true,
		},
		{
			name:         "no approvers in pkg/OWNERS file",
			filesChanged: []string{"pkg/OWNERS", "b.go"},
			ownersFile:   "noApprovers",
			shouldLabel:  false,
		},
		{
			name:         "no approvers in pkg/OWNERS file with filters",
			filesChanged: []string{"pkg/OWNERS", "b.go"},
			ownersFile:   "noApproversFilters",
			shouldLabel:  false,
		},
	}
	lg, c, err := localgit.New()
	if err != nil {
		t.Fatalf("Making localgit: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Cleaning up localgit: %v", err)
		}
		if err := c.Clean(); err != nil {
			t.Errorf("Cleaning up client: %v", err)
		}
	}()
	if err := lg.MakeFakeRepo("org", "repo"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	for i, test := range tests {
		pr := i + 1
		// make sure we're on master before branching
		if err := lg.Checkout("org", "repo", "master"); err != nil {
			t.Fatalf("Switching to master branch: %v", err)
		}
		if err := lg.CheckoutNewBranch("org", "repo", fmt.Sprintf("pull/%d/head", pr)); err != nil {
			t.Fatalf("Checking out pull branch: %v", err)
		}
		pullFiles := map[string][]byte{}
		for _, file := range test.filesChanged {
			if strings.Contains(file, "OWNERS") {
				pullFiles[file] = ownerFiles[test.ownersFile]
			} else {
				pullFiles[file] = []byte("foo")
			}
		}
		if err := lg.AddCommit("org", "repo", pullFiles); err != nil {
			t.Fatalf("Adding PR commit: %v", err)
		}
		pre := &github.PullRequestEvent{
			Number:      pr,
			PullRequest: github.PullRequest{User: github.User{Login: "author"}},
			Repo:        github.Repo{FullName: "org/repo"},
		}
		fghc := newFakeGithubClient(test.filesChanged, pr)
		if err := handle(fghc, c, logrus.WithField("plugin", PluginName), pre, []string{labels.Approved, labels.LGTM}); err != nil {
			t.Fatalf("Handle PR: %v", err)
		}
		if !test.shouldLabel && IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
			t.Errorf("%s: didn't expect label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			continue
		} else if test.shouldLabel && !IssueLabelsAddedContain(fghc.IssueLabelsAdded, labels.InvalidOwners) {
			t.Errorf("%s: expected label %s in %s", test.name, labels.InvalidOwners, fghc.IssueLabelsAdded)
			continue
		}
	}
}

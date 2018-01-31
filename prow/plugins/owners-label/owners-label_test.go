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

package ownerslabel

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

type fakeGithubClient struct {
	changes       []github.PullRequestChange
	initialLabels []github.Label
	labelsAdded   sets.String
}

func newFakeGithubClient(filesChanged []string, initialLabels []string) *fakeGithubClient {
	changes := make([]github.PullRequestChange, 0, len(filesChanged))
	for _, name := range filesChanged {
		changes = append(changes, github.PullRequestChange{Filename: name})
	}
	labels := make([]github.Label, 0, len(initialLabels))
	for _, label := range initialLabels {
		labels = append(labels, github.Label{Name: label})
	}
	return &fakeGithubClient{
		changes:       changes,
		initialLabels: labels,
		labelsAdded:   sets.NewString(),
	}
}

func (c *fakeGithubClient) AddLabel(org, repo string, number int, label string) error {
	if org != "org" {
		return errors.New("org should be 'org'")
	}
	if repo != "repo" {
		return errors.New("repo should be 'repo'")
	}
	if number != 5 {
		return errors.New("number should be 5")
	}
	c.labelsAdded.Insert(label)
	return nil
}

func (c *fakeGithubClient) GetPullRequestChanges(org, repo string, num int) ([]github.PullRequestChange, error) {
	if org != "org" {
		return nil, errors.New("org should be 'org'")
	}
	if repo != "repo" {
		return nil, errors.New("repo should be 'repo'")
	}
	if num != 5 {
		return nil, errors.New("number should be 5")
	}
	return c.changes, nil
}

func (c *fakeGithubClient) GetIssueLabels(org, repo string, num int) ([]github.Label, error) {
	if org != "org" {
		return nil, errors.New("org should be 'org'")
	}
	if repo != "repo" {
		return nil, errors.New("repo should be 'repo'")
	}
	if num != 5 {
		return nil, errors.New("number should be 5")
	}
	return c.initialLabels, nil
}

type fakeOwnersClient struct {
	labels map[string]sets.String
}

func (foc *fakeOwnersClient) FindLabelsForFile(path string) sets.String {
	return foc.labels[path]
}

// TestHandle tests that the handle function requests reviews from the correct number of unique users.
func TestHandle(t *testing.T) {
	foc := &fakeOwnersClient{
		labels: map[string]sets.String{
			"a.go": sets.NewString("lgtm", "approved", "kind/docs"),
			"b.go": sets.NewString("lgtm"),
			"c.go": sets.NewString("lgtm", "dnm/frozen-docs"),
			"d.sh": sets.NewString("dnm/bash"),
			"e.sh": sets.NewString("dnm/bash"),
		},
	}

	var testcases = []struct {
		name          string
		filesChanged  []string
		initialLabels []string
		expectedAdded sets.String
	}{
		{
			name:          "no labels",
			filesChanged:  []string{"other.go", "something.go"},
			expectedAdded: sets.NewString(),
		},
		{
			name:          "1 file 1 label",
			filesChanged:  []string{"b.go"},
			expectedAdded: sets.NewString("lgtm"),
		},
		{
			name:          "1 file 3 labels",
			filesChanged:  []string{"a.go"},
			expectedAdded: sets.NewString("lgtm", "approved", "kind/docs"),
		},
		{
			name:          "2 files no overlap",
			filesChanged:  []string{"c.go", "d.sh"},
			expectedAdded: sets.NewString("lgtm", "dnm/frozen-docs", "dnm/bash"),
		},
		{
			name:          "2 files partial overlap",
			filesChanged:  []string{"a.go", "b.go"},
			expectedAdded: sets.NewString("lgtm", "approved", "kind/docs"),
		},
		{
			name:          "2 files complete overlap",
			filesChanged:  []string{"d.sh", "e.sh"},
			expectedAdded: sets.NewString("dnm/bash"),
		},
		{
			name:          "3 files partial overlap",
			filesChanged:  []string{"a.go", "b.go", "c.go"},
			expectedAdded: sets.NewString("lgtm", "approved", "kind/docs", "dnm/frozen-docs"),
		},
		{
			name:          "no labels to add, initial unrelated label",
			filesChanged:  []string{"other.go", "something.go"},
			initialLabels: []string{"lgtm"},
			expectedAdded: sets.NewString(),
		},
		{
			name:          "1 file 1 label, already present",
			filesChanged:  []string{"b.go"},
			initialLabels: []string{"lgtm"},
			expectedAdded: sets.NewString(),
		},
		{
			name:          "2 files no overlap, 1 label already present",
			filesChanged:  []string{"c.go", "d.sh"},
			initialLabels: []string{"dnm/bash", "approved"},
			expectedAdded: sets.NewString("lgtm", "dnm/frozen-docs"),
		},
		{
			name:          "2 files complete overlap, label already present",
			filesChanged:  []string{"d.sh", "e.sh"},
			initialLabels: []string{"dnm/bash"},
			expectedAdded: sets.NewString(),
		},
	}
	for _, tc := range testcases {
		fghc := newFakeGithubClient(tc.filesChanged, tc.initialLabels)
		pre := &github.PullRequestEvent{
			Number:      5,
			PullRequest: github.PullRequest{User: github.User{Login: "author"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}

		if err := handle(fghc, foc, logrus.WithField("plugin", pluginName), pre); err != nil {
			t.Errorf("[%s] unexpected error from handle: %v", tc.name, err)
			continue
		}
		if !fghc.labelsAdded.Equal(tc.expectedAdded) {
			t.Errorf("[%s] expected the labels %q to be added, but got %q.", tc.name, tc.expectedAdded.List(), fghc.labelsAdded.List())
		}
	}
}

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

package help

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakePruner struct{}

func (fp *fakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}

func formatLabels(labels ...string) []string {
	r := []string{}
	for _, l := range labels {
		r = append(r, fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 1, l))
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

func TestLabel(t *testing.T) {
	type testCase struct {
		name                  string
		body                  string
		expectedNewLabels     []string
		expectedRemovedLabels []string
		issueLabels           []string
	}
	testcases := []testCase{
		{
			name:                  "Irrelevant comment",
			body:                  "irrelelvant",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Want helpLabel",
			body:                  "/help",
			expectedNewLabels:     formatLabels(helpLabel),
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
		{
			name:                  "Want helpLabel, already have it.",
			body:                  "/help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{helpLabel},
		},
		{
			name:                  "Want to remove helpLabel, have it",
			body:                  "/remove-help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatLabels(helpLabel),
			issueLabels:           []string{helpLabel},
		},
		{
			name:                  "Want to remove helpLabel, don't have it",
			body:                  "/remove-help",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			issueLabels:           []string{},
		},
	}

	for _, tc := range testcases {
		sort.Strings(tc.expectedNewLabels)
		fakeClient := &fakegithub.FakeClient{
			Issues:         make([]github.Issue, 1),
			IssueComments:  make(map[int][]github.IssueComment),
			ExistingLabels: []string{helpLabel},
			LabelsAdded:    []string{},
			LabelsRemoved:  []string{},
		}
		// Add initial labels
		for _, label := range tc.issueLabels {
			fakeClient.AddLabel("org", "repo", 1, label)
		}
		e := &github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			Body:   tc.body,
			Number: 1,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			User:   github.User{Login: "Alice"},
		}
		err := handle(fakeClient, logrus.WithField("plugin", pluginName), &fakePruner{}, e)
		if err != nil {
			t.Errorf("For case %s, didn't expect error from label test: %v", tc.name, err)
			continue
		}

		// Check that all the correct labels (and only the correct labels) were added.
		expectLabels := append(formatLabels(tc.issueLabels...), tc.expectedNewLabels...)
		if expectLabels == nil {
			expectLabels = []string{}
		}
		sort.Strings(expectLabels)
		sort.Strings(fakeClient.LabelsAdded)
		if !reflect.DeepEqual(expectLabels, fakeClient.LabelsAdded) {
			t.Errorf("(%s): Expected the labels %q to be added, but %q were added.", tc.name, expectLabels, fakeClient.LabelsAdded)
		}

		sort.Strings(tc.expectedRemovedLabels)
		sort.Strings(fakeClient.LabelsRemoved)
		if !reflect.DeepEqual(tc.expectedRemovedLabels, fakeClient.LabelsRemoved) {
			t.Errorf("(%s): Expected the labels %q to be removed, but %q were removed.", tc.name, tc.expectedRemovedLabels, fakeClient.LabelsRemoved)
		}
	}
}

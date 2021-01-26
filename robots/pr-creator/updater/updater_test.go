/*
Copyright 2020 The Kubernetes Authors.

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

package updater

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestEnsurePRWithLabel(t *testing.T) {
	testCases := []struct {
		name   string
		client *fakegithub.FakeClient
	}{
		{
			name:   "pr is created",
			client: &fakegithub.FakeClient{},
		},
		{
			name: "pr is updated",
			client: &fakegithub.FakeClient{
				PullRequests: map[int]*github.PullRequest{
					22: {Number: 22},
				},
				Issues: map[int]*github.Issue{
					22: {Number: 22},
				},
			},
		},
		{
			name: "existing labels are considered",
			client: &fakegithub.FakeClient{
				PullRequests: map[int]*github.PullRequest{
					42: {Number: 42},
				},
				Issues: map[int]*github.Issue{
					42: {
						Number: 42,
						Labels: []github.Label{{Name: "a"}},
					},
				},
				IssueLabelsAdded: []string{"org/repo#42:a"},
			},
		},
	}

	org, repo, labels := "org", "repo", []string{"a", "b"}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prNumberPtr, err := EnsurePRWithLabels(org, repo, "title", "body", "source", "branch", "matchTitle", PreventMods, tc.client, labels)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if n := len(tc.client.PullRequests); n != 1 {
				t.Fatalf("expected to find one PR, got %d", n)
			}

			expectedLabels := sets.NewString()
			for _, label := range labels {
				expectedLabels.Insert(fmt.Sprintf("%s/%s#%d:%s", org, repo, *prNumberPtr, label))
			}

			if diff := sets.NewString(tc.client.IssueLabelsAdded...).Difference(expectedLabels); len(diff) != 0 {
				t.Errorf("found labels do not match expected, diff: %v", diff)
			}
		})
	}
}

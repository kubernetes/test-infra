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

package report

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
)

func TestParseIssueComment(t *testing.T) {
	var testcases = []struct {
		name             string
		context          string
		state            string
		ics              []github.IssueComment
		expectedDeletes  []int
		expectedContexts []string
		expectedUpdate   int
	}{
		{
			name:    "should delete old style comments",
			context: "Jenkins foo test",
			state:   github.StatusSuccess,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "Jenkins foo test **failed** for such-and-such.",
					ID:   12345,
				},
				{
					User: github.User{Login: "someone-else"},
					Body: "Jenkins foo test **failed**!? Why?",
					ID:   12356,
				},
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "Jenkins foo test **failed** for so-and-so.",
					ID:   12367,
				},
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "Jenkins bar test **failed** for something-or-other.",
					ID:   12378,
				},
			},
			expectedDeletes: []int{12345, 12367},
		},
		{
			name:             "should create a new comment",
			context:          "bla test",
			state:            github.StatusFailure,
			expectedContexts: []string{"bla test"},
		},
		{
			name:    "should not delete an up-to-date comment",
			context: "bla test",
			state:   github.StatusSuccess,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\nfoo test | something | or other\n\n",
				},
			},
		},
		{
			name:    "should delete when all tests pass",
			context: "bla test",
			state:   github.StatusSuccess,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\nbla test | something | or other\n\n" + commentTag,
					ID:   123,
				},
			},
			expectedDeletes:  []int{123},
			expectedContexts: []string{},
		},
		{
			name:    "should delete a passing test with \\r",
			context: "bla test",
			state:   github.StatusSuccess,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\r\nbla test | something | or other\r\n\r\n" + commentTag,
					ID:   123,
				},
			},
			expectedDeletes:  []int{123},
			expectedContexts: []string{},
		},

		{
			name:    "should update a failed test",
			context: "bla test",
			state:   github.StatusFailure,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\nbla test | something | or other\n\n" + commentTag,
					ID:   123,
				},
			},
			expectedDeletes:  []int{123},
			expectedContexts: []string{"bla test"},
		},
		{
			name:    "should preserve old results when updating",
			context: "bla test",
			state:   github.StatusFailure,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\nbla test | something | or other\nfoo test | wow | aye\n\n" + commentTag,
					ID:   123,
				},
			},
			expectedDeletes:  []int{123},
			expectedContexts: []string{"bla test", "foo test"},
		},
		{
			name:    "should merge duplicates",
			context: "bla test",
			state:   github.StatusFailure,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\nbla test | something | or other\nfoo test | wow such\n\n" + commentTag,
					ID:   123,
				},
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\nfoo test | beep | boop\n\n" + commentTag,
					ID:   124,
				},
			},
			expectedDeletes:  []int{123, 124},
			expectedContexts: []string{"bla test", "foo test"},
		},
		{
			name:    "should update an old comment when a test passes",
			context: "bla test",
			state:   github.StatusSuccess,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\nbla test | something | or other\nfoo test | wow | aye\n\n" + commentTag,
					ID:   123,
				},
			},
			expectedDeletes:  []int{},
			expectedContexts: []string{"foo test"},
			expectedUpdate:   123,
		},
	}
	for _, tc := range testcases {
		pj := kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Context: tc.context,
				Refs:    &kube.Refs{Pulls: []kube.Pull{{}}},
			},
			Status: kube.ProwJobStatus{
				State: kube.ProwJobState(tc.state),
			},
		}
		deletes, entries, update := parseIssueComments(pj, "k8s-ci-robot", tc.ics)
		if len(deletes) != len(tc.expectedDeletes) {
			t.Errorf("It %s: wrong number of deletes. Got %v, expected %v", tc.name, deletes, tc.expectedDeletes)
		} else {
			for _, edel := range tc.expectedDeletes {
				found := false
				for _, del := range deletes {
					if del == edel {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("It %s: expected to find %d in %v", tc.name, edel, deletes)
				}
			}
		}
		if len(entries) != len(tc.expectedContexts) {
			t.Errorf("It %s: wrong number of entries. Got %v, expected %v", tc.name, entries, tc.expectedContexts)
		} else {
			for _, econt := range tc.expectedContexts {
				found := false
				for _, ent := range entries {
					if strings.Contains(ent, econt) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("It %s: expected to find %s in %v", tc.name, econt, entries)
				}
			}
		}
		if tc.expectedUpdate != update {
			t.Errorf("It %s: expected update %d, got %d", tc.name, tc.expectedUpdate, update)
		}
	}
}

type fakeGhClient struct {
	status []github.Status
}

func (gh fakeGhClient) BotName() (string, error) {
	return "BotName", nil
}
func (gh *fakeGhClient) CreateStatus(org, repo, ref string, s github.Status) error {
	gh.status = append(gh.status, s)
	return nil

}
func (gh fakeGhClient) ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) {
	return nil, nil
}
func (gh fakeGhClient) CreateComment(org, repo string, number int, comment string) error {
	return nil
}
func (gh fakeGhClient) DeleteComment(org, repo string, ID int) error {
	return nil
}
func (gh fakeGhClient) EditComment(org, repo string, ID int, comment string) error {
	return nil
}

func createChildren(pj *kube.ProwJobSpec, d int) {
	for i := 0; i < d; i++ {
		npj := &kube.ProwJobSpec{
			// TODO: Support testing this via defining expected behavior in TestReportStatus
			Type:    kube.PresubmitJob,
			Report:  true,
			Context: fmt.Sprintf("%s/child_%d", pj.Context, i),
			Refs: &kube.Refs{
				Org:  "k8s",
				Repo: "test-infra",
				Pulls: []kube.Pull{{
					Author: "me",
					Number: 1,
					SHA:    "abcdef",
				}},
			},
		}
		pj.RunAfterSuccess = append(pj.RunAfterSuccess, *npj)
	}
}

func TestReportStatus(t *testing.T) {
	tests := []struct {
		name string

		// TODO: This should be the RunAfterSuccess spec and not a single int.
		children int
		state    kube.ProwJobState
		report   bool

		expectedStatuses []string
	}{
		{
			name: "Successful prowjob with report true and children should set status for itself but not its children",

			children: 3,
			state:    kube.SuccessState,
			report:   true,

			expectedStatuses: []string{"success"},
		},
		{
			name: "Successful prowjob with report false and children should not set status for itself and its children",

			children: 3,
			state:    kube.SuccessState,
			report:   false,

			expectedStatuses: []string{},
		},
		{
			name: "Pending prowjob with report true and children should set status for itself and its children",

			children: 3,
			state:    kube.PendingState,
			report:   true,

			expectedStatuses: []string{"pending", "pending", "pending", "pending"},
		},
		{
			name: "Pending prowjob with report false and children should not set status for itself, but still cover children jobs",

			children: 3,
			state:    kube.PendingState,
			report:   false,

			expectedStatuses: []string{"pending", "pending", "pending"},
		},
		{
			name: "Aborted prowjob with report true should set failure status",

			state:  kube.AbortedState,
			report: true,

			expectedStatuses: []string{"failure"},
		},
		{
			name: "Triggered prowjob with report true should set pending status",

			state:  kube.TriggeredState,
			report: true,

			expectedStatuses: []string{"pending"},
		},
	}

	createPJ := func(state kube.ProwJobState, report bool, children int) kube.ProwJob {
		pj := kube.ProwJob{
			Status: kube.ProwJobStatus{
				State:       state,
				Description: "message",
				URL:         "http://mytest.com",
			},
			Spec: kube.ProwJobSpec{
				Job:     "job-name",
				Type:    kube.PresubmitJob,
				Context: "parent",
				Report:  report,
				Refs: &kube.Refs{
					Org:  "k8s",
					Repo: "test-infra",
					Pulls: []kube.Pull{{
						Author: "me",
						Number: 1,
						SHA:    "abcdef",
					}},
				},
			},
		}
		createChildren(&pj.Spec, children)
		return pj
	}
	for _, tc := range tests {
		t.Logf("Running scenario %q", tc.name)
		// Setup
		ghc := &fakeGhClient{}
		pj := createPJ(tc.state, tc.report, tc.children)
		// Run
		if err := reportStatus(ghc, pj, "Parent Status Changed"); err != nil {
			t.Error(err)
		}
		// Check
		if len(ghc.status) != len(tc.expectedStatuses) {
			t.Errorf("expected %d status(es), found %d", len(tc.expectedStatuses), len(ghc.status))
			continue
		}
		for i, status := range ghc.status {
			if status.State != tc.expectedStatuses[i] {
				t.Errorf("unexpected status: %s, expected: %s", status.State, tc.expectedStatuses[i])
			}
		}
	}
}

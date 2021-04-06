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

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
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
		pj := prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Context: tc.context,
				Refs:    &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.ProwJobState(tc.state),
			},
		}
		isBot := func(candidate string) bool {
			return candidate == "k8s-ci-robot"
		}
		deletes, entries, update := parseIssueComments(pj, isBot, tc.ics)
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

func (gh fakeGhClient) BotUserChecker() (func(string) bool, error) {
	return func(candidate string) bool {
		return candidate == "BotName"
	}, nil
}

const maxLen = 140

func (gh *fakeGhClient) CreateStatus(org, repo, ref string, s github.Status) error {
	if d := s.Description; len(d) > maxLen {
		return fmt.Errorf("%s is len %d, more than max of %d chars", d, len(d), maxLen)
	}
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

func shout(i int) string {
	if i == 0 {
		return "start"
	}
	return fmt.Sprintf("%s part%d", shout(i-1), i)
}

func TestReportStatus(t *testing.T) {
	const (
		defMsg = "default-message"
	)
	tests := []struct {
		name string

		state            prowapi.ProwJobState
		report           bool
		desc             string // override default msg
		pjType           prowapi.ProwJobType
		expectedStatuses []string
		expectedDesc     string
	}{
		{
			name: "Successful prowjob with report true should set status",

			state:            prowapi.SuccessState,
			pjType:           prowapi.PresubmitJob,
			report:           true,
			expectedStatuses: []string{"success"},
		},
		{
			name: "Successful prowjob with report false should not set status",

			state:            prowapi.SuccessState,
			pjType:           prowapi.PresubmitJob,
			report:           false,
			expectedStatuses: []string{},
		},
		{
			name: "Pending prowjob with report true should set status",

			state:            prowapi.PendingState,
			report:           true,
			pjType:           prowapi.PresubmitJob,
			expectedStatuses: []string{"pending"},
		},
		{
			name: "Aborted presubmit job with report true should set failure status",

			state:            prowapi.AbortedState,
			report:           true,
			pjType:           prowapi.PresubmitJob,
			expectedStatuses: []string{"failure"},
		},
		{
			name: "Triggered presubmit job with report true should set pending status",

			state:            prowapi.TriggeredState,
			report:           true,
			pjType:           prowapi.PresubmitJob,
			expectedStatuses: []string{"pending"},
		},
		{
			name: "really long description is truncated",

			state:            prowapi.TriggeredState,
			report:           true,
			expectedStatuses: []string{"pending"},
			desc:             shout(maxLen), // resulting string will exceed maxLen
			expectedDesc:     config.ContextDescriptionWithBaseSha(shout(maxLen), ""),
		},
		{
			name: "Successful postsubmit job with report true should set success status",

			state:  prowapi.SuccessState,
			report: true,
			pjType: prowapi.PostsubmitJob,

			expectedStatuses: []string{"success"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			ghc := &fakeGhClient{}

			if tc.desc == "" {
				tc.desc = defMsg
			}
			if tc.expectedDesc == "" {
				tc.expectedDesc = defMsg
			}
			pj := prowapi.ProwJob{
				Status: prowapi.ProwJobStatus{
					State:       tc.state,
					Description: tc.desc,
					URL:         "http://mytest.com",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "job-name",
					Type:    tc.pjType,
					Context: "parent",
					Report:  tc.report,
					Refs: &prowapi.Refs{
						Org:  "k8s",
						Repo: "test-infra",
						Pulls: []prowapi.Pull{{
							Author: "me",
							Number: 1,
							SHA:    "abcdef",
						}},
					},
				},
			}
			// Run
			if err := reportStatus(ghc, pj); err != nil {
				t.Error(err)
			}
			// Check
			if len(ghc.status) != len(tc.expectedStatuses) {
				t.Errorf("expected %d status(es), found %d", len(tc.expectedStatuses), len(ghc.status))
				return
			}
			for i, status := range ghc.status {
				if status.State != tc.expectedStatuses[i] {
					t.Errorf("unexpected status: %s, expected: %s", status.State, tc.expectedStatuses[i])
				}
				if i == 0 && status.Description != tc.expectedDesc {
					t.Errorf("description %d %s != expected %s", i, status.Description, tc.expectedDesc)
				}
			}
		})
	}
}

func TestShouldReport(t *testing.T) {
	var testcases = []struct {
		name       string
		pj         prowapi.ProwJob
		validTypes []prowapi.ProwJobType
		report     bool
	}{
		{
			name: "should not report skip report job",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Type:   prowapi.PresubmitJob,
					Report: false,
				},
			},
			validTypes: []prowapi.ProwJobType{prowapi.PresubmitJob},
		},
		{
			name: "should report presubmit job",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Type:   prowapi.PresubmitJob,
					Report: true,
				},
			},
			validTypes: []prowapi.ProwJobType{prowapi.PresubmitJob},
			report:     true,
		},
		{
			name: "should not report postsubmit job",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Type:   prowapi.PostsubmitJob,
					Report: true,
				},
			},
			validTypes: []prowapi.ProwJobType{prowapi.PresubmitJob},
		},
		{
			name: "should report postsubmit job if told to",
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Type:   prowapi.PostsubmitJob,
					Report: true,
				},
			},
			validTypes: []prowapi.ProwJobType{prowapi.PresubmitJob, prowapi.PostsubmitJob},
			report:     true,
		},
	}

	for _, tc := range testcases {
		r := ShouldReport(tc.pj, tc.validTypes)

		if r != tc.report {
			t.Errorf("Unexpected result from test: %s.\nExpected: %v\nGot: %v",
				tc.name, tc.report, r)
		}
	}
}

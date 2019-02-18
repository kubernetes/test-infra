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
	"k8s.io/test-infra/prow/plugins"
	"strings"
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/github"
)

func TestParseIssueComment(t *testing.T) {
	var testcases = []struct {
		name             string
		context          string
		state            string
		ics              []github.IssueComment
		sha              string
		expectedDeletes  []int
		expectedContexts []string
		expectedUpdate   int
	}{
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
			expectedContexts: []string{"bla test"},
			expectedUpdate:   123,
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
			expectedContexts: []string{"bla test", "foo test"},
			expectedUpdate:   123,
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
			expectedDeletes:  []int{123},
			expectedUpdate:   124,
			expectedContexts: []string{"bla test", "foo test"},
		},
		{
			name:    "should update existing comment to remove job that passes",
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
		{
			name:    "should update existing comment for existing job on the same commit",
			context: "bla test",
			state:   github.StatusFailure,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "sha123\n--- | --- | ---\nbla test | wow | aye\n\n" + commentTag,
					ID:   123,
				},
			},
			sha:              "sha123",
			expectedDeletes:  []int{},
			expectedContexts: []string{"bla test"},
			expectedUpdate:   123,
		},
		{
			name:    "should update existing comment to add failure about new job on the same commit",
			context: "foo test",
			state:   github.StatusFailure,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "sha123\n--- | --- | ---\nbla test | wow | aye\n\n" + commentTag,
					ID:   123,
				},
			},
			sha:              "sha123",
			expectedDeletes:  []int{},
			expectedContexts: []string{"bla test", "foo test"},
			expectedUpdate:   123,
		},
		{
			name:    "should create new comment for the same job but on a new commit",
			context: "bla test",
			state:   github.StatusFailure,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "sha123\n--- | --- | ---\nbla test | wow | aye\n\n" + commentTag,
					ID:   123,
				},
			},
			sha:              "sha456",
			expectedDeletes:  []int{123},
			expectedContexts: []string{"bla test"},
		},
		{
			name:    "should create new comment for a new job on a new commit",
			context: "foo test",
			state:   github.StatusFailure,
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "sha123\n--- | --- | ---\nbla test | wow | aye\n\n" + commentTag,
					ID:   123,
				},
			},
			sha:              "sha456",
			expectedDeletes:  []int{123},
			expectedContexts: []string{"foo test"},
		},
	}
	for _, tc := range testcases {
		pj := prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Context: tc.context,
				Refs:    &prowapi.Refs{Pulls: []prowapi.Pull{{SHA: tc.sha}}},
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.ProwJobState(tc.state),
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
	if d := s.Description; len(d) > maxLen {
		return fmt.Errorf("%s is len %d, more than max of %d chars", d, len(d), maxLen)
	}
	gh.status = append(gh.status, s)
	return nil

}
func (gh fakeGhClient) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	return nil, nil
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
			expectedDesc:     truncate(shout(maxLen)),
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

func TestTruncate(t *testing.T) {
	if el := len(elide) * 2; maxLen < el {
		t.Fatalf("maxLen must be at least %d (twice %s), got %d", el, elide, maxLen)
	}
	if s := shout(maxLen); len(s) <= maxLen {
		t.Fatalf("%s should be at least %d, got %d", s, maxLen, len(s))
	}
	big := shout(maxLen)
	outLen := maxLen
	if (maxLen-len(elide))%2 == 1 {
		outLen--
	}
	cases := []struct {
		name   string
		in     string
		out    string
		outLen int
		front  string
		back   string
		middle string
	}{
		{
			name: "do not change short strings",
			in:   "foo",
			out:  "foo",
		},
		{
			name: "do not change at boundary",
			in:   big[:maxLen],
			out:  big[:maxLen],
		},
		{
			name: "do not change boundary-1",
			in:   big[:maxLen-1],
			out:  big[:maxLen-1],
		},
		{
			name:   "truncated messages have the right length",
			in:     big,
			outLen: outLen,
		},
		{
			name:  "truncated message include beginning",
			in:    big,
			front: big[:maxLen/4], // include a lot of the start
		},
		{
			name: "truncated messages include ending",
			in:   big,
			back: big[len(big)-maxLen/4:],
		},
		{
			name:   "truncated messages include a ...",
			in:     big,
			middle: elide,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := truncate(tc.in)
			exact := true
			if tc.front != "" {
				exact = false
				if !strings.HasPrefix(out, tc.front) {
					t.Errorf("%s does not start with %s", out, tc.front)
				}
			}
			if tc.middle != "" {
				exact = false
				if !strings.Contains(out, tc.middle) {
					t.Errorf("%s does not contain %s", out, tc.middle)
				}
			}
			if tc.back != "" {
				exact = false
				if !strings.HasSuffix(out, tc.back) {
					t.Errorf("%s does not end with %s", out, tc.back)
				}
			}
			if tc.outLen > 0 {
				exact = false
				if len(out) != tc.outLen {
					t.Errorf("%s len %d != expected %d", out, len(out), tc.outLen)
				}
			}
			if exact && out != tc.out {
				t.Errorf("%s != expected %s", out, tc.out)
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

func TestGetOldEntries(t *testing.T) {
	cases := []struct {
		name            string
		bodyLines       []string
		expectedEntries []string
	}{
		{
			name: "should parse a regular list of entries",
			bodyLines: []string{
				tableLine,
				"a | foo | bar",
				"b | foo | bar",
			},
			expectedEntries: []string{
				"a | foo | bar",
				"b | foo | bar",
			},
		},
		{
			name: "should stop tracking on empty line",
			bodyLines: []string{
				tableLine,
				"a | foo | bar",
				"b | foo | bar",
				"",
			},
			expectedEntries: []string{
				"a | foo | bar",
				"b | foo | bar",
			},
		},
		{
			name: "should trim space in lines",
			bodyLines: []string{
				tableLine,
				"    a | foo | bar",
				"b | foo | bar    ",
			},
			expectedEntries: []string{
				"a | foo | bar",
				"b | foo | bar",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Join(tc.bodyLines, "\n")
			entries := strings.Join(getOldEntries(body), "\n")
			expectedEntries := strings.Join(tc.expectedEntries, "\n")
			if entries != expectedEntries {
				t.Errorf("%s\nexpected entries:\n\n%s\n\nfound:\n\n%s", tc.name, expectedEntries, entries)
			}
		})
	}
}

func TestGetNewEntries(t *testing.T) {
	cases := []struct {
		name            string
		context         string
		entries         []string
		expectedEntries []string
	}{
		{
			name: "should remove duplicate entry [1]",
			entries: []string{
				"a | foo | bar",
				"a | foo | bar",
			},
			expectedEntries: []string{
				"a | foo | bar",
			},
		},
		{
			name: "should remove duplicate entry [2]",
			entries: []string{
				"a | foo | bar",
				"a | foo | bar",
				"b | foo | bar",
				"b | foo | bar",
			},
			expectedEntries: []string{
				"a | foo | bar",
				"b | foo | bar",
			},
		},
		{
			name:    "should remove existing entry if context is found [1]",
			context: "test",
			entries: []string{
				"a | foo | bar",
				"test | foo | bar",
			},
			expectedEntries: []string{
				"a | foo | bar",
			},
		},
		{
			name:    "should remove existing entry if context is found [2]",
			context: "test",
			entries: []string{
				"test | foo | bar",
			},
		},
		{
			name:    "should remove existing entry if context is found [3]",
			context: "test",
			entries: []string{
				"test | foo | bar",
				"test | foo | bar",
			},
			expectedEntries: []string{},
		},
		{
			name: "should keep unique old context entries",
			entries: []string{
				"a | foo | bar",
				"b | foo | bar",
			},
			expectedEntries: []string{
				"a | foo | bar",
				"b | foo | bar",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exp := strings.Join(tc.expectedEntries, "\n")
			res := strings.Join(getNewEntries(tc.context, tc.entries), "\n")
			if res != exp {
				t.Errorf("%s\nexpected entries:\n\n%s\n\nfound:\n\n%s", tc.name, exp, res)
			}
		})
	}
}

func TestCreateComment(t *testing.T) {
	cases := []struct {
		name          string
		pj            prowapi.ProwJob
		entries       []string
		commits       []github.RepositoryCommit
		expectedLines []string
	}{
		{
			name:    "should return valid comment for single entry",
			commits: []github.RepositoryCommit{{SHA: "1234"}, {SHA: "5678"}},
			entries: []string{"context | foo | bar"},
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{
							{Author: "someuser", SHA: "135734"},
						},
					},
				},
			},
			expectedLines: []string{
				"@someuser: The following test **failed** for commit 135734, say `/retest` to rerun them:",
				"",
				"Test name | Details | Rerun command",
				"--- | --- | ---",
				"context | foo | bar",
				"",
				"<details>",
				"",
				plugins.AboutThisBot,
				"</details>",
				commentTag,
			},
		},
		{
			name:    "should return valid comment for multiple entries",
			commits: []github.RepositoryCommit{{SHA: "1234"}, {SHA: "5678"}},
			entries: []string{"context | foo | bar", "context2 | foo | bar"},
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{
							{Author: "someuser", SHA: "135734"},
						},
					},
				},
			},
			expectedLines: []string{
				"@someuser: The following tests **failed** for commit 135734, say `/retest` to rerun them:",
				"",
				"Test name | Details | Rerun command",
				"--- | --- | ---",
				"context | foo | bar",
				"context2 | foo | bar",
				"",
				"<details>",
				"",
				plugins.AboutThisBot,
				"</details>",
				commentTag,
			},
		},
		{
			name:    "should return valid comment without commit list",
			entries: []string{"context | foo | bar"},
			pj: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{
							{Author: "someuser", SHA: "135734"},
						},
					},
				},
			},
			expectedLines: []string{
				"@someuser: The following test **failed** for commit 135734, say `/retest` to rerun them:",
				"",
				"Test name | Details | Rerun command",
				"--- | --- | ---",
				"context | foo | bar",
				"",
				"<details>",
				"",
				plugins.AboutThisBot,
				"</details>",
				commentTag,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comment, _ := createComment(nil, tc.pj, tc.entries)
			lines := strings.Join(tc.expectedLines, "\n")
			if comment != lines {
				t.Errorf("%s\nexpected result:\n\n%s\n\ngot:\n\n%s\n", tc.name, lines, comment)
			}
		})
	}
}

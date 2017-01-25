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

package crier

import (
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
)

func TestPRLink(t *testing.T) {
	var testcases = []struct {
		org    string
		repo   string
		number int
		suffix string
	}{
		{
			org:    "o",
			repo:   "r",
			number: 4,
			suffix: "o_r/4",
		},
		{
			org:    "kubernetes",
			repo:   "test-infra",
			number: 123,
			suffix: "test-infra/123",
		},
		{
			org:    "kubernetes",
			repo:   "kubernetes",
			number: 123,
			suffix: "123",
		},
		{
			org:    "o",
			repo:   "kubernetes",
			number: 456,
			suffix: "o_kubernetes/456",
		},
	}
	for _, tc := range testcases {
		prl := prLink(Report{
			RepoOwner: tc.org,
			RepoName:  tc.repo,
			Number:    tc.number,
		})
		if prl[len(guberPrefix):] != tc.suffix {
			t.Errorf("Expected failed case %+v, got %s", tc, prl)
		}
	}
}

func TestParseIssueComment(t *testing.T) {
	var testcases = []struct {
		name             string
		r                Report
		ics              []github.IssueComment
		expectedDeletes  []int
		expectedContexts []string
	}{
		{
			name: "should delete old style comments",
			r: Report{
				Context: "Jenkins foo test",
				State:   github.StatusSuccess,
			},
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
			name: "should create a new comment",
			r: Report{
				Context: "bla test",
				State:   github.StatusFailure,
			},
			expectedContexts: []string{"bla test"},
		},
		{
			name: "should not delete an up-to-date comment",
			r: Report{
				Context: "bla test",
				State:   github.StatusSuccess,
			},
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\r\nfoo test | something | or other\r\n\r\n",
				},
			},
		},
		{
			name: "should delete a passing test",
			r: Report{
				Context: "bla test",
				State:   github.StatusSuccess,
			},
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
			name: "should update a failed test",
			r: Report{
				Context: "bla test",
				State:   github.StatusFailure,
			},
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\r\nbla test | something | or other\r\n\r\n" + commentTag,
					ID:   123,
				},
			},
			expectedDeletes:  []int{123},
			expectedContexts: []string{"bla test"},
		},
		{
			name: "should preserve old results when updating",
			r: Report{
				Context: "bla test",
				State:   github.StatusFailure,
			},
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\r\nbla test | something | or other\r\nfoo test | wow | aye\r\n\r\n" + commentTag,
					ID:   123,
				},
			},
			expectedDeletes:  []int{123},
			expectedContexts: []string{"bla test", "foo test"},
		},
		{
			name: "should merge duplicates",
			r: Report{
				Context: "bla test",
				State:   github.StatusFailure,
			},
			ics: []github.IssueComment{
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\r\nbla test | something | or other\r\nfoo test | wow such\r\n\r\n" + commentTag,
					ID:   123,
				},
				{
					User: github.User{Login: "k8s-ci-robot"},
					Body: "--- | --- | ---\r\nfoo test | beep | boop\r\n\r\n" + commentTag,
					ID:   124,
				},
			},
			expectedDeletes:  []int{123, 124},
			expectedContexts: []string{"bla test", "foo test"},
		},
	}
	for _, tc := range testcases {
		deletes, entries := parseIssueComments(tc.r, tc.ics)
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
	}
}

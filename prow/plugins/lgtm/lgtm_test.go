/*
Copyright 2016 The Kubernetes Authors.

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

package lgtm

import (
	"fmt"
	"testing"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestLGTMComment(t *testing.T) {
	// "a" is the author, "a", "r1", and "r2" are reviewers.
	var testcases = []struct {
		name          string
		action        string
		body          string
		commenter     string
		hasLGTM       bool
		shouldToggle  bool
		shouldComment bool
		shouldAssign  bool
	}{
		{
			name:         "non-lgtm comment",
			action:       "created",
			body:         "uh oh",
			commenter:    "o",
			hasLGTM:      false,
			shouldToggle: false,
		},
		{
			name:         "lgtm comment by reviewer, no lgtm on pr",
			action:       "created",
			body:         "/lgtm",
			commenter:    "r1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "LGTM comment by reviewer, no lgtm on pr",
			action:       "created",
			body:         "/LGTM",
			commenter:    "r1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "lgtm comment by reviewer, lgtm on pr",
			action:       "created",
			body:         "/lgtm",
			commenter:    "r1",
			hasLGTM:      true,
			shouldToggle: false,
		},
		{
			name:          "lgtm comment by author",
			action:        "created",
			body:          "/lgtm",
			commenter:     "a",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: true,
		},
		{
			name:         "lgtm cancel by author",
			action:       "created",
			body:         "/lgtm cancel",
			commenter:    "a",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:          "lgtm comment by non-reviewer",
			action:        "created",
			body:          "/lgtm",
			commenter:     "o",
			hasLGTM:       false,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "lgtm comment by rando",
			action:        "created",
			body:          "/lgtm",
			commenter:     "not-in-the-org",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:          "lgtm cancel by non-reviewer",
			action:        "created",
			body:          "/lgtm cancel",
			commenter:     "o",
			hasLGTM:       true,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "lgtm cancel by rando",
			action:        "created",
			body:          "/lgtm cancel",
			commenter:     "not-in-the-org",
			hasLGTM:       true,
			shouldToggle:  false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:         "lgtm cancel comment by reviewer",
			action:       "created",
			body:         "/lgtm cancel",
			commenter:    "r1",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:         "lgtm cancel comment by reviewer, no lgtm",
			action:       "created",
			body:         "/lgtm cancel",
			commenter:    "r1",
			hasLGTM:      false,
			shouldToggle: false,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		e := &event{
			action:        tc.action,
			body:          tc.body,
			commentAuthor: tc.commenter,
			prAuthor:      "a",
			number:        5,
			assignees:     []github.User{{Login: "a"}, {Login: "r1"}, {Login: "r2"}},
			org:           "org",
			repo:          "repo",
			htmlurl:       "<url>",
		}
		if tc.hasLGTM {
			e.hasLabel = func(label string) (bool, error) { return label == lgtmLabel, nil }
		} else {
			e.hasLabel = func(label string) (bool, error) { return false, nil }
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), e); err != nil {
			t.Errorf("For case %s, didn't expect error from lgtmComment: %v", tc.name, err)
			continue
		}
		if tc.shouldAssign {
			found := false
			for _, a := range fc.AssigneesAdded {
				if a == fmt.Sprintf("%s/%s#%d:%s", e.org, e.repo, 5, tc.commenter) {
					found = true
					break
				}
			}
			if !found || len(fc.AssigneesAdded) != 1 {
				t.Errorf("For case %s, should have assigned %s but added assignees are %s", tc.name, tc.commenter, fc.AssigneesAdded)
			}
		} else if len(fc.AssigneesAdded) != 0 {
			t.Errorf("For case %s, should not have assigned anyone but assigned %s", tc.name, fc.AssigneesAdded)
		}
		if tc.shouldToggle {
			if tc.hasLGTM {
				if len(fc.LabelsRemoved) == 0 {
					t.Errorf("For case %s, should have removed LGTM.", tc.name)
				} else if len(fc.LabelsAdded) > 0 {
					t.Errorf("For case %s, should not have added LGTM.", tc.name)
				}
			} else {
				if len(fc.LabelsAdded) == 0 {
					t.Errorf("For case %s, should have added LGTM.", tc.name)
				} else if len(fc.LabelsRemoved) > 0 {
					t.Errorf("For case %s, should not have removed LGTM.", tc.name)
				}
			}
		} else if len(fc.LabelsRemoved) > 0 || len(fc.LabelsAdded) > 0 {
			t.Errorf("For case %s, should not have added/removed LGTM.", tc.name)
		}
		if tc.shouldComment && len(fc.IssueComments[5]) != 1 {
			t.Errorf("For case %s, should have commented.", tc.name)
		} else if !tc.shouldComment && len(fc.IssueComments[5]) != 0 {
			t.Errorf("For case %s, should not have commented.", tc.name)
		}
	}
}

func TestPRLabelChecker(t *testing.T) {
	testCases := []struct {
		name       string
		labels     []string
		check      string
		shouldPass bool
	}{
		{
			name:       "no labels",
			labels:     []string{},
			check:      "good-pr",
			shouldPass: false,
		},
		{
			name:       "no labels, check empty",
			labels:     []string{},
			check:      "",
			shouldPass: false,
		},
		{
			name:       "some labels, check empty",
			labels:     []string{"some-label", "lgtm"},
			check:      "",
			shouldPass: false,
		},
		{
			name:       "some labels, does not have check",
			labels:     []string{"some-label", "definitely-not-lgtm"},
			check:      "good-pr",
			shouldPass: false,
		},
		{
			name:       "has label last",
			labels:     []string{"some-label", "definitely-not-lgtm", "good-pr"},
			check:      "good-pr",
			shouldPass: true,
		},
		{
			name:       "has label first",
			labels:     []string{"good-pr", "some-label", "definitely-not-lgtm"},
			check:      "good-pr",
			shouldPass: true,
		},
	}
	for _, test := range testCases {
		fc := &fakegithub.FakeClient{}
		for _, label := range test.labels {
			fc.AddLabel("k8s", "kuber", 42, label)
		}
		res, err := prLabelChecker(fc, nil, "k8s", "kuber", 42)(test.check)
		if err != nil {
			t.Errorf("Unexpected error from prLabelChecker on test '%s': %v.", test.name, err)
			continue
		}
		if res != test.shouldPass {
			var not string
			if !test.shouldPass {
				not = "not "
			}
			t.Errorf(
				"Error on test '%s': %q should %sbe recognized as a label on #42 which has labels %q.",
				test.name,
				test.check,
				not,
				test.labels,
			)
		}
	}
}

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

package shrug

import (
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
)

func TestShrugComment(t *testing.T) {
	var testcases = []struct {
		name          string
		body          string
		hasShrug      bool
		shouldShrug   bool
		shouldUnshrug bool
	}{
		{
			name:          "non-shrug comment",
			body:          "uh oh",
			hasShrug:      false,
			shouldShrug:   false,
			shouldUnshrug: false,
		},
		{
			name:          "shrug",
			body:          "/shrug",
			hasShrug:      false,
			shouldShrug:   true,
			shouldUnshrug: false,
		},
		{
			name:          "shrug over shrug",
			body:          "/shrug",
			hasShrug:      true,
			shouldShrug:   false,
			shouldUnshrug: false,
		},
		{
			name:          "unshrug nothing",
			body:          "/unshrug",
			hasShrug:      false,
			shouldShrug:   false,
			shouldUnshrug: false,
		},
		{
			name:          "unshrug the shrug",
			body:          "/unshrug",
			hasShrug:      true,
			shouldShrug:   false,
			shouldUnshrug: true,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		e := &github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			Body:   tc.body,
			Number: 5,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if tc.hasShrug {
			fc.IssueLabelsAdded = []string{"org/repo#5:" + labels.Shrug}
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), e); err != nil {
			t.Errorf("For case %s, didn't expect error: %v", tc.name, err)
			continue
		}

		hadShrug := 0
		if tc.hasShrug {
			hadShrug = 1
		}
		if tc.shouldShrug {
			if len(fc.IssueLabelsAdded)-hadShrug != 1 {
				t.Errorf("For case %s, should add shrug.", tc.name)
			}
			if len(fc.IssueLabelsRemoved) != 0 {
				t.Errorf("For case %s, should not remove label.", tc.name)
			}
		} else if tc.shouldUnshrug {
			if len(fc.IssueLabelsAdded)-hadShrug != 0 {
				t.Errorf("For case %s, should not add shrug.", tc.name)
			}
			if len(fc.IssueLabelsRemoved) != 1 {
				t.Errorf("For case %s, should remove shrug.", tc.name)
			}
		} else if len(fc.IssueLabelsAdded)-hadShrug > 0 || len(fc.IssueLabelsRemoved) > 0 {
			t.Errorf("For case %s, should not have added/removed shrug.", tc.name)
		}
	}
}

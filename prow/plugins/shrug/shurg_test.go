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
		e := &event{
			body: tc.body,
		}
		if tc.hasShrug {
			e.hasLabel = func(label string) (bool, error) { return label == shrugLabel, nil }
		} else {
			e.hasLabel = func(label string) (bool, error) { return false, nil }
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), e); err != nil {
			t.Errorf("For case %s, didn't expect error: %v", tc.name, err)
			continue
		}

		if tc.shouldShrug {
			if len(fc.LabelsAdded) != 1 {
				t.Errorf("For case %s, should add shrug.", tc.name)
			}
			if len(fc.LabelsRemoved) != 0 {
				t.Errorf("For case %s, should not remove label.", tc.name)
			}
		} else if tc.shouldUnshrug {
			if len(fc.LabelsAdded) != 0 {
				t.Errorf("For case %s, should not add shrug.", tc.name)
			}
			if len(fc.LabelsRemoved) != 1 {
				t.Errorf("For case %s, should remove shrug.", tc.name)
			}
		} else if len(fc.LabelsAdded) > 0 || len(fc.LabelsRemoved) > 0 {
			t.Errorf("For case %s, should not have added/removed shrug.", tc.name)
		}
	}
}

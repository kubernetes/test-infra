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

package lifecycle

import (
	"reflect"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type fakeClient struct {
	commented bool
	added     []string
	removed   []string
}

func (c *fakeClient) CreateComment(owner, repo string, number int, comment string) error {
	c.commented = true
	return nil
}

func (c *fakeClient) AddLabel(owner, repo string, number int, label string) error {
	c.added = append(c.added, label)
	return nil
}

func (c *fakeClient) RemoveLabel(owner, repo string, number int, label string) error {
	c.removed = append(c.removed, label)
	return nil
}

func TestDeprecatedClose(t *testing.T) {
	fc := &fakeClient{}
	gce := &github.GenericCommentEvent{}
	ticker := make(chan time.Time, 1)
	deprecatedTick = ticker
	err := deprecate(fc, "fake", "org", "repo", 1, gce)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.commented {
		t.Fatalf("should not comment on empty ticker")
	}
	ticker <- time.Now()
	err = deprecate(fc, "fake", "org", "repo", 1, gce)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fc.commented {
		t.Fatalf("must comment on filled timer")
	}

}

func TestAddLifecycleLabels(t *testing.T) {
	var testcases = []struct {
		body    string
		added   []string
		removed []string
		comment bool
	}{
		{
			body: "/random-command",
		},
		{
			body: "/remove-lifecycle",
		},
		{
			body: "/lifecycle",
		},
		{
			body: "/lifecycle random",
		},
		{
			body: "/remove-lifecycle random",
		},
		{
			body: "/lifecycle frozen putrid stale",
		},
		{
			body: "/lifecycle frozen cancel",
		},
		{
			body:  "/lifecycle frozen",
			added: []string{"lifecycle/frozen"},
		},
		{
			body:  "/lifecycle stale",
			added: []string{"lifecycle/stale"},
		},
		{
			body:  "/lifecycle putrid",
			added: []string{"lifecycle/putrid"},
		},
		{
			body:  "/lifecycle rotten",
			added: []string{"lifecycle/rotten"},
		},
		{
			body:    "/remove-lifecycle frozen",
			removed: []string{"lifecycle/frozen"},
		},
		{
			body:    "/remove-lifecycle stale",
			removed: []string{"lifecycle/stale"},
		},
		{
			body:    "/remove-lifecycle stale\n/remove-lifecycle putrid\n/lifecycle frozen",
			added:   []string{"lifecycle/frozen"},
			removed: []string{"lifecycle/stale", "lifecycle/putrid"},
		},
	}
	for _, tc := range testcases {
		fc := &fakeClient{}
		e := &github.GenericCommentEvent{
			Body:   tc.body,
			Action: github.GenericCommentActionCreated,
		}
		err := handle(fc, logrus.WithField("plugin", "fake-lifecyle"), e)
		switch {
		case err != nil:
			t.Errorf("%s: unexpected error: %v", tc.body, err)
		case tc.comment != fc.commented:
			t.Errorf("%s: acutal comment %t != expected %t", tc.body, tc.comment, fc.commented)
		case !reflect.DeepEqual(tc.added, fc.added):
			t.Errorf("%s: added %v != actual %v", tc.body, tc.added, fc.added)
		case !reflect.DeepEqual(tc.removed, fc.removed):
			t.Errorf("%s: removed %v != actual %v", tc.body, tc.removed, fc.removed)
		}
	}
}

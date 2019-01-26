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

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
)

type fakeClient struct {
	// current labels
	labels []string
	// labels that are added
	added []string
	// labels that are removed
	removed []string
}

func (c *fakeClient) AddLabel(owner, repo string, number int, label string) error {
	c.added = append(c.added, label)
	c.labels = append(c.labels, label)
	return nil
}

func (c *fakeClient) RemoveLabel(owner, repo string, number int, label string) error {
	c.removed = append(c.removed, label)

	// remove from existing labels
	for k, v := range c.labels {
		if label == v {
			c.labels = append(c.labels[:k], c.labels[k+1:]...)
			break
		}
	}

	return nil
}

func (c *fakeClient) GetIssueLabels(owner, repo string, number int) ([]github.Label, error) {
	la := []github.Label{}
	for _, l := range c.labels {
		la = append(la, github.Label{Name: l})
	}
	return la, nil
}

func TestAddLifecycleLabels(t *testing.T) {
	var testcases = []struct {
		name    string
		body    string
		added   []string
		removed []string
		labels  []string
	}{
		{
			name:    "random command -> no-op",
			body:    "/random-command",
			added:   []string{},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "remove lifecycle but don't specify state -> no-op",
			body:    "/remove-lifecycle",
			added:   []string{},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "add lifecycle but don't specify state -> no-op",
			body:    "/lifecycle",
			added:   []string{},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "add lifecycle random -> no-op",
			body:    "/lifecycle random",
			added:   []string{},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "remove lifecycle random -> no-op",
			body:    "/remove-lifecycle random",
			added:   []string{},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "add frozen and stale with single command -> no-op",
			body:    "/lifecycle frozen stale",
			added:   []string{},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "add frozen and random with single command -> no-op",
			body:    "/lifecycle frozen random",
			added:   []string{},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "add frozen, don't have it -> frozen added",
			body:    "/lifecycle frozen",
			added:   []string{labels.LifecycleFrozen},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "add stale, don't have it -> stale added",
			body:    "/lifecycle stale",
			added:   []string{labels.LifecycleStale},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "add rotten, don't have it -> rotten added",
			body:    "/lifecycle rotten",
			added:   []string{labels.LifecycleRotten},
			removed: []string{},
			labels:  []string{},
		},
		{
			name:    "remove frozen, have it -> frozen removed",
			body:    "/remove-lifecycle frozen",
			added:   []string{},
			removed: []string{labels.LifecycleFrozen},
			labels:  []string{labels.LifecycleFrozen},
		},
		{
			name:    "remove stale, have it -> stale removed",
			body:    "/remove-lifecycle stale",
			added:   []string{},
			removed: []string{labels.LifecycleStale},
			labels:  []string{labels.LifecycleStale},
		},
		{
			name:    "remove rotten, have it -> rotten removed",
			body:    "/remove-lifecycle rotten",
			added:   []string{},
			removed: []string{labels.LifecycleRotten},
			labels:  []string{labels.LifecycleRotten},
		},
		{
			name:    "add frozen but have it -> no-op",
			body:    "/lifecycle frozen",
			added:   []string{},
			removed: []string{},
			labels:  []string{labels.LifecycleFrozen},
		},
		{
			name:    "add stale, have active -> stale added, remove active",
			body:    "/lifecycle stale",
			added:   []string{labels.LifecycleStale},
			removed: []string{labels.LifecycleActive},
			labels:  []string{labels.LifecycleActive},
		},
		{
			name:    "add frozen, have rotten -> frozen added, rotten removed",
			body:    "/lifecycle frozen",
			added:   []string{labels.LifecycleFrozen},
			removed: []string{labels.LifecycleRotten},
			labels:  []string{labels.LifecycleRotten},
		},
		{
			name:    "add rotten, have stale -> rotten added, stale removed",
			body:    "/lifecycle rotten",
			added:   []string{labels.LifecycleRotten},
			removed: []string{labels.LifecycleStale},
			labels:  []string{labels.LifecycleStale},
		},
		{
			name:    "add frozen, have stale and rotten -> frozen added, stale and rotten removed",
			body:    "/lifecycle frozen",
			added:   []string{labels.LifecycleFrozen},
			removed: []string{labels.LifecycleStale, labels.LifecycleRotten},
			labels:  []string{labels.LifecycleStale, labels.LifecycleRotten},
		},
		{
			name:    "remove stale, then remove rotten and then add frozen -> stale and rotten removed, frozen added",
			body:    "/remove-lifecycle stale\n/remove-lifecycle rotten\n/lifecycle frozen",
			added:   []string{labels.LifecycleFrozen},
			removed: []string{labels.LifecycleStale, labels.LifecycleRotten},
			labels:  []string{labels.LifecycleStale, labels.LifecycleRotten},
		},
	}
	for _, tc := range testcases {
		fc := &fakeClient{
			labels:  tc.labels,
			added:   []string{},
			removed: []string{},
		}
		e := &github.GenericCommentEvent{
			Body:   tc.body,
			Action: github.GenericCommentActionCreated,
		}
		err := handle(fc, logrus.WithField("plugin", "fake-lifecyle"), e)
		switch {
		case err != nil:
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		case !reflect.DeepEqual(tc.added, fc.added):
			t.Errorf("%s: added %v != actual %v", tc.name, tc.added, fc.added)
		case !reflect.DeepEqual(tc.removed, fc.removed):
			t.Errorf("%s: removed %v != actual %v", tc.name, tc.removed, fc.removed)
		}
	}
}

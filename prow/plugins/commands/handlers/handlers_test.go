/*
Copyright 2018 The Kubernetes Authors.

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

package handlers

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

type fakeLabelClient struct {
	current, added, removed sets.String
}

func newFakeLabelClient(initial ...string) *fakeLabelClient {
	return &fakeLabelClient{
		current: sets.NewString(initial...),
		added:   sets.NewString(),
		removed: sets.NewString(),
	}
}

func (f *fakeLabelClient) GetIssueLabels(org, repo string, number int) ([]github.Label, error) {
	if org != "org" || repo != "repo" || number != 5 {
		return nil, fmt.Errorf("client received an unexpected org/repo#number: %s/%s#%d", org, repo, number)
	}
	var labels []github.Label
	for _, l := range f.current.List() {
		labels = append(labels, github.Label{Name: l})
	}
	return labels, nil
}

func (f *fakeLabelClient) AddLabel(org, repo string, number int, label string) error {
	if org != "org" || repo != "repo" || number != 5 {
		return nil
	}
	f.current.Insert(label)
	f.added.Insert(label)
	return nil
}

func (f *fakeLabelClient) RemoveLabel(org, repo string, number int, label string) error {
	if org != "org" || repo != "repo" || number != 5 {
		return nil
	}
	var err error
	if !f.current.Has(label) {
		err = &github.LabelNotFound{}
	}
	f.current.Delete(label)
	f.removed.Insert(label)
	return err
}

func TestDoEnsureLabel(t *testing.T) {
	tcs := []struct {
		name          string
		initialLabels []string
		expectedAdded sets.String
	}{
		{
			name:          "has label",
			initialLabels: []string{"foo"},
		},
		{
			name:          "does not have label",
			expectedAdded: sets.NewString("foo"),
		},
	}
	for _, tc := range tcs {
		t.Logf("Test case: %s.", tc.name)

		client := newFakeLabelClient(tc.initialLabels...)
		e := &github.GenericCommentEvent{
			Repo: github.Repo{
				Owner: github.User{
					Login: "org",
				},
				Name: "repo",
			},
			Number: 5,
		}
		if err := doEnsureLabel(client, e, "foo"); err != nil {
			t.Errorf("Unexpected error: %v.", err)
		}
		if !client.added.Equal(tc.expectedAdded) {
			t.Errorf("Expected the following labels to be added: %q, but got %q.", tc.expectedAdded.List(), client.added.List())
		}
	}
}

func TestDoRemoveLabel(t *testing.T) {
	tcs := []struct {
		name            string
		initialLabels   []string
		expectedRemoved sets.String
	}{
		{
			name:            "has label",
			initialLabels:   []string{"foo"},
			expectedRemoved: sets.NewString("foo"),
		},
		{
			name:            "does not have label",
			expectedRemoved: sets.NewString("foo"),
		},
	}

	for _, tc := range tcs {
		t.Logf("Test case: %s.", tc.name)

		client := newFakeLabelClient(tc.initialLabels...)
		e := &github.GenericCommentEvent{
			Repo: github.Repo{
				Owner: github.User{
					Login: "org",
				},
				Name: "repo",
			},
			Number: 5,
		}
		if err := doRemoveLabel(client, e, "foo"); err != nil {
			t.Errorf("Unexpected error: %v.", err)
		}
		if !client.removed.Equal(tc.expectedRemoved) {
			t.Errorf("Expected the following labels to be removed: %q, but got %q.", tc.expectedRemoved.List(), client.removed.List())
		}
	}
}

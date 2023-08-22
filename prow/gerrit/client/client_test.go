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

package client

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	gerrit "github.com/andygrunwald/go-gerrit"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
)

type fgc struct {
	instance string
	changes  map[string][]gerrit.ChangeInfo
	comments map[string]map[string][]gerrit.CommentInfo
}

func (f *fgc) GetRelatedChanges(changeID string, revisionID string) (*gerrit.RelatedChangesInfo, *gerrit.Response, error) {
	return &gerrit.RelatedChangesInfo{}, nil, nil
}

func (f *fgc) ListChangeComments(id string) (*map[string][]gerrit.CommentInfo, *gerrit.Response, error) {
	comments := map[string][]gerrit.CommentInfo{}

	val, ok := f.comments[id]
	if !ok {
		return &comments, nil, nil
	}

	for path, retComments := range val {
		comments[path] = append(comments[path], retComments...)
	}

	return &comments, nil, nil
}

func (f *fgc) SubmitChange(changeID string, input *gerrit.SubmitInput) (*ChangeInfo, *gerrit.Response, error) {
	return nil, nil, nil
}

func TestApplyGlobalConfigOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.txt")
	// Empty opener so *syncTime won't panic.
	opener, err := io.NewOpener(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Failed to create opener: %v", err)
	}

	// Fixed org/repo, as this test doesn't check the output.
	cfg := config.Config{
		ProwConfig: config.ProwConfig{
			Gerrit: config.Gerrit{
				OrgReposConfig: &config.GerritOrgRepoConfigs{
					{
						Org:   "foo1",
						Repos: []string{"bar1"},
					},
				},
			},
		},
	}

	// A thread safe map for checking additionalFunc.
	var mux sync.RWMutex
	records := make(map[string]string)
	setRecond := func(key, val string) {
		mux.Lock()
		defer mux.Unlock()
		records[key] = val
	}
	getRecord := func(key string) string {
		mux.RLock()
		defer mux.RUnlock()
		return records[key]
	}

	tests := []struct {
		name                string
		orgRepoConfigGetter func() *config.GerritOrgRepoConfigs
		lastSyncTracker     *SyncTime
		additionalFunc      func()
		expect              func(t *testing.T)
	}{
		{
			name: "base",
			orgRepoConfigGetter: func() *config.GerritOrgRepoConfigs {
				return cfg.Gerrit.OrgReposConfig
			},
			lastSyncTracker: NewSyncTime(path, opener, context.Background()),
			additionalFunc: func() {
				setRecond("base", "base")
			},
			expect: func(t *testing.T) {
				if got, want := getRecord("base"), "base"; got != want {
					t.Fatalf("Output mismatch. Want: %s, got: %s", want, got)
				}
			},
		},
		{
			name: "nil-lastsynctracker",
			orgRepoConfigGetter: func() *config.GerritOrgRepoConfigs {
				return cfg.Gerrit.OrgReposConfig
			},
			additionalFunc: func() {
				setRecond("nil-lastsynctracker", "nil-lastsynctracker")
			},
			expect: func(t *testing.T) {
				if got, want := getRecord("nil-lastsynctracker"), "nil-lastsynctracker"; got != want {
					t.Fatalf("Output mismatch. Want: %s, got: %s", want, got)
				}
			},
		},
		{
			name: "empty-addtionalfunc",
			orgRepoConfigGetter: func() *config.GerritOrgRepoConfigs {
				return cfg.Gerrit.OrgReposConfig
			},
			additionalFunc: func() {},
			expect: func(t *testing.T) {
				// additionalFunc is nil, there is nothing expected
			},
		},
		{
			name: "nil-addtionalfunc",
			orgRepoConfigGetter: func() *config.GerritOrgRepoConfigs {
				return cfg.Gerrit.OrgReposConfig
			},
			additionalFunc: nil,
			expect: func(t *testing.T) {
				// additionalFunc is nil, there is nothing expected
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fc := &Client{}
			fc.applyGlobalConfigOnce(tc.orgRepoConfigGetter, tc.lastSyncTracker, "", "", tc.additionalFunc)
			if tc.expect != nil {
				tc.expect(t)
			}
		})
	}
}

func TestQueryStringsFromQueryFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		filters  *config.GerritQueryFilter
		expected []string
	}{
		{
			name: "nil",
		},
		{
			name: "single-branch",
			filters: &config.GerritQueryFilter{
				Branches: []string{"foo"},
			},
			expected: []string{"(branch:foo)"},
		},
		{
			name: "multiple-branches",
			filters: &config.GerritQueryFilter{
				Branches: []string{"foo1", "foo2", "foo3"},
			},
			expected: []string{"(branch:foo1+OR+branch:foo2+OR+branch:foo3)"},
		},
		{
			name: "branches-and-excluded",
			filters: &config.GerritQueryFilter{
				Branches:         []string{"foo1", "foo2", "foo3"},
				ExcludedBranches: []string{"bar1", "bar2", "bar3"},
			},
			expected: []string{
				"(branch:foo1+OR+branch:foo2+OR+branch:foo3)",
				"(-branch:bar1+AND+-branch:bar2+AND+-branch:bar3)",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tc.expected, queryStringsFromQueryFilter(tc.filters)); diff != "" {
				t.Fatalf("Output mismatch. Want(-), got(+):\n%s", diff)
			}
		})
	}
}

func (f *fgc) QueryChanges(opt *gerrit.QueryChangeOptions) (*[]gerrit.ChangeInfo, *gerrit.Response, error) {
	changes := []gerrit.ChangeInfo{}

	changeInfos, ok := f.changes[f.instance]
	if !ok {
		return &changes, nil, nil
	}

	project := ""
	for _, query := range opt.Query {
		for _, q := range strings.Split(query, "+") {
			if strings.HasPrefix(q, "project:") {
				project = q[8:]
			}
		}
	}

	for idx, change := range changeInfos {
		if idx >= opt.Start && len(changes) <= opt.Limit {
			if project == change.Project {
				changes = append(changes, change)
			}
		}
	}

	return &changes, nil, nil
}

func (f *fgc) SetReview(changeID, revisionID string, input *gerrit.ReviewInput) (*gerrit.ReviewResult, *gerrit.Response, error) {
	return nil, nil, nil
}

func (f *fgc) GetChange(changeId string, opt *gerrit.ChangeOptions) (*ChangeInfo, *gerrit.Response, error) {
	return nil, nil, nil
}

func makeStamp(t time.Time) gerrit.Timestamp {
	return gerrit.Timestamp{Time: t}
}

func newStamp(t time.Time) *gerrit.Timestamp {
	gt := makeStamp(t)
	return &gt
}

func TestUpdateClients(t *testing.T) {
	tests := []struct {
		name              string
		existingInstances map[string]map[string]*config.GerritQueryFilter
		newInstances      map[string]map[string]*config.GerritQueryFilter
		wantInstances     map[string]map[string]*config.GerritQueryFilter
	}{
		{
			name:              "normal",
			existingInstances: map[string]map[string]*config.GerritQueryFilter{"foo1": {"bar1": nil}},
			newInstances:      map[string]map[string]*config.GerritQueryFilter{"foo2": {"bar2": nil}},
			wantInstances:     map[string]map[string]*config.GerritQueryFilter{"foo2": {"bar2": nil}},
		},
		{
			name:              "same instance",
			existingInstances: map[string]map[string]*config.GerritQueryFilter{"foo1": {"bar1": nil}},
			newInstances:      map[string]map[string]*config.GerritQueryFilter{"foo1": {"bar2": nil}},
			wantInstances:     map[string]map[string]*config.GerritQueryFilter{"foo1": {"bar2": nil}},
		},
		{
			name:              "delete",
			existingInstances: map[string]map[string]*config.GerritQueryFilter{"foo1": {"bar1": nil}, "foo2": {"bar2": nil}},
			newInstances:      map[string]map[string]*config.GerritQueryFilter{"foo1": {"bar1": nil}},
			wantInstances:     map[string]map[string]*config.GerritQueryFilter{"foo1": {"bar1": nil}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &Client{
				handlers: make(map[string]*gerritInstanceHandler),
			}
			for instance, projects := range tc.existingInstances {
				client.handlers[instance] = &gerritInstanceHandler{
					instance: instance,
					projects: projects,
				}
			}

			if err := client.UpdateClients(tc.newInstances); err != nil {
				t.Fatal(err)
			}
			gotInstances := make(map[string]map[string]*config.GerritQueryFilter)
			for instance, handler := range client.handlers {
				gotInstances[instance] = handler.projects
			}
			if diff := cmp.Diff(tc.wantInstances, gotInstances); diff != "" {
				t.Fatalf("mismatch. got(+), want(-):\n%s", diff)
			}
		})
	}
}

func TestDedupeIntoResult(t *testing.T) {
	var testcases = []struct {
		name  string
		input []gerrit.ChangeInfo
		want  []gerrit.ChangeInfo
	}{
		{
			name:  "no changes",
			input: []gerrit.ChangeInfo{},
			want:  []gerrit.ChangeInfo{},
		},
		{
			name: "no dupes",
			input: []gerrit.ChangeInfo{
				{
					Number:          1,
					CurrentRevision: "1-1",
				},
				{
					Number:          2,
					CurrentRevision: "2-1",
				},
			},
			want: []gerrit.ChangeInfo{
				{
					Number:          1,
					CurrentRevision: "1-1",
				},
				{
					Number:          2,
					CurrentRevision: "2-1",
				},
			},
		},
		{
			name: "single dupe",
			input: []gerrit.ChangeInfo{
				{
					Number:          1,
					CurrentRevision: "1-1",
				},
				{
					Number:          2,
					CurrentRevision: "2-1",
				},
				{
					Number:          1,
					CurrentRevision: "1-2",
				},
			},
			want: []gerrit.ChangeInfo{
				{
					Number:          2,
					CurrentRevision: "2-1",
				},
				{
					Number:          1,
					CurrentRevision: "1-2",
				},
			},
		},
		{
			name: "many dupes",
			input: []gerrit.ChangeInfo{
				{
					Number:          1,
					CurrentRevision: "1-1",
				},
				{
					Number:          2,
					CurrentRevision: "2-1",
				},
				{
					Number:          1,
					CurrentRevision: "1-2",
				},
				{
					Number:          2,
					CurrentRevision: "2-2",
				},
				{
					Number:          1,
					CurrentRevision: "1-3",
				},
				{
					Number:          1,
					CurrentRevision: "1-4",
				},
				{
					Number:          3,
					CurrentRevision: "3-1",
				},
			},
			want: []gerrit.ChangeInfo{
				{
					Number:          2,
					CurrentRevision: "2-2",
				},
				{
					Number:          1,
					CurrentRevision: "1-4",
				},
				{
					Number:          3,
					CurrentRevision: "3-1",
				},
			},
		},
	}

	for _, tc := range testcases {
		deduper := &deduper{
			result:  []gerrit.ChangeInfo{},
			seenPos: make(map[int]int),
		}

		for _, ci := range tc.input {
			deduper.dedupeIntoResult(ci)
		}

		if diff := cmp.Diff(tc.want, deduper.result); diff != "" {
			t.Fatalf("Output mismatch. Want(-), got(+):\n%s", diff)
		}
	}
}

func TestQueryChange(t *testing.T) {
	now := time.Now().UTC()

	var testcases = []struct {
		name       string
		lastUpdate map[string]time.Time
		changes    map[string][]gerrit.ChangeInfo
		comments   map[string]map[string][]gerrit.CommentInfo
		// expected
		revisions map[string][]string
		messages  map[string][]gerrit.ChangeMessageInfo
	}{
		{
			name: "no changes",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			revisions: map[string][]string{},
		},
		{
			name: "one outdated change",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now.Add(-time.Hour)),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now.Add(-time.Hour)),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{},
		},
		{
			name: "find comments in special patchset file",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "bar~branch~random-string",
						Number:          1,
						ChangeID:        "random-string",
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now.Add(-time.Hour)),
								Number:  1,
							},
						},
						Status: "NEW",
						Messages: []gerrit.ChangeMessageInfo{
							{
								Date:           makeStamp(now),
								Message:        "first",
								RevisionNumber: 1,
							},
							{
								Date:           makeStamp(now.Add(2 * time.Second)),
								Message:        "second",
								RevisionNumber: 1,
							},
						},
					},
				},
			},
			comments: map[string]map[string][]gerrit.CommentInfo{
				"bar~branch~random-string": {
					"/PATCHSET_LEVEL": {
						{
							Message:  "before",
							Updated:  newStamp(now.Add(-time.Second)),
							PatchSet: 1,
						},
						{
							Message:  "after",
							Updated:  newStamp(now.Add(time.Second)),
							PatchSet: 1,
						},
					},
					"random.yaml": {
						{
							Message:  "ignore this file",
							Updated:  newStamp(now.Add(-time.Second)),
							PatchSet: 1,
						},
					},
				},
			},
			revisions: map[string][]string{"foo": {"1-1"}},
			messages: map[string][]gerrit.ChangeMessageInfo{
				"random-string": {
					{
						Date:           makeStamp(now.Add(-time.Second)),
						Message:        "before",
						RevisionNumber: 1,
					},
					{
						Date:           makeStamp(now),
						Message:        "first",
						RevisionNumber: 1,
					},
					{
						Date:           makeStamp(now.Add(time.Second)),
						Message:        "after",
						RevisionNumber: 1,
					},
					{
						Date:           makeStamp(now.Add(2 * time.Second)),
						Message:        "second",
						RevisionNumber: 1,
					},
				},
			},
		},
		{
			name: "one outdated change, but there's a new message",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "100",
						Number:          100,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now.Add(-time.Hour)),
								Number:  1,
							},
						},
						Status: "NEW",
						Messages: []gerrit.ChangeMessageInfo{
							{
								Date:           makeStamp(now),
								Message:        "some message",
								RevisionNumber: 1,
							},
						},
					},
				},
			},
			revisions: map[string][]string{"foo": {"1-1"}},
		},
		{
			name: "one up-to-date change",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"1-1"},
			},
		},
		{
			name: "one up-to-date change, same timestamp",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"1-1"},
			},
		},
		{
			name: "one up-to-date change but stale commit",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now.Add(-time.Hour)),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{},
		},
		{
			name: "one up-to-date change, wrong instance",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"evil": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{},
		},
		{
			name: "one up-to-date change, wrong project",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "evil",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{},
		},
		{
			name: "two up-to-date changes, two projects",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
					{
						Project:         "bar",
						ID:              "2",
						Number:          2,
						CurrentRevision: "2-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"2-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"1-1", "2-1"},
			},
		},
		{
			name: "one good one bad",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
					{
						Project:         "bar",
						ID:              "2",
						Number:          2,
						CurrentRevision: "2-1",
						Updated:         makeStamp(now.Add(-time.Hour)),
						Revisions: map[string]gerrit.RevisionInfo{
							"2-1": {
								Created: makeStamp(now.Add(-time.Hour)),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"1-1"},
			},
		},
		{
			name: "multiple up-to-date changes",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
				"boo": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
					{
						Project:         "bar",
						ID:              "2",
						Number:          2,
						CurrentRevision: "2-1",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"2-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
				},
				"baz": {
					{
						Project:         "boo",
						ID:              "3",
						Number:          3,
						CurrentRevision: "3-2",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"3-2": {
								Created: makeStamp(now),
							},
							"3-1": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
					{
						Project:         "evil",
						ID:              "4",
						Number:          4,
						CurrentRevision: "4-1",
						Updated:         makeStamp(now.Add(-time.Hour)),
						Revisions: map[string]gerrit.RevisionInfo{
							"4-1": {
								Created: makeStamp(now.Add(-time.Hour)),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"1-1", "2-1"},
				"baz": {"3-2"},
			},
		},
		{
			name: "one up-to-date merged change",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Submitted:       newStamp(now),
						Status:          "MERGED",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"1-1"},
			},
		},
		{
			name: "one up-to-date abandoned change",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Submitted:       newStamp(now),
						Status:          "ABANDONED",
					},
				},
			},
			revisions: map[string][]string{},
		},
		{
			name: "merged change recently updated but submitted before last update",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Submitted:       newStamp(now.Add(-2 * time.Minute)),
						Status:          "MERGED",
					},
				},
			},
			revisions: map[string][]string{},
		},
		{
			name: "one abandoned, one merged",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now),
						Status:          "ABANDONED",
					},
					{
						Project:         "bar",
						ID:              "2",
						CurrentRevision: "2-1",
						Updated:         makeStamp(now),
						Submitted:       newStamp(now),
						Status:          "MERGED",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"2-1"},
			},
		},
		{
			name: "merged change with new message, should ignore",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Minute),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "2",
						Number:          2,
						CurrentRevision: "2-1",
						Updated:         makeStamp(now),
						Submitted:       newStamp(now.Add(-time.Hour)),
						Status:          "MERGED",
						Messages: []gerrit.ChangeMessageInfo{
							{
								Date:           makeStamp(now),
								Message:        "some message",
								RevisionNumber: 1,
							},
						},
					},
				},
			},
			revisions: map[string][]string{},
		},
		{
			name: "one up-to-date change found twice due to pagination. Duplicate should be removed",
			lastUpdate: map[string]time.Time{
				"bar": now.Add(-time.Hour),
			},
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-1",
						Updated:         makeStamp(now.Add(-time.Minute)),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now.Add(-time.Minute)),
							},
						},
						Status: "NEW",
					},
					{
						Project:         "bar",
						ID:              "2",
						Number:          2,
						CurrentRevision: "2-1",
						Updated:         makeStamp(now.Add(-time.Minute)),
						Revisions: map[string]gerrit.RevisionInfo{
							"2-1": {
								Created: makeStamp(now.Add(-time.Minute)),
							},
						},
						Status: "NEW",
					},
					{
						Project:         "bar",
						ID:              "1",
						Number:          1,
						CurrentRevision: "1-2",
						Updated:         makeStamp(now),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: makeStamp(now.Add(-time.Minute)),
							},
							"1-2": {
								Created: makeStamp(now),
							},
						},
						Status: "NEW",
					},
				},
			},
			revisions: map[string][]string{
				"foo": {"2-1", "1-2"},
			},
		},
	}

	for _, tc := range testcases {
		client := &Client{
			handlers: map[string]*gerritInstanceHandler{
				"foo": {
					instance: "foo",
					projects: map[string]*config.GerritQueryFilter{"bar": nil},
					changeService: &fgc{
						changes:  tc.changes,
						instance: "foo",
						comments: tc.comments,
					},
					log: logrus.WithField("host", "foo"),
				},
				"baz": {
					instance: "baz",
					projects: map[string]*config.GerritQueryFilter{"boo": nil},
					changeService: &fgc{
						changes:  tc.changes,
						instance: "baz",
					},
					log: logrus.WithField("host", "baz"),
				},
			},
		}

		testLastSync := LastSyncState{"foo": tc.lastUpdate, "baz": tc.lastUpdate}
		changes := client.QueryChanges(testLastSync, 2)

		revisions := map[string][]string{}
		messages := map[string][]gerrit.ChangeMessageInfo{}
		seen := sets.NewInt()
		for instance, changes := range changes {
			revisions[instance] = []string{}
			for _, change := range changes {
				if seen.Has(change.Number) {
					t.Errorf("Change number %d appears multiple times in the query results.", change.Number)
				}
				seen.Insert(change.Number)
				revisions[instance] = append(revisions[instance], change.CurrentRevision)
				messages[change.ChangeID] = append(messages[change.ChangeID], change.Messages...)
			}
		}

		if !reflect.DeepEqual(revisions, tc.revisions) {
			t.Errorf("tc %s - wrong revisions: got %#v, expect %#v", tc.name, revisions, tc.revisions)
		}

		if tc.messages != nil && !reflect.DeepEqual(messages, tc.messages) {
			t.Errorf("tc %s - wrong messages:\nhave %#v,\nwant %#v", tc.name, messages, tc.messages)
		}
	}
}

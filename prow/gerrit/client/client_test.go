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
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	gerrit "github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
)

type fgc struct {
	instance string
	changes  map[string][]gerrit.ChangeInfo
	comments map[string]map[string][]gerrit.CommentInfo
}

func (f *fgc) ListChangeComments(id string) (*map[string][]gerrit.CommentInfo, *gerrit.Response, error) {
	comments := map[string][]gerrit.CommentInfo{}

	val, ok := f.comments[id]
	if !ok {
		return &comments, nil, nil
	}

	for path, retComments := range val {
		for _, comment := range retComments {
			comments[path] = append(comments[path], comment)
		}
	}

	return &comments, nil, nil

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

func makeStamp(t time.Time) gerrit.Timestamp {
	return gerrit.Timestamp{Time: t}
}

func newStamp(t time.Time) *gerrit.Timestamp {
	gt := makeStamp(t)
	return &gt
}

func TestQueryChange(t *testing.T) {
	now := time.Now().UTC()

	var testcases = []struct {
		name       string
		lastUpdate map[string]time.Time
		changes    map[string][]gerrit.ChangeInfo
		comments   map[string]map[string][]gerrit.CommentInfo
		revisions  map[string][]string
		messages   map[string][]gerrit.ChangeMessageInfo
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
	}

	for _, tc := range testcases {
		client := &Client{
			handlers: map[string]*gerritInstanceHandler{
				"foo": {
					instance: "foo",
					projects: []string{"bar"},
					changeService: &fgc{
						changes:  tc.changes,
						instance: "foo",
						comments: tc.comments,
					},
					log: logrus.WithField("host", "foo"),
				},
				"baz": {
					instance: "baz",
					projects: []string{"boo"},
					changeService: &fgc{
						changes:  tc.changes,
						instance: "baz",
					},
					log: logrus.WithField("host", "baz"),
				},
			},
		}

		testLastSync := LastSyncState{"foo": tc.lastUpdate, "baz": tc.lastUpdate}
		changes := client.QueryChanges(testLastSync, 5)

		revisions := map[string][]string{}
		messages := map[string][]gerrit.ChangeMessageInfo{}
		for instance, changes := range changes {
			revisions[instance] = []string{}
			for _, change := range changes {
				revisions[instance] = append(revisions[instance], change.CurrentRevision)
				for _, m := range change.Messages {
					messages[change.ChangeID] = append(messages[change.ChangeID], m)
				}
			}
			sort.Strings(revisions[instance])
		}

		if !reflect.DeepEqual(revisions, tc.revisions) {
			t.Errorf("tc %s - wrong revisions: got %#v, expect %#v", tc.name, revisions, tc.revisions)
		}

		if tc.messages != nil && !reflect.DeepEqual(messages, tc.messages) {
			t.Errorf("tc %s - wrong messages:\nhave %#v,\nwant %#v", tc.name, messages, tc.messages)
		}
	}
}

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

package gerrit

import (
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

func (f *fca) Config() *config.Config {
	f.Lock()
	defer f.Unlock()
	return f.c
}

type fkc struct {
	sync.Mutex
	prowjobs []kube.ProwJob
}

func (f *fkc) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	f.prowjobs = append(f.prowjobs, pj)
	return pj, nil
}

type fgc struct {
	changes  map[string][]gerrit.ChangeInfo
	instance string
}

func (f *fgc) QueryChanges(opt *gerrit.QueryChangeOptions) (*[]gerrit.ChangeInfo, *gerrit.Response, error) {
	changes := []gerrit.ChangeInfo{}

	project := ""
	for _, query := range opt.Query {
		for _, q := range strings.Split(query, "+") {
			if strings.HasPrefix(q, "project:") {
				project = q[8:]
			}
		}
	}

	if changeInfos, ok := f.changes[project]; !ok {
		return &changes, nil, nil
	} else {
		for idx, change := range changeInfos {
			if idx >= opt.Start && len(changes) <= opt.Limit {
				changes = append(changes, change)
			}
		}
	}

	return &changes, nil, nil
}

func (f *fgc) SetReview(changeID, revisionID string, input *gerrit.ReviewInput) (*gerrit.ReviewResult, *gerrit.Response, error) {
	return nil, nil, nil
}

func TestQueryChange(t *testing.T) {
	now := time.Now().UTC()
	layout := "2006-01-02 15:04:05"

	var testcases = []struct {
		name       string
		limit      int
		lastUpdate time.Time
		changes    map[string][]gerrit.ChangeInfo
		revisions  []string
	}{
		{
			name:       "no changes",
			limit:      2,
			lastUpdate: now,
			revisions:  []string{},
		},
		{
			name:       "one outdated change",
			limit:      2,
			lastUpdate: now,
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						ID:              "1",
						CurrentRevision: "1-1",
						Updated:         now.Add(-time.Hour).Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: now.Add(-time.Hour).Format(layout),
							},
						},
					},
				},
			},
			revisions: []string{},
		},
		{
			name:       "one up-to-date change",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						ID:              "1",
						CurrentRevision: "1-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: now.Format(layout),
							},
						},
					},
				},
			},
			revisions: []string{"1-1"},
		},
		{
			name:       "one up-to-date change but stale commit",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						ID:              "1",
						CurrentRevision: "1-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: now.Add(-time.Hour).Format(layout),
							},
						},
					},
				},
			},
			revisions: []string{},
		},
		{
			name:       "one up-to-date change, wrong project",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: map[string][]gerrit.ChangeInfo{
				"evil": {
					{
						ID:              "1",
						CurrentRevision: "1-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: now.Format(layout),
							},
						},
					},
				},
			},
			revisions: []string{},
		},
		{
			name:       "two up-to-date changes, two projects",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						ID:              "1",
						CurrentRevision: "1-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: now.Format(layout),
							},
						},
					},
				},
				"bar": {
					{
						ID:              "2",
						CurrentRevision: "2-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"2-1": {
								Created: now.Format(layout),
							},
						},
					},
				},
			},
			revisions: []string{"1-1", "2-1"},
		},
		{
			name:       "one good one bad",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						ID:              "1",
						CurrentRevision: "1-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: now.Format(layout),
							},
						},
					},
					{
						ID:              "2",
						CurrentRevision: "2-1",
						Updated:         now.Add(-time.Hour).Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"2-1": {
								Created: now.Add(-time.Hour).Format(layout),
							},
						},
					},
				},
			},
			revisions: []string{"1-1"},
		},
		{
			name:       "multiple up-to-date changes",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: map[string][]gerrit.ChangeInfo{
				"foo": {
					{
						ID:              "1",
						CurrentRevision: "1-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"1-1": {
								Created: now.Format(layout),
							},
						},
					},
					{
						ID:              "2",
						CurrentRevision: "2-1",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"2-1": {
								Created: now.Format(layout),
							},
						},
					},
					{
						ID:              "3",
						CurrentRevision: "3-2",
						Updated:         now.Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"3-2": {
								Created: now.Format(layout),
							},
							"3-1": {
								Created: now.Format(layout),
							},
						},
					},
					{
						ID:              "4",
						CurrentRevision: "4-1",
						Updated:         now.Add(-time.Hour).Format(layout),
						Revisions: map[string]gerrit.RevisionInfo{
							"4-1": {
								Created: now.Add(-time.Hour).Format(layout),
							},
						},
					},
				},
			},
			revisions: []string{"1-1", "2-1", "3-2"},
		},
	}

	for _, tc := range testcases {
		fgc := &fgc{
			changes: tc.changes,
		}

		fca := &fca{
			c: &config.Config{
				ProwConfig: config.ProwConfig{
					Gerrit: config.Gerrit{
						RateLimit: tc.limit,
					},
				},
			},
		}

		c := &Controller{
			ca:         fca,
			gc:         fgc,
			projects:   []string{"foo", "bar"},
			lastUpdate: tc.lastUpdate,
		}

		changes := c.QueryChanges()

		revisions := []string{}
		for _, change := range changes {
			revisions = append(revisions, change.CurrentRevision)
		}
		sort.Strings(revisions)

		if !reflect.DeepEqual(revisions, tc.revisions) {
			t.Errorf("tc %s - wrong revisions: got %#v, expect %#v", tc.name, revisions, tc.revisions)
		}
	}
}

func TestProcessChange(t *testing.T) {
	var testcases = []struct {
		name        string
		change      gerrit.ChangeInfo
		numPJ       int
		pjRef       string
		shouldError bool
	}{
		{
			name: "no revisions",
			change: gerrit.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
			},
			shouldError: true,
		},
		{
			name: "wrong project",
			change: gerrit.ChangeInfo{
				CurrentRevision: "1",
				Project:         "woof",
				Revisions: map[string]gerrit.RevisionInfo{
					"1": {},
				},
			},
		},
		{
			name: "normal",
			change: gerrit.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Revisions: map[string]gerrit.RevisionInfo{
					"1": {
						Ref: "refs/changes/00/1/1",
					},
				},
			},
			numPJ: 1,
			pjRef: "refs/changes/00/1/1",
		},
		{
			name: "multiple revisions",
			change: gerrit.ChangeInfo{
				CurrentRevision: "2",
				Project:         "test-infra",
				Revisions: map[string]gerrit.RevisionInfo{
					"1": {
						Ref: "refs/changes/00/2/1",
					},
					"2": {
						Ref: "refs/changes/00/2/2",
					},
				},
			},
			numPJ: 1,
			pjRef: "refs/changes/00/2/2",
		},
	}

	for _, tc := range testcases {
		fca := &fca{
			c: &config.Config{
				JobConfig: config.JobConfig{
					Presubmits: map[string][]config.Presubmit{
						"gerrit/test-infra": {
							{
								Name: "test-foo",
							},
						},
					},
				},
			},
		}

		fkc := &fkc{}
		fgc := &fgc{}

		c := &Controller{
			ca:       fca,
			kc:       fkc,
			gc:       fgc,
			instance: "gerrit",
		}

		err := c.ProcessChange(tc.change)
		if err != nil && !tc.shouldError {
			t.Errorf("tc %s, expect no error, but got %v", tc.name, err)
			continue
		} else if err == nil && tc.shouldError {
			t.Errorf("tc %s, expect error, but got none", tc.name)
			continue
		}

		if len(fkc.prowjobs) != tc.numPJ {
			t.Errorf("tc %s - should make %d prowjob, got %d", tc.name, tc.numPJ, len(fkc.prowjobs))
		}

		if len(fkc.prowjobs) > 0 {
			if fkc.prowjobs[0].Spec.Refs.Pulls[0].Ref != tc.pjRef {
				t.Errorf("tc %s - ref should be %s, got %s", tc.name, tc.pjRef, fkc.prowjobs[0].Spec.Refs.Pulls[0].Ref)
			}
		}
	}
}

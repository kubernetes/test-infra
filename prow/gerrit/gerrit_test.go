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
	changes  []gerrit.ChangeInfo
	instance string
}

func (f *fgc) QueryChanges(opt *gerrit.QueryChangeOptions) (*[]gerrit.ChangeInfo, *gerrit.Response, error) {
	changes := []gerrit.ChangeInfo{}

	for idx, change := range f.changes {
		if idx >= opt.Start && len(changes) <= opt.Limit {
			changes = append(changes, change)
		}
	}

	return &changes, nil, nil
}

func TestQueryChange(t *testing.T) {
	now := time.Now().UTC()
	layout := "2006-01-02 15:04:05"

	var testcases = []struct {
		name        string
		limit       int
		lastUpdate  time.Time
		changes     []gerrit.ChangeInfo
		revisions   []string
		shouldError bool
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
			changes: []gerrit.ChangeInfo{
				{
					ID:              "1",
					CurrentRevision: "1-1",
					Updated:         now.Add(-time.Hour).Format(layout),
				},
			},
			revisions: []string{},
		},
		{
			name:       "one up-to-date change",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: []gerrit.ChangeInfo{
				{
					ID:              "1",
					CurrentRevision: "1-1",
					Updated:         now.Format(layout),
				},
			},
			revisions: []string{"1-1"},
		},
		{
			name:       "one good one bad",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: []gerrit.ChangeInfo{
				{
					ID:              "1",
					CurrentRevision: "1-1",
					Updated:         now.Format(layout),
				},
				{
					ID:              "2",
					CurrentRevision: "2-1",
					Updated:         now.Add(-time.Hour).Format(layout),
				},
			},
			revisions: []string{"1-1"},
		},
		{
			name:       "multiple up-to-date changes",
			limit:      2,
			lastUpdate: now.Add(-time.Minute),
			changes: []gerrit.ChangeInfo{
				{
					ID:              "1",
					CurrentRevision: "1-1",
					Updated:         now.Format(layout),
				},
				{
					ID:              "2",
					CurrentRevision: "2-1",
					Updated:         now.Format(layout),
				},
				{
					ID:              "3",
					CurrentRevision: "3-2",
					Updated:         now.Format(layout),
				},
				{
					ID:              "3",
					CurrentRevision: "3-1",
					Updated:         now.Format(layout),
				},
				{
					ID:              "4",
					CurrentRevision: "4-1",
					Updated:         now.Add(-time.Hour).Format(layout),
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
				Gerrit: config.Gerrit{
					RateLimit: tc.limit,
				},
			},
		}

		c := &Controller{
			ca:         fca,
			gc:         fgc,
			lastUpdate: tc.lastUpdate,
		}

		changes, err := c.QueryChanges()
		if err != nil && !tc.shouldError {
			t.Errorf("tc %s, expect no error, but got %v", tc.name, err)
			continue
		} else if err == nil && tc.shouldError {
			t.Errorf("tc %s, expect error, but got none", tc.name)
			continue
		}

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
					"1": {
						Ref: "foo",
					},
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
						Ref: "foo",
					},
				},
			},
			numPJ: 1,
			pjRef: "foo",
		},
		{
			name: "multiple revisions",
			change: gerrit.ChangeInfo{
				CurrentRevision: "2",
				Project:         "test-infra",
				Revisions: map[string]gerrit.RevisionInfo{
					"1": {
						Ref: "foo",
					},
					"2": {
						Ref: "bar",
					},
				},
			},
			numPJ: 1,
			pjRef: "bar",
		},
	}

	for _, tc := range testcases {
		fca := &fca{
			c: &config.Config{
				Presubmits: map[string][]config.Presubmit{
					"gerrit/test-infra": {
						{
							Name: "test-foo",
						},
					},
				},
			},
		}

		fkc := &fkc{}

		c := &Controller{
			ca:       fca,
			kc:       fkc,
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
			if fkc.prowjobs[0].Spec.Refs.Pulls[0].SHA != tc.pjRef {
				t.Errorf("tc %s - ref should be %s, got %s", tc.name, tc.pjRef, fkc.prowjobs[0].Spec.Refs.BaseRef)
			}
		}
	}
}

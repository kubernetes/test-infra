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

package adapter

import (
	"sync"
	"testing"
	"time"

	"k8s.io/test-infra/prow/gerrit/client"

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

type fgc struct{}

func (f *fgc) QueryChanges(lastUpdate time.Time, rateLimit int) map[string][]gerrit.ChangeInfo {
	return nil
}

func (f *fgc) SetReview(instance, id, revision, message string) error {
	return nil
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

		c := &Controller{
			ca: fca,
			kc: fkc,
			gc: &fgc{},
		}

		err := c.ProcessChange("gerrit", tc.change)
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

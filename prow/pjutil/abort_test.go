/*
Copyright 2019 The Kubernetes Authors.

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

package pjutil

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestOlderPresubmits(t *testing.T) {
	type fakeJob struct {
		name     string
		complete bool
		kind     prowjobv1.ProwJobType
		number   int
		org      string
		repo     string
		job      string
		start    time.Time
	}

	const (
		presubmit = prowjobv1.PresubmitJob
	)

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	twoDaysAgo := yesterday.Add(-24 * time.Hour)
	cases := []struct {
		name     string
		jobs     []fakeJob
		expected sets.String
	}{
		{
			name: "empty works",
		},
		{
			name: "no duplicates",
			jobs: []fakeJob{
				{
					name: "foo",
					kind: presubmit,
					job:  "foo",
				},
				{
					name: "bar",
					kind: presubmit,
					job:  "bar",
				},
			},
		},
		{
			name: "ignore postsubmits",
			jobs: []fakeJob{
				{
					name: "old",
					kind: prowjobv1.PostsubmitJob,
				},
				{
					name: "new",
					kind: prowjobv1.PostsubmitJob,
				},
			},
		},
		{
			name: "ignore periodics",
			jobs: []fakeJob{
				{
					name: "old",
					kind: prowjobv1.PeriodicJob,
				},
				{
					name: "new",
					kind: prowjobv1.PeriodicJob,
				},
			},
		},
		{
			name: "ignore completed jobs",
			jobs: []fakeJob{
				{
					name:     "aborted",
					complete: true,
					kind:     presubmit,
				},
				{
					name: "current",
					kind: presubmit,
				},
			},
		},
		{
			name: "ignore different repos",
			jobs: []fakeJob{
				{
					name: "foo",
					repo: "foo",
					kind: presubmit,
				},
				{
					name: "bar",
					repo: "bar",
					kind: presubmit,
				},
			},
		},
		{
			name: "ignore different orgs",
			jobs: []fakeJob{
				{
					name: "foo",
					org:  "foo",
					kind: presubmit,
				},
				{
					name: "bar",
					org:  "bar",
					kind: presubmit,
				},
			},
		},
		{
			name: "ignore different PRs",
			jobs: []fakeJob{
				{
					name:   "one",
					number: 1,
					kind:   presubmit,
				},
				{
					name:   "two",
					number: 2,
					kind:   presubmit,
				},
			},
		},
		{
			name: "identify duplicates",
			jobs: []fakeJob{
				{
					name:  "older",
					kind:  presubmit,
					start: twoDaysAgo,
				},
				{
					name:  "new",
					kind:  presubmit,
					start: now,
				},
				{
					name:  "old",
					kind:  presubmit,
					start: yesterday,
				},
			},
			expected: sets.NewString("older", "old"),
		},
		{
			name: "multiple duplicated jobs",
			jobs: []fakeJob{
				{
					name:  "old-foo",
					kind:  presubmit,
					job:   "foo",
					start: yesterday,
				},
				{
					name:  "new-foo",
					kind:  presubmit,
					job:   "foo",
					start: now,
				},
				{
					name:  "new-bar",
					kind:  presubmit,
					job:   "bar",
					start: now,
				},
				{
					name:  "old-bar",
					kind:  presubmit,
					job:   "bar",
					start: yesterday,
				},
				{
					name: "ignore",
					kind: presubmit,
					job:  "ignore",
				},
			},
			expected: sets.NewString("old-foo", "old-bar"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pjs []prowjobv1.ProwJob
			for _, j := range tc.jobs {
				var pj prowjobv1.ProwJob
				pj.Name = j.name
				pj.Spec.Job = j.job
				pj.Spec.Type = j.kind
				if j.complete {
					pj.SetComplete()
				}
				pj.Spec.Refs = &prowjobv1.Refs{
					Org:  j.org,
					Repo: j.repo,
					Pulls: []prowjobv1.Pull{
						{
							Number: j.number,
						},
					},
				}
				pj.Status.StartTime = metav1.NewTime(j.start)
				pjs = append(pjs, pj)
			}
			actual := sets.String{}
			dups := olderPresubmits(pjs)
			for _, d := range dups {
				actual.Insert(d.Name)
			}
			if tc.expected == nil {
				tc.expected = sets.String{}
			}
			if !tc.expected.Equal(actual) {
				t.Errorf("%v != expected %v", actual, tc.expected)
			}
		})
	}
}

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
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestTerminateOlderJobs(t *testing.T) {
	fakePJNS := "prow-job"
	now := time.Now()
	nowFn := func() *metav1.Time {
		reallyNow := metav1.NewTime(now)
		return &reallyNow
	}
	cases := []struct {
		name               string
		pjs                []prowv1.ProwJob
		expectedAbortedPJs sets.String
	}{
		{
			name: "terminate all older presubmit jobs",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "postsubmit", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PostsubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			expectedAbortedPJs: sets.NewString("old", "older"),
		},
		{
			name: "Don't terminate older batch jobs",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.BatchJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.BatchJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.BatchJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "postsubmit", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PostsubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.BatchJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			expectedAbortedPJs: sets.NewString(),
		},
		{
			name: "terminate older jobs with different orders of refs",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}, {Number: 2}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 2}, {Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			expectedAbortedPJs: sets.NewString("old"),
		},
		{
			name: "terminate older jobs with different orders of extra refs",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
						ExtraRefs: []prowv1.Refs{
							{
								Repo:  "other",
								Pulls: []prowv1.Pull{{Number: 2}},
							},
							{
								Repo:  "something",
								Pulls: []prowv1.Pull{{Number: 3}},
							},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1}},
						},
						ExtraRefs: []prowv1.Refs{
							{
								Repo:  "something",
								Pulls: []prowv1.Pull{{Number: 3}},
							},
							{
								Repo:  "other",
								Pulls: []prowv1.Pull{{Number: 2}},
							},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			expectedAbortedPJs: sets.NewString("old"),
		},
		{
			name: "terminate older jobs with no main refs, only extra refs",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						ExtraRefs: []prowv1.Refs{
							{
								Repo:  "test",
								Pulls: []prowv1.Pull{{Number: 1}},
							},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						ExtraRefs: []prowv1.Refs{
							{
								Repo:  "test",
								Pulls: []prowv1.Pull{{Number: 1}},
							},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			expectedAbortedPJs: sets.NewString("old"),
		},
		{
			name: "terminate older jobs with different base SHA",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:    "test",
							BaseSHA: "foo",
							Pulls:   []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:    "test",
							BaseSHA: "bar",
							Pulls:   []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			expectedAbortedPJs: sets.NewString("old"),
		},
		{
			name: "don't terminate older jobs with different base refs",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:    "test",
							BaseRef: "foo",
							Pulls:   []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:    "test",
							BaseRef: "bar",
							Pulls:   []prowv1.Pull{{Number: 1}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			expectedAbortedPJs: sets.NewString(),
		},
		{
			name: "terminate older jobs with different pull sha",
			pjs: []prowv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1, SHA: "foo"}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowv1.ProwJobSpec{
						Type: prowv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowv1.Refs{
							Repo:  "test",
							Pulls: []prowv1.Pull{{Number: 1, SHA: "bar"}},
						},
					},
					Status: prowv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			expectedAbortedPJs: sets.NewString("old"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var clientPJs []runtime.Object
			var origPJs []prowv1.ProwJob
			for i := range tc.pjs {
				clientPJs = append(clientPJs, &tc.pjs[i])
				origPJs = append(origPJs, tc.pjs[i])
			}
			fakeProwJobClient := fakectrlruntimeclient.NewFakeClient(clientPJs...)
			log := logrus.NewEntry(logrus.StandardLogger())
			if err := TerminateOlderJobs(fakeProwJobClient, log, tc.pjs); err != nil {
				t.Fatalf("%s: error terminating the older presubmit jobs: %v", tc.name, err)
			}

			var actualPJs prowv1.ProwJobList
			if err := fakeProwJobClient.List(context.Background(), &actualPJs); err != nil {
				t.Fatalf("failed to list prowjobs: %v", err)
			}

			actuallyAbortedJobs := sets.String{}
			for _, job := range actualPJs.Items {
				if job.Status.State == prowv1.AbortedState {
					if job.Complete() {
						t.Errorf("job %s was set to complete, TerminateOlderJobs must never set prowjobs as completed", job.Name)
					}
					actuallyAbortedJobs.Insert(job.Name)
				}
			}

			if missing := tc.expectedAbortedPJs.Difference(actuallyAbortedJobs); missing.Len() > 0 {
				t.Errorf("%s: did not replace the expected jobs: %v", tc.name, missing.Len())
			}
			if extra := actuallyAbortedJobs.Difference(tc.expectedAbortedPJs); extra.Len() > 0 {
				t.Errorf("%s: found unexpectedly replaced job: %v", tc.name, extra.List())
			}

			// Validate that terminated PJs are marked terminated in the passed slice.
			// Only consider jobs that we expected to be replaced and that were replaced.
			replacedAsExpected := actuallyAbortedJobs.Intersection(tc.expectedAbortedPJs)
			for i := range origPJs {
				if replacedAsExpected.Has(origPJs[i].Name) {
					if reflect.DeepEqual(origPJs[i], tc.pjs[i]) {
						t.Errorf("%s: job %q was terminated, but not updated in the slice", tc.name, origPJs[i].Name)
					}
				}
			}
		})
	}
}

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
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestTerminateOlderJobs(t *testing.T) {
	fakePJNS := "prow-job"
	now := time.Now()
	nowFn := func() *metav1.Time {
		reallyNow := metav1.NewTime(now)
		return &reallyNow
	}
	cases := []struct {
		name           string
		pjs            []prowjobv1.ProwJob
		terminateddPJs sets.String
	}{
		{
			name: "terminate all older presubmit jobs",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "postsubmit", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PostsubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			terminateddPJs: sets.NewString("old", "older"),
		},
		{
			name: "Don't terminate older batch jobs",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.BatchJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.BatchJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.BatchJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "postsubmit", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PostsubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.BatchJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			terminateddPJs: sets.NewString(),
		},
		{
			name: "terminate older jobs with different orders of refs",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}, {Number: 2}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 2}, {Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			terminateddPJs: sets.NewString("old"),
		},
		{
			name: "terminate older jobs with different orders of extra refs",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
						ExtraRefs: []prowjobv1.Refs{
							{
								Repo:  "other",
								Pulls: []prowjobv1.Pull{{Number: 2}},
							},
							{
								Repo:  "something",
								Pulls: []prowjobv1.Pull{{Number: 3}},
							},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
						ExtraRefs: []prowjobv1.Refs{
							{
								Repo:  "something",
								Pulls: []prowjobv1.Pull{{Number: 3}},
							},
							{
								Repo:  "other",
								Pulls: []prowjobv1.Pull{{Number: 2}},
							},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			terminateddPJs: sets.NewString("old"),
		},
		{
			name: "terminate older jobs with no main refs, only extra refs",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						ExtraRefs: []prowjobv1.Refs{
							{
								Repo:  "test",
								Pulls: []prowjobv1.Pull{{Number: 1}},
							},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						ExtraRefs: []prowjobv1.Refs{
							{
								Repo:  "test",
								Pulls: []prowjobv1.Pull{{Number: 1}},
							},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			terminateddPJs: sets.NewString("old"),
		},
		{
			name: "terminate older jobs with different base SHA",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:    "test",
							BaseSHA: "foo",
							Pulls:   []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:    "test",
							BaseSHA: "bar",
							Pulls:   []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			terminateddPJs: sets.NewString("old"),
		},
		{
			name: "don't terminate older jobs with different base refs",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:    "test",
							BaseRef: "foo",
							Pulls:   []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:    "test",
							BaseRef: "bar",
							Pulls:   []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			terminateddPJs: sets.NewString(),
		},
		{
			name: "terminate older jobs with different pull sha",
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1, SHA: "foo"}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Type: prowjobv1.PresubmitJob,
						Job:  "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1, SHA: "bar"}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},
			terminateddPJs: sets.NewString("old"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var clientPJs []runtime.Object
			var origPJs []prowjobv1.ProwJob
			for i := range tc.pjs {
				clientPJs = append(clientPJs, &tc.pjs[i])
				origPJs = append(origPJs, tc.pjs[i])
			}
			fakeProwJobClient := &patchTrackingFakeClient{Client: fakectrlruntimeclient.NewFakeClient(clientPJs...)}
			log := logrus.NewEntry(logrus.StandardLogger())
			cleanedupPJs := sets.NewString()
			err := TerminateOlderJobs(fakeProwJobClient, log, tc.pjs, func(pj prowjobv1.ProwJob) error {
				cleanedupPJs.Insert(pj.GetName())
				return nil
			})
			if err != nil {
				t.Fatalf("%s: error terminating the older presubmit jobs: %v", tc.name, err)
			}

			if missing := tc.terminateddPJs.Difference(cleanedupPJs); missing.Len() > 0 {
				t.Errorf("%s: did not cleaned up the expected jobs: %v", tc.name, missing.List())
			}
			if extra := cleanedupPJs.Difference(tc.terminateddPJs); extra.Len() > 0 {
				t.Errorf("%s: found unexpectedly cleaned up jobs: %v", tc.name, extra.List())
			}

			replacedJobs := fakeProwJobClient.patched
			if missing := tc.terminateddPJs.Difference(replacedJobs); missing.Len() > 0 {
				t.Errorf("%s: did not replace the expected jobs: %v", tc.name, missing.Len())
			}
			if extra := replacedJobs.Difference(tc.terminateddPJs); extra.Len() > 0 {
				t.Errorf("%s: found unexpectedly replaced job: %v", tc.name, extra.List())
			}

			// Validate that terminated PJs are marked terminated in the passed slice.
			// Only consider jobs that we expected to be replaced and that were replaced.
			replacedAsExpected := replacedJobs.Intersection(tc.terminateddPJs)
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

type patchTrackingFakeClient struct {
	ctrlruntimeclient.Client
	patched sets.String
}

func (c *patchTrackingFakeClient) Patch(ctx context.Context, obj runtime.Object, patch ctrlruntimeclient.Patch, opts ...ctrlruntimeclient.PatchOption) error {
	if c.patched == nil {
		c.patched = sets.NewString()
	}
	metaObject, ok := obj.(metav1.Object)
	if !ok {
		return errors.New("Object is no metav1.Object")
	}
	c.patched.Insert(metaObject.GetName())
	return c.Client.Patch(ctx, obj, patch, opts...)
}

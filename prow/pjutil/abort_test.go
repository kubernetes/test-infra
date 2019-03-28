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

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

type fakeProwClient struct {
	repalcedJobs map[string]*prowjobv1.ProwJob
}

func newFakeProwClient() *fakeProwClient {
	return &fakeProwClient{
		repalcedJobs: map[string]*prowjobv1.ProwJob{},
	}
}

func (c *fakeProwClient) ReplaceProwJob(name string, pj prowjobv1.ProwJob) (prowjobv1.ProwJob, error) {
	c.repalcedJobs[name] = pj.DeepCopy()
	return pj, nil
}

func TestTerminateOlderPresubmitJobs(t *testing.T) {
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pjc := newFakeProwClient()
			log := logrus.NewEntry(logrus.StandardLogger())
			cleanedupPJs := sets.NewString()
			err := TerminateOlderPresubmitJobs(pjc, log, tc.pjs, func(pj prowjobv1.ProwJob) error {
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

			replacedJobs := sets.NewString()
			for _, pj := range pjc.repalcedJobs {
				if pj.Status.State != prowjobv1.AbortedState {
					t.Errorf("%s: did not aborted the prow job: name=%s, state=%s", tc.name, pj.GetName(), pj.Status.State)
				}
				replacedJobs.Insert(pj.GetName())
			}
			if missing := tc.terminateddPJs.Difference(replacedJobs); missing.Len() > 0 {
				t.Errorf("%s: did not replace the expected jobs: %v", tc.name, missing.Len())
			}
			if extra := replacedJobs.Difference(tc.terminateddPJs); extra.Len() > 0 {
				t.Errorf("%s: found unexpectedly replaced job: %v", tc.name, extra.List())
			}
		})
	}
}

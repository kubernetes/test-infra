/*
Copyright 2016 The Kubernetes Authors.

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

package plank

import (
	"testing"
	"time"

	"k8s.io/test-infra/prow/kube"
)

type fakeKubeClient struct {
	pj              kube.ProwJob
	createdKubeJob  bool
	replacedProwJob bool
}

func (f *fakeKubeClient) ListJobs(map[string]string) ([]kube.Job, error) {
	return nil, nil
}
func (f *fakeKubeClient) ListProwJobs(map[string]string) ([]kube.ProwJob, error) {
	return nil, nil
}
func (f *fakeKubeClient) ReplaceProwJob(s string, j kube.ProwJob) (kube.ProwJob, error) {
	f.pj = j
	f.replacedProwJob = true
	return j, nil
}
func (f *fakeKubeClient) CreateJob(kube.Job) (kube.Job, error) {
	f.createdKubeJob = true
	return kube.Job{}, nil
}

func TestSyncJob(t *testing.T) {
	var testcases = []struct {
		name string

		complete    bool
		jobType     kube.ProwJobType
		startState  kube.ProwJobState
		kubeJobName string
		kubeJob     *kube.Job

		expectedError bool
		shouldCreate  bool
		shouldReplace bool
		expectedState kube.ProwJobState
	}{
		{
			name:     "completed job",
			complete: true,
		},
		{
			name:          "unhandled job type",
			jobType:       kube.PresubmitJob,
			expectedError: true,
		},
		{
			name:          "start new job",
			jobType:       kube.PeriodicJob,
			startState:    kube.TriggeredState,
			shouldCreate:  true,
			shouldReplace: true,
			expectedState: kube.PendingState,
		},
		{
			name:          "missing kube job",
			kubeJobName:   "something",
			expectedError: true,
		},
		{
			name:        "complete kube job",
			startState:  kube.PendingState,
			kubeJobName: "something",
			kubeJob: &kube.Job{
				Metadata: kube.ObjectMeta{
					Annotations: map[string]string{
						"state": kube.FailureState,
					},
				},
				Status: kube.JobStatus{
					Succeeded:      1,
					CompletionTime: time.Now(),
				},
			},
			shouldReplace: true,
			expectedState: kube.FailureState,
		},
		{
			name:        "incomplete kube job",
			kubeJobName: "something",
			kubeJob: &kube.Job{
				Metadata: kube.ObjectMeta{
					Annotations: map[string]string{
						"state": kube.PendingState,
					},
				},
			},
		},
	}
	for _, tc := range testcases {
		var pj kube.ProwJob
		pj.Spec.Type = tc.jobType
		pj.Status.KubeJobName = tc.kubeJobName
		pj.Status.State = tc.startState
		if tc.complete {
			pj.Status.CompletionTime = time.Now()
		}
		jsm := map[string]*kube.Job{}
		if tc.kubeJob != nil {
			jsm[tc.kubeJobName] = tc.kubeJob
		}
		fc := &fakeKubeClient{pj: pj}
		c := &Controller{kc: fc}
		err := c.syncJob(pj, jsm)
		if err != nil && !tc.expectedError {
			t.Fatalf("Unexpected error for %s: %v", tc.name, err)
		}
		if fc.replacedProwJob != tc.shouldReplace {
			t.Fatalf("Wrong usage of ReplaceProwJob for %s", tc.name)
		}
		if tc.shouldReplace && fc.pj.Status.State != tc.expectedState {
			t.Fatalf("Wrong final state for %s, got %v", tc.name, tc.expectedState)
		}
	}
}

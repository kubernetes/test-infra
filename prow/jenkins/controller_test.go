/*
Copyright 2017 The Kubernetes Authors.

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

package jenkins

import (
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"text/template"
	"time"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/npj"
)

type fca struct {
	c *config.Config
}

func newFakeConfigAgent() *fca {
	return &fca{
		c: &config.Config{
			Plank: config.Plank{
				JobURLTemplate: template.Must(template.New("test").Parse("{{.Status.PodName}}/")),
			},
		},
	}

}

func (f *fca) Config() *config.Config {
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

func (f *fkc) ListProwJobs(map[string]string) ([]kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	return f.prowjobs, nil
}

func (f *fkc) ReplaceProwJob(name string, job kube.ProwJob) (kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	for i := range f.prowjobs {
		if f.prowjobs[i].Metadata.Name == name {
			f.prowjobs[i] = job
			return job, nil
		}
	}
	return kube.ProwJob{}, fmt.Errorf("did not find prowjob %s", name)
}

type fjc struct {
	built    bool
	enqueued bool
	status   Status
	err      error
}

func (f *fjc) Build(br BuildRequest) (*Build, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.built = true
	url, _ := url.Parse("localhost")
	return &Build{
		JobName:  br.JobName,
		QueueURL: url,
	}, nil
}

func (f *fjc) Enqueued(string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.enqueued, nil
}

func (f *fjc) Status(job, id string) (*Status, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &f.status, nil
}

func TestPartitionProwJobs(t *testing.T) {
	tests := []struct {
		pjs []kube.ProwJob

		pending    map[string]struct{}
		nonPending map[string]struct{}
	}{
		{
			pjs: []kube.ProwJob{
				{
					Metadata: kube.ObjectMeta{
						Name: "foo",
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "bar",
					},
					Status: kube.ProwJobStatus{
						State: kube.PendingState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "baz",
					},
					Status: kube.ProwJobStatus{
						State: kube.SuccessState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "error",
					},
					Status: kube.ProwJobStatus{
						State: kube.ErrorState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "bak",
					},
					Status: kube.ProwJobStatus{
						State: kube.PendingState,
					},
				},
			},
			pending: map[string]struct{}{
				"bar": {}, "bak": {},
			},
			nonPending: map[string]struct{}{
				"foo": {}, "baz": {}, "error": {},
			},
		},
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		pendingCh, nonPendingCh := partitionProwJobs(test.pjs)
		for job := range pendingCh {
			if _, ok := test.pending[job.Metadata.Name]; !ok {
				t.Errorf("didn't find pending job %#v", job)
			}
		}
		for job := range nonPendingCh {
			if _, ok := test.nonPending[job.Metadata.Name]; !ok {
				t.Errorf("didn't find non-pending job %#v", job)
			}
		}
	}
}

func TestSyncNonPendingJobs(t *testing.T) {
	var testcases = []struct {
		name        string
		pj          kube.ProwJob
		pendingJobs map[string]int

		enqueued bool
		status   Status
		err      error

		expectedState    kube.ProwJobState
		expectedBuild    bool
		expectedComplete bool
		expectedReport   bool
		expectedEnqueued bool
		expectedError    bool
	}{
		{
			name: "complete pj",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					CompletionTime: time.Now(),
				},
			},
			expectedComplete: true,
		},
		{
			name: "start new job",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type: kube.PostsubmitJob,
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			expectedBuild:    true,
			expectedReport:   true,
			expectedState:    kube.PendingState,
			expectedEnqueued: true,
		},
		{
			name: "start new job, error",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type: kube.PresubmitJob,
					Refs: kube.Refs{
						Pulls: []kube.Pull{{
							Number: 1,
							SHA:    "fake-sha",
						}},
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			err:              errors.New("oh no"),
			expectedReport:   true,
			expectedState:    kube.ErrorState,
			expectedComplete: true,
			expectedError:    true,
		},
	}
	for _, tc := range testcases {
		fjc := &fjc{
			enqueued: tc.enqueued,
			status:   tc.status,
			err:      tc.err,
		}
		fkc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}

		c := Controller{
			kc:          fkc,
			jc:          fjc,
			ca:          newFakeConfigAgent(),
			lock:        sync.RWMutex{},
			pendingJobs: make(map[string]int),
		}

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncNonPendingJob(tc.pj, reports); err != nil != tc.expectedError {
			t.Errorf("for case %s got wrong error: %v", tc.name, err)
			continue
		}
		close(reports)

		actual := fkc.prowjobs[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %s got state %v", tc.name, actual.Status.State)
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %s got wrong completion", tc.name)
		}
		if tc.expectedReport && len(reports) != 1 {
			t.Errorf("for case %s wanted one report but got %d", tc.name, len(reports))
		}
		if !tc.expectedReport && len(reports) != 0 {
			t.Errorf("for case %s did not wany any reports but got %d", tc.name, len(reports))
		}
		if fjc.built != tc.expectedBuild {
			t.Errorf("for case %s got wrong built", tc.name)
		}
		if actual.Status.JenkinsEnqueued != tc.expectedEnqueued {
			t.Errorf("for case %s got wrong enqueued", tc.name)
		}
	}
}

func TestSyncPendingJobs(t *testing.T) {
	var testcases = []struct {
		name        string
		pj          kube.ProwJob
		pendingJobs map[string]int

		enqueued bool
		status   Status
		err      error

		expectedState    kube.ProwJobState
		expectedBuild    bool
		expectedComplete bool
		expectedReport   bool
		expectedEnqueued bool
		expectedError    bool
	}{
		{
			name: "enqueued",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State:           kube.PendingState,
					JenkinsEnqueued: true,
				},
			},
			enqueued:         true,
			expectedState:    kube.PendingState,
			expectedEnqueued: true,
		},
		{
			name: "finished queue",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State:           kube.PendingState,
					JenkinsEnqueued: true,
				},
			},
			enqueued:         false,
			expectedState:    kube.PendingState,
			expectedEnqueued: false,
		},
		{
			name: "enqueued, error",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type:   kube.PresubmitJob,
					Report: true,
					Refs: kube.Refs{
						Pulls: []kube.Pull{{
							Number: 1,
							SHA:    "fake-sha",
						}},
					},
				},
				Status: kube.ProwJobStatus{
					State:           kube.PendingState,
					JenkinsEnqueued: true,
				},
			},
			err:              errors.New("oh no"),
			expectedState:    kube.ErrorState,
			expectedError:    true,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "building",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			status: Status{
				Building: true,
			},
			expectedState:  kube.PendingState,
			expectedReport: true,
		},
		{
			name: "building, error",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type: kube.PresubmitJob,
					Refs: kube.Refs{
						Pulls: []kube.Pull{{
							Number: 1,
							SHA:    "fake-sha",
						}},
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			err:              errors.New("oh no"),
			expectedState:    kube.ErrorState,
			expectedError:    true,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "finished, success",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			status: Status{
				Building: false,
				Success:  true,
			},
			expectedState:    kube.SuccessState,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "finished, failed",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			status: Status{
				Building: false,
				Success:  false,
			},
			expectedState:    kube.FailureState,
			expectedComplete: true,
			expectedReport:   true,
		},
	}
	for _, tc := range testcases {
		fjc := &fjc{
			enqueued: tc.enqueued,
			status:   tc.status,
			err:      tc.err,
		}
		fkc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}

		c := Controller{
			kc:          fkc,
			jc:          fjc,
			ca:          newFakeConfigAgent(),
			lock:        sync.RWMutex{},
			pendingJobs: make(map[string]int),
		}

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncPendingJob(tc.pj, reports); err != nil != tc.expectedError {
			t.Errorf("for case %s got wrong error: %v", tc.name, err)
			continue
		}
		close(reports)

		actual := fkc.prowjobs[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %s got state %v", tc.name, actual.Status.State)
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %s got wrong completion", tc.name)
		}
		if tc.expectedReport && len(reports) != 1 {
			t.Errorf("for case %s wanted one report but got %d", tc.name, len(reports))
		}
		if !tc.expectedReport && len(reports) != 0 {
			t.Errorf("for case %s did not wany any reports but got %d", tc.name, len(reports))
		}
		if fjc.built != tc.expectedBuild {
			t.Errorf("for case %s got wrong built", tc.name)
		}
		if actual.Status.JenkinsEnqueued != tc.expectedEnqueued {
			t.Errorf("for case %s got wrong enqueued", tc.name)
		}
	}
}

// TestBatch walks through the happy path of a batch job on Jenkins.
func TestBatch(t *testing.T) {
	pre := config.Presubmit{
		Name:    "pr-some-job",
		Agent:   "jenkins",
		Context: "Some Job Context",
	}
	fc := &fkc{
		prowjobs: []kube.ProwJob{npj.NewProwJob(npj.BatchSpec(pre, kube.Refs{
			Org:     "o",
			Repo:    "r",
			BaseRef: "master",
			BaseSHA: "123",
			Pulls: []kube.Pull{
				{
					Number: 1,
					SHA:    "abc",
				},
				{
					Number: 2,
					SHA:    "qwe",
				},
			},
		}))},
	}
	jc := &fjc{}
	c := Controller{
		kc:          fc,
		jc:          jc,
		ca:          newFakeConfigAgent(),
		pendingJobs: make(map[string]int),
		lock:        sync.RWMutex{},
	}

	if err := c.Sync(); err != nil {
		t.Fatalf("Error on first sync: %v", err)
	}
	if fc.prowjobs[0].Status.State != kube.PendingState {
		t.Fatalf("Wrong state: %v", fc.prowjobs[0].Status.State)
	}
	if !fc.prowjobs[0].Status.JenkinsEnqueued {
		t.Fatal("Wrong enqueued.")
	}
	jc.enqueued = true
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on second sync: %v", err)
	}
	if !fc.prowjobs[0].Status.JenkinsEnqueued {
		t.Fatal("Wrong enqueued steady state.")
	}
	jc.enqueued = false
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on third sync: %v", err)
	}
	if fc.prowjobs[0].Status.JenkinsEnqueued {
		t.Fatal("Wrong enqueued after leaving queue.")
	}
	jc.status = Status{Building: true}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on fourth sync: %v", err)
	}
	if fc.prowjobs[0].Status.State != kube.PendingState {
		t.Fatalf("Wrong state: %v", fc.prowjobs[0].Status.State)
	}
	jc.status = Status{
		Building: false,
		Number:   42,
	}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on fifth sync: %v", err)
	}
	if fc.prowjobs[0].Status.PodName != "pr-some-job-42" {
		t.Fatalf("Wrong PodName: %s", fc.prowjobs[0].Status.PodName)
	}
	if fc.prowjobs[0].Status.State != kube.FailureState {
		t.Fatalf("Wrong state: %v", fc.prowjobs[0].Status.State)
	}

	// This is what the SQ reads.
	if fc.prowjobs[0].Spec.Context != "Some Job Context" {
		t.Fatalf("Wrong context: %v", fc.prowjobs[0].Spec.Context)
	}
}

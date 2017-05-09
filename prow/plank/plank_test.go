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

package plank

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
)

type fkc struct {
	prowjobs []kube.ProwJob
	pods     []kube.Pod
}

func (f *fkc) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	f.prowjobs = append(f.prowjobs, pj)
	return pj, nil
}

func (f *fkc) ListProwJobs(map[string]string) ([]kube.ProwJob, error) {
	return f.prowjobs, nil
}

func (f *fkc) ReplaceProwJob(name string, job kube.ProwJob) (kube.ProwJob, error) {
	for i := range f.prowjobs {
		if f.prowjobs[i].Metadata.Name == name {
			f.prowjobs[i] = job
			return job, nil
		}
	}
	return kube.ProwJob{}, fmt.Errorf("did not find prowjob %s", name)
}

func (f *fkc) CreatePod(pod kube.Pod) (kube.Pod, error) {
	f.pods = append(f.pods, pod)
	return pod, nil
}

func (f *fkc) ListPods(map[string]string) ([]kube.Pod, error) {
	return f.pods, nil
}

func (f *fkc) DeletePod(name string) error {
	for i := range f.pods {
		if f.pods[i].Metadata.Name == name {
			f.pods = append(f.pods[:i], f.pods[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("did not find pod %s", name)
}

func TestTerminateDupes(t *testing.T) {
	now := time.Now()
	var testcases = []struct {
		name      string
		job       string
		startTime time.Time
		complete  bool

		shouldTerminate bool
	}{
		{
			name:            "newest",
			job:             "j1",
			startTime:       now.Add(-time.Minute),
			complete:        false,
			shouldTerminate: false,
		},
		{
			name:            "old",
			job:             "j1",
			startTime:       now.Add(-time.Hour),
			complete:        false,
			shouldTerminate: true,
		},
		{
			name:            "older",
			job:             "j1",
			startTime:       now.Add(-2 * time.Hour),
			complete:        false,
			shouldTerminate: true,
		},
		{
			name:            "complete",
			job:             "j1",
			startTime:       now.Add(-3 * time.Hour),
			complete:        true,
			shouldTerminate: false,
		},
		{
			name:            "newest j2",
			job:             "j2",
			startTime:       now.Add(-time.Minute),
			complete:        false,
			shouldTerminate: false,
		},
		{
			name:            "old j2",
			job:             "j2",
			startTime:       now.Add(-time.Hour),
			complete:        false,
			shouldTerminate: true,
		},
	}
	fkc := &fkc{}
	c := Controller{kc: fkc}
	for _, tc := range testcases {
		var pj = kube.ProwJob{
			Metadata: kube.ObjectMeta{Name: tc.name},
			Spec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  tc.job,
				Refs: kube.Refs{Pulls: []kube.Pull{{}}},
			},
			Status: kube.ProwJobStatus{
				StartTime: tc.startTime,
			},
		}
		if tc.complete {
			pj.Status.CompletionTime = now
		}
		fkc.prowjobs = append(fkc.prowjobs, pj)
	}
	if err := c.terminateDupes(fkc.prowjobs); err != nil {
		t.Fatalf("Error terminating dupes: %v", err)
	}
	for i := range testcases {
		terminated := fkc.prowjobs[i].Status.State == kube.AbortedState
		if terminated != testcases[i].shouldTerminate {
			t.Errorf("Wrong termination for %s", testcases[i].name)
		}
	}
}

type fjc struct {
	built    bool
	enqueued bool
	status   jenkins.Status
	err      error
}

func (f *fjc) Build(br jenkins.BuildRequest) (*jenkins.Build, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.built = true
	url, _ := url.Parse("localhost")
	return &jenkins.Build{
		JobName:  br.JobName,
		ID:       "4",
		QueueURL: url,
	}, nil
}

func (f *fjc) Enqueued(string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.enqueued, nil
}

func (f *fjc) Status(job, id string) (*jenkins.Status, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &f.status, nil
}

func handleTot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "42")
}

func handleCrier(w http.ResponseWriter, r *http.Request) {
}

func TestSyncJenkinsJob(t *testing.T) {
	var testcases = []struct {
		name string
		pj   kube.ProwJob

		enqueued bool
		status   jenkins.Status
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
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			expectedBuild:    true,
			expectedState:    kube.PendingState,
			expectedEnqueued: true,
		},
		{
			name: "start new job, report",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type:   kube.PresubmitJob,
					Report: true,
					Refs: kube.Refs{
						Pulls: []kube.Pull{kube.Pull{}},
					},
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
			name: "start new job, error, report",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type:   kube.PresubmitJob,
					Report: true,
					Refs: kube.Refs{
						Pulls: []kube.Pull{kube.Pull{}},
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			err:              errors.New("oh no!"),
			expectedReport:   true,
			expectedState:    kube.ErrorState,
			expectedComplete: true,
			expectedError:    true,
		},
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
						Pulls: []kube.Pull{kube.Pull{}},
					},
				},

				Status: kube.ProwJobStatus{
					State:           kube.PendingState,
					JenkinsEnqueued: true,
				},
			},
			err:              errors.New("oh no!"),
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
			status: jenkins.Status{
				Building: true,
			},
			expectedState: kube.PendingState,
		},
		{
			name: "building, error",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type:   kube.PresubmitJob,
					Report: true,
					Refs: kube.Refs{
						Pulls: []kube.Pull{kube.Pull{}},
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			err:              errors.New("oh no!"),
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
			status: jenkins.Status{
				Building: false,
				Success:  true,
			},
			expectedState:    kube.SuccessState,
			expectedComplete: true,
		},
		{
			name: "finished, failed",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			status: jenkins.Status{
				Building: false,
				Success:  false,
			},
			expectedState:    kube.FailureState,
			expectedComplete: true,
		},
	}
	for _, tc := range testcases {
		var reported bool
		var handleCrier = func(w http.ResponseWriter, r *http.Request) {
			reported = true
		}
		crierServ := httptest.NewServer(http.HandlerFunc(handleCrier))
		defer crierServ.Close()
		fjc := &fjc{
			enqueued: tc.enqueued,
			status:   tc.status,
			err:      tc.err,
		}
		fkc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}

		c := Controller{
			kc:       fkc,
			jc:       fjc,
			crierURL: crierServ.URL,
		}
		if err := c.syncJenkinsJob(tc.pj); err != nil != tc.expectedError {
			t.Errorf("for case %s got wrong error: %v", tc.name, err)
			continue
		}
		actual := fkc.prowjobs[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %s got state %v", tc.name, actual.Status.State)
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %s got wrong completion", tc.name)
		}
		if reported != tc.expectedReport {
			t.Errorf("for case %s got wrong report", tc.name)
		}
		if fjc.built != tc.expectedBuild {
			t.Errorf("for case %s got wrong built", tc.name)
		}
		if actual.Status.JenkinsEnqueued != tc.expectedEnqueued {
			t.Errorf("for case %s got wrong enqueued", tc.name)
		}
	}
}

func TestSyncKubernetesJob(t *testing.T) {
	var testcases = []struct {
		name string

		pj   kube.ProwJob
		pods []kube.Pod

		expectedState      kube.ProwJobState
		expectedPodName    string
		expectedNumPods    int
		expectedComplete   bool
		expectedCreatedPJs int
	}{
		{
			name: "completed prow job",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					CompletionTime: time.Now(),
					State:          kube.FailureState,
				},
			},
			expectedState:    kube.FailureState,
			expectedComplete: true,
		},
		{
			name: "start new pod",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job: "boop",
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			expectedState:   kube.PendingState,
			expectedPodName: "boop-42",
			expectedNumPods: 1,
		},
		{
			name: "reset when pod goes missing",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-41",
				},
			},
			expectedState:   kube.PendingState,
			expectedPodName: "",
		},
		{
			name: "delete pod in unknown state",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-41",
				},
			},
			pods: []kube.Pod{
				{
					Metadata: kube.ObjectMeta{
						Name: "boop-41",
					},
					Status: kube.PodStatus{
						Phase: kube.PodUnknown,
					},
				},
			},
			expectedState:   kube.PendingState,
			expectedPodName: "",
			expectedNumPods: 0,
		},
		{
			name: "succeeded pod",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					RunAfterSuccess: []kube.ProwJobSpec{{}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []kube.Pod{
				{
					Metadata: kube.ObjectMeta{
						Name: "boop-42",
					},
					Status: kube.PodStatus{
						Phase: kube.PodSucceeded,
					},
				},
			},
			expectedComplete:   true,
			expectedState:      kube.SuccessState,
			expectedPodName:    "boop-42",
			expectedNumPods:    1,
			expectedCreatedPJs: 1,
		},
		{
			name: "failed pod",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					RunAfterSuccess: []kube.ProwJobSpec{{}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []kube.Pod{
				{
					Metadata: kube.ObjectMeta{
						Name: "boop-42",
					},
					Status: kube.PodStatus{
						Phase: kube.PodFailed,
					},
				},
			},
			expectedComplete: true,
			expectedState:    kube.FailureState,
			expectedPodName:  "boop-42",
			expectedNumPods:  1,
		},
		{
			name: "evicted pod",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []kube.Pod{
				{
					Metadata: kube.ObjectMeta{
						Name: "boop-42",
					},
					Status: kube.PodStatus{
						Phase:  kube.PodFailed,
						Reason: kube.Evicted,
					},
				},
			},
			expectedComplete: false,
			expectedState:    kube.PendingState,
			expectedPodName:  "",
			expectedNumPods:  1,
		},
		{
			name: "running pod",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					RunAfterSuccess: []kube.ProwJobSpec{{}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []kube.Pod{
				{
					Metadata: kube.ObjectMeta{
						Name: "boop-42",
					},
					Status: kube.PodStatus{
						Phase: kube.PodRunning,
					},
				},
			},
			expectedState:   kube.PendingState,
			expectedPodName: "boop-42",
			expectedNumPods: 1,
		},
	}
	for _, tc := range testcases {
		totServ := httptest.NewServer(http.HandlerFunc(handleTot))
		defer totServ.Close()
		crierServ := httptest.NewServer(http.HandlerFunc(handleCrier))
		defer crierServ.Close()
		pm := make(map[string]kube.Pod)
		for i := range tc.pods {
			pm[tc.pods[i].Metadata.Name] = tc.pods[i]
		}
		fc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
			pods:     tc.pods,
		}
		c := Controller{
			kc:       fc,
			totURL:   totServ.URL,
			crierURL: crierServ.URL,
		}
		if err := c.syncKubernetesJob(tc.pj, pm); err != nil {
			t.Errorf("for case %s got an error: %v", tc.name, err)
			continue
		}
		actual := fc.prowjobs[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %s got state %v", tc.name, actual.Status.State)
		}
		if actual.Status.PodName != tc.expectedPodName {
			t.Errorf("for case %s got pod name %s", tc.name, actual.Status.PodName)
		}
		if len(fc.pods) != tc.expectedNumPods {
			t.Errorf("for case %s got %d pods", tc.name, len(fc.pods))
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %s got wrong completion", tc.name)
		}
		if len(fc.prowjobs) != tc.expectedCreatedPJs+1 {
			t.Errorf("for case %s got %d created prowjobs", tc.name, len(fc.prowjobs)-1)
		}
	}
}

// TestBatch walks through the happy path of a batch job on Jenkins.
func TestBatch(t *testing.T) {
	pre := config.Presubmit{
		Name:    "pr-some-job",
		Context: "Some Job Context",
	}
	crierServ := httptest.NewServer(http.HandlerFunc(handleCrier))
	defer crierServ.Close()
	fc := &fkc{
		prowjobs: []kube.ProwJob{NewProwJob(BatchSpec(pre, kube.Refs{
			Org:     "o",
			Repo:    "r",
			BaseRef: "master",
			BaseSHA: "123",
			Pulls: []kube.Pull{
				kube.Pull{
					Number: 1,
					SHA:    "abc",
				},
				kube.Pull{
					Number: 2,
					SHA:    "qwe",
				},
			},
		}))},
	}
	jc := &fjc{}
	c := Controller{
		kc:       fc,
		jc:       jc,
		crierURL: crierServ.URL,
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
	jc.status = jenkins.Status{Building: true}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on fourth sync: %v", err)
	}
	if fc.prowjobs[0].Status.State != kube.PendingState {
		t.Fatalf("Wrong state: %v", fc.prowjobs[0].Status.State)
	}
	jc.status = jenkins.Status{
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

// TestPeriodic walks through the happy path of a periodic job.
func TestPeriodic(t *testing.T) {
	per := config.Periodic{
		Name: "ci-periodic-job",
		Spec: &kube.PodSpec{
			Containers: []kube.Container{{}},
		},
		RunAfterSuccess: []config.Periodic{
			config.Periodic{
				Name: "ci-periodic-job-2",
				Spec: &kube.PodSpec{},
			},
		},
	}

	totServ := httptest.NewServer(http.HandlerFunc(handleTot))
	defer totServ.Close()
	crierServ := httptest.NewServer(http.HandlerFunc(handleCrier))
	defer crierServ.Close()
	fc := &fkc{
		prowjobs: []kube.ProwJob{NewProwJob(PeriodicSpec(per))},
	}
	c := Controller{
		kc:       fc,
		totURL:   totServ.URL,
		crierURL: crierServ.URL,
	}

	if err := c.Sync(); err != nil {
		t.Fatalf("Error on first sync: %v", err)
	}
	if fc.prowjobs[0].Spec.PodSpec.Containers[0].Name != "" {
		t.Fatal("Sync step updated the TPR spec.")
	}
	if len(fc.pods) != 1 {
		t.Fatal("Didn't create pod on first sync.")
	}
	if fc.pods[0].Metadata.Name != "ci-periodic-job-42" {
		t.Fatalf("Wrong pod name: %s", fc.pods[0].Metadata.Name)
	}
	if len(fc.pods[0].Spec.Containers) != 1 {
		t.Fatal("Wiped container list.")
	}
	if len(fc.pods[0].Spec.Containers[0].Env) == 0 {
		t.Fatal("Container has no env set.")
	}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on second sync: %v", err)
	}
	if len(fc.pods) != 1 {
		t.Fatalf("Wrong number of pods after second sync: %d", len(fc.pods))
	}
	fc.pods[0].Status.Phase = kube.PodSucceeded
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on third sync: %v", err)
	}
	if !fc.prowjobs[0].Complete() {
		t.Fatal("Prow job didn't complete.")
	}
	if fc.prowjobs[0].Status.State != kube.SuccessState {
		t.Fatalf("Should be success: %v", fc.prowjobs[0].Status.State)
	}
	if len(fc.prowjobs) != 2 {
		t.Fatalf("Wrong number of prow jobs: %d", len(fc.prowjobs))
	}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on fourth sync: %v", err)
	}
	if len(fc.pods) != 2 {
		t.Fatalf("Wrong number of pods: %d", len(fc.pods))
	}
}

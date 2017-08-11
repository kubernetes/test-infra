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
	"sync"
	"testing"
	"text/template"
	"time"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
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
	pods     []kube.Pod
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

func (f *fkc) CreatePod(pod kube.Pod) (kube.Pod, error) {
	f.Lock()
	defer f.Unlock()
	f.pods = append(f.pods, pod)
	return pod, nil
}

func (f *fkc) ListPods(map[string]string) ([]kube.Pod, error) {
	f.Lock()
	defer f.Unlock()
	return f.pods, nil
}

func (f *fkc) DeletePod(name string) error {
	f.Lock()
	defer f.Unlock()
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
	c := Controller{kc: fkc, pkc: fkc}
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

func TestSyncJenkinsJob(t *testing.T) {
	var testcases = []struct {
		name        string
		pj          kube.ProwJob
		pendingJobs map[string]int

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
			expectedReport:   true,
			expectedState:    kube.PendingState,
			expectedEnqueued: true,
		},
		{
			name: "start new job, error",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type: kube.PresubmitJob,
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
						Pulls: []kube.Pull{{}},
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
			status: jenkins.Status{
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
			status: jenkins.Status{
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
			status: jenkins.Status{
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
			kc: fkc,
			jc: fjc,
			ca: newFakeConfigAgent(),
		}
		c.setPending(tc.pendingJobs)

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncJenkinsJob(tc.pj, reports); err != nil != tc.expectedError {
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

func TestSyncKubernetesJob(t *testing.T) {
	var testcases = []struct {
		name string

		pj          kube.ProwJob
		pendingJobs map[string]int
		pods        []kube.Pod

		expectedState      kube.ProwJobState
		expectedPodHasName bool
		expectedNumPods    int
		expectedComplete   bool
		expectedCreatedPJs int
		expectedReport     bool
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
			name: "completed prow job, clean up pod",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					CompletionTime: time.Now(),
					State:          kube.FailureState,
					PodName:        "boop-41",
				},
			},
			pods: []kube.Pod{
				{
					Metadata: kube.ObjectMeta{
						Name: "boop-41",
					},
					Status: kube.PodStatus{
						Phase: kube.PodFailed,
					},
				},
			},
			expectedState:    kube.FailureState,
			expectedNumPods:  0,
			expectedComplete: true,
		},
		{
			name: "completed prow job, missing pod",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					CompletionTime: time.Now(),
					State:          kube.FailureState,
					PodName:        "boop-41",
				},
			},
			expectedState:    kube.FailureState,
			expectedNumPods:  0,
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
			expectedState:      kube.PendingState,
			expectedPodHasName: true,
			expectedNumPods:    1,
			expectedReport:     true,
		},
		{
			name: "reset when pod goes missing",
			pj: kube.ProwJob{
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-41",
				},
			},
			expectedState: kube.PendingState,
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
			expectedPodHasName: true,
			expectedNumPods:    1,
			expectedCreatedPJs: 1,
			expectedReport:     true,
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
			expectedComplete:   true,
			expectedState:      kube.FailureState,
			expectedPodHasName: true,
			expectedNumPods:    1,
			expectedReport:     true,
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
			expectedState:      kube.PendingState,
			expectedPodHasName: true,
			expectedNumPods:    1,
		},
		{
			name: "pod with a max concurrency of 1",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:            "same",
					MaxConcurrency: 1,
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pendingJobs: map[string]int{
				"same": 1,
			},
			pods: []kube.Pod{
				{
					Metadata: kube.ObjectMeta{
						Name: "same-42",
					},
					Status: kube.PodStatus{
						Phase: kube.PodRunning,
					},
				},
			},
			expectedState:   kube.TriggeredState,
			expectedNumPods: 1,
		},
	}
	for _, tc := range testcases {
		totServ := httptest.NewServer(http.HandlerFunc(handleTot))
		defer totServ.Close()
		pm := make(map[string]kube.Pod)
		for i := range tc.pods {
			pm[tc.pods[i].Metadata.Name] = tc.pods[i]
		}
		fc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}
		fpc := &fkc{
			pods: tc.pods,
		}
		c := Controller{
			kc:     fc,
			pkc:    fpc,
			ca:     newFakeConfigAgent(),
			totURL: totServ.URL,
		}
		c.setPending(tc.pendingJobs)

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncKubernetesJob(tc.pj, pm, reports); err != nil {
			t.Errorf("for case %s got an error: %v", tc.name, err)
			continue
		}
		close(reports)

		actual := fc.prowjobs[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %s got state %v", tc.name, actual.Status.State)
		}
		if (actual.Status.PodName == "") && tc.expectedPodHasName {
			t.Errorf("for case %s got no pod name, expected one", tc.name)
		} else if (actual.Status.PodName != "") && !tc.expectedPodHasName {
			t.Errorf("for case %s got pod name, expected none", tc.name)
		}
		if len(fpc.pods) != tc.expectedNumPods {
			t.Errorf("for case %s got %d pods", tc.name, len(fc.pods))
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %s got wrong completion", tc.name)
		}
		if len(fc.prowjobs) != tc.expectedCreatedPJs+1 {
			t.Errorf("for case %s got %d created prowjobs", tc.name, len(fc.prowjobs)-1)
		}
		if tc.expectedReport && len(reports) != 1 {
			t.Errorf("for case %s wanted one report but got %d", tc.name, len(reports))
		}
		if !tc.expectedReport && len(reports) != 0 {
			t.Errorf("for case %s did not wany any reports but got %d", tc.name, len(reports))
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
		prowjobs: []kube.ProwJob{NewProwJob(BatchSpec(pre, kube.Refs{
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
		pkc:         fc,
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
		Name:  "ci-periodic-job",
		Agent: "kubernetes",
		Spec: &kube.PodSpec{
			Containers: []kube.Container{{}},
		},
		RunAfterSuccess: []config.Periodic{
			{
				Name:  "ci-periodic-job-2",
				Agent: "kubernetes",
				Spec:  &kube.PodSpec{},
			},
		},
	}

	totServ := httptest.NewServer(http.HandlerFunc(handleTot))
	defer totServ.Close()
	fc := &fkc{
		prowjobs: []kube.ProwJob{NewProwJob(PeriodicSpec(per))},
	}
	c := Controller{
		kc:          fc,
		pkc:         fc,
		ca:          newFakeConfigAgent(),
		totURL:      totServ.URL,
		pendingJobs: make(map[string]int),
		lock:        sync.RWMutex{},
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
	if len(fc.pods) != 1 {
		t.Fatalf("Wrong number of pods: %d", len(fc.pods))
	}
}

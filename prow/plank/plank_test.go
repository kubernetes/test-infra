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
	"fmt"
	"net/http"
	"net/http/httptest"
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

func handleTot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "42")
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
		prowjobs: []kube.ProwJob{npj.NewProwJob(npj.PeriodicSpec(per))},
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

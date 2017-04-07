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
	"testing"
	"time"

	"k8s.io/test-infra/prow/config"
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

func handleTot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "42")
}

func handleCrier(w http.ResponseWriter, r *http.Request) {
}

func TestSyncJob(t *testing.T) {
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
		if err := c.syncJob(tc.pj, pm); err != nil {
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

// TestPeriodic walks through the happy path of a periodic job.
func TestPeriodic(t *testing.T) {
	per := config.Periodic{
		Name: "ci-periodic-job",
		Spec: &kube.PodSpec{},
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
	if len(fc.pods) != 1 {
		t.Fatal("Didn't create pod on first sync.")
	}
	if fc.pods[0].Metadata.Name != "ci-periodic-job-42" {
		t.Fatalf("Wrong pod name: %s", fc.pods[0].Metadata.Name)
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

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
	"reflect"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clienttesting "k8s.io/client-go/testing"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/reporter"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

const (
	podPendingTimeout = time.Hour
)

func newFakeConfigAgent(t *testing.T, maxConcurrency int) *fca {
	presubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name: "test-bazel-build",
			},
			RunAfterSuccess: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "test-kubeadm-cloud",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^(cmd/kubeadm|build/debs).*$",
					},
				},
			},
		},
		{
			JobBase: config.JobBase{
				Name: "test-e2e",
			},
			RunAfterSuccess: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "push-image",
					},
				},
			},
		},
		{
			JobBase: config.JobBase{
				Name: "test-bazel-test",
			},
		},
	}
	if err := config.SetPresubmitRegexes(presubmits); err != nil {
		t.Fatal(err)
	}
	presubmitMap := map[string][]config.Presubmit{
		"kubernetes/kubernetes": presubmits,
	}

	return &fca{
		c: &config.Config{
			ProwConfig: config.ProwConfig{
				PodNamespace: "pods",
				Plank: config.Plank{
					Controller: config.Controller{
						JobURLTemplate: template.Must(template.New("test").Parse("{{.ObjectMeta.Name}}/{{.Status.State}}")),
						MaxConcurrency: maxConcurrency,
						MaxGoroutines:  20,
					},
					PodPendingTimeout: podPendingTimeout,
				},
			},
			JobConfig: config.JobConfig{
				Presubmits: presubmitMap,
			},
		},
	}
}

func (f *fca) Config() *config.Config {
	f.Lock()
	defer f.Unlock()
	return f.c
}

type fkc struct {
	sync.Mutex
	prowjobs []kube.ProwJob
	err      error
}

func (f *fkc) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	f.prowjobs = append(f.prowjobs, pj)
	return pj, nil
}

func (f *fkc) GetProwJob(name string) (kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	for _, pj := range f.prowjobs {
		if pj.ObjectMeta.Name == name {
			return pj, nil
		}
	}

	return kube.ProwJob{}, fmt.Errorf("did not find prowjob %s", name)
}

func (f *fkc) ListProwJobs(selector string) ([]kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	return f.prowjobs, nil
}

func (f *fkc) ReplaceProwJob(name string, job kube.ProwJob) (kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	for i := range f.prowjobs {
		if f.prowjobs[i].ObjectMeta.Name == name {
			f.prowjobs[i] = job
			return job, nil
		}
	}
	return kube.ProwJob{}, fmt.Errorf("did not find prowjob %s", name)
}

type fghc struct {
	sync.Mutex
	changes []github.PullRequestChange
	err     error
}

func (f *fghc) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	f.Lock()
	defer f.Unlock()
	return f.changes, f.err
}

func (f *fghc) BotName() (string, error)                                  { return "bot", nil }
func (f *fghc) CreateStatus(org, repo, ref string, s github.Status) error { return nil }
func (f *fghc) ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) {
	return nil, nil
}
func (f *fghc) CreateComment(org, repo string, number int, comment string) error { return nil }
func (f *fghc) DeleteComment(org, repo string, ID int) error                     { return nil }
func (f *fghc) EditComment(org, repo string, ID int, comment string) error       { return nil }

func TestTerminateDupes(t *testing.T) {
	now := time.Now()
	nowFn := func() *metav1.Time {
		reallyNow := metav1.NewTime(now)
		return &reallyNow
	}
	var testcases = []struct {
		name string

		allowCancellations bool
		pjs                []kube.ProwJob
		pm                 map[string]coreapi.Pod

		terminatedPJs  sets.String
		terminatedPods sets.String
	}{
		{
			name: "terminate all duplicates",

			pjs: []kube.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j1",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j1",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j1",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "complete"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j1",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-3 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest_j2"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j2",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old_j2"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j2",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old_j3"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j3",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "new_j3"},
					Spec: kube.ProwJobSpec{
						Type: kube.PresubmitJob,
						Job:  "j3",
						Refs: &kube.Refs{Pulls: []kube.Pull{{}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},

			terminatedPJs: sets.NewString("old", "older", "old_j2", "old_j3"),
		},
		{
			name: "should also terminate pods",

			allowCancellations: true,
			pjs: []kube.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest"},
					Spec: kube.ProwJobSpec{
						Type:    kube.PresubmitJob,
						Job:     "j1",
						Refs:    &kube.Refs{Pulls: []kube.Pull{{}}},
						PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old"},
					Spec: kube.ProwJobSpec{
						Type:    kube.PresubmitJob,
						Job:     "j1",
						Refs:    &kube.Refs{Pulls: []kube.Pull{{}}},
						PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					},
					Status: kube.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			pm: map[string]coreapi.Pod{
				"newest": {ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: "pods"}},
				"old":    {ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "pods"}},
			},

			terminatedPJs:  sets.NewString("old"),
			terminatedPods: sets.NewString("old"),
		},
	}

	for _, tc := range testcases {
		var pods []runtime.Object
		for name := range tc.pm {
			pod := tc.pm[name]
			pods = append(pods, &pod)
		}
		fkc := &fkc{prowjobs: tc.pjs}
		fakePodClient := fake.NewSimpleClientset(pods...)
		fca := &fca{
			c: &config.Config{
				ProwConfig: config.ProwConfig{
					PodNamespace: "pods",
					Plank: config.Plank{
						Controller: config.Controller{
							AllowCancellations: tc.allowCancellations,
						},
					},
				},
			},
		}
		c := Controller{
			kc:           fkc,
			buildClients: map[string]corev1.PodInterface{kube.DefaultClusterAlias: fakePodClient.CoreV1().Pods("pods")},
			log:          logrus.NewEntry(logrus.StandardLogger()),
			config:       fca.Config,
		}

		if err := c.terminateDupes(fkc.prowjobs, tc.pm); err != nil {
			t.Fatalf("Error terminating dupes: %v", err)
		}

		for terminatedName := range tc.terminatedPJs {
			terminated := false
			for _, pj := range fkc.prowjobs {
				if pj.ObjectMeta.Name == terminatedName && !pj.Complete() {
					t.Errorf("expected prowjob %q to be terminated!", terminatedName)
				} else {
					terminated = true
				}
			}
			if !terminated {
				t.Errorf("expected prowjob %q to be terminated, got %+v", terminatedName, fkc.prowjobs)
			}
		}

		observedTerminatedPods := sets.NewString()
		for _, action := range fakePodClient.Fake.Actions() {
			switch action := action.(type) {
			case clienttesting.DeleteActionImpl:
				observedTerminatedPods.Insert(action.Name)
			}
		}
		if missing := tc.terminatedPods.Difference(observedTerminatedPods); missing.Len() > 0 {
			t.Errorf("%s: did not delete expected pods: %v", tc.name, missing.List())
		}
		if extra := observedTerminatedPods.Difference(tc.terminatedPods); extra.Len() > 0 {
			t.Errorf("%s: found unexpectedly deleted pods: %v", tc.name, extra.List())
		}
	}
}

func handleTot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "42")
}

func TestSyncTriggeredJobs(t *testing.T) {
	var testcases = []struct {
		name string

		pj             kube.ProwJob
		pendingJobs    map[string]int
		maxConcurrency int
		pods           map[string][]coreapi.Pod
		podErr         error

		expectedState         kube.ProwJobState
		expectedPodHasName    bool
		expectedNumPods       map[string]int
		expectedComplete      bool
		expectedCreatedPJs    int
		expectedReport        bool
		expectPrevReportState map[string]kube.ProwJobState
		expectedURL           string
		expectedBuildID       string
		expectError           bool
	}{
		{
			name: "start new pod",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "blabla",
				},
				Spec: kube.ProwJobSpec{
					Job:     "boop",
					Type:    kube.PeriodicJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pods:               map[string][]coreapi.Pod{"default": {}},
			expectedState:      kube.PendingState,
			expectedPodHasName: true,
			expectedNumPods:    map[string]int{"default": 1},
			expectedReport:     true,
			expectPrevReportState: map[string]kube.ProwJobState{
				reporter.GithubReporterName: kube.PendingState,
			},
			expectedURL: "blabla/pending",
		},
		{
			name: "pod with a max concurrency of 1",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:            "same",
					Type:           kube.PeriodicJob,
					MaxConcurrency: 1,
					PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pendingJobs: map[string]int{
				"same": 1,
			},
			pods: map[string][]coreapi.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "same-42",
							Namespace: "pods",
						},
						Status: coreapi.PodStatus{
							Phase: coreapi.PodRunning,
						},
					},
				},
			},
			expectedState:   kube.TriggeredState,
			expectedNumPods: map[string]int{"default": 1},
		},
		{
			name: "trusted pod with a max concurrency of 1",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:            "same",
					Type:           kube.PeriodicJob,
					Cluster:        "trusted",
					MaxConcurrency: 1,
					PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pendingJobs: map[string]int{
				"same": 1,
			},
			pods: map[string][]coreapi.Pod{
				"trusted": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "same-42",
							Namespace: "pods",
						},
						Status: coreapi.PodStatus{
							Phase: coreapi.PodRunning,
						},
					},
				},
			},
			expectedState:   kube.TriggeredState,
			expectedNumPods: map[string]int{"trusted": 1},
		},
		{
			name: "trusted pod with a max concurrency of 1 (can start)",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some",
				},
				Spec: kube.ProwJobSpec{
					Job:            "some",
					Type:           kube.PeriodicJob,
					Cluster:        "trusted",
					MaxConcurrency: 1,
					PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pods: map[string][]coreapi.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-42",
							Namespace: "pods",
						},
						Status: coreapi.PodStatus{
							Phase: coreapi.PodRunning,
						},
					},
				},
				"trusted": {},
			},
			expectedState:      kube.PendingState,
			expectedNumPods:    map[string]int{"default": 1, "trusted": 1},
			expectedPodHasName: true,
			expectedReport:     true,
			expectPrevReportState: map[string]kube.ProwJobState{
				reporter.GithubReporterName: kube.PendingState,
			},
			expectedURL: "some/pending",
		},
		{
			name: "do not exceed global maxconcurrency",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beer",
				},
				Spec: kube.ProwJobSpec{
					Job:     "same",
					Type:    kube.PeriodicJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			maxConcurrency: 20,
			pendingJobs:    map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			expectedState:  kube.TriggeredState,
		},
		{
			name: "global maxconcurrency allows new jobs when possible",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beer",
				},
				Spec: kube.ProwJobSpec{
					Job:     "same",
					Type:    kube.PeriodicJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pods:            map[string][]coreapi.Pod{"default": {}},
			maxConcurrency:  21,
			pendingJobs:     map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			expectedState:   kube.PendingState,
			expectedNumPods: map[string]int{"default": 1},
			expectedReport:  true,
			expectPrevReportState: map[string]kube.ProwJobState{
				reporter.GithubReporterName: kube.PendingState,
			},
			expectedURL: "beer/pending",
		},
		{
			name: "unprocessable prow job",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:     "boop",
					Type:    kube.PeriodicJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pods:             map[string][]coreapi.Pod{"default": {}},
			podErr:           kube.NewUnprocessableEntityError(errors.New("no way jose")),
			expectedState:    kube.ErrorState,
			expectedComplete: true,
			expectedReport:   true,
			expectPrevReportState: map[string]kube.ProwJobState{
				reporter.GithubReporterName: kube.ErrorState,
			},
		},
		{
			name: "conflict error starting pod",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:     "boop",
					Type:    kube.PeriodicJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			podErr:        kube.NewConflictError(errors.New("no way jose")),
			expectedState: kube.TriggeredState,
			expectError:   true,
		},
		{
			name: "unknown error starting pod",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:     "boop",
					Type:    kube.PeriodicJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			podErr:        errors.New("no way unknown jose"),
			expectedState: kube.TriggeredState,
			expectError:   true,
		},
		{
			name: "running pod, failed prowjob update",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: kube.ProwJobSpec{
					Job:     "boop",
					Type:    kube.PeriodicJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pods: map[string][]coreapi.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "pods",
						},
						Spec: coreapi.PodSpec{
							Containers: []v1.Container{
								{
									Env: []v1.EnvVar{
										{
											Name:  "BUILD_ID",
											Value: "0987654321",
										},
									},
								},
							},
						},
						Status: coreapi.PodStatus{
							Phase: coreapi.PodRunning,
						},
					},
				},
			},
			expectedState:   kube.PendingState,
			expectedNumPods: map[string]int{"default": 1},
			expectedReport:  true,
			expectPrevReportState: map[string]kube.ProwJobState{
				reporter.GithubReporterName: kube.PendingState,
			},
			expectedURL:     "foo/pending",
			expectedBuildID: "0987654321",
		},
	}
	for _, tc := range testcases {
		totServ := httptest.NewServer(http.HandlerFunc(handleTot))
		defer totServ.Close()
		pm := make(map[string]coreapi.Pod)
		for _, pods := range tc.pods {
			for i := range pods {
				pm[pods[i].ObjectMeta.Name] = pods[i]
			}
		}
		fc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}
		buildClients := map[string]corev1.PodInterface{}
		for alias, pods := range tc.pods {
			var data []runtime.Object
			for i := range pods {
				pod := pods[i]
				data = append(data, &pod)
			}
			fakeClient := fake.NewSimpleClientset(data...)
			if tc.podErr != nil {
				fakeClient.PrependReactor("create", "pods", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, tc.podErr
				})
			}
			buildClients[alias] = fakeClient.CoreV1().Pods("pods")
		}
		c := Controller{
			kc:           fc,
			buildClients: buildClients,
			log:          logrus.NewEntry(logrus.StandardLogger()),
			config:       newFakeConfigAgent(t, tc.maxConcurrency).Config,
			totURL:       totServ.URL,
			pendingJobs:  make(map[string]int),
		}
		if tc.pendingJobs != nil {
			c.pendingJobs = tc.pendingJobs
		}

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncTriggeredJob(tc.pj, pm, reports); (err != nil) != tc.expectError {
			if tc.expectError {
				t.Errorf("for case %q expected an error, but got none", tc.name)
			} else {
				t.Errorf("for case %q got an unexpected error: %v", tc.name, err)
			}
			continue
		}
		close(reports)

		numReports := len(reports)
		// for asserting recorded report states
		for report := range reports {
			if err := c.setPreviousReportState(report); err != nil {
				t.Errorf("for case %q got error in setPreviousReportState : %v", tc.name, err)
			}
		}

		actual := fc.prowjobs[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %q got state %v", tc.name, actual.Status.State)
		}
		if (actual.Status.PodName == "") && tc.expectedPodHasName {
			t.Errorf("for case %q got no pod name, expected one", tc.name)
		}
		for alias, expected := range tc.expectedNumPods {
			actualPods, err := buildClients[alias].List(metav1.ListOptions{})
			if err != nil {
				t.Fatalf("for case %q could not list pods from the client: %v", tc.name, err)
			}
			if got := len(actualPods.Items); got != expected {
				t.Errorf("for case %q got %d pods for alias %q, but expected %d", tc.name, got, alias, expected)
			}
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %q got wrong completion", tc.name)
		}
		if len(fc.prowjobs) != tc.expectedCreatedPJs+1 {
			t.Errorf("for case %q got %d created prowjobs", tc.name, len(fc.prowjobs)-1)
		}
		if tc.expectedReport && numReports != 1 {
			t.Errorf("for case %q wanted one report but got %d", tc.name, numReports)
		}
		if !tc.expectedReport && numReports != 0 {
			t.Errorf("for case %q did not want any reports but got %d", tc.name, numReports)
		}
		if !reflect.DeepEqual(tc.expectPrevReportState, actual.Status.PrevReportStates) {
			t.Errorf("for case %q want prev report state %v, got %v", tc.name, tc.expectPrevReportState, actual.Status.PrevReportStates)
		}

		if tc.expectedReport {
			if got, want := actual.Status.URL, tc.expectedURL; got != want {
				t.Errorf("for case %q, report.Status.URL: got %q, want %q", tc.name, got, want)
			}
			if got, want := actual.Status.BuildID, tc.expectedBuildID; want != "" && got != want {
				t.Errorf("for case %q, report.Status.ProwJobID: got %q, want %q", tc.name, got, want)
			}
		}
	}
}

func startTime(s time.Time) *metav1.Time {
	start := metav1.NewTime(s)
	return &start
}

func TestSyncPendingJob(t *testing.T) {
	var testcases = []struct {
		name string

		pj   kube.ProwJob
		pods []coreapi.Pod
		err  error

		expectedState      kube.ProwJobState
		expectedNumPods    int
		expectedComplete   bool
		expectedCreatedPJs int
		expectedReport     bool
		expectedURL        string
	}{
		{
			name: "reset when pod goes missing",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-41",
				},
				Spec: kube.ProwJobSpec{
					Type:    kube.PostsubmitJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					Refs:    &kube.Refs{Org: "fejtaverse"},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-41",
				},
			},
			expectedState:   kube.PendingState,
			expectedReport:  true,
			expectedNumPods: 1,
			expectedURL:     "boop-41/pending",
		},
		{
			name: "delete pod in unknown state",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-41",
				},
				Spec: kube.ProwJobSpec{
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-41",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-41",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase: coreapi.PodUnknown,
					},
				},
			},
			expectedState:   kube.PendingState,
			expectedNumPods: 0,
		},
		{
			name: "succeeded pod",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-42",
				},
				Spec: kube.ProwJobSpec{
					Type:    kube.BatchJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					RunAfterSuccess: []kube.ProwJobSpec{{
						Job:     "job-name",
						Type:    kube.PeriodicJob,
						PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					}},
					Refs: &kube.Refs{Org: "fejtaverse"},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase: coreapi.PodSucceeded,
					},
				},
			},
			expectedComplete:   true,
			expectedState:      kube.SuccessState,
			expectedNumPods:    1,
			expectedCreatedPJs: 1,
			expectedReport:     true,
			expectedURL:        "boop-42/success",
		},
		{
			name: "failed pod",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-42",
				},
				Spec: kube.ProwJobSpec{
					Type: kube.PresubmitJob,
					Refs: &kube.Refs{
						Org: "kubernetes", Repo: "kubernetes",
						BaseRef: "baseref", BaseSHA: "basesha",
						Pulls: []kube.Pull{{Number: 100, Author: "me", SHA: "sha"}},
					},
					PodSpec:         &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					RunAfterSuccess: []kube.ProwJobSpec{{}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase: coreapi.PodFailed,
					},
				},
			},
			expectedComplete: true,
			expectedState:    kube.FailureState,
			expectedNumPods:  1,
			expectedReport:   true,
			expectedURL:      "boop-42/failure",
		},
		{
			name: "delete evicted pod",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-42",
				},
				Spec: kube.ProwJobSpec{
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase:  coreapi.PodFailed,
						Reason: kube.Evicted,
					},
				},
			},
			expectedComplete: false,
			expectedState:    kube.PendingState,
			expectedNumPods:  0,
		},
		{
			name: "don't delete evicted pod w/ error_on_eviction, complete PJ instead",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-42",
				},
				Spec: kube.ProwJobSpec{
					ErrorOnEviction: true,
					PodSpec:         &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase:  coreapi.PodFailed,
						Reason: kube.Evicted,
					},
				},
			},
			expectedComplete: true,
			expectedState:    kube.ErrorState,
			expectedNumPods:  1,
			expectedReport:   true,
			expectedURL:      "boop-42/error",
		},
		{
			name: "running pod",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-42",
				},
				Spec: kube.ProwJobSpec{
					RunAfterSuccess: []kube.ProwJobSpec{{
						Job:     "job-name",
						Type:    kube.PeriodicJob,
						PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase: coreapi.PodRunning,
					},
				},
			},
			expectedState:   kube.PendingState,
			expectedNumPods: 1,
		},
		{
			name: "pod changes url status",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "boop-42",
				},
				Spec: kube.ProwJobSpec{
					RunAfterSuccess: []kube.ProwJobSpec{{
						Job:     "job-name",
						Type:    kube.PeriodicJob,
						PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "boop-42",
					URL:     "boop-42/pending",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase: coreapi.PodSucceeded,
					},
				},
			},
			expectedComplete:   true,
			expectedState:      kube.SuccessState,
			expectedNumPods:    1,
			expectedCreatedPJs: 1,
			expectedReport:     true,
			expectedURL:        "boop-42/success",
		},
		{
			name: "unprocessable prow job",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "jose",
				},
				Spec: kube.ProwJobSpec{
					Job:     "boop",
					Type:    kube.PostsubmitJob,
					PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					Refs:    &kube.Refs{Org: "fejtaverse"},
				},
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			err:              kube.NewUnprocessableEntityError(errors.New("no way jose")),
			expectedState:    kube.ErrorState,
			expectedComplete: true,
			expectedReport:   true,
			expectedURL:      "jose/error",
		},
		{
			name: "stale pending prow job",
			pj: kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nightmare",
				},
				Spec: kube.ProwJobSpec{
					RunAfterSuccess: []kube.ProwJobSpec{{
						Job:     "job-name",
						Type:    kube.PeriodicJob,
						PodSpec: &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
					}},
				},
				Status: kube.ProwJobStatus{
					State:   kube.PendingState,
					PodName: "nightmare",
				},
			},
			pods: []coreapi.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nightmare",
						Namespace: "pods",
					},
					Status: coreapi.PodStatus{
						Phase:     coreapi.PodPending,
						StartTime: startTime(time.Now().Add(-podPendingTimeout)),
					},
				},
			},
			expectedState:    kube.AbortedState,
			expectedNumPods:  1,
			expectedComplete: true,
			expectedReport:   true,
			expectedURL:      "nightmare/aborted",
		},
	}
	for _, tc := range testcases {
		t.Logf("Running test case %q", tc.name)
		totServ := httptest.NewServer(http.HandlerFunc(handleTot))
		defer totServ.Close()
		pm := make(map[string]coreapi.Pod)
		for i := range tc.pods {
			pm[tc.pods[i].ObjectMeta.Name] = tc.pods[i]
		}
		fc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}
		var data []runtime.Object
		for i := range tc.pods {
			pod := tc.pods[i]
			data = append(data, &pod)
		}
		fakeClient := fake.NewSimpleClientset(data...)
		if tc.err != nil {
			fakeClient.PrependReactor("create", "pods", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, tc.err
			})
		}
		buildClients := map[string]corev1.PodInterface{
			kube.DefaultClusterAlias: fakeClient.CoreV1().Pods("pods"),
		}
		c := Controller{
			kc:           fc,
			buildClients: buildClients,
			log:          logrus.NewEntry(logrus.StandardLogger()),
			config:       newFakeConfigAgent(t, 0).Config,
			totURL:       totServ.URL,
			pendingJobs:  make(map[string]int),
		}

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncPendingJob(tc.pj, pm, reports); err != nil {
			t.Errorf("for case %q got an error: %v", tc.name, err)
			continue
		}
		close(reports)

		actual := fc.prowjobs[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %q got state %v", tc.name, actual.Status.State)
		}
		actualPods, err := buildClients[kube.DefaultClusterAlias].List(metav1.ListOptions{})
		if err != nil {
			t.Fatalf("for case %q could not list pods from the client: %v", tc.name, err)
		}
		if got := len(actualPods.Items); got != tc.expectedNumPods {
			t.Errorf("for case %q got %d pods, expected %d", tc.name, len(actualPods.Items), tc.expectedNumPods)
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %q got wrong completion", tc.name)
		}
		if len(fc.prowjobs) != tc.expectedCreatedPJs+1 {
			t.Errorf("for case %q got %d created prowjobs", tc.name, len(fc.prowjobs)-1)
		}
		if tc.expectedReport && len(reports) != 1 {
			t.Errorf("for case %q wanted one report but got %d", tc.name, len(reports))
		}
		if !tc.expectedReport && len(reports) != 0 {
			t.Errorf("for case %q did not wany any reports but got %d", tc.name, len(reports))
		}
		if tc.expectedReport {
			r := <-reports

			if got, want := r.Status.URL, tc.expectedURL; got != want {
				t.Errorf("for case %q, report.Status.URL: got %q, want %q", tc.name, got, want)
			}
		}
	}
}

// TestPeriodic walks through the happy path of a periodic job.
func TestPeriodic(t *testing.T) {
	per := config.Periodic{
		JobBase: config.JobBase{
			Name:    "ci-periodic-job",
			Agent:   "kubernetes",
			Cluster: "trusted",
			Spec:    &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
		},

		RunAfterSuccess: []config.Periodic{
			{
				JobBase: config.JobBase{
					Name:  "ci-periodic-job-2",
					Agent: "kubernetes",
					Spec:  &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
				},
			},
		},
	}

	totServ := httptest.NewServer(http.HandlerFunc(handleTot))
	defer totServ.Close()
	fc := &fkc{
		prowjobs: []kube.ProwJob{pjutil.NewProwJob(pjutil.PeriodicSpec(per), nil)},
	}
	buildClients := map[string]corev1.PodInterface{
		kube.DefaultClusterAlias: fake.NewSimpleClientset().CoreV1().Pods("pods"),
		"trusted":                fake.NewSimpleClientset().CoreV1().Pods("pods"),
	}
	c := Controller{
		kc:           fc,
		ghc:          &fghc{},
		buildClients: buildClients,
		log:          logrus.NewEntry(logrus.StandardLogger()),
		config:       newFakeConfigAgent(t, 0).Config,
		totURL:       totServ.URL,
		pendingJobs:  make(map[string]int),
		lock:         sync.RWMutex{},
	}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on first sync: %v", err)
	}
	if len(fc.prowjobs[0].Spec.PodSpec.Containers) != 1 || fc.prowjobs[0].Spec.PodSpec.Containers[0].Name != "test-name" {
		t.Fatalf("Sync step updated the pod spec: %#v", fc.prowjobs[0].Spec.PodSpec)
	}
	podsAfterSync, err := buildClients["trusted"].List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("could not list pods from the client: %v", err)
	}
	if len(podsAfterSync.Items) != 1 {
		t.Fatal("Didn't create pod on first sync.")
	}
	if len(podsAfterSync.Items[0].Spec.Containers) != 1 {
		t.Fatal("Wiped container list.")
	}
	if len(podsAfterSync.Items[0].Spec.Containers[0].Env) == 0 {
		t.Fatal("Container has no env set.")
	}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on second sync: %v", err)
	}
	podsAfterSecondSync, err := buildClients["trusted"].List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("could not list pods from the client: %v", err)
	}
	if len(podsAfterSecondSync.Items) != 1 {
		t.Fatalf("Wrong number of pods after second sync: %d", len(podsAfterSecondSync.Items))
	}
	update := podsAfterSecondSync.Items[0].DeepCopy()
	update.Status.Phase = coreapi.PodSucceeded
	if _, err := buildClients["trusted"].Update(update); err != nil {
		t.Fatalf("could not update pod to be succeeded: %v", err)
	}
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
}

func TestRunAfterSuccessCanRun(t *testing.T) {
	tests := []struct {
		name string

		parent *kube.ProwJob
		child  *kube.ProwJob

		changes []github.PullRequestChange
		err     error

		expected bool
	}{
		{
			name: "child does not require specific changes",
			parent: &kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:  "test-e2e",
					Type: kube.PresubmitJob,
					Refs: &kube.Refs{
						Org:  "kubernetes",
						Repo: "kubernetes",
						Pulls: []kube.Pull{
							{Number: 123},
						},
					},
				},
			},
			child: &kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job: "push-image",
				},
			},
			expected: true,
		},
		{
			name: "child requires specific changes that are done",
			parent: &kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:  "test-bazel-build",
					Type: kube.PresubmitJob,
					Refs: &kube.Refs{
						Org:  "kubernetes",
						Repo: "kubernetes",
						Pulls: []kube.Pull{
							{Number: 123},
						},
					},
				},
			},
			child: &kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job: "test-kubeadm-cloud",
				},
			},
			changes: []github.PullRequestChange{
				{Filename: "cmd/kubeadm/kubeadm.go"},
				{Filename: "vendor/BUILD"},
				{Filename: ".gitatrributes"},
			},
			expected: true,
		},
		{
			name: "child requires specific changes that are not done",
			parent: &kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:  "test-bazel-build",
					Type: kube.PresubmitJob,
					Refs: &kube.Refs{
						Org:  "kubernetes",
						Repo: "kubernetes",
						Pulls: []kube.Pull{
							{Number: 123},
						},
					},
				},
			},
			child: &kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job: "test-kubeadm-cloud",
				},
			},
			changes: []github.PullRequestChange{
				{Filename: "vendor/BUILD"},
				{Filename: ".gitatrributes"},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Logf("scenario %q", test.name)

		fakeGH := &fghc{
			changes: test.changes,
			err:     test.err,
		}

		c := Controller{
			log:    logrus.NewEntry(logrus.StandardLogger()),
			config: newFakeConfigAgent(t, 0).Config,
			ghc:    fakeGH,
		}

		got := c.runAfterSuccessCanRun(test.parent, test.child)
		if got != test.expected {
			t.Errorf("expected to run: %t, got: %t", test.expected, got)
		}
	}
}

func TestMaxConcurrencyWithNewlyTriggeredJobs(t *testing.T) {
	tests := []struct {
		name         string
		pjs          []kube.ProwJob
		pendingJobs  map[string]int
		expectedPods int
	}{
		{
			name: "avoid starting a triggered job",
			pjs: []kube.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "first",
					},
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 1,
						PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
						Refs:           &kube.Refs{Org: "fejtaverse"},
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "second",
					},
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 1,
						PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
						Refs:           &kube.Refs{Org: "fejtaverse"},
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
			},
			pendingJobs:  make(map[string]int),
			expectedPods: 1,
		},
		{
			name: "both triggered jobs can start",
			pjs: []kube.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "first",
					},
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 2,
						PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
						Refs:           &kube.Refs{Org: "fejtaverse"},
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "second",
					},
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 2,
						PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
						Refs:           &kube.Refs{Org: "fejtaverse"},
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
			},
			pendingJobs:  make(map[string]int),
			expectedPods: 2,
		},
		{
			name: "no triggered job can start",
			pjs: []kube.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "first",
					},
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 5,
						PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
						Refs:           &kube.Refs{Org: "fejtaverse"},
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "second",
					},
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 5,
						PodSpec:        &coreapi.PodSpec{Containers: []coreapi.Container{{Name: "test-name", Env: []coreapi.EnvVar{}}}},
						Refs:           &kube.Refs{Org: "fejtaverse"},
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
			},
			pendingJobs:  map[string]int{"test-bazel-build": 5},
			expectedPods: 0,
		},
	}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)
		jobs := make(chan kube.ProwJob, len(test.pjs))
		for _, pj := range test.pjs {
			jobs <- pj
		}
		close(jobs)

		fc := &fkc{
			prowjobs: test.pjs,
		}
		buildClients := map[string]corev1.PodInterface{
			kube.DefaultClusterAlias: fake.NewSimpleClientset().CoreV1().Pods("pods"),
		}
		c := Controller{
			kc:           fc,
			buildClients: buildClients,
			log:          logrus.NewEntry(logrus.StandardLogger()),
			config:       newFakeConfigAgent(t, 0).Config,
			pendingJobs:  test.pendingJobs,
		}

		reports := make(chan kube.ProwJob, len(test.pjs))
		errors := make(chan error, len(test.pjs))
		pm := make(map[string]coreapi.Pod)

		syncProwJobs(c.log, c.syncTriggeredJob, 20, jobs, reports, errors, pm)
		podsAfterSync, err := buildClients[kube.DefaultClusterAlias].List(metav1.ListOptions{})
		if err != nil {
			t.Fatalf("could not list pods from the client: %v", err)
		}
		if len(podsAfterSync.Items) != test.expectedPods {
			t.Errorf("expected pods: %d, got: %d", test.expectedPods, len(podsAfterSync.Items))
		}
	}
}

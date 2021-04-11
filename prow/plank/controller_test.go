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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

const (
	podPendingTimeout     = time.Hour
	podRunningTimeout     = time.Hour * 2
	podUnscheduledTimeout = time.Minute * 5
)

func newFakeConfigAgent(t *testing.T, maxConcurrency int) *fca {
	presubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name: "test-bazel-build",
			},
		},
		{
			JobBase: config.JobBase{
				Name: "test-e2e",
			},
		},
		{
			AlwaysRun: true,
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
				ProwJobNamespace: "prowjobs",
				PodNamespace:     "pods",
				Plank: config.Plank{
					Controller: config.Controller{
						JobURLTemplate: template.Must(template.New("test").Parse("{{.ObjectMeta.Name}}/{{.Status.State}}")),
						MaxConcurrency: maxConcurrency,
						MaxGoroutines:  20,
					},
					PodPendingTimeout:     &metav1.Duration{Duration: podPendingTimeout},
					PodRunningTimeout:     &metav1.Duration{Duration: podRunningTimeout},
					PodUnscheduledTimeout: &metav1.Duration{Duration: podUnscheduledTimeout},
				},
			},
			JobConfig: config.JobConfig{
				PresubmitsStatic: presubmitMap,
			},
		},
	}
}

func (f *fca) Config() *config.Config {
	f.Lock()
	defer f.Unlock()
	return f.c
}

func TestTerminateDupes(t *testing.T) {
	now := time.Now()
	nowFn := func() *metav1.Time {
		reallyNow := metav1.NewTime(now)
		return &reallyNow
	}
	type testCase struct {
		Name string

		PJs []prowapi.ProwJob

		TerminatedPJs sets.String
	}
	var testcases = []testCase{
		{
			Name: "terminate all duplicates",

			PJs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j1",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j1",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j1",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "complete", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j1",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-3 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest_j2", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j2",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old_j2", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j2",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old_j3", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j3",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "new_j3", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Job:  "j3",
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
			},

			TerminatedPJs: sets.NewString("old", "older", "old_j2", "old_j3"),
		},
		{
			Name: "should also terminate pods",

			PJs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PresubmitJob,
						Job:     "j1",
						Refs:    &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
						PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "prowjobs"},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PresubmitJob,
						Job:     "j1",
						Refs:    &prowapi.Refs{Pulls: []prowapi.Pull{{}}},
						PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
					},
					Status: prowapi.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},

			TerminatedPJs: sets.NewString("old"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			var prowJobs []runtime.Object
			for i := range tc.PJs {
				pj := &tc.PJs[i]
				prowJobs = append(prowJobs, pj)
			}
			fakeProwJobClient := &patchTrackingFakeClient{
				Client: fakectrlruntimeclient.NewFakeClient(prowJobs...),
			}
			fca := &fca{
				c: &config.Config{
					ProwConfig: config.ProwConfig{
						ProwJobNamespace: "prowjobs",
						PodNamespace:     "pods",
					},
				},
			}
			log := logrus.NewEntry(logrus.StandardLogger())

			r := &reconciler{
				pjClient: fakeProwJobClient,
				log:      log,
				config:   fca.Config,
				clock:    clock.RealClock{},
			}
			for _, pj := range tc.PJs {
				res, err := r.reconcile(context.Background(), &pj)
				if res != nil {
					err = utilerrors.NewAggregate([]error{err, fmt.Errorf("expected reconcile.Result to be nil, was %v", res)})
				}
				if err != nil {
					t.Fatalf("Error terminating dupes: %v", err)
				}
			}

			observedCompletedProwJobs := fakeProwJobClient.patched
			if missing := tc.TerminatedPJs.Difference(observedCompletedProwJobs); missing.Len() > 0 {
				t.Errorf("did not delete expected prowJobs: %v", missing.List())
			}
			if extra := observedCompletedProwJobs.Difference(tc.TerminatedPJs); extra.Len() > 0 {
				t.Errorf("found unexpectedly deleted prowJobs: %v", extra.List())
			}

		})
	}
}

func handleTot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "0987654321")
}

func TestSyncTriggeredJobs(t *testing.T) {
	fakeClock := clock.NewFakeClock(time.Now().Truncate(1 * time.Second))
	pendingTime := metav1.NewTime(fakeClock.Now())

	type testCase struct {
		Name string

		PJ             prowapi.ProwJob
		PendingJobs    map[string]int
		MaxConcurrency int
		Pods           map[string][]v1.Pod
		PodErr         error

		ExpectedState       prowapi.ProwJobState
		ExpectedPodHasName  bool
		ExpectedNumPods     map[string]int
		ExpectedCreatedPJs  int
		ExpectedComplete    bool
		ExpectedURL         string
		ExpectedBuildID     string
		ExpectError         bool
		ExpectedPendingTime *metav1.Time
	}

	var testcases = []testCase{
		{
			Name: "start new pod",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "blabla",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods:                map[string][]v1.Pod{"default": {}},
			ExpectedState:       prowapi.PendingState,
			ExpectedPendingTime: &pendingTime,
			ExpectedPodHasName:  true,
			ExpectedNumPods:     map[string]int{"default": 1},
			ExpectedURL:         "blabla/pending",
			ExpectedBuildID:     "0987654321",
		},
		{
			Name: "pod with a max concurrency of 1",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "blabla",
					Namespace:         "prowjobs",
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					Job:            "same",
					Type:           prowapi.PeriodicJob,
					MaxConcurrency: 1,
					PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			PendingJobs: map[string]int{
				"same": 1,
			},
			Pods: map[string][]v1.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "same-42",
							Namespace: "pods",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			ExpectedState:   prowapi.TriggeredState,
			ExpectedNumPods: map[string]int{"default": 1},
		},
		{
			Name: "trusted pod with a max concurrency of 1",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "blabla",
					Namespace:         "prowjobs",
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					Job:            "same",
					Type:           prowapi.PeriodicJob,
					Cluster:        "trusted",
					MaxConcurrency: 1,
					PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			PendingJobs: map[string]int{
				"same": 1,
			},
			Pods: map[string][]v1.Pod{
				"trusted": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "same-42",
							Namespace: "pods",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			ExpectedState:   prowapi.TriggeredState,
			ExpectedNumPods: map[string]int{"trusted": 1},
		},
		{
			Name: "trusted pod with a max concurrency of 1 (can start)",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:            "some",
					Type:           prowapi.PeriodicJob,
					Cluster:        "trusted",
					MaxConcurrency: 1,
					PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods: map[string][]v1.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-42",
							Namespace: "pods",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
				"trusted": {},
			},
			ExpectedState:       prowapi.PendingState,
			ExpectedNumPods:     map[string]int{"default": 1, "trusted": 1},
			ExpectedPodHasName:  true,
			ExpectedPendingTime: &pendingTime,
			ExpectedURL:         "some/pending",
		},
		{
			Name: "do not exceed global maxconcurrency",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "beer",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "same",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			MaxConcurrency: 20,
			PendingJobs:    map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			ExpectedState:  prowapi.TriggeredState,
		},
		{
			Name: "global maxconcurrency allows new jobs when possible",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "beer",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "same",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods:                map[string][]v1.Pod{"default": {}},
			MaxConcurrency:      21,
			PendingJobs:         map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			ExpectedState:       prowapi.PendingState,
			ExpectedNumPods:     map[string]int{"default": 1},
			ExpectedURL:         "beer/pending",
			ExpectedPendingTime: &pendingTime,
		},
		{
			Name: "unprocessable prow job",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "beer",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods: map[string][]v1.Pod{"default": {}},
			PodErr: &kapierrors.StatusError{ErrStatus: metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusUnprocessableEntity,
				Reason: metav1.StatusReasonInvalid,
			}},
			ExpectedState:    prowapi.ErrorState,
			ExpectedComplete: true,
		},
		{
			Name: "forbidden prow job",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "beer",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods: map[string][]v1.Pod{"default": {}},
			PodErr: &kapierrors.StatusError{ErrStatus: metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusForbidden,
				Reason: metav1.StatusReasonForbidden,
			}},
			ExpectedState:    prowapi.ErrorState,
			ExpectedComplete: true,
		},
		{
			Name: "conflict error starting pod",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "beer",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods: map[string][]v1.Pod{"default": {}},
			PodErr: &kapierrors.StatusError{ErrStatus: metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusConflict,
				Reason: metav1.StatusReasonAlreadyExists,
			}},
			ExpectedState:    prowapi.ErrorState,
			ExpectedComplete: true,
		},
		{
			Name: "unknown error starting pod",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "beer",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			PodErr:        errors.New("no way unknown jose"),
			ExpectedState: prowapi.TriggeredState,
			ExpectError:   true,
		},
		{
			Name: "running pod, failed prowjob update",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods: map[string][]v1.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "pods",
							Labels: map[string]string{
								kube.ProwBuildIDLabel: "0987654321",
							},
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			ExpectedState:       prowapi.PendingState,
			ExpectedNumPods:     map[string]int{"default": 1},
			ExpectedPendingTime: &pendingTime,
			ExpectedURL:         "foo/pending",
			ExpectedBuildID:     "0987654321",
			ExpectedPodHasName:  true,
		},
		{
			Name: "running pod, failed prowjob update, backwards compatible on pods with build label not set",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PeriodicJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			Pods: map[string][]v1.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "pods",
							Labels: map[string]string{
								kube.ProwBuildIDLabel: "",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name: "test-name",
									Env: []v1.EnvVar{
										{
											Name:  "BUILD_ID",
											Value: "0987654321",
										},
									},
								},
							},
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			ExpectedState:       prowapi.PendingState,
			ExpectedNumPods:     map[string]int{"default": 1},
			ExpectedPendingTime: &pendingTime,
			ExpectedURL:         "foo/pending",
			ExpectedBuildID:     "0987654321",
			ExpectedPodHasName:  true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			totServ := httptest.NewServer(http.HandlerFunc(handleTot))
			defer totServ.Close()
			pm := make(map[string]v1.Pod)
			for _, pods := range tc.Pods {
				for i := range pods {
					pm[pods[i].ObjectMeta.Name] = pods[i]
				}
			}
			tc.PJ.Spec.Agent = prowapi.KubernetesAgent
			fakeProwJobClient := fakectrlruntimeclient.NewFakeClient(&tc.PJ)
			buildClients := map[string]ctrlruntimeclient.Client{}
			for alias, pods := range tc.Pods {
				var data []runtime.Object
				for i := range pods {
					pod := pods[i]
					data = append(data, &pod)
				}
				fakeClient := &clientWrapper{
					Client:      fakectrlruntimeclient.NewFakeClient(data...),
					createError: tc.PodErr,
				}
				buildClients[alias] = fakeClient
			}
			if _, exists := buildClients[prowapi.DefaultClusterAlias]; !exists {
				buildClients[prowapi.DefaultClusterAlias] = &clientWrapper{
					Client:      fakectrlruntimeclient.NewFakeClient(),
					createError: tc.PodErr,
				}
			}

			for jobName, numJobsToCreate := range tc.PendingJobs {
				for i := 0; i < numJobsToCreate; i++ {
					if err := fakeProwJobClient.Create(context.Background(), &prowapi.ProwJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-%d", jobName, i),
							Namespace: "prowjobs",
						},
						Spec: prowapi.ProwJobSpec{
							Agent: prowapi.KubernetesAgent,
							Job:   jobName,
						},
					}); err != nil {
						t.Fatalf("failed to create prowJob: %v", err)
					}
				}
			}
			r := &reconciler{
				pjClient:     fakeProwJobClient,
				buildClients: buildClients,
				log:          logrus.NewEntry(logrus.StandardLogger()),
				config:       newFakeConfigAgent(t, tc.MaxConcurrency).Config,
				totURL:       totServ.URL,
				clock:        fakeClock,
			}
			pj := tc.PJ.DeepCopy()
			pj.UID = types.UID("under-test")
			if _, err := r.syncTriggeredJob(context.Background(), pj); (err != nil) != tc.ExpectError {
				if tc.ExpectError {
					t.Errorf("for case %q expected an error, but got none", tc.Name)
				} else {
					t.Errorf("for case %q got an unexpected error: %v", tc.Name, err)
				}
				return
			}
			// In PlankV2 we throw them all into the same client and then count the resulting number
			for _, pendingJobs := range tc.PendingJobs {
				tc.ExpectedCreatedPJs += pendingJobs
			}

			actualProwJobs := &prowapi.ProwJobList{}
			if err := fakeProwJobClient.List(context.Background(), actualProwJobs); err != nil {
				t.Errorf("could not list prowJobs from the client: %v", err)
			}
			if len(actualProwJobs.Items) != tc.ExpectedCreatedPJs+1 {
				t.Errorf("got %d created prowjobs, expected %d", len(actualProwJobs.Items)-1, tc.ExpectedCreatedPJs)
			}
			var actual prowapi.ProwJob
			if err := fakeProwJobClient.Get(context.Background(), types.NamespacedName{Namespace: tc.PJ.Namespace, Name: tc.PJ.Name}, &actual); err != nil {
				t.Errorf("failed to get prowjob from client: %v", err)
			}
			if actual.Status.State != tc.ExpectedState {
				t.Errorf("expected state %v, got state %v", tc.ExpectedState, actual.Status.State)
			}
			if !reflect.DeepEqual(actual.Status.PendingTime, tc.ExpectedPendingTime) {
				t.Errorf("got pending time %v, expected %v", actual.Status.PendingTime, tc.ExpectedPendingTime)
			}
			if (actual.Status.PodName == "") && tc.ExpectedPodHasName {
				t.Errorf("got no pod name, expected one")
			}
			if tc.ExpectedBuildID != "" && actual.Status.BuildID != tc.ExpectedBuildID {
				t.Errorf("expected BuildID: %q, got: %q", tc.ExpectedBuildID, actual.Status.BuildID)
			}
			for alias, expected := range tc.ExpectedNumPods {
				actualPods := &v1.PodList{}
				if err := buildClients[alias].List(context.Background(), actualPods); err != nil {
					t.Errorf("could not list pods from the client: %v", err)
				}
				if got := len(actualPods.Items); got != expected {
					t.Errorf("got %d pods for alias %q, but expected %d", got, alias, expected)
				}
			}
			if actual.Complete() != tc.ExpectedComplete {
				t.Error("got wrong completion")
			}

		})
	}
}

func startTime(s time.Time) *metav1.Time {
	start := metav1.NewTime(s)
	return &start
}

func TestSyncPendingJob(t *testing.T) {

	type testCase struct {
		Name string

		PJ   prowapi.ProwJob
		Pods []v1.Pod
		Err  error

		ExpectedState      prowapi.ProwJobState
		ExpectedNumPods    int
		ExpectedComplete   bool
		ExpectedCreatedPJs int
		ExpectedReport     bool
		ExpectedURL        string
		ExpectedBuildID    string
	}
	var testcases = []testCase{
		{
			Name: "reset when pod goes missing",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-41",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type:    prowapi.PostsubmitJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
					Refs:    &prowapi.Refs{Org: "fejtaverse"},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-41",
				},
			},
			ExpectedState:   prowapi.PendingState,
			ExpectedReport:  true,
			ExpectedNumPods: 1,
			ExpectedURL:     "boop-41/pending",
			ExpectedBuildID: "0987654321",
		},
		{
			Name: "delete pod in unknown state",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-41",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-41",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-41",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase: v1.PodUnknown,
					},
				},
			},
			ExpectedState:   prowapi.PendingState,
			ExpectedNumPods: 0,
		},
		{
			Name: "delete pod in unknown state with gcsreporter finalizer",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-41",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-41",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "boop-41",
						Namespace:  "pods",
						Finalizers: []string{"prow.x-k8s.io/gcsk8sreporter"},
					},
					Status: v1.PodStatus{
						Phase: v1.PodUnknown,
					},
				},
			},
			ExpectedState:   prowapi.PendingState,
			ExpectedNumPods: 0,
		},
		{
			Name: "succeeded pod",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type:    prowapi.BatchJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
					Refs:    &prowapi.Refs{Org: "fejtaverse"},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase: v1.PodSucceeded,
					},
				},
			},
			ExpectedComplete:   true,
			ExpectedState:      prowapi.SuccessState,
			ExpectedNumPods:    1,
			ExpectedCreatedPJs: 0,
			ExpectedURL:        "boop-42/success",
		},
		{
			Name: "succeeded pod with unfinished containers",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type:    prowapi.BatchJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
					Refs:    &prowapi.Refs{Org: "fejtaverse"},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase:             v1.PodSucceeded,
						ContainerStatuses: []v1.ContainerStatus{{LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{}}}},
					},
				},
			},
			ExpectedComplete:   true,
			ExpectedState:      prowapi.ErrorState,
			ExpectedNumPods:    1,
			ExpectedCreatedPJs: 0,
			ExpectedURL:        "boop-42/success",
		},
		{
			Name: "succeeded pod with unfinished initcontainers",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type:    prowapi.BatchJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
					Refs:    &prowapi.Refs{Org: "fejtaverse"},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase:                 v1.PodSucceeded,
						InitContainerStatuses: []v1.ContainerStatus{{LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{}}}},
					},
				},
			},
			ExpectedComplete:   true,
			ExpectedState:      prowapi.ErrorState,
			ExpectedNumPods:    1,
			ExpectedCreatedPJs: 0,
			ExpectedURL:        "boop-42/success",
		},
		{
			Name: "failed pod",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PresubmitJob,
					Refs: &prowapi.Refs{
						Org: "kubernetes", Repo: "kubernetes",
						BaseRef: "baseref", BaseSHA: "basesha",
						Pulls: []prowapi.Pull{{Number: 100, Author: "me", SHA: "sha"}},
					},
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase: v1.PodFailed,
					},
				},
			},
			ExpectedComplete: true,
			ExpectedState:    prowapi.FailureState,
			ExpectedNumPods:  1,
			ExpectedURL:      "boop-42/failure",
		},
		{
			Name: "delete evicted pod",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase:  v1.PodFailed,
						Reason: Evicted,
					},
				},
			},
			ExpectedComplete: false,
			ExpectedState:    prowapi.PendingState,
			ExpectedNumPods:  0,
		},
		{
			Name: "delete evicted pod and remove its k8sreporter finalizer",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "boop-42",
						Namespace:  "pods",
						Finalizers: []string{"prow.x-k8s.io/gcsk8sreporter"},
					},
					Status: v1.PodStatus{
						Phase:  v1.PodFailed,
						Reason: Evicted,
					},
				},
			},
			ExpectedComplete: false,
			ExpectedState:    prowapi.PendingState,
			ExpectedNumPods:  0,
		},
		{
			Name: "don't delete evicted pod w/ error_on_eviction, complete PJ instead",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					ErrorOnEviction: true,
					PodSpec:         &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
				},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase:  v1.PodFailed,
						Reason: Evicted,
					},
				},
			},
			ExpectedComplete: true,
			ExpectedState:    prowapi.ErrorState,
			ExpectedNumPods:  1,
			ExpectedURL:      "boop-42/error",
		},
		{
			Name: "running pod",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
			},
			ExpectedState:   prowapi.PendingState,
			ExpectedNumPods: 1,
		},
		{
			Name: "pod changes url status",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boop-42",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "boop-42",
					URL:     "boop-42/pending",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "boop-42",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase: v1.PodSucceeded,
					},
				},
			},
			ExpectedComplete:   true,
			ExpectedState:      prowapi.SuccessState,
			ExpectedNumPods:    1,
			ExpectedCreatedPJs: 0,
			ExpectedURL:        "boop-42/success",
		},
		{
			Name: "unprocessable prow job",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jose",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:     "boop",
					Type:    prowapi.PostsubmitJob,
					PodSpec: &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
					Refs:    &prowapi.Refs{Org: "fejtaverse"},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.PendingState,
				},
			},
			Err: &kapierrors.StatusError{ErrStatus: metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusUnprocessableEntity,
				Reason: metav1.StatusReasonInvalid,
			}},
			ExpectedState:    prowapi.ErrorState,
			ExpectedComplete: true,
			ExpectedURL:      "jose/error",
		},
		{
			Name: "stale pending prow job",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nightmare",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "nightmare",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "nightmare",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-podPendingTimeout)},
					},
					Status: v1.PodStatus{
						Phase:     v1.PodPending,
						StartTime: startTime(time.Now().Add(-podPendingTimeout)),
					},
				},
			},
			ExpectedState:    prowapi.ErrorState,
			ExpectedNumPods:  0,
			ExpectedComplete: true,
			ExpectedURL:      "nightmare/error",
		},
		{
			Name: "stale running prow job",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endless",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "endless",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "endless",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-podRunningTimeout)},
					},
					Status: v1.PodStatus{
						Phase:     v1.PodRunning,
						StartTime: startTime(time.Now().Add(-podRunningTimeout)),
					},
				},
			},
			ExpectedState:    prowapi.AbortedState,
			ExpectedNumPods:  0,
			ExpectedComplete: true,
			ExpectedURL:      "endless/aborted",
		},
		{
			Name: "stale unschedulable prow job",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "homeless",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "homeless",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "homeless",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-podUnscheduledTimeout - time.Second)},
					},
					Status: v1.PodStatus{
						Phase: v1.PodPending,
					},
				},
			},
			ExpectedState:    prowapi.ErrorState,
			ExpectedNumPods:  0,
			ExpectedComplete: true,
			ExpectedURL:      "homeless/error",
		},
		{
			Name: "scheduled, pending started more than podUnscheduledTimeout ago",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "slowpoke",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "slowpoke",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "slowpoke",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-podUnscheduledTimeout * 2)},
					},
					Status: v1.PodStatus{
						Phase:     v1.PodPending,
						StartTime: startTime(time.Now().Add(-podUnscheduledTimeout * 2)),
					},
				},
			},
			ExpectedState:   prowapi.PendingState,
			ExpectedNumPods: 1,
		},
		{
			Name: "unscheduled, created less than podUnscheduledTimeout ago",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "just-waiting",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "just-waiting",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "just-waiting",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-time.Second)},
					},
					Status: v1.PodStatus{
						Phase: v1.PodPending,
					},
				},
			},
			ExpectedState:   prowapi.PendingState,
			ExpectedNumPods: 1,
		},
		{
			Name: "Pod deleted in pending phase, job marked as errored",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted-pod-in-pending-marks-job-as-errored",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "deleted-pod-in-pending-marks-job-as-errored",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "deleted-pod-in-pending-marks-job-as-errored",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-time.Second)},
						DeletionTimestamp: func() *metav1.Time { n := metav1.Now(); return &n }(),
					},
					Status: v1.PodStatus{
						Phase: v1.PodPending,
					},
				},
			},
			ExpectedState:    prowapi.ErrorState,
			ExpectedComplete: true,
			ExpectedNumPods:  1,
		},
		{
			Name: "Pod deleted in unset phase, job marked as errored",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-deleted-in-unset-phase",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "pod-deleted-in-unset-phase",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod-deleted-in-unset-phase",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-time.Second)},
						DeletionTimestamp: func() *metav1.Time { n := metav1.Now(); return &n }(),
					},
				},
			},
			ExpectedState:    prowapi.ErrorState,
			ExpectedComplete: true,
			ExpectedNumPods:  1,
		},
		{
			Name: "Pod deleted in running phase, job marked as errored",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-deleted-in-unset-phase",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "pod-deleted-in-unset-phase",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod-deleted-in-unset-phase",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-time.Second)},
						DeletionTimestamp: func() *metav1.Time { n := metav1.Now(); return &n }(),
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
			},
			ExpectedState:    prowapi.ErrorState,
			ExpectedComplete: true,
			ExpectedNumPods:  1,
		},
		{
			Name: "Pod deleted with NodeLost reason in running phase, pod finalizer gets cleaned up",
			PJ: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-deleted-in-running-phase",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{},
				Status: prowapi.ProwJobStatus{
					State:   prowapi.PendingState,
					PodName: "pod-deleted-in-running-phase",
				},
			},
			Pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod-deleted-in-running-phase",
						Namespace:         "pods",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-time.Second)},
						DeletionTimestamp: func() *metav1.Time { n := metav1.Now(); return &n }(),
						Finalizers:        []string{"prow.x-k8s.io/gcsk8sreporter"},
					},
					Status: v1.PodStatus{
						Phase:  v1.PodRunning,
						Reason: "NodeLost",
					},
				},
			},
			ExpectedState:    prowapi.PendingState,
			ExpectedComplete: false,
			ExpectedNumPods:  1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			totServ := httptest.NewServer(http.HandlerFunc(handleTot))
			defer totServ.Close()
			pm := make(map[string]v1.Pod)
			for i := range tc.Pods {
				pm[tc.Pods[i].ObjectMeta.Name] = tc.Pods[i]
			}
			fakeProwJobClient := fakectrlruntimeclient.NewFakeClient(&tc.PJ)
			var data []runtime.Object
			for i := range tc.Pods {
				pod := tc.Pods[i]
				data = append(data, &pod)
			}
			fakeClient := &clientWrapper{
				Client:                   fakectrlruntimeclient.NewFakeClient(data...),
				createError:              tc.Err,
				errOnDeleteWithFinalizer: true,
			}
			buildClients := map[string]ctrlruntimeclient.Client{
				prowapi.DefaultClusterAlias: fakeClient,
			}

			r := &reconciler{
				pjClient:     fakeProwJobClient,
				buildClients: buildClients,
				log:          logrus.NewEntry(logrus.StandardLogger()),
				config:       newFakeConfigAgent(t, 0).Config,
				totURL:       totServ.URL,
				clock:        clock.RealClock{},
			}
			if err := r.syncPendingJob(context.Background(), &tc.PJ); err != nil {
				t.Fatalf("syncPendingJob failed: %v", err)
			}

			actualProwJobs := &prowapi.ProwJobList{}
			if err := fakeProwJobClient.List(context.Background(), actualProwJobs); err != nil {
				t.Errorf("could not list prowJobs from the client: %v", err)
			}
			if len(actualProwJobs.Items) != tc.ExpectedCreatedPJs+1 {
				t.Errorf("got %d created prowjobs", len(actualProwJobs.Items)-1)
			}
			actual := actualProwJobs.Items[0]
			if actual.Status.State != tc.ExpectedState {
				t.Errorf("got state %v", actual.Status.State)
			}
			if tc.ExpectedBuildID != "" && actual.Status.BuildID != tc.ExpectedBuildID {
				t.Errorf("expected BuildID %q, got %q", tc.ExpectedBuildID, actual.Status.BuildID)
			}
			actualPods := &v1.PodList{}
			if err := buildClients[prowapi.DefaultClusterAlias].List(context.Background(), actualPods); err != nil {
				t.Errorf("could not list pods from the client: %v", err)
			}
			if got := len(actualPods.Items); got != tc.ExpectedNumPods {
				t.Errorf("got %d pods, expected %d", len(actualPods.Items), tc.ExpectedNumPods)
			}
			for _, pod := range actualPods.Items {
				if pod.DeletionTimestamp != nil && len(pod.Finalizers) != 0 {
					t.Errorf("pod %s was deleted but still had finalizers: %v", pod.Name, pod.Finalizers)
				}
			}
			if actual := actual.Complete(); actual != tc.ExpectedComplete {
				t.Errorf("expected complete: %t, got complete: %t", tc.ExpectedComplete, actual)
			}

		})
	}
}

// TestPeriodic walks through the happy path of a periodic job.
func TestPeriodic(t *testing.T) {
	per := config.Periodic{
		JobBase: config.JobBase{
			Name:    "ci-periodic-job",
			Agent:   "kubernetes",
			Cluster: "trusted",
			Spec:    &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
		},
	}

	totServ := httptest.NewServer(http.HandlerFunc(handleTot))
	defer totServ.Close()
	pj := pjutil.NewProwJob(pjutil.PeriodicSpec(per), nil, nil)
	pj.Namespace = "prowjobs"
	fakeProwJobClient := fakectrlruntimeclient.NewFakeClient(&pj)
	buildClients := map[string]ctrlruntimeclient.Client{
		prowapi.DefaultClusterAlias: fakectrlruntimeclient.NewFakeClient(),
		"trusted":                   fakectrlruntimeclient.NewFakeClient(),
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	log := logrus.NewEntry(logger)
	r := reconciler{
		pjClient:     fakeProwJobClient,
		buildClients: buildClients,
		log:          log,
		config:       newFakeConfigAgent(t, 0).Config,
		totURL:       totServ.URL,
		clock:        clock.RealClock{},
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "prowjobs", Name: pj.Name}}); err != nil {
		t.Fatalf("Error on first sync: %v", err)
	}

	afterFirstSync := &prowapi.ProwJobList{}
	if err := fakeProwJobClient.List(context.Background(), afterFirstSync); err != nil {
		t.Fatalf("could not list prowJobs from the client: %v", err)
	}
	if len(afterFirstSync.Items) != 1 {
		t.Fatalf("saw %d prowjobs after sync, not 1", len(afterFirstSync.Items))
	}
	if len(afterFirstSync.Items[0].Spec.PodSpec.Containers) != 1 || afterFirstSync.Items[0].Spec.PodSpec.Containers[0].Name != "test-name" {
		t.Fatalf("Sync step updated the pod spec: %#v", afterFirstSync.Items[0].Spec.PodSpec)
	}
	podsAfterSync := &v1.PodList{}
	if err := buildClients["trusted"].List(context.Background(), podsAfterSync); err != nil {
		t.Fatalf("could not list pods from the client: %v", err)
	}
	if len(podsAfterSync.Items) != 1 {
		t.Fatalf("expected exactly one pod, got %d", len(podsAfterSync.Items))
	}
	if len(podsAfterSync.Items[0].Spec.Containers) != 1 {
		t.Fatal("Wiped container list.")
	}
	if len(podsAfterSync.Items[0].Spec.Containers[0].Env) == 0 {
		t.Fatal("Container has no env set.")
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "prowjobs", Name: pj.Name}}); err != nil {
		t.Fatalf("Error on second sync: %v", err)
	}
	podsAfterSecondSync := &v1.PodList{}
	if err := buildClients["trusted"].List(context.Background(), podsAfterSecondSync); err != nil {
		t.Fatalf("could not list pods from the client: %v", err)
	}
	if len(podsAfterSecondSync.Items) != 1 {
		t.Fatalf("Wrong number of pods after second sync: %d", len(podsAfterSecondSync.Items))
	}
	update := podsAfterSecondSync.Items[0].DeepCopy()
	update.Status.Phase = v1.PodSucceeded
	if err := buildClients["trusted"].Update(context.Background(), update); err != nil {
		t.Fatalf("could not update pod to be succeeded: %v", err)
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "prowjobs", Name: pj.Name}}); err != nil {
		t.Fatalf("Error on third sync: %v", err)
	}
	afterThirdSync := &prowapi.ProwJobList{}
	if err := fakeProwJobClient.List(context.Background(), afterThirdSync); err != nil {
		t.Fatalf("could not list prowJobs from the client: %v", err)
	}
	if len(afterThirdSync.Items) != 1 {
		t.Fatalf("Wrong number of prow jobs: %d", len(afterThirdSync.Items))
	}
	if !afterThirdSync.Items[0].Complete() {
		t.Fatal("Prow job didn't complete.")
	}
	if afterThirdSync.Items[0].Status.State != prowapi.SuccessState {
		t.Fatalf("Should be success: %v", afterThirdSync.Items[0].Status.State)
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "prowjobs", Name: pj.Name}}); err != nil {
		t.Fatalf("Error on fourth sync: %v", err)
	}
}

func TestMaxConcurrencyWithNewlyTriggeredJobs(t *testing.T) {
	type testCase struct {
		Name         string
		PJs          []prowapi.ProwJob
		PendingJobs  map[string]int
		ExpectedPods int
	}

	tests := []testCase{
		{
			Name: "avoid starting a triggered job",
			PJs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "first",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 1,
						PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
						Refs:           &prowapi.Refs{Org: "fejtaverse"},
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "second",
						CreationTimestamp: metav1.Now(),
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 1,
						PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
						Refs:           &prowapi.Refs{Org: "fejtaverse"},
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
			},
			PendingJobs:  make(map[string]int),
			ExpectedPods: 1,
		},
		{
			Name: "both triggered jobs can start",
			PJs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "first",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 2,
						PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
						Refs:           &prowapi.Refs{Org: "fejtaverse"},
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "second",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 2,
						PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
						Refs:           &prowapi.Refs{Org: "fejtaverse"},
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
			},
			PendingJobs:  make(map[string]int),
			ExpectedPods: 2,
		},
		{
			Name: "no triggered job can start",
			PJs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "first",
						CreationTimestamp: metav1.Now(),
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 5,
						PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
						Refs:           &prowapi.Refs{Org: "fejtaverse"},
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "second",
						CreationTimestamp: metav1.Now(),
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 5,
						PodSpec:        &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
						Refs:           &prowapi.Refs{Org: "fejtaverse"},
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
			},
			PendingJobs:  map[string]int{"test-bazel-build": 5},
			ExpectedPods: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			jobs := make(chan prowapi.ProwJob, len(test.PJs))
			for _, pj := range test.PJs {
				jobs <- pj
			}
			close(jobs)

			var prowJobs []runtime.Object
			for i := range test.PJs {
				test.PJs[i].Namespace = "prowjobs"
				test.PJs[i].Spec.Agent = prowapi.KubernetesAgent
				test.PJs[i].UID = types.UID(strconv.Itoa(i))
				prowJobs = append(prowJobs, &test.PJs[i])
			}
			fakeProwJobClient := fakectrlruntimeclient.NewFakeClient(prowJobs...)
			buildClients := map[string]ctrlruntimeclient.Client{
				prowapi.DefaultClusterAlias: fakectrlruntimeclient.NewFakeClient(),
			}
			for jobName, numJobsToCreate := range test.PendingJobs {
				for i := 0; i < numJobsToCreate; i++ {
					if err := fakeProwJobClient.Create(context.Background(), &prowapi.ProwJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-%d", jobName, i),
							Namespace: "prowjobs",
						},
						Spec: prowapi.ProwJobSpec{
							Agent: prowapi.KubernetesAgent,
							Job:   jobName,
						},
						Status: prowapi.ProwJobStatus{
							State: prowapi.PendingState,
						},
					}); err != nil {
						t.Fatalf("failed to create prowJob: %v", err)
					}
				}
			}
			r := newReconciler(context.Background(),
				&indexingClient{
					Client:     fakeProwJobClient,
					indexFuncs: map[string]ctrlruntimeclient.IndexerFunc{prowJobIndexName: prowJobIndexer("prowjobs")},
				}, nil, newFakeConfigAgent(t, 0).Config, "")
			r.buildClients = buildClients
			for _, job := range test.PJs {
				request := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      job.Name,
					Namespace: job.Namespace,
				}}
				if _, err := r.Reconcile(context.Background(), request); err != nil {
					t.Fatalf("failed to reconcile job %s: %v", request.String(), err)
				}
			}

			podsAfterSync := &v1.PodList{}
			if err := buildClients[prowapi.DefaultClusterAlias].List(context.Background(), podsAfterSync); err != nil {
				t.Fatalf("could not list pods from the client: %v", err)
			}
			if len(podsAfterSync.Items) != test.ExpectedPods {
				t.Errorf("expected pods: %d, got: %d", test.ExpectedPods, len(podsAfterSync.Items))
			}
		})
	}
}

func TestMaxConcurency(t *testing.T) {
	type testCase struct {
		Name             string
		ProwJob          prowapi.ProwJob
		ExistingProwJobs []prowapi.ProwJob
		PendingJobs      map[string]int

		ExpectedResult bool
	}
	testCases := []testCase{
		{
			Name:           "Max concurency 0 always runs",
			ProwJob:        prowapi.ProwJob{Spec: prowapi.ProwJobSpec{MaxConcurrency: 0}},
			ExpectedResult: true,
		},
		{
			Name: "Num pending exceeds max concurrency",
			ProwJob: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.Now()},
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 10,
					Job:            "my-pj"}},
			PendingJobs:    map[string]int{"my-pj": 10},
			ExpectedResult: false,
		},
		{
			Name: "Num pending plus older instances equals max concurency",
			ProwJob: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 10,
					Job:            "my-pj"},
			},
			ExistingProwJobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: "prowjobs"},
					Spec:       prowapi.ProwJobSpec{Agent: prowapi.KubernetesAgent, Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					}},
			},
			PendingJobs:    map[string]int{"my-pj": 9},
			ExpectedResult: false,
		},
		{
			Name: "Num pending plus older instances exceeds max concurency",
			ProwJob: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 10,
					Job:            "my-pj"},
			},
			ExistingProwJobs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					}},
			},
			PendingJobs:    map[string]int{"my-pj": 10},
			ExpectedResult: false,
		},
		{
			Name: "Have other jobs that are newer, can execute",
			ProwJob: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 1,
					Job:            "my-pj"},
			},
			ExistingProwJobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.Now(),
					},
					Spec: prowapi.ProwJobSpec{Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					}},
			},
			ExpectedResult: true,
		},
		{
			Name: "Have older jobs that are not triggered, can execute",
			ProwJob: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 2,
					Job:            "my-pj"},
			},
			ExistingProwJobs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						CompletionTime: &[]metav1.Time{{}}[0],
					}},
			},
			PendingJobs:    map[string]int{"my-pj": 1},
			ExpectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {

			if tc.PendingJobs == nil {
				tc.PendingJobs = map[string]int{}
			}
			buildClients := map[string]ctrlruntimeclient.Client{}
			logrus.SetLevel(logrus.DebugLevel)

			var prowJobs []runtime.Object
			for i := range tc.ExistingProwJobs {
				tc.ExistingProwJobs[i].Namespace = "prowjobs"
				prowJobs = append(prowJobs, &tc.ExistingProwJobs[i])
			}
			for jobName, numJobsToCreate := range tc.PendingJobs {
				for i := 0; i < numJobsToCreate; i++ {
					prowJobs = append(prowJobs, &prowapi.ProwJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-%d", jobName, i),
							Namespace: "prowjobs",
						},
						Spec: prowapi.ProwJobSpec{
							Agent: prowapi.KubernetesAgent,
							Job:   jobName,
						},
						Status: prowapi.ProwJobStatus{
							State: prowapi.PendingState,
						},
					})
				}
			}
			r := &reconciler{
				pjClient: &indexingClient{
					Client:     fakectrlruntimeclient.NewFakeClient(prowJobs...),
					indexFuncs: map[string]ctrlruntimeclient.IndexerFunc{prowJobIndexName: prowJobIndexer("prowjobs")},
				},
				buildClients: buildClients,
				log:          logrus.NewEntry(logrus.StandardLogger()),
				config:       newFakeConfigAgent(t, 0).Config,
				clock:        clock.RealClock{},
			}
			// We filter ourselves out via the UID, so make sure its not the empty string
			tc.ProwJob.UID = types.UID("under-test")
			result, err := r.canExecuteConcurrently(context.Background(), &tc.ProwJob)
			if err != nil {
				t.Fatalf("canExecuteConcurrently: %v", err)
			}

			if result != tc.ExpectedResult {
				t.Errorf("Expected max_concurrency to allow job: %t, result was %t", tc.ExpectedResult, result)
			}
		})
	}

}

type patchTrackingFakeClient struct {
	ctrlruntimeclient.Client
	patched sets.String
}

func (c *patchTrackingFakeClient) Patch(ctx context.Context, obj ctrlruntimeclient.Object, patch ctrlruntimeclient.Patch, opts ...ctrlruntimeclient.PatchOption) error {
	if c.patched == nil {
		c.patched = sets.NewString()
	}
	c.patched.Insert(obj.GetName())
	return c.Client.Patch(ctx, obj, patch, opts...)
}

type deleteTrackingFakeClient struct {
	deleteError error
	ctrlruntimeclient.Client
	deleted sets.String
}

func (c *deleteTrackingFakeClient) Delete(ctx context.Context, obj ctrlruntimeclient.Object, opts ...ctrlruntimeclient.DeleteOption) error {
	if c.deleteError != nil {
		return c.deleteError
	}
	if c.deleted == nil {
		c.deleted = sets.String{}
	}
	if err := c.Client.Delete(ctx, obj, opts...); err != nil {
		return err
	}
	c.deleted.Insert(obj.GetName())
	return nil
}

type clientWrapper struct {
	ctrlruntimeclient.Client
	createError              error
	errOnDeleteWithFinalizer bool
}

func (c *clientWrapper) Create(ctx context.Context, obj ctrlruntimeclient.Object, opts ...ctrlruntimeclient.CreateOption) error {
	if c.createError != nil {
		return c.createError
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *clientWrapper) Delete(ctx context.Context, obj ctrlruntimeclient.Object, opts ...ctrlruntimeclient.DeleteOption) error {
	if len(obj.GetFinalizers()) > 0 {
		return fmt.Errorf("object still had finalizers when attempting to delete: %v", obj.GetFinalizers())
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestSyncAbortedJob(t *testing.T) {
	t.Parallel()

	type testCase struct {
		Name           string
		Pod            *v1.Pod
		DeleteError    error
		ExpectSyncFail bool
		ExpectDelete   bool
		ExpectComplete bool
	}

	testCases := []testCase{
		{
			Name:           "Pod is deleted",
			Pod:            &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-pj"}},
			ExpectDelete:   true,
			ExpectComplete: true,
		},
		{
			Name:           "No pod there",
			ExpectDelete:   false,
			ExpectComplete: true,
		},
		{
			Name:           "NotFound on delete is tolerated",
			Pod:            &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-pj"}},
			DeleteError:    kapierrors.NewNotFound(schema.GroupResource{}, "my-pj"),
			ExpectDelete:   false,
			ExpectComplete: true,
		},
		{
			Name:           "Failed delete does not set job to completed",
			Pod:            &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-pj"}},
			DeleteError:    errors.New("erroring as requested"),
			ExpectSyncFail: true,
			ExpectDelete:   false,
			ExpectComplete: false,
		},
	}

	const cluster = "cluster"
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {

			pj := &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pj",
				},
				Spec: prowapi.ProwJobSpec{
					Cluster: cluster,
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.AbortedState,
				},
			}

			var pods []runtime.Object
			if tc.Pod != nil {
				pods = append(pods, tc.Pod)
			}
			podClient := &deleteTrackingFakeClient{
				deleteError: tc.DeleteError,
				Client:      fakectrlruntimeclient.NewFakeClient(pods...),
			}

			pjClient := fakectrlruntimeclient.NewFakeClient(pj)
			r := &reconciler{
				log:          logrus.NewEntry(logrus.New()),
				config:       func() *config.Config { return &config.Config{} },
				pjClient:     pjClient,
				buildClients: map[string]ctrlruntimeclient.Client{cluster: podClient},
			}

			res, err := r.reconcile(context.Background(), pj)
			if (err != nil) != tc.ExpectSyncFail {
				t.Fatalf("sync failed: %v, expected it to fail: %t", err, tc.ExpectSyncFail)
			}
			if res != nil {
				t.Errorf("expected reconcile.Result to be nil, was %v", res)
			}

			if err := pjClient.Get(context.Background(), types.NamespacedName{Name: pj.Name}, pj); err != nil {
				t.Fatalf("failed to get job from client: %v", err)
			}
			if pj.Complete() != tc.ExpectComplete {
				t.Errorf("expected complete: %t, got complete: %t", tc.ExpectComplete, pj.Complete())
			}

			if tc.ExpectDelete != podClient.deleted.Has(pj.Name) {
				t.Errorf("expected delete: %t, got delete: %t", tc.ExpectDelete, podClient.deleted.Has(pj.Name))
			}
		})
	}
}

type indexingClient struct {
	ctrlruntimeclient.Client
	indexFuncs map[string]ctrlruntimeclient.IndexerFunc
}

func (c *indexingClient) List(ctx context.Context, list ctrlruntimeclient.ObjectList, opts ...ctrlruntimeclient.ListOption) error {
	if err := c.Client.List(ctx, list, opts...); err != nil {
		return err
	}

	listOpts := &ctrlruntimeclient.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	if listOpts.FieldSelector == nil {
		return nil
	}

	if n := len(listOpts.FieldSelector.Requirements()); n == 0 {
		return nil
	} else if n > 1 {
		return fmt.Errorf("the indexing client supports at most one field selector requirement, got %d", n)
	}

	indexKey := listOpts.FieldSelector.Requirements()[0].Field
	if indexKey == "" {
		return nil
	}

	indexFunc, ok := c.indexFuncs[indexKey]
	if !ok {
		return fmt.Errorf("no index with key %q found", indexKey)
	}

	pjList, ok := list.(*prowapi.ProwJobList)
	if !ok {
		return errors.New("indexes are only supported for ProwJobLists")
	}

	result := prowapi.ProwJobList{}
	for _, pj := range pjList.Items {
		for _, indexVal := range indexFunc(&pj) {
			logrus.Infof("indexVal: %q, requirementVal: %q, match: %t, name: %s", indexVal, listOpts.FieldSelector.Requirements()[0].Value, indexVal == listOpts.FieldSelector.Requirements()[0].Value, pj.Name)
			if indexVal == listOpts.FieldSelector.Requirements()[0].Value {
				result.Items = append(result.Items, pj)
			}
		}
	}

	*pjList = result
	return nil
}

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
	v1 "k8s.io/api/core/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clienttesting "k8s.io/client-go/testing"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowfake "k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/reporter"
	"k8s.io/test-infra/prow/pjutil"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

const (
	podPendingTimeout = time.Hour
	podRunningTimeout = time.Hour * 2
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
					PodPendingTimeout: &metav1.Duration{Duration: podPendingTimeout},
					PodRunningTimeout: &metav1.Duration{Duration: podRunningTimeout},
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
		pjs                []prowapi.ProwJob
		pm                 map[string]v1.Pod

		terminatedPJs  sets.String
		terminatedPods sets.String
	}{
		{
			name: "terminate all duplicates",

			pjs: []prowapi.ProwJob{
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

			terminatedPJs: sets.NewString("old", "older", "old_j2", "old_j3"),
		},
		{
			name: "should also terminate pods",

			allowCancellations: true,
			pjs: []prowapi.ProwJob{
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
			pm: map[string]v1.Pod{
				"newest": {ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: "prowjobs"}},
				"old":    {ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "prowjobs"}},
			},

			terminatedPJs:  sets.NewString("old"),
			terminatedPods: sets.NewString("old"),
		},
	}

	for _, tc := range testcases {
		var prowJobs []runtime.Object
		for i := range tc.pjs {
			prowJobs = append(prowJobs, &tc.pjs[i])
		}
		fakeProwJobClient := prowfake.NewSimpleClientset(prowJobs...)
		var pods []runtime.Object
		for name := range tc.pm {
			pod := tc.pm[name]
			pods = append(pods, &pod)
		}
		fakePodClient := fake.NewSimpleClientset(pods...)
		fca := &fca{
			c: &config.Config{
				ProwConfig: config.ProwConfig{
					ProwJobNamespace: "prowjobs",
					PodNamespace:     "pods",
					Plank: config.Plank{
						Controller: config.Controller{
							AllowCancellations: tc.allowCancellations,
						},
					},
				},
			},
		}
		log := logrus.NewEntry(logrus.StandardLogger())
		c := Controller{
			prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			buildClients:  map[string]corev1.PodInterface{prowapi.DefaultClusterAlias: fakePodClient.CoreV1().Pods("pods")},
			log:           log,
			config:        fca.Config,
		}

		if err := c.terminateDupes(tc.pjs, tc.pm); err != nil {
			t.Fatalf("Error terminating dupes: %v", err)
		}

		observedCompletedProwJobs := sets.NewString()
		for _, action := range fakeProwJobClient.Fake.Actions() {
			switch action := action.(type) {
			case clienttesting.UpdateActionImpl:
				if prowJob, ok := action.Object.(*prowapi.ProwJob); ok && prowJob.Complete() {
					observedCompletedProwJobs.Insert(prowJob.Name)
				}
			}
		}
		if missing := tc.terminatedPJs.Difference(observedCompletedProwJobs); missing.Len() > 0 {
			t.Errorf("%s: did not delete expected prowJobs: %v", tc.name, missing.List())
		}
		if extra := observedCompletedProwJobs.Difference(tc.terminatedPJs); extra.Len() > 0 {
			t.Errorf("%s: found unexpectedly deleted prowJobs: %v", tc.name, extra.List())
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

		pj             prowapi.ProwJob
		pendingJobs    map[string]int
		maxConcurrency int
		pods           map[string][]v1.Pod
		podErr         error

		expectedState         prowapi.ProwJobState
		expectedPodHasName    bool
		expectedNumPods       map[string]int
		expectedComplete      bool
		expectedCreatedPJs    int
		expectedReport        bool
		expectPrevReportState map[string]prowapi.ProwJobState
		expectedURL           string
		expectedBuildID       string
		expectError           bool
	}{
		{
			name: "start new pod",
			pj: prowapi.ProwJob{
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
			pods:               map[string][]v1.Pod{"default": {}},
			expectedState:      prowapi.PendingState,
			expectedPodHasName: true,
			expectedNumPods:    map[string]int{"default": 1},
			expectedReport:     true,
			expectPrevReportState: map[string]prowapi.ProwJobState{
				reporter.GitHubReporterName: prowapi.PendingState,
			},
			expectedURL: "blabla/pending",
		},
		{
			name: "pod with a max concurrency of 1",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "blabla",
					Namespace: "prowjobs",
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
			pendingJobs: map[string]int{
				"same": 1,
			},
			pods: map[string][]v1.Pod{
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
			expectedState:   prowapi.TriggeredState,
			expectedNumPods: map[string]int{"default": 1},
		},
		{
			name: "trusted pod with a max concurrency of 1",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "blabla",
					Namespace: "prowjobs",
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
			pendingJobs: map[string]int{
				"same": 1,
			},
			pods: map[string][]v1.Pod{
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
			expectedState:   prowapi.TriggeredState,
			expectedNumPods: map[string]int{"trusted": 1},
		},
		{
			name: "trusted pod with a max concurrency of 1 (can start)",
			pj: prowapi.ProwJob{
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
			pods: map[string][]v1.Pod{
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
			expectedState:      prowapi.PendingState,
			expectedNumPods:    map[string]int{"default": 1, "trusted": 1},
			expectedPodHasName: true,
			expectedReport:     true,
			expectPrevReportState: map[string]prowapi.ProwJobState{
				reporter.GitHubReporterName: prowapi.PendingState,
			},
			expectedURL: "some/pending",
		},
		{
			name: "do not exceed global maxconcurrency",
			pj: prowapi.ProwJob{
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
			maxConcurrency: 20,
			pendingJobs:    map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			expectedState:  prowapi.TriggeredState,
		},
		{
			name: "global maxconcurrency allows new jobs when possible",
			pj: prowapi.ProwJob{
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
			pods:            map[string][]v1.Pod{"default": {}},
			maxConcurrency:  21,
			pendingJobs:     map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			expectedState:   prowapi.PendingState,
			expectedNumPods: map[string]int{"default": 1},
			expectedReport:  true,
			expectPrevReportState: map[string]prowapi.ProwJobState{
				reporter.GitHubReporterName: prowapi.PendingState,
			},
			expectedURL: "beer/pending",
		},
		{
			name: "unprocessable prow job",
			pj: prowapi.ProwJob{
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
			pods: map[string][]v1.Pod{"default": {}},
			podErr: &kapierrors.StatusError{ErrStatus: metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusUnprocessableEntity,
				Reason: metav1.StatusReasonInvalid,
			}},
			expectedState:    prowapi.ErrorState,
			expectedComplete: true,
			expectedReport:   true,
			expectPrevReportState: map[string]prowapi.ProwJobState{
				reporter.GitHubReporterName: prowapi.ErrorState,
			},
		},
		{
			name: "conflict error starting pod",
			pj: prowapi.ProwJob{
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
			podErr: &kapierrors.StatusError{ErrStatus: metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusConflict,
				Reason: metav1.StatusReasonAlreadyExists,
			}},
			expectedState: prowapi.TriggeredState,
			expectError:   true,
		},
		{
			name: "unknown error starting pod",
			pj: prowapi.ProwJob{
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
			podErr:        errors.New("no way unknown jose"),
			expectedState: prowapi.TriggeredState,
			expectError:   true,
		},
		{
			name: "running pod, failed prowjob update",
			pj: prowapi.ProwJob{
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
			pods: map[string][]v1.Pod{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "pods",
						},
						Spec: v1.PodSpec{
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
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			expectedState:   prowapi.PendingState,
			expectedNumPods: map[string]int{"default": 1},
			expectedReport:  true,
			expectPrevReportState: map[string]prowapi.ProwJobState{
				reporter.GitHubReporterName: prowapi.PendingState,
			},
			expectedURL:     "foo/pending",
			expectedBuildID: "0987654321",
		},
	}
	for _, tc := range testcases {
		totServ := httptest.NewServer(http.HandlerFunc(handleTot))
		defer totServ.Close()
		pm := make(map[string]v1.Pod)
		for _, pods := range tc.pods {
			for i := range pods {
				pm[pods[i].ObjectMeta.Name] = pods[i]
			}
		}
		fakeProwJobClient := prowfake.NewSimpleClientset(&tc.pj)
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
			prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			buildClients:  buildClients,
			log:           logrus.NewEntry(logrus.StandardLogger()),
			config:        newFakeConfigAgent(t, tc.maxConcurrency).Config,
			totURL:        totServ.URL,
			pendingJobs:   make(map[string]int),
		}
		if tc.pendingJobs != nil {
			c.pendingJobs = tc.pendingJobs
		}

		reports := make(chan prowapi.ProwJob, 100)
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

		actualProwJobs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(metav1.ListOptions{})
		if err != nil {
			t.Errorf("for case %q could not list prowJobs from the client: %v", tc.name, err)
		}
		if len(actualProwJobs.Items) != tc.expectedCreatedPJs+1 {
			t.Errorf("for case %q got %d created prowjobs", tc.name, len(actualProwJobs.Items)-1)
		}
		actual := actualProwJobs.Items[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %q got state %v", tc.name, actual.Status.State)
		}
		if (actual.Status.PodName == "") && tc.expectedPodHasName {
			t.Errorf("for case %q got no pod name, expected one", tc.name)
		}
		for alias, expected := range tc.expectedNumPods {
			actualPods, err := buildClients[alias].List(metav1.ListOptions{})
			if err != nil {
				t.Errorf("for case %q could not list pods from the client: %v", tc.name, err)
			}
			if got := len(actualPods.Items); got != expected {
				t.Errorf("for case %q got %d pods for alias %q, but expected %d", tc.name, got, alias, expected)
			}
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %q got wrong completion", tc.name)
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

		pj   prowapi.ProwJob
		pods []v1.Pod
		err  error

		expectedState      prowapi.ProwJobState
		expectedNumPods    int
		expectedComplete   bool
		expectedCreatedPJs int
		expectedReport     bool
		expectedURL        string
	}{
		{
			name: "reset when pod goes missing",
			pj: prowapi.ProwJob{
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
			expectedState:   prowapi.PendingState,
			expectedReport:  true,
			expectedNumPods: 1,
			expectedURL:     "boop-41/pending",
		},
		{
			name: "delete pod in unknown state",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
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
			expectedState:   prowapi.PendingState,
			expectedNumPods: 0,
		},
		{
			name: "succeeded pod",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
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
			expectedComplete:   true,
			expectedState:      prowapi.SuccessState,
			expectedNumPods:    1,
			expectedCreatedPJs: 0,
			expectedReport:     true,
			expectedURL:        "boop-42/success",
		},
		{
			name: "failed pod",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
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
			expectedComplete: true,
			expectedState:    prowapi.FailureState,
			expectedNumPods:  1,
			expectedReport:   true,
			expectedURL:      "boop-42/failure",
		},
		{
			name: "delete evicted pod",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
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
			expectedComplete: false,
			expectedState:    prowapi.PendingState,
			expectedNumPods:  0,
		},
		{
			name: "don't delete evicted pod w/ error_on_eviction, complete PJ instead",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
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
			expectedComplete: true,
			expectedState:    prowapi.ErrorState,
			expectedNumPods:  1,
			expectedReport:   true,
			expectedURL:      "boop-42/error",
		},
		{
			name: "running pod",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
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
			expectedState:   prowapi.PendingState,
			expectedNumPods: 1,
		},
		{
			name: "pod changes url status",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
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
			expectedComplete:   true,
			expectedState:      prowapi.SuccessState,
			expectedNumPods:    1,
			expectedCreatedPJs: 0,
			expectedReport:     true,
			expectedURL:        "boop-42/success",
		},
		{
			name: "unprocessable prow job",
			pj: prowapi.ProwJob{
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
			err: &kapierrors.StatusError{ErrStatus: metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusUnprocessableEntity,
				Reason: metav1.StatusReasonInvalid,
			}},
			expectedState:    prowapi.ErrorState,
			expectedComplete: true,
			expectedReport:   true,
			expectedURL:      "jose/error",
		},
		{
			name: "stale pending prow job",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nightmare",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase:     v1.PodPending,
						StartTime: startTime(time.Now().Add(-podPendingTimeout)),
					},
				},
			},
			expectedState:    prowapi.ErrorState,
			expectedNumPods:  0,
			expectedComplete: true,
			expectedReport:   true,
			expectedURL:      "nightmare/error",
		},
		{
			name: "stale running prow job",
			pj: prowapi.ProwJob{
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
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "endless",
						Namespace: "pods",
					},
					Status: v1.PodStatus{
						Phase:     v1.PodRunning,
						StartTime: startTime(time.Now().Add(-podRunningTimeout)),
					},
				},
			},
			expectedState:    prowapi.AbortedState,
			expectedNumPods:  0,
			expectedComplete: true,
			expectedReport:   true,
			expectedURL:      "endless/aborted",
		},
	}
	for _, tc := range testcases {
		t.Logf("Running test case %q", tc.name)
		totServ := httptest.NewServer(http.HandlerFunc(handleTot))
		defer totServ.Close()
		pm := make(map[string]v1.Pod)
		for i := range tc.pods {
			pm[tc.pods[i].ObjectMeta.Name] = tc.pods[i]
		}
		fakeProwJobClient := prowfake.NewSimpleClientset(&tc.pj)
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
			prowapi.DefaultClusterAlias: fakeClient.CoreV1().Pods("pods"),
		}
		c := Controller{
			prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			buildClients:  buildClients,
			log:           logrus.NewEntry(logrus.StandardLogger()),
			config:        newFakeConfigAgent(t, 0).Config,
			totURL:        totServ.URL,
			pendingJobs:   make(map[string]int),
		}

		reports := make(chan prowapi.ProwJob, 100)
		if err := c.syncPendingJob(tc.pj, pm, reports); err != nil {
			t.Errorf("for case %q got an error: %v", tc.name, err)
			continue
		}
		close(reports)

		actualProwJobs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(metav1.ListOptions{})
		if err != nil {
			t.Errorf("for case %q could not list prowJobs from the client: %v", tc.name, err)
		}
		if len(actualProwJobs.Items) != tc.expectedCreatedPJs+1 {
			t.Errorf("for case %q got %d created prowjobs", tc.name, len(actualProwJobs.Items)-1)
		}
		actual := actualProwJobs.Items[0]
		if actual.Status.State != tc.expectedState {
			t.Errorf("for case %q got state %v", tc.name, actual.Status.State)
		}
		actualPods, err := buildClients[prowapi.DefaultClusterAlias].List(metav1.ListOptions{})
		if err != nil {
			t.Errorf("for case %q could not list pods from the client: %v", tc.name, err)
		}
		if got := len(actualPods.Items); got != tc.expectedNumPods {
			t.Errorf("for case %q got %d pods, expected %d", tc.name, len(actualPods.Items), tc.expectedNumPods)
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("for case %q got wrong completion", tc.name)
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

func TestOrderedJobs(t *testing.T) {
	totServ := httptest.NewServer(http.HandlerFunc(handleTot))
	defer totServ.Close()
	var pjs []prowapi.ProwJob

	// Add 3 jobs with incrementing timestamp
	for i := 0; i < 3; i++ {
		job := pjutil.NewProwJob(pjutil.PeriodicSpec(config.Periodic{
			JobBase: config.JobBase{
				Name:    fmt.Sprintf("ci-periodic-job-%d", i),
				Agent:   "kubernetes",
				Cluster: "trusted",
				Spec:    &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
			},
		}), nil, nil)
		job.ObjectMeta.CreationTimestamp = metav1.Time{
			Time: time.Now(),
		}
		job.Namespace = "prowjobs"
		pjs = append(pjs, job)
	}
	expOut := []string{"ci-periodic-job-0", "ci-periodic-job-1", "ci-periodic-job-2"}

	for _, orders := range [][]int{
		{0, 1, 2},
		{1, 2, 0},
		{2, 0, 1},
	} {
		newPjs := make([]runtime.Object, 3)
		for i := 0; i < len(pjs); i++ {
			newPjs[i] = &pjs[orders[i]]
		}
		fakeProwJobClient := prowfake.NewSimpleClientset(newPjs...)
		buildClients := map[string]corev1.PodInterface{
			"trusted": fake.NewSimpleClientset().CoreV1().Pods("pods"),
		}
		log := logrus.NewEntry(logrus.StandardLogger())
		c := Controller{
			prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			ghc:           &fghc{},
			buildClients:  buildClients,
			log:           log,
			config:        newFakeConfigAgent(t, 0).Config,
			totURL:        totServ.URL,
			pendingJobs:   make(map[string]int),
			lock:          sync.RWMutex{},
		}
		if err := c.Sync(); err != nil {
			t.Fatalf("Error on first sync: %v", err)
		}
		for i, name := range expOut {
			if c.pjs[i].Spec.Job != name {
				t.Fatalf("Error in keeping order, want: '%s', got '%s'", name, c.pjs[i].Spec.Job)
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
			Spec:    &v1.PodSpec{Containers: []v1.Container{{Name: "test-name", Env: []v1.EnvVar{}}}},
		},
	}

	totServ := httptest.NewServer(http.HandlerFunc(handleTot))
	defer totServ.Close()
	pj := pjutil.NewProwJob(pjutil.PeriodicSpec(per), nil, nil)
	pj.Namespace = "prowjobs"
	fakeProwJobClient := prowfake.NewSimpleClientset(&pj)
	buildClients := map[string]corev1.PodInterface{
		prowapi.DefaultClusterAlias: fake.NewSimpleClientset().CoreV1().Pods("pods"),
		"trusted":                   fake.NewSimpleClientset().CoreV1().Pods("pods"),
	}
	log := logrus.NewEntry(logrus.StandardLogger())
	c := Controller{
		prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
		ghc:           &fghc{},
		buildClients:  buildClients,
		log:           log,
		config:        newFakeConfigAgent(t, 0).Config,
		totURL:        totServ.URL,
		pendingJobs:   make(map[string]int),
		lock:          sync.RWMutex{},
	}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on first sync: %v", err)
	}
	afterFirstSync, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("could not list prowJobs from the client: %v", err)
	}
	if len(afterFirstSync.Items) != 1 {
		t.Fatalf("saw %d prowjobs after sync, not 1", len(afterFirstSync.Items))
	}
	if len(afterFirstSync.Items[0].Spec.PodSpec.Containers) != 1 || afterFirstSync.Items[0].Spec.PodSpec.Containers[0].Name != "test-name" {
		t.Fatalf("Sync step updated the pod spec: %#v", afterFirstSync.Items[0].Spec.PodSpec)
	}
	podsAfterSync, err := buildClients["trusted"].List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("could not list pods from the client: %v", err)
	}
	if len(podsAfterSync.Items) != 1 {
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
	update.Status.Phase = v1.PodSucceeded
	if _, err := buildClients["trusted"].Update(update); err != nil {
		t.Fatalf("could not update pod to be succeeded: %v", err)
	}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on third sync: %v", err)
	}
	afterThirdSync, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(metav1.ListOptions{})
	if err != nil {
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
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on fourth sync: %v", err)
	}
}

func TestMaxConcurrencyWithNewlyTriggeredJobs(t *testing.T) {
	tests := []struct {
		name         string
		pjs          []prowapi.ProwJob
		pendingJobs  map[string]int
		expectedPods int
	}{
		{
			name: "avoid starting a triggered job",
			pjs: []prowapi.ProwJob{
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
						Name: "second",
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
			pendingJobs:  make(map[string]int),
			expectedPods: 1,
		},
		{
			name: "both triggered jobs can start",
			pjs: []prowapi.ProwJob{
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
			pendingJobs:  make(map[string]int),
			expectedPods: 2,
		},
		{
			name: "no triggered job can start",
			pjs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "first",
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
						Name: "second",
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
			pendingJobs:  map[string]int{"test-bazel-build": 5},
			expectedPods: 0,
		},
	}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)
		jobs := make(chan prowapi.ProwJob, len(test.pjs))
		for _, pj := range test.pjs {
			jobs <- pj
		}
		close(jobs)

		var prowJobs []runtime.Object
		for i := range test.pjs {
			prowJobs = append(prowJobs, &test.pjs[i])
		}
		fakeProwJobClient := prowfake.NewSimpleClientset(prowJobs...)
		buildClients := map[string]corev1.PodInterface{
			prowapi.DefaultClusterAlias: fake.NewSimpleClientset().CoreV1().Pods("pods"),
		}
		c := Controller{
			prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			buildClients:  buildClients,
			log:           logrus.NewEntry(logrus.StandardLogger()),
			config:        newFakeConfigAgent(t, 0).Config,
			pendingJobs:   test.pendingJobs,
		}

		reports := make(chan prowapi.ProwJob, len(test.pjs))
		errors := make(chan error, len(test.pjs))
		pm := make(map[string]v1.Pod)

		syncProwJobs(c.log, c.syncTriggeredJob, 20, jobs, reports, errors, pm)
		podsAfterSync, err := buildClients[prowapi.DefaultClusterAlias].List(metav1.ListOptions{})
		if err != nil {
			t.Fatalf("could not list pods from the client: %v", err)
		}
		if len(podsAfterSync.Items) != test.expectedPods {
			t.Errorf("expected pods: %d, got: %d", test.expectedPods, len(podsAfterSync.Items))
		}
	}
}

func TestMaxConcurency(t *testing.T) {
	testCases := []struct {
		name             string
		prowJob          prowapi.ProwJob
		existingProwJobs []prowapi.ProwJob
		pendingJobs      map[string]int
		expectedResult   bool
	}{
		{
			name:           "Max concurency 0 always runs",
			prowJob:        prowapi.ProwJob{Spec: prowapi.ProwJobSpec{MaxConcurrency: 0}},
			expectedResult: true,
		},
		{
			name: "Num pending exceeds max concurrency",
			prowJob: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 10,
					Job:            "my-pj"}},
			pendingJobs:    map[string]int{"my-pj": 10},
			expectedResult: false,
		},
		{
			name: "Num pending plus older instances equals max concurency",
			prowJob: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 10,
					Job:            "my-pj"},
			},
			existingProwJobs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					}},
			},
			pendingJobs:    map[string]int{"my-pj": 9},
			expectedResult: false,
		},
		{
			name: "Num pending plus older instances exceeds max concurency",
			prowJob: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 10,
					Job:            "my-pj"},
			},
			existingProwJobs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					}},
			},
			pendingJobs:    map[string]int{"my-pj": 10},
			expectedResult: false,
		},
		{
			name: "Have other jobs that are newer, can execute",
			prowJob: prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 1,
					Job:            "my-pj"},
			},
			existingProwJobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.Now(),
					},
					Spec: prowapi.ProwJobSpec{Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					}},
			},
			expectedResult: true,
		},
		{
			name: "Have older jobs that are not triggered, can execute",
			prowJob: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Now(),
				},
				Spec: prowapi.ProwJobSpec{
					MaxConcurrency: 2,
					Job:            "my-pj"},
			},
			existingProwJobs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{Job: "my-pj"},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					}},
			},
			pendingJobs:    map[string]int{"my-pj": 1},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if tc.pendingJobs == nil {
				tc.pendingJobs = map[string]int{}
			}
			var prowJobs []runtime.Object
			for i := range tc.existingProwJobs {
				prowJobs = append(prowJobs, &tc.existingProwJobs[i])
			}
			buildClients := map[string]corev1.PodInterface{}
			c := Controller{
				pjs:          tc.existingProwJobs,
				buildClients: buildClients,
				log:          logrus.NewEntry(logrus.StandardLogger()),
				config:       newFakeConfigAgent(t, 0).Config,
				pendingJobs:  tc.pendingJobs,
			}
			logrus.SetLevel(logrus.DebugLevel)

			result := c.canExecuteConcurrently(&tc.prowJob)

			if result != tc.expectedResult {
				t.Errorf("Expected result to be %t but was %t", tc.expectedResult, result)
			}
		})
	}

}

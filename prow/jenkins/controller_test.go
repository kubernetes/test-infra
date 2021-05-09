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
	"context"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

func newFakeConfigAgent(t *testing.T, maxConcurrency int, operators []config.JenkinsOperator) *fca {
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

	ca := &fca{
		c: &config.Config{
			ProwConfig: config.ProwConfig{
				ProwJobNamespace: "prowjobs",
				JenkinsOperators: []config.JenkinsOperator{
					{
						Controller: config.Controller{
							JobURLTemplate: template.Must(template.New("test").Parse("{{.Status.PodName}}/{{.Status.State}}")),
							MaxConcurrency: maxConcurrency,
							MaxGoroutines:  20,
						},
					},
				},
				StatusErrorLink: "https://github.com/kubernetes/test-infra/issues",
			},
			JobConfig: config.JobConfig{
				PresubmitsStatic: presubmitMap,
			},
		},
	}
	if len(operators) > 0 {
		ca.c.JenkinsOperators = operators
	}
	return ca
}

func (f *fca) Config() *config.Config {
	f.Lock()
	defer f.Unlock()
	return f.c
}

type fjc struct {
	sync.Mutex
	built       bool
	pjs         []prowapi.ProwJob
	err         error
	builds      map[string]Build
	didAbort    bool
	abortErrors bool
}

func (f *fjc) Build(pj *prowapi.ProwJob, buildID string) error {
	f.Lock()
	defer f.Unlock()
	if f.err != nil {
		return f.err
	}
	f.built = true
	f.pjs = append(f.pjs, *pj)
	return nil
}

func (f *fjc) ListBuilds(jobs []BuildQueryParams) (map[string]Build, error) {
	f.Lock()
	defer f.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.builds, nil
}

func (f *fjc) Abort(job string, build *Build) error {
	f.Lock()
	defer f.Unlock()
	if f.abortErrors {
		return errors.New("erroring on abort as requested")
	}
	f.didAbort = true
	return nil
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

func (f *fghc) BotUserChecker() (func(string) bool, error) {
	return func(candidate string) bool {
		return candidate == "bot"
	}, nil
}
func (f *fghc) CreateStatus(org, repo, ref string, s github.Status) error {
	f.Lock()
	defer f.Unlock()
	return nil
}
func (f *fghc) ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) {
	f.Lock()
	defer f.Unlock()
	return nil, nil
}
func (f *fghc) CreateComment(org, repo string, number int, comment string) error {
	f.Lock()
	defer f.Unlock()
	return nil
}
func (f *fghc) DeleteComment(org, repo string, ID int) error {
	f.Lock()
	defer f.Unlock()
	return nil
}
func (f *fghc) EditComment(org, repo string, ID int, comment string) error {
	f.Lock()
	defer f.Unlock()
	return nil
}

func TestSyncTriggeredJobs(t *testing.T) {
	fakeClock := clock.NewFakeClock(time.Now().Truncate(1 * time.Second))
	pendingTime := metav1.NewTime(fakeClock.Now())

	var testcases = []struct {
		name           string
		pj             prowapi.ProwJob
		pendingJobs    map[string]int
		maxConcurrency int
		builds         map[string]Build
		err            error

		expectedState       prowapi.ProwJobState
		expectedBuild       bool
		expectedComplete    bool
		expectedReport      bool
		expectedEnqueued    bool
		expectedError       bool
		expectedPendingTime *metav1.Time
	}{
		{
			name: "start new job",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PostsubmitJob,
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			expectedBuild:       true,
			expectedReport:      true,
			expectedState:       prowapi.PendingState,
			expectedEnqueued:    true,
			expectedPendingTime: &pendingTime,
		},
		{
			name: "start new job, error",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PresubmitJob,
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{{
							Number: 1,
							SHA:    "fake-sha",
						}},
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			err:              errors.New("oh no"),
			expectedReport:   true,
			expectedState:    prowapi.ErrorState,
			expectedComplete: true,
			expectedError:    true,
		},
		{
			name: "block running new job",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PostsubmitJob,
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			pendingJobs:      map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			maxConcurrency:   20,
			expectedBuild:    false,
			expectedReport:   false,
			expectedState:    prowapi.TriggeredState,
			expectedEnqueued: false,
		},
		{
			name: "allow running new job",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PostsubmitJob,
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.TriggeredState,
				},
			},
			pendingJobs:         map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			maxConcurrency:      21,
			expectedBuild:       true,
			expectedReport:      true,
			expectedState:       prowapi.PendingState,
			expectedEnqueued:    true,
			expectedPendingTime: &pendingTime,
		},
	}
	for _, tc := range testcases {
		totServ := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "42")
		}))
		defer totServ.Close()
		t.Logf("scenario %q", tc.name)
		fjc := &fjc{
			err: tc.err,
		}
		fakeProwJobClient := fake.NewSimpleClientset(&tc.pj)

		c := Controller{
			prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			jc:            fjc,
			log:           logrus.NewEntry(logrus.StandardLogger()),
			cfg:           newFakeConfigAgent(t, tc.maxConcurrency, nil).Config,
			totURL:        totServ.URL,
			lock:          sync.RWMutex{},
			pendingJobs:   make(map[string]int),
			clock:         fakeClock,
		}
		if tc.pendingJobs != nil {
			c.pendingJobs = tc.pendingJobs
		}

		reports := make(chan prowapi.ProwJob, 100)
		if err := c.syncTriggeredJob(tc.pj, reports, tc.builds); err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		close(reports)

		actualProwJobs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list prowjobs from client %v", err)
		}
		if len(actualProwJobs.Items) != 1 {
			t.Fatalf("Didn't create just one ProwJob, but %d", len(actualProwJobs.Items))
		}
		actual := actualProwJobs.Items[0]
		if tc.expectedError && actual.Status.Description != "Error starting Jenkins job." {
			t.Errorf("expected description %q, got %q", "Error starting Jenkins job.", actual.Status.Description)
			continue
		}
		if actual.Status.State != tc.expectedState {
			t.Errorf("expected state %q, got %q", tc.expectedState, actual.Status.State)
			continue
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("expected complete prowjob, got %v", actual)
			continue
		}
		if tc.expectedReport && len(reports) != 1 {
			t.Errorf("wanted one report but got %d", len(reports))
			continue
		}
		if !tc.expectedReport && len(reports) != 0 {
			t.Errorf("did not wany any reports but got %d", len(reports))
			continue
		}
		if fjc.built != tc.expectedBuild {
			t.Errorf("expected build: %t, got: %t", tc.expectedBuild, fjc.built)
			continue
		}
		if tc.expectedEnqueued && actual.Status.Description != "Jenkins job enqueued." {
			t.Errorf("expected enqueued prowjob, got %v", actual)
		}
		if !reflect.DeepEqual(actual.Status.PendingTime, tc.expectedPendingTime) {
			t.Errorf("for case %q got pending time %v, expected %v", tc.name, actual.Status.PendingTime, tc.expectedPendingTime)
		}
	}
}

func TestSyncPendingJobs(t *testing.T) {
	var testcases = []struct {
		name        string
		pj          prowapi.ProwJob
		pendingJobs map[string]int
		builds      map[string]Build
		err         error

		// TODO: Change to pass a ProwJobStatus
		expectedState    prowapi.ProwJobState
		expectedBuild    bool
		expectedURL      string
		expectedComplete bool
		expectedReport   bool
		expectedEnqueued bool
		expectedError    bool
	}{
		{
			name: "enqueued",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foofoo",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job: "test-job",
				},
				Status: prowapi.ProwJobStatus{
					State:       prowapi.PendingState,
					Description: "Jenkins job enqueued.",
				},
			},
			builds: map[string]Build{
				"foofoo": {enqueued: true, Number: 10},
			},
			expectedState:    prowapi.PendingState,
			expectedEnqueued: true,
		},
		{
			name: "finished queue",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boing",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job: "test-job",
				},
				Status: prowapi.ProwJobStatus{
					State:       prowapi.PendingState,
					Description: "Jenkins job enqueued.",
				},
			},
			builds: map[string]Build{
				"boing": {enqueued: false, Number: 10},
			},
			expectedURL:      "boing/pending",
			expectedState:    prowapi.PendingState,
			expectedEnqueued: false,
			expectedReport:   true,
		},
		{
			name: "building",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "firstoutthetrenches",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job: "test-job",
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.PendingState,
				},
			},
			builds: map[string]Build{
				"firstoutthetrenches": {enqueued: false, Number: 10},
			},
			expectedURL:    "firstoutthetrenches/pending",
			expectedState:  prowapi.PendingState,
			expectedReport: true,
		},
		{
			name: "missing build",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "blabla",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PresubmitJob,
					Job:  "test-job",
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{{
							Number: 1,
							SHA:    "fake-sha",
						}},
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.PendingState,
				},
			},
			// missing build
			builds: map[string]Build{
				"other": {enqueued: false, Number: 10},
			},
			expectedURL:      "https://github.com/kubernetes/test-infra/issues",
			expectedState:    prowapi.ErrorState,
			expectedError:    true,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "finished, success",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "winwin",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job: "test-job",
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.PendingState,
				},
			},
			builds: map[string]Build{
				"winwin": {Result: pState(success), Number: 11},
			},
			expectedURL:      "winwin/success",
			expectedState:    prowapi.SuccessState,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "finished, failed",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "whatapity",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job: "test-job",
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.PendingState,
				},
			},
			builds: map[string]Build{
				"whatapity": {Result: pState(failure), Number: 12},
			},
			expectedURL:      "whatapity/failure",
			expectedState:    prowapi.FailureState,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "aborted outside of prow",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "needtoretry",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job: "test-job",
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.PendingState,
				},
			},
			builds: map[string]Build{
				"needtoretry": {Result: pState(aborted), Number: 13},
			},
			expectedState:    prowapi.PendingState,
			expectedEnqueued: true,
			expectedBuild:    true,
			expectedReport:   true,
		},
		{
			name: "aborted from prow",
			pj: prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dontretry",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job: "test-job",
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.AbortedState,
				},
			},
			builds: map[string]Build{
				"dontretry": {Result: pState(aborted), Number: 14},
			},
			expectedURL:      "dontretry/aborted",
			expectedState:    prowapi.AbortedState,
			expectedComplete: true,
			expectedReport:   true,
		},
	}
	for _, tc := range testcases {
		t.Logf("scenario %q", tc.name)
		totServ := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "42")
		}))
		defer totServ.Close()
		fjc := &fjc{
			err: tc.err,
		}
		fakeProwJobClient := fake.NewSimpleClientset(&tc.pj)

		c := Controller{
			prowJobClient:    fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			jc:               fjc,
			log:              logrus.NewEntry(logrus.StandardLogger()),
			cfg:              newFakeConfigAgent(t, 0, nil).Config,
			totURL:           totServ.URL,
			lock:             sync.RWMutex{},
			pendingJobs:      make(map[string]int),
			retryAbortedJobs: true,
			clock:            clock.RealClock{},
		}

		reports := make(chan prowapi.ProwJob, 100)
		if err := c.syncPendingJob(tc.pj, reports, tc.builds); err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		close(reports)

		actualProwJobs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatalf("failed to list prowjobs from client %v", err)
		}
		if len(actualProwJobs.Items) != 1 {
			t.Fatalf("Didn't create just one ProwJob, but %d", len(actualProwJobs.Items))
		}
		actual := actualProwJobs.Items[0]
		if tc.expectedError && actual.Status.Description != "Error finding Jenkins job." {
			t.Errorf("expected description %q, got %q", "Error finding Jenkins job.", actual.Status.Description)
			continue
		}
		if actual.Status.State != tc.expectedState {
			t.Errorf("expected state %q, got %q", tc.expectedState, actual.Status.State)
			continue
		}
		if actual.Complete() != tc.expectedComplete {
			t.Errorf("expected complete prowjob, got %v", actual)
			continue
		}
		if tc.expectedReport && len(reports) != 1 {
			t.Errorf("wanted one report but got %d", len(reports))
			continue
		}
		if !tc.expectedReport && len(reports) != 0 {
			t.Errorf("did not wany any reports but got %d", len(reports))
			continue
		}
		if fjc.built != tc.expectedBuild {
			t.Errorf("expected build: %t, got: %t", tc.expectedBuild, fjc.built)
			continue
		}
		if tc.expectedEnqueued && actual.Status.Description != "Jenkins job enqueued." {
			t.Errorf("expected enqueued prowjob, got %v", actual)
		}
		if tc.expectedURL != actual.Status.URL {
			t.Errorf("expected status URL: %s, got: %s", tc.expectedURL, actual.Status.URL)
		}
	}
}

func pState(state string) *string {
	s := state
	return &s
}

// TestBatch walks through the happy path of a batch job on Jenkins.
func TestBatch(t *testing.T) {
	pre := config.Presubmit{
		JobBase: config.JobBase{
			Name:  "pr-some-job",
			Agent: "jenkins",
		},
		Reporter: config.Reporter{
			Context: "Some Job Context",
		},
	}
	pj := pjutil.NewProwJob(pjutil.BatchSpec(pre, prowapi.Refs{
		Org:     "o",
		Repo:    "r",
		BaseRef: "master",
		BaseSHA: "123",
		Pulls: []prowapi.Pull{
			{
				Number: 1,
				SHA:    "abc",
			},
			{
				Number: 2,
				SHA:    "qwe",
			},
		},
	}), nil, nil)
	pj.ObjectMeta.Name = "known_name"
	pj.ObjectMeta.Namespace = "prowjobs"
	fakeProwJobClient := fake.NewSimpleClientset(&pj)
	jc := &fjc{
		builds: map[string]Build{
			"known_name": { /* Running */ },
		},
	}
	totServ := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "42")
	}))
	defer totServ.Close()
	c := Controller{
		prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
		ghc:           &fghc{},
		jc:            jc,
		log:           logrus.NewEntry(logrus.StandardLogger()),
		cfg:           newFakeConfigAgent(t, 0, nil).Config,
		totURL:        totServ.URL,
		pendingJobs:   make(map[string]int),
		lock:          sync.RWMutex{},
		clock:         clock.RealClock{},
	}

	if err := c.Sync(); err != nil {
		t.Fatalf("Error on first sync: %v", err)
	}
	afterFirstSync, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").Get(context.Background(), "known_name", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get prowjob from client: %v", err)
	}
	if afterFirstSync.Status.State != prowapi.PendingState {
		t.Fatalf("Wrong state: %v", afterFirstSync.Status.State)
	}
	if afterFirstSync.Status.Description != "Jenkins job enqueued." {
		t.Fatalf("Expected description %q, got %q.", "Jenkins job enqueued.", afterFirstSync.Status.Description)
	}
	jc.builds["known_name"] = Build{Number: 42}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on second sync: %v", err)
	}
	afterSecondSync, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").Get(context.Background(), "known_name", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get prowjob from client: %v", err)
	}
	if afterSecondSync.Status.Description != "Jenkins job running." {
		t.Fatalf("Expected description %q, got %q.", "Jenkins job running.", afterSecondSync.Status.Description)
	}
	if afterSecondSync.Status.PodName != "known_name" {
		t.Fatalf("Wrong PodName: %s", afterSecondSync.Status.PodName)
	}
	jc.builds["known_name"] = Build{Result: pState(success)}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on third sync: %v", err)
	}
	afterThirdSync, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").Get(context.Background(), "known_name", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get prowjob from client: %v", err)
	}
	if afterThirdSync.Status.Description != "Jenkins job succeeded." {
		t.Fatalf("Expected description %q, got %q.", "Jenkins job succeeded.", afterThirdSync.Status.Description)
	}
	if afterThirdSync.Status.State != prowapi.SuccessState {
		t.Fatalf("Wrong state: %v", afterThirdSync.Status.State)
	}
	// This is what the SQ reads.
	if afterThirdSync.Spec.Context != "Some Job Context" {
		t.Fatalf("Wrong context: %v", afterThirdSync.Spec.Context)
	}
}

func TestMaxConcurrencyWithNewlyTriggeredJobs(t *testing.T) {
	tests := []struct {
		name           string
		pjs            []prowapi.ProwJob
		pendingJobs    map[string]int
		expectedBuilds int
	}{
		{
			name: "avoid starting a triggered job",
			pjs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "first",
						Namespace: "prowjobs",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 1,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "second",
						Namespace: "prowjobs",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 1,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
			},
			pendingJobs:    make(map[string]int),
			expectedBuilds: 1,
		},
		{
			name: "both triggered jobs can start",
			pjs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "first",
						Namespace: "prowjobs",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 2,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "second",
						Namespace: "prowjobs",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 2,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
			},
			pendingJobs:    make(map[string]int),
			expectedBuilds: 2,
		},
		{
			name: "no triggered job can start",
			pjs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "first",
						Namespace: "prowjobs",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 5,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "second",
						Namespace: "prowjobs",
					},
					Spec: prowapi.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           prowapi.PostsubmitJob,
						MaxConcurrency: 5,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
			},
			pendingJobs:    map[string]int{"test-bazel-build": 5},
			expectedBuilds: 0,
		},
	}

	for _, test := range tests {
		totServ := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "42")
		}))
		defer totServ.Close()
		jobs := make(chan prowapi.ProwJob, len(test.pjs))
		for _, pj := range test.pjs {
			jobs <- pj
		}
		close(jobs)

		var prowJobs []runtime.Object
		for i := range test.pjs {
			prowJobs = append(prowJobs, &test.pjs[i])
		}
		fakeProwJobClient := fake.NewSimpleClientset(prowJobs...)
		fjc := &fjc{}
		c := Controller{
			prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			jc:            fjc,
			log:           logrus.NewEntry(logrus.StandardLogger()),
			cfg:           newFakeConfigAgent(t, 0, nil).Config,
			totURL:        totServ.URL,
			pendingJobs:   test.pendingJobs,
			clock:         clock.RealClock{},
		}

		reports := make(chan<- prowapi.ProwJob, len(test.pjs))
		errors := make(chan<- error, len(test.pjs))

		syncProwJobs(c.log, c.syncTriggeredJob, 20, jobs, reports, errors, nil)
		if len(fjc.pjs) != test.expectedBuilds {
			t.Errorf("expected builds: %d, got: %d", test.expectedBuilds, len(fjc.pjs))
		}
	}
}

func TestGetJenkinsJobs(t *testing.T) {
	now := func() *metav1.Time {
		n := metav1.Now()
		return &n
	}
	tests := []struct {
		name     string
		pjs      []prowapi.ProwJob
		expected []string
	}{
		{
			name: "both complete and running",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						Job: "coolio",
					},
					Status: prowapi.ProwJobStatus{
						CompletionTime: now(),
					},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "maradona",
					},
					Status: prowapi.ProwJobStatus{},
				},
			},
			expected: []string{"maradona"},
		},
		{
			name: "only complete",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						Job: "coolio",
					},
					Status: prowapi.ProwJobStatus{
						CompletionTime: now(),
					},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "maradona",
					},
					Status: prowapi.ProwJobStatus{
						CompletionTime: now(),
					},
				},
			},
			expected: nil,
		},
		{
			name: "only running",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						Job: "coolio",
					},
					Status: prowapi.ProwJobStatus{},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "maradona",
					},
					Status: prowapi.ProwJobStatus{},
				},
			},
			expected: []string{"maradona", "coolio"},
		},
		{
			name: "running jenkins jobs",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						Job:   "coolio",
						Agent: "jenkins",
						JenkinsSpec: &prowapi.JenkinsSpec{
							GitHubBranchSourceJob: true,
						},
						Refs: &prowapi.Refs{
							BaseRef: "master",
							Pulls: []prowapi.Pull{{
								Number: 12,
							}},
						},
					},
					Status: prowapi.ProwJobStatus{},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job:   "maradona",
						Agent: "jenkins",
						JenkinsSpec: &prowapi.JenkinsSpec{
							GitHubBranchSourceJob: true,
						},
						Refs: &prowapi.Refs{
							BaseRef: "master",
						},
					},
					Status: prowapi.ProwJobStatus{},
				},
			},
			expected: []string{"maradona/job/master", "coolio/view/change-requests/job/PR-12"},
		},
	}

	for _, test := range tests {
		t.Logf("scenario %q", test.name)
		got := getJenkinsJobs(test.pjs)
		if len(got) != len(test.expected) {
			t.Errorf("unexpected job amount: %d (%v), expected: %d (%v)",
				len(got), got, len(test.expected), test.expected)
		}
		for _, ej := range test.expected {
			var found bool
			for _, gj := range got {
				if ej == gj.JobName {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected jobs: %v\ngot: %v", test.expected, got)
			}
		}
	}
}

func TestOperatorConfig(t *testing.T) {
	tests := []struct {
		name string

		operators     []config.JenkinsOperator
		labelSelector string

		expected config.Controller
	}{
		{
			name: "single operator config",

			operators:     nil, // defaults to a single operator
			labelSelector: "",

			expected: config.Controller{
				JobURLTemplate: template.Must(template.New("test").Parse("{{.Status.PodName}}/{{.Status.State}}")),
				MaxConcurrency: 10,
				MaxGoroutines:  20,
			},
		},
		{
			name: "single operator config, --label-selector used",

			operators:     nil, // defaults to a single operator
			labelSelector: "master=ci.jenkins.org",

			expected: config.Controller{
				JobURLTemplate: template.Must(template.New("test").Parse("{{.Status.PodName}}/{{.Status.State}}")),
				MaxConcurrency: 10,
				MaxGoroutines:  20,
			},
		},
		{
			name: "multiple operator config",

			operators: []config.JenkinsOperator{
				{
					Controller: config.Controller{
						JobURLTemplate: template.Must(template.New("test").Parse("{{.Status.PodName}}/{{.Status.State}}")),
						MaxConcurrency: 5,
						MaxGoroutines:  10,
					},
					LabelSelectorString: "master=ci.openshift.org",
				},
				{
					Controller: config.Controller{
						MaxConcurrency: 100,
						MaxGoroutines:  100,
					},
					LabelSelectorString: "master=ci.jenkins.org",
				},
			},
			labelSelector: "master=ci.jenkins.org",

			expected: config.Controller{
				MaxConcurrency: 100,
				MaxGoroutines:  100,
			},
		},
	}

	for _, test := range tests {
		t.Logf("scenario %q", test.name)

		c := Controller{
			cfg:      newFakeConfigAgent(t, 10, test.operators).Config,
			selector: test.labelSelector,
			clock:    clock.RealClock{},
		}

		got := c.config()
		if !reflect.DeepEqual(got, test.expected) {
			t.Errorf("expected controller:\n%#v\ngot controller:\n%#v\n", test.expected, got)
		}
	}
}

func TestSyncAbortedJob(t *testing.T) {
	testCases := []struct {
		name           string
		hasBuild       bool
		abortErrors    bool
		expectAbort    bool
		expectComplete bool
	}{
		{
			name:           "Build is aborted",
			hasBuild:       true,
			expectAbort:    true,
			expectComplete: true,
		},
		{
			name:           "No build, no abort",
			hasBuild:       false,
			expectAbort:    false,
			expectComplete: true,
		},
		{
			name:           "Abort errors, job is not marked completed",
			hasBuild:       true,
			abortErrors:    true,
			expectComplete: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			pj := &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pj",
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.AbortedState,
				},
			}

			var buildMap map[string]Build
			if tc.hasBuild {
				buildMap = map[string]Build{pj.Name: {}}
			}
			pjClient := fake.NewSimpleClientset(pj)
			jobClient := &fjc{abortErrors: tc.abortErrors}
			c := &Controller{
				log:           logrus.NewEntry(logrus.New()),
				prowJobClient: pjClient.ProwV1().ProwJobs(""),
				jc:            jobClient,
			}

			if err := c.syncAbortedJob(*pj, nil, buildMap); (err != nil) != tc.abortErrors {
				t.Fatalf("syncAbortedJob failed: %v", err)
			}

			pj, err := pjClient.ProwV1().ProwJobs("").Get(context.Background(), pj.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("failed to get prowjob: %v", err)
			}

			if pj.Complete() != tc.expectComplete {
				t.Errorf("expected completed job: %t, got completed job: %t", tc.expectComplete, pj.Complete())
			}

			if jobClient.didAbort != tc.expectAbort {
				t.Errorf("expected abort: %t, did abort: %t", tc.expectAbort, jobClient.didAbort)
			}
		})
	}
}

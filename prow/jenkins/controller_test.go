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
	"sync"
	"testing"
	"text/template"
	"time"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

func newFakeConfigAgent(t *testing.T, maxConcurrency int) *fca {
	presubmits := []config.Presubmit{
		{
			Name: "test-bazel-build",
			RunAfterSuccess: []config.Presubmit{
				{
					Name:         "test-kubeadm-cloud",
					RunIfChanged: "^(cmd/kubeadm|build/debs).*$",
				},
			},
		},
		{
			Name: "test-e2e",
			RunAfterSuccess: []config.Presubmit{
				{
					Name: "push-image",
				},
			},
		},
		{
			Name: "test-bazel-test",
		},
	}
	if err := config.SetRegexes(presubmits); err != nil {
		t.Fatal(err)
	}
	presubmitMap := map[string][]config.Presubmit{
		"kubernetes/kubernetes": presubmits,
	}

	return &fca{
		c: &config.Config{
			JenkinsOperator: config.JenkinsOperator{
				Controller: config.Controller{
					JobURLTemplate: template.Must(template.New("test").Parse("{{.Status.PodName}}/{{.Status.State}}")),
					MaxConcurrency: maxConcurrency,
				},
			},
			Presubmits: presubmitMap,
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
}

func (f *fkc) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	f.Lock()
	defer f.Unlock()
	f.prowjobs = append(f.prowjobs, pj)
	return pj, nil
}

func (f *fkc) ListProwJobs(string) ([]kube.ProwJob, error) {
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
	sync.Mutex
	built  bool
	pjs    []kube.ProwJob
	err    error
	builds map[string]JenkinsBuild
}

func (f *fjc) Build(pj *kube.ProwJob) error {
	f.Lock()
	defer f.Unlock()
	if f.err != nil {
		return f.err
	}
	f.built = true
	f.pjs = append(f.pjs, *pj)
	return nil
}

func (f *fjc) ListBuilds(jobs []string) (map[string]JenkinsBuild, error) {
	f.Lock()
	defer f.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.builds, nil
}

func (f *fjc) Abort(job string, build *JenkinsBuild) error {
	f.Lock()
	defer f.Unlock()
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

func (f *fghc) BotName() (string, error) {
	f.Lock()
	defer f.Unlock()
	return "bot", nil
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

func TestSyncNonPendingJobs(t *testing.T) {
	var testcases = []struct {
		name           string
		pj             kube.ProwJob
		pendingJobs    map[string]int
		maxConcurrency int
		builds         map[string]JenkinsBuild
		err            error

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
		{
			name: "block running new job",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type: kube.PostsubmitJob,
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pendingJobs:      map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			maxConcurrency:   20,
			expectedBuild:    false,
			expectedReport:   false,
			expectedState:    kube.TriggeredState,
			expectedEnqueued: false,
		},
		{
			name: "allow running new job",
			pj: kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Type: kube.PostsubmitJob,
				},
				Status: kube.ProwJobStatus{
					State: kube.TriggeredState,
				},
			},
			pendingJobs:      map[string]int{"motherearth": 10, "allagash": 8, "krusovice": 2},
			maxConcurrency:   21,
			expectedBuild:    true,
			expectedReport:   true,
			expectedState:    kube.PendingState,
			expectedEnqueued: true,
		},
	}
	for _, tc := range testcases {
		t.Logf("scenario %q", tc.name)
		fjc := &fjc{
			err: tc.err,
		}
		fkc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}

		c := Controller{
			kc:          fkc,
			jc:          fjc,
			ca:          newFakeConfigAgent(t, tc.maxConcurrency),
			lock:        sync.RWMutex{},
			pendingJobs: make(map[string]int),
		}
		if tc.pendingJobs != nil {
			c.pendingJobs = tc.pendingJobs
		}

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncNonPendingJob(tc.pj, reports, tc.builds); err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		close(reports)

		actual := fkc.prowjobs[0]
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
	}
}

func TestSyncPendingJobs(t *testing.T) {
	var testcases = []struct {
		name        string
		pj          kube.ProwJob
		pendingJobs map[string]int
		builds      map[string]JenkinsBuild
		err         error

		// TODO: Change to pass a ProwJobStatus
		expectedState    kube.ProwJobState
		expectedBuild    bool
		expectedURL      string
		expectedComplete bool
		expectedReport   bool
		expectedEnqueued bool
		expectedError    bool
	}{
		{
			name: "enqueued",
			pj: kube.ProwJob{
				Metadata: kube.ObjectMeta{
					Name: "foofoo",
				},
				Spec: kube.ProwJobSpec{
					Job: "test-job",
				},
				Status: kube.ProwJobStatus{
					State:       kube.PendingState,
					Description: "Jenkins job enqueued.",
				},
			},
			builds: map[string]JenkinsBuild{
				"foofoo": {enqueued: true, Number: 10},
			},
			expectedState:    kube.PendingState,
			expectedEnqueued: true,
		},
		{
			name: "finished queue",
			pj: kube.ProwJob{
				Metadata: kube.ObjectMeta{
					Name: "boing",
				},
				Spec: kube.ProwJobSpec{
					Job: "test-job",
				},
				Status: kube.ProwJobStatus{
					State:       kube.PendingState,
					Description: "Jenkins job enqueued.",
				},
			},
			builds: map[string]JenkinsBuild{
				"boing": {enqueued: false, Number: 10},
			},
			expectedURL:      "test-job-10/pending",
			expectedState:    kube.PendingState,
			expectedEnqueued: false,
			expectedReport:   true,
		},
		{
			name: "building",
			pj: kube.ProwJob{
				Metadata: kube.ObjectMeta{
					Name: "firstoutthetrenches",
				},
				Spec: kube.ProwJobSpec{
					Job: "test-job",
				},
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			builds: map[string]JenkinsBuild{
				"firstoutthetrenches": {enqueued: false, Number: 10},
			},
			expectedURL:    "test-job-10/pending",
			expectedState:  kube.PendingState,
			expectedReport: true,
		},
		{
			name: "missing build",
			pj: kube.ProwJob{
				Metadata: kube.ObjectMeta{
					Name: "blabla",
				},
				Spec: kube.ProwJobSpec{
					Type: kube.PresubmitJob,
					Job:  "test-job",
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
			// missing build
			builds: map[string]JenkinsBuild{
				"other": {enqueued: false, Number: 10},
			},
			expectedURL:      "https://github.com/kubernetes/test-infra/issues",
			expectedState:    kube.ErrorState,
			expectedError:    true,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "finished, success",
			pj: kube.ProwJob{
				Metadata: kube.ObjectMeta{
					Name: "winwin",
				},
				Spec: kube.ProwJobSpec{
					Job: "test-job",
				},
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			builds: map[string]JenkinsBuild{
				"winwin": {Result: pState(Succeess), Number: 11},
			},
			expectedURL:      "test-job-11/success",
			expectedState:    kube.SuccessState,
			expectedComplete: true,
			expectedReport:   true,
		},
		{
			name: "finished, failed",
			pj: kube.ProwJob{
				Metadata: kube.ObjectMeta{
					Name: "whatapity",
				},
				Spec: kube.ProwJobSpec{
					Job: "test-job",
				},
				Status: kube.ProwJobStatus{
					State: kube.PendingState,
				},
			},
			builds: map[string]JenkinsBuild{
				"whatapity": {Result: pState(Failure), Number: 12},
			},
			expectedURL:      "test-job-12/failure",
			expectedState:    kube.FailureState,
			expectedComplete: true,
			expectedReport:   true,
		},
	}
	for _, tc := range testcases {
		t.Logf("scenario %q", tc.name)
		fjc := &fjc{
			err: tc.err,
		}
		fkc := &fkc{
			prowjobs: []kube.ProwJob{tc.pj},
		}

		c := Controller{
			kc:          fkc,
			jc:          fjc,
			ca:          newFakeConfigAgent(t, 0),
			lock:        sync.RWMutex{},
			pendingJobs: make(map[string]int),
		}

		reports := make(chan kube.ProwJob, 100)
		if err := c.syncPendingJob(tc.pj, reports, tc.builds); err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		close(reports)

		actual := fkc.prowjobs[0]
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
		Name:    "pr-some-job",
		Agent:   "jenkins",
		Context: "Some Job Context",
	}
	pj := pjutil.NewProwJob(pjutil.BatchSpec(pre, kube.Refs{
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
	}), nil)
	pj.Metadata.Name = "known_name"
	fc := &fkc{
		prowjobs: []kube.ProwJob{pj},
	}
	jc := &fjc{
		builds: map[string]JenkinsBuild{
			"known_name": { /* Running */ },
		},
	}
	c := Controller{
		kc:          fc,
		jc:          jc,
		ca:          newFakeConfigAgent(t, 0),
		pendingJobs: make(map[string]int),
		lock:        sync.RWMutex{},
	}

	if err := c.Sync(); err != nil {
		t.Fatalf("Error on first sync: %v", err)
	}
	if fc.prowjobs[0].Status.State != kube.PendingState {
		t.Fatalf("Wrong state: %v", fc.prowjobs[0].Status.State)
	}
	if fc.prowjobs[0].Status.Description != "Jenkins job enqueued." {
		t.Fatalf("Expected description %q, got %q.", "Jenkins job enqueued.", fc.prowjobs[0].Status.Description)
	}
	jc.builds["known_name"] = JenkinsBuild{Number: 42}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on second sync: %v", err)
	}
	if fc.prowjobs[0].Status.Description != "Jenkins job running." {
		t.Fatalf("Expected description %q, got %q.", "Jenkins job running.", fc.prowjobs[0].Status.Description)
	}
	if fc.prowjobs[0].Status.PodName != "pr-some-job-42" {
		t.Fatalf("Wrong PodName: %s", fc.prowjobs[0].Status.PodName)
	}
	jc.builds["known_name"] = JenkinsBuild{Result: pState(Succeess)}
	if err := c.Sync(); err != nil {
		t.Fatalf("Error on third sync: %v", err)
	}
	if fc.prowjobs[0].Status.Description != "Jenkins job succeeded." {
		t.Fatalf("Expected description %q, got %q.", "Jenkins job succeeded.", fc.prowjobs[0].Status.Description)
	}
	if fc.prowjobs[0].Status.State != kube.SuccessState {
		t.Fatalf("Wrong state: %v", fc.prowjobs[0].Status.State)
	}
	// This is what the SQ reads.
	if fc.prowjobs[0].Spec.Context != "Some Job Context" {
		t.Fatalf("Wrong context: %v", fc.prowjobs[0].Spec.Context)
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
					Refs: kube.Refs{
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
					Refs: kube.Refs{
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
					Refs: kube.Refs{
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

		got := RunAfterSuccessCanRun(test.parent, test.child, newFakeConfigAgent(t, 0), fakeGH)
		if got != test.expected {
			t.Errorf("expected to run: %t, got: %t", test.expected, got)
		}
	}
}

func TestMaxConcurrencyWithNewlyTriggeredJobs(t *testing.T) {
	tests := []struct {
		name           string
		pjs            []kube.ProwJob
		pendingJobs    map[string]int
		expectedBuilds int
	}{
		{
			name: "avoid starting a triggered job",
			pjs: []kube.ProwJob{
				{
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 1,
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 1,
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
			},
			pendingJobs:    make(map[string]int),
			expectedBuilds: 1,
		},
		{
			name: "both triggered jobs can start",
			pjs: []kube.ProwJob{
				{
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 2,
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 2,
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
			},
			pendingJobs:    make(map[string]int),
			expectedBuilds: 2,
		},
		{
			name: "no triggered job can start",
			pjs: []kube.ProwJob{
				{
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 5,
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					Spec: kube.ProwJobSpec{
						Job:            "test-bazel-build",
						Type:           kube.PostsubmitJob,
						MaxConcurrency: 5,
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
			},
			pendingJobs:    map[string]int{"test-bazel-build": 5},
			expectedBuilds: 0,
		},
	}

	for _, test := range tests {
		jobs := make(chan kube.ProwJob, len(test.pjs))
		for _, pj := range test.pjs {
			jobs <- pj
		}
		close(jobs)

		fc := &fkc{
			prowjobs: test.pjs,
		}
		fjc := &fjc{}
		c := Controller{
			kc:          fc,
			jc:          fjc,
			ca:          newFakeConfigAgent(t, 0),
			pendingJobs: test.pendingJobs,
		}

		reports := make(chan<- kube.ProwJob, len(test.pjs))
		errors := make(chan<- error, len(test.pjs))

		syncProwJobs(c.syncNonPendingJob, jobs, reports, errors, nil)
		if len(fjc.pjs) != test.expectedBuilds {
			t.Errorf("expected builds: %d, got: %d", test.expectedBuilds, len(fjc.pjs))
		}
	}
}

func TestGetJenkinsJobs(t *testing.T) {
	tests := []struct {
		name     string
		pjs      []kube.ProwJob
		expected []string
	}{
		{
			name: "both complete and running",
			pjs: []kube.ProwJob{
				{
					Spec: kube.ProwJobSpec{
						Job: "coolio",
					},
					Status: kube.ProwJobStatus{
						CompletionTime: time.Now(),
					},
				},
				{
					Spec: kube.ProwJobSpec{
						Job: "maradona",
					},
					Status: kube.ProwJobStatus{},
				},
			},
			expected: []string{"maradona"},
		},
		{
			name: "only complete",
			pjs: []kube.ProwJob{
				{
					Spec: kube.ProwJobSpec{
						Job: "coolio",
					},
					Status: kube.ProwJobStatus{
						CompletionTime: time.Now(),
					},
				},
				{
					Spec: kube.ProwJobSpec{
						Job: "maradona",
					},
					Status: kube.ProwJobStatus{
						CompletionTime: time.Now(),
					},
				},
			},
			expected: nil,
		},
		{
			name: "only running",
			pjs: []kube.ProwJob{
				{
					Spec: kube.ProwJobSpec{
						Job: "coolio",
					},
					Status: kube.ProwJobStatus{},
				},
				{
					Spec: kube.ProwJobSpec{
						Job: "maradona",
					},
					Status: kube.ProwJobStatus{},
				},
			},
			expected: []string{"maradona", "coolio"},
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
				if ej == gj {
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

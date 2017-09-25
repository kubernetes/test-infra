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
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

func newFakeConfigAgent(t *testing.T) *fca {
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
			Plank: config.Plank{
				JobURLTemplate: template.Must(template.New("test").Parse("{{.Status.PodName}}/")),
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
	pjs      []kube.ProwJob
	enqueued bool
	status   Status
	err      error
}

func (f *fjc) Build(pj *kube.ProwJob) (*url.URL, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.built = true
	f.pjs = append(f.pjs, *pj)
	url, _ := url.Parse("localhost")
	return url, nil
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
			ca:          newFakeConfigAgent(t),
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
			ca:          newFakeConfigAgent(t),
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
		prowjobs: []kube.ProwJob{pjutil.NewProwJob(pjutil.BatchSpec(pre, kube.Refs{
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
		ca:          newFakeConfigAgent(t),
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

		got := RunAfterSuccessCanRun(test.parent, test.child, newFakeConfigAgent(t), fakeGH)
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
			ca:          newFakeConfigAgent(t),
			pendingJobs: test.pendingJobs,
		}

		reports := make(chan kube.ProwJob, len(test.pjs))
		errors := make(chan error, len(test.pjs))

		syncProwJobs(c.syncNonPendingJob, jobs, reports, errors)
		if len(fjc.pjs) != test.expectedBuilds {
			t.Errorf("expected builds: %d, got: %d", test.expectedBuilds, len(fjc.pjs))
		}
	}
}

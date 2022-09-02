/*
Copyright 2018 The Kubernetes Authors.

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

package github

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"

	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestShouldReport(t *testing.T) {
	var testcases = []struct {
		name        string
		pj          v1.ProwJob
		report      bool
		reportAgent v1.ProwJobAgent
	}{
		{
			name: "should not report periodic job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PeriodicJob,
					Report: true,
				},
			},
			report: false,
		},
		{
			name: "should report postsubmit job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PostsubmitJob,
					Report: true,
				},
			},
			report: true,
		},
		{
			name: "should not report batch job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.BatchJob,
					Report: true,
				},
			},
			report: false,
		},
		{
			name: "should report presubmit job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Report: true,
				},
			},
			report: true,
		},
		{
			name: "github should not report gerrit jobs",
			pj: v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritReportLabel: "plus-one-this-gerrit-label-please",
					},
				},
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Report: true,
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewReporter(nil, nil, tc.reportAgent, nil)
			if r := c.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), &tc.pj); r == tc.report {
				return
			}
			if tc.report {
				t.Error("failed to report")
			} else {
				t.Error("unexpectedly reported")
			}
		})
	}
}

// TestPresumitReportingLocks verifies locking happens
// for Presubmit reporting. Must be run with -race, relies
// on k8s.io/test-infra/prow/github/fakegithub not being
// threadsafe.
func TestPresumitReportingLocks(t *testing.T) {
	reporter := NewReporter(
		fakegithub.NewFakeClient(),
		func() *config.Config {
			return &config.Config{
				ProwConfig: config.ProwConfig{
					GitHubReporter: config.GitHubReporter{
						JobTypesToReport: []v1.ProwJobType{v1.PresubmitJob},
					},
				},
			}
		},
		v1.ProwJobAgent(""),
		nil,
	)

	pj := &v1.ProwJob{
		Spec: v1.ProwJobSpec{
			Refs: &v1.Refs{
				Org:   "org",
				Repo:  "repo",
				Pulls: []v1.Pull{{Number: 1}},
			},
			Type:   v1.PresubmitJob,
			Report: true,
		},
		Status: v1.ProwJobStatus{
			State:          v1.ErrorState,
			CompletionTime: &metav1.Time{},
		},
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if _, _, err := reporter.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), pj); err != nil {
			t.Errorf("error reporting: %v", err)
		}
		wg.Done()
	}()
	go func() {
		if _, _, err := reporter.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), pj); err != nil {
			t.Errorf("error reporting: %v", err)
		}
		wg.Done()
	}()

	wg.Wait()
}

func TestReport(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                              string
		createStatusContextError          error
		listIssueCommentsWithContextError error
		expectedError                     string
	}{
		{
			name: "Success",
		},
		{
			name:                     "Maximum sha error gets swallowed",
			createStatusContextError: errors.New(`This SHA and context has reached the maximum number of statuses`),
		},
		{
			name:                     "Error from user side gets swallowed",
			createStatusContextError: errors.New(`error setting status: status code 404 not one of [201], body: {"message":"Not Found","documentation_url":"https://docs.github.com/rest/reference/repos#create-a-commit-status"}`),
		},
		{
			name:                     "Error from user side gets swallowed2",
			createStatusContextError: errors.New(`failed to report job: error setting status: status code 422 not one of [201], body: {"message":"No commit found for SHA: 9d04799d1a22e9e604c50f6bbbec067aaccc1b32","documentation_url":"https://docs.github.com/rest/reference/repos#create-a-commit-status"}`),
		},
		{
			name:                     "Other error get returned",
			createStatusContextError: errors.New("something went wrong :("),
			expectedError:            "error setting status: something went wrong :(",
		},
		{
			name:                              "Comment error_Maximum sha error gets swallowed",
			listIssueCommentsWithContextError: errors.New(`This SHA and context has reached the maximum number of statuses`),
			expectedError:                     "error listing comments: This SHA and context has reached the maximum number of statuses",
		},
		{
			name:                              "Comment error_Error from user side gets swallowed",
			listIssueCommentsWithContextError: errors.New(`error setting status: status code 404 not one of [201], body: {"message":"Not Found","documentation_url":"https://docs.github.com/rest/reference/repos#create-a-commit-status"}`),
			expectedError:                     "error listing comments: error setting status: status code 404 not one of [201], body: {\"message\":\"Not Found\",\"documentation_url\":\"https://docs.github.com/rest/reference/repos#create-a-commit-status\"}",
		},
		{
			name:                              "Comment error_Error from user side gets swallowed2",
			listIssueCommentsWithContextError: errors.New(`failed to report job: error setting status: status code 422 not one of [201], body: {"message":"No commit found for SHA: 9d04799d1a22e9e604c50f6bbbec067aaccc1b32","documentation_url":"https://docs.github.com/rest/reference/repos#create-a-commit-status"}`),
			expectedError:                     "error listing comments: failed to report job: error setting status: status code 422 not one of [201], body: {\"message\":\"No commit found for SHA: 9d04799d1a22e9e604c50f6bbbec067aaccc1b32\",\"documentation_url\":\"https://docs.github.com/rest/reference/repos#create-a-commit-status\"}",
		},
		{
			name:                              "Comment error_Other error get returned",
			listIssueCommentsWithContextError: errors.New("something went wrong :("),
			expectedError:                     "error listing comments: something went wrong :(",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fghc := fakegithub.NewFakeClient()
			fghc.Error = tc.createStatusContextError
			fghc.ListIssueCommentsWithContextError = tc.listIssueCommentsWithContextError
			c := Client{
				gc: fghc,
				config: func() *config.Config {
					return &config.Config{
						ProwConfig: config.ProwConfig{
							GitHubReporter: config.GitHubReporter{
								JobTypesToReport: []v1.ProwJobType{v1.PostsubmitJob},
							},
						},
					}
				},
			}
			pj := &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PostsubmitJob,
					Report: true,
					Refs: &v1.Refs{
						Pulls: []v1.Pull{
							{},
						},
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{},
				},
			}

			errMsg := ""
			_, _, err := c.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), pj)
			if err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.expectedError {
				t.Errorf("expected error %q got error %q", tc.expectedError, errMsg)
			}
		})
	}
}

func TestPjsToReport(t *testing.T) {
	timeNow := time.Now().Truncate(time.Second) // Truncate so that comparison works.
	var testcases = []struct {
		name        string
		pj          *v1.ProwJob
		existingPJs []*v1.ProwJob
		wantPjs     []v1.ProwJob
		wantErr     bool
	}{
		{
			name: "two-jobs-finished",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org",
						kube.RepoLabel:        "repo",
						kube.PullLabel:        "123",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						PrevReportStates: map[string]v1.ProwJobState{
							"github-reporter": v1.FailureState,
						},
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
			wantPjs: []v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "0",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						ResourceVersion: "999",
					},
					Status: v1.ProwJobStatus{
						State:          v1.SuccessState,
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Type: v1.PresubmitJob,
						Refs: &v1.Refs{
							Repo: "foo",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-foo",
						Report: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						ResourceVersion: "999",
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						PrevReportStates: map[string]v1.ProwJobState{
							"github-reporter": v1.FailureState,
						},
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
		},
		{
			name: "one-job-still-running",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org",
						kube.RepoLabel:        "repo",
						kube.PullLabel:        "123",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						PrevReportStates: map[string]v1.ProwJobState{
							"github-reporter": v1.FailureState,
						},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
		},
		{
			name: "mix-of-finished-and-running",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org",
						kube.RepoLabel:        "repo",
						kube.PullLabel:        "123",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						PrevReportStates: map[string]v1.ProwJobState{
							"github-reporter": v1.FailureState,
						},
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "2",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.PendingState,
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-baz",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
		},
		{
			name: "current-job-only",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org",
						kube.RepoLabel:        "repo",
						kube.PullLabel:        "123",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			wantPjs: []v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "0",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						ResourceVersion: "999",
					},
					Status: v1.ProwJobStatus{
						State:          v1.SuccessState,
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Type: v1.PresubmitJob,
						Refs: &v1.Refs{
							Repo: "foo",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-foo",
						Report: true,
					},
				},
			},
		},
		{
			name: "current-job-not-finished",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org",
						kube.RepoLabel:        "repo",
						kube.PullLabel:        "123",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
		},
		{
			name: "job-not-same-pr",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org",
						kube.RepoLabel:        "repo",
						kube.PullLabel:        "456",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						PrevReportStates: map[string]v1.ProwJobState{
							"github-reporter": v1.FailureState,
						},
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
			wantPjs: []v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "0",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "456",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						ResourceVersion: "999",
					},
					Status: v1.ProwJobStatus{
						State:          v1.SuccessState,
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Type: v1.PresubmitJob,
						Refs: &v1.Refs{
							Repo: "foo",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-foo",
						Report: true,
					},
				},
			},
		},
		{
			name: "job-not-same-org",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org-different",
						kube.RepoLabel:        "repo",
						kube.PullLabel:        "123",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						PrevReportStates: map[string]v1.ProwJobState{
							"github-reporter": v1.FailureState,
						},
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
			wantPjs: []v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "0",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org-different",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						ResourceVersion: "999",
					},
					Status: v1.ProwJobStatus{
						State:          v1.SuccessState,
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Type: v1.PresubmitJob,
						Refs: &v1.Refs{
							Repo: "foo",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-foo",
						Report: true,
					},
				},
			},
		},
		{
			name: "job-not-same-pr",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "0",
					Labels: map[string]string{
						kube.ProwJobTypeLabel: "presubmit",
						kube.OrgLabel:         "org",
						kube.RepoLabel:        "repo-different",
						kube.PullLabel:        "123",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-time.Hour),
					},
				},
				Status: v1.ProwJobStatus{
					State:          v1.SuccessState,
					CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 123,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "1",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						PrevReportStates: map[string]v1.ProwJobState{
							"github-reporter": v1.FailureState,
						},
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
			wantPjs: []v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "0",
						Labels: map[string]string{
							kube.ProwJobTypeLabel: "presubmit",
							kube.OrgLabel:         "org",
							kube.RepoLabel:        "repo-different",
							kube.PullLabel:        "123",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						ResourceVersion: "999",
					},
					Status: v1.ProwJobStatus{
						State:          v1.SuccessState,
						CompletionTime: &metav1.Time{Time: timeNow.Add(-time.Minute)},
					},
					Spec: v1.ProwJobSpec{
						Type: v1.PresubmitJob,
						Refs: &v1.Refs{
							Repo: "foo",
							Pulls: []v1.Pull{
								{
									Number: 123,
								},
							},
						},
						Job:    "ci-foo",
						Report: true,
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			allpj := []runtime.Object{tc.pj}
			for _, pj := range tc.existingPJs {
				allpj = append(allpj, pj)
			}

			lister := fakectrlruntimeclient.NewFakeClient(allpj...)

			gotPjs, gotErr := pjsToReport(context.Background(), &logrus.Entry{}, lister, tc.pj)
			if (gotErr != nil && !tc.wantErr) || (gotErr == nil && tc.wantErr) {
				t.Fatalf("error mismatch. got: %v, want: %v", gotErr, tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantPjs, gotPjs, cmpopts.SortSlices(func(a, b v1.ProwJob) bool {
				return a.Name > b.Name
			})); diff != "" {
				t.Fatalf("pjs mismatch. got(+), want(-):\n%s", diff)
			}
		})
	}
}

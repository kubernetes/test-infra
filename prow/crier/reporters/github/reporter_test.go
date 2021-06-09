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

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/github/fakegithub"
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
						client.GerritReportLabel: "plus-one-this-gerrit-label-please",
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
			c := NewReporter(nil, nil, tc.reportAgent)
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

func TestShardedLockCleanup(t *testing.T) {
	t.Parallel()
	sl := &shardedLock{mapLock: &sync.Mutex{}, locks: map[simplePull]*sync.Mutex{}}
	key := simplePull{"org", "repo", 1}
	sl.locks[key] = &sync.Mutex{}
	sl.cleanup()
	if _, exists := sl.locks[key]; exists {
		t.Error("lock didn't get cleaned up")
	}

}

func TestReport(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		githubError   error
		expectedError string
	}{
		{
			name: "Success",
		},
		{
			name:        "Maximum sha error gets swallowed",
			githubError: errors.New(`This SHA and context has reached the maximum number of statuses`),
		},
		{
			name:        "Error from user side gets swallowed",
			githubError: errors.New(`error setting status: status code 404 not one of [201], body: {"message":"Not Found","documentation_url":"https://docs.github.com/rest/reference/repos#create-a-commit-status"}`),
		},
		{
			name:        "Error from user side gets swallowed2",
			githubError: errors.New(`failed to report job: error setting status: status code 422 not one of [201], body: {"message":"No commit found for SHA: 9d04799d1a22e9e604c50f6bbbec067aaccc1b32","documentation_url":"https://docs.github.com/rest/reference/repos#create-a-commit-status"}`),
		},
		{
			name:          "Other error get returned",
			githubError:   errors.New("something went wrong :("),
			expectedError: "error setting status: something went wrong :(",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fghc := fakegithub.NewFakeClient()
			fghc.Error = tc.githubError
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
					Refs:   &v1.Refs{},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
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

/*
Copyright 2020 The Kubernetes Authors.

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

package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/internal/testutil"
)

func TestReportJobFinished(t *testing.T) {
	completionTime := &metav1.Time{Time: time.Date(2010, 10, 10, 19, 00, 0, 0, time.UTC)}
	tests := []struct {
		jobState       prowv1.ProwJobState
		completionTime *metav1.Time
		passed         bool
		expectErr      bool
	}{
		{
			jobState:  prowv1.TriggeredState,
			expectErr: true,
		},
		{
			jobState:  prowv1.PendingState,
			expectErr: true,
		},
		{
			jobState:       prowv1.SuccessState,
			completionTime: completionTime,
			passed:         true,
		},
		{
			jobState:       prowv1.AbortedState,
			completionTime: completionTime,
		},
		{
			jobState:       prowv1.ErrorState,
			completionTime: completionTime,
		},
		{
			jobState:       prowv1.FailureState,
			completionTime: completionTime,
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("report %s job", tc.jobState), func(t *testing.T) {
			ctx := context.Background()
			cfg := testutil.Fca{C: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: map[string]*prowv1.DecorationConfig{"*": {
							GCSConfiguration: &prowv1.GCSConfiguration{
								Bucket:       "kubernetes-jenkins",
								PathPrefix:   "some-prefix",
								PathStrategy: prowv1.PathStrategyLegacy,
								DefaultOrg:   "kubernetes",
								DefaultRepo:  "kubernetes",
							},
						}},
					},
				},
			}}.Config
			ta := &testutil.TestAuthor{}
			reporter := newWithAuthor(cfg, ta, false)

			pj := &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type: prowv1.PresubmitJob,
					Refs: &prowv1.Refs{
						Org:   "kubernetes",
						Repo:  "test-infra",
						Pulls: []prowv1.Pull{{Number: 12345}},
					},
					Agent: prowv1.KubernetesAgent,
					Job:   "my-little-job",
				},
				Status: prowv1.ProwJobStatus{
					State:          tc.jobState,
					StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					CompletionTime: tc.completionTime,
					PodName:        "some-pod",
					BuildID:        "123",
				},
			}

			err := reporter.reportFinishedJob(ctx, logrus.NewEntry(logrus.StandardLogger()), pj)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("Expected an error, but didn't get one.")
				}
				return
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !strings.HasSuffix(ta.Path, prowv1.FinishedStatusFile) {
				t.Errorf("Expected file to be written to finished.json, but got %q", ta.Path)
			}
			if ta.Overwrite {
				t.Errorf("Expected file to be written without overwriting, but overwrite was enabled")
			}

			var result metadata.Finished
			if err := json.Unmarshal(ta.Content, &result); err != nil {
				t.Errorf("Couldn't decode result as metadata.Finished: %v", err)
			}
			if result.Timestamp == nil {
				t.Errorf("Expected finished.json timestamp to be %d, but it was nil", pj.Status.CompletionTime.Unix())
			} else if *result.Timestamp != pj.Status.CompletionTime.Unix() {
				t.Errorf("Expected finished.json timestamp to be %d, but got %d", pj.Status.CompletionTime.Unix(), *result.Timestamp)
			}
			if result.Passed == nil {
				t.Errorf("Expected finished.json passed to be %v, but it was nil", tc.passed)
			} else if *result.Passed != tc.passed {
				t.Errorf("Expected finished.json passed to be %v, but got %v", tc.passed, *result.Passed)
			}
		})
	}
}

func TestReportJobStarted(t *testing.T) {
	states := []prowv1.ProwJobState{prowv1.TriggeredState, prowv1.PendingState, prowv1.SuccessState, prowv1.AbortedState, prowv1.ErrorState, prowv1.FailureState}
	for _, state := range states {
		t.Run(fmt.Sprintf("report %s job started", state), func(t *testing.T) {
			ctx := context.Background()
			cfg := testutil.Fca{C: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: map[string]*prowv1.DecorationConfig{"*": {
							GCSConfiguration: &prowv1.GCSConfiguration{
								Bucket:       "kubernetes-jenkins",
								PathPrefix:   "some-prefix",
								PathStrategy: prowv1.PathStrategyLegacy,
								DefaultOrg:   "kubernetes",
								DefaultRepo:  "kubernetes",
							},
						}},
					},
				},
			}}.Config
			ta := &testutil.TestAuthor{}
			reporter := newWithAuthor(cfg, ta, false)

			pj := &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type: prowv1.PresubmitJob,
					Refs: &prowv1.Refs{
						Org:   "kubernetes",
						Repo:  "test-infra",
						Pulls: []prowv1.Pull{{Number: 12345}},
					},
					Agent: prowv1.KubernetesAgent,
					Job:   "my-little-job",
				},
				Status: prowv1.ProwJobStatus{
					State:     state,
					StartTime: metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					PodName:   "some-pod",
					BuildID:   "123",
				},
			}

			err := reporter.reportStartedJob(ctx, logrus.NewEntry(logrus.StandardLogger()), pj)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !strings.HasSuffix(ta.Path, prowv1.StartedStatusFile) {
				t.Errorf("Expected file to be written to started.json, but got %q", ta.Path)
			}
			if ta.Overwrite {
				t.Errorf("Expected file to be written without overwriting, but overwrite was enabled")
			}

			var result metadata.Started
			if err := json.Unmarshal(ta.Content, &result); err != nil {
				t.Errorf("Couldn't decode result as metadata.Started: %v", err)
			}
			if result.Timestamp != pj.Status.StartTime.Unix() {
				t.Errorf("Expected started.json timestamp to be %d, but got %d", pj.Status.StartTime.Unix(), result.Timestamp)
			}
		})
	}
}

func TestReportProwJob(t *testing.T) {
	ctx := context.Background()
	cfg := testutil.Fca{C: config.Config{
		ProwConfig: config.ProwConfig{
			Plank: config.Plank{
				DefaultDecorationConfigs: map[string]*prowv1.DecorationConfig{"*": {
					GCSConfiguration: &prowv1.GCSConfiguration{
						Bucket:       "kubernetes-jenkins",
						PathPrefix:   "some-prefix",
						PathStrategy: prowv1.PathStrategyLegacy,
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
				}},
			},
		},
	}}.Config
	ta := &testutil.TestAuthor{}
	reporter := newWithAuthor(cfg, ta, false)

	pj := &prowv1.ProwJob{
		Spec: prowv1.ProwJobSpec{
			Type: prowv1.PresubmitJob,
			Refs: &prowv1.Refs{
				Org:   "kubernetes",
				Repo:  "test-infra",
				Pulls: []prowv1.Pull{{Number: 12345}},
			},
			Agent: prowv1.KubernetesAgent,
			Job:   "my-little-job",
		},
		Status: prowv1.ProwJobStatus{
			State:          prowv1.SuccessState,
			StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
			CompletionTime: &metav1.Time{Time: time.Date(2010, 10, 10, 19, 00, 0, 0, time.UTC)},
			PodName:        "some-pod",
			BuildID:        "123",
		},
	}

	if err := reporter.reportProwjob(ctx, logrus.NewEntry(logrus.StandardLogger()), pj); err != nil {
		t.Fatalf("Unexpected error calling reportProwjob: %v", err)
	}

	if !strings.HasSuffix(ta.Path, "/prowjob.json") {
		t.Errorf("Expected prowjob to be written to prowjob.json, got %q", ta.Path)
	}

	if !ta.Overwrite {
		t.Errorf("Expected prowjob.json to be written with overwrite enabled, but it was not.")
	}

	var result prowv1.ProwJob
	if err := json.Unmarshal(ta.Content, &result); err != nil {
		t.Fatalf("Couldn't unmarshal prowjob.json: %v", err)
	}
	if !cmp.Equal(*pj, result) {
		t.Fatalf("Input prowjob mismatches output prowjob:\n%s", cmp.Diff(*pj, result))
	}
}

func TestShouldReport(t *testing.T) {
	tests := []struct {
		name         string
		buildID      string
		shouldReport bool
	}{
		{
			name:         "tests with a build ID should be reported",
			buildID:      "123",
			shouldReport: true,
		},
		{
			name:         "tests without a build ID should not be reported",
			buildID:      "",
			shouldReport: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pj := &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:  prowv1.PostsubmitJob,
					Agent: prowv1.KubernetesAgent,
					Job:   "my-little-job",
				},
				Status: prowv1.ProwJobStatus{
					State:     prowv1.TriggeredState,
					StartTime: metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					BuildID:   tc.buildID,
				},
			}
			gr := newWithAuthor(testutil.Fca{}.Config, nil, false)
			result := gr.ShouldReport(logrus.NewEntry(logrus.StandardLogger()), pj)
			if result != tc.shouldReport {
				t.Errorf("Got ShouldReport() returned %v, but expected %v", result, tc.shouldReport)
			}
		})
	}
}

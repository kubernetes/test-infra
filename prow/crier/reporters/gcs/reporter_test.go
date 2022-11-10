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
	stdio "io"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/fakeopener"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

type fca struct {
	c config.Config
}

func (ca fca) Config() *config.Config {
	return &ca.c
}

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
			cfg := fca{c: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: config.DefaultDecorationMapToSliceTesting(
							map[string]*prowv1.DecorationConfig{"*": {
								GCSConfiguration: &prowv1.GCSConfiguration{
									Bucket:       "kubernetes-jenkins",
									PathPrefix:   "some-prefix",
									PathStrategy: prowv1.PathStrategyLegacy,
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
								},
							}}),
					},
				},
			}}.Config
			fakeOpener := &fakeopener.FakeOpener{}
			reporter := New(cfg, fakeOpener, false)

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

			var content []byte
			for path, buf := range fakeOpener.Buffer {
				if strings.HasSuffix(path, prowv1.FinishedStatusFile) {
					content, err = stdio.ReadAll(buf)
					if err != nil {
						t.Fatalf("Failed reading content: %v", err)
					}
					break
				}
			}
			var result metadata.Finished
			if err := json.Unmarshal(content, &result); err != nil {
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
	tests := []struct {
		name            string
		existingStarted *metadata.Started
		state           prowv1.ProwJobState
		cloneRecord     []clone.Record
		expect          metadata.Started
	}{
		{
			name:  "TriggeredState",
			state: prowv1.TriggeredState,
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit:            "def456",
				DeprecatedRepoVersion: "def456",
				Metadata:              metadata.Metadata{"uploader": string("crier")},
			},
		},
		{
			name:  "PendingState",
			state: prowv1.PendingState,
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit:            "def456",
				DeprecatedRepoVersion: "def456",
				Metadata:              metadata.Metadata{"uploader": string("crier")},
			},
		},
		{
			name:  "SuccessState",
			state: prowv1.SuccessState,
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit:            "def456",
				DeprecatedRepoVersion: "def456",
				Metadata:              metadata.Metadata{"uploader": string("crier")},
			},
		},
		{
			name:  "AbortedState",
			state: prowv1.AbortedState,
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit:            "def456",
				DeprecatedRepoVersion: "def456",
				Metadata:              metadata.Metadata{"uploader": string("crier")},
			},
		},
		{
			name:  "ErrorState",
			state: prowv1.ErrorState,
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit:            "def456",
				DeprecatedRepoVersion: "def456",
				Metadata:              metadata.Metadata{"uploader": string("crier")},
			},
		},
		{
			name:  "FailureState",
			state: prowv1.ErrorState,
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit:            "def456",
				DeprecatedRepoVersion: "def456",
				Metadata:              metadata.Metadata{"uploader": string("crier")},
			},
		},
		{
			name:  "overwrite-crier-uploaded",
			state: prowv1.SuccessState,
			existingStarted: &metadata.Started{
				Timestamp:  1286735400,
				Pull:       "12345",
				Repos:      map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit: "def456",
				Metadata:   metadata.Metadata{"uploader": string("crier")},
			},
			cloneRecord: []clone.Record{
				{
					Refs: prowv1.Refs{
						Org:     "kubernetes",
						Repo:    "test-infra",
						BaseRef: "main",
						Pulls:   []prowv1.Pull{{Number: 12345, SHA: "def456"}},
					},
					FinalSHA: "abc123",
				},
			},
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				Metadata:              metadata.Metadata{"uploader": string("crier")},
				RepoCommit:            "abc123",
				DeprecatedRepoVersion: "abc123",
			},
		},
		{
			name:  "overwrite-crier-uploaded-without-SHA",
			state: prowv1.SuccessState,
			existingStarted: &metadata.Started{
				Timestamp:  1286735400,
				Pull:       "12345",
				Repos:      map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit: "def456",
				Metadata:   metadata.Metadata{"uploader": string("crier")},
			},
			cloneRecord: []clone.Record{
				{
					Refs: prowv1.Refs{
						Org:     "kubernetes",
						Repo:    "test-infra",
						BaseRef: "main",
						Pulls:   []prowv1.Pull{{Number: 12345, SHA: "def456"}},
					},
					FinalSHA: "abc123",
				},
			},
			expect: metadata.Started{
				Timestamp:             1286735400,
				Pull:                  "12345",
				Repos:                 map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				Metadata:              metadata.Metadata{"uploader": string("crier")},
				RepoCommit:            "abc123",
				DeprecatedRepoVersion: "abc123",
			},
		},
		{
			name:  "no-overwrite-others-uploaded",
			state: prowv1.SuccessState,
			existingStarted: &metadata.Started{
				Timestamp: 1286735400,
				Pull:      "12345",
				Repos:     map[string]string{"kubernetes/test-infra": "main,12345:def456"},
			},
			cloneRecord: []clone.Record{
				{
					Refs: prowv1.Refs{
						Org:     "kubernetes",
						Repo:    "test-infra",
						BaseRef: "main",
						Pulls:   []prowv1.Pull{{Number: 12345}},
					},
					FinalSHA: "abc123",
				},
			},
			expect: metadata.Started{
				Timestamp: 1286735400,
				Pull:      "12345",
				Repos:     map[string]string{"kubernetes/test-infra": "main,12345:def456"},
			},
		},
		{
			name:  "no-cloneref-self-update",
			state: prowv1.SuccessState,
			existingStarted: &metadata.Started{
				Timestamp:  100, // Intentionally wrong timestamp. Crier will change it if it overwrites.
				Pull:       "12345",
				Repos:      map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit: "main",
				Metadata:   metadata.Metadata{"uploader": string("crier")},
			},
			expect: metadata.Started{
				Timestamp:  100,
				Pull:       "12345",
				Repos:      map[string]string{"kubernetes/test-infra": "main,12345:def456"},
				RepoCommit: "main",
				Metadata:   metadata.Metadata{"uploader": string("crier")},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := fca{c: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: config.DefaultDecorationMapToSliceTesting(
							map[string]*prowv1.DecorationConfig{"*": {
								GCSConfiguration: &prowv1.GCSConfiguration{
									Bucket:       "kubernetes-jenkins",
									PathPrefix:   "some-prefix",
									PathStrategy: prowv1.PathStrategyLegacy,
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
								},
							}}),
					},
				},
			}}.Config
			// Storage path decided by Prow
			const subDir = "some-prefix/pr-logs/pull/test-infra/12345/my-little-job/123"

			opener := &fakeopener.FakeOpener{}
			if tc.existingStarted != nil {
				content, err := json.Marshal(*tc.existingStarted)
				if err != nil {
					t.Fatalf("Failed to marshal started.json: %v", err)
				}
				storagePath, _ := providers.StoragePath("kubernetes-jenkins", path.Join(subDir, "started.json"))
				if err := io.WriteContent(ctx, logrus.NewEntry(logrus.StandardLogger()), opener, storagePath, content); err != nil {
					t.Fatalf("Failed creating started.json: %v", err)
				}
			}
			if len(tc.cloneRecord) > 0 {
				content, err := json.Marshal(tc.cloneRecord)
				if err != nil {
					t.Fatalf("Failed to marshal clone record: %v", err)
				}
				storagePath, _ := providers.StoragePath("kubernetes-jenkins", path.Join(subDir, "clone-records.json"))
				if err := io.WriteContent(ctx, logrus.NewEntry(logrus.StandardLogger()), opener, storagePath, content); err != nil {
					t.Fatalf("Failed seeding clone-records.json: %v", err)
				}
			}

			reporter := New(cfg, opener, false)

			pj := &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type: prowv1.PresubmitJob,
					Refs: &prowv1.Refs{
						Org:     "kubernetes",
						Repo:    "test-infra",
						BaseRef: "main",
						Pulls:   []prowv1.Pull{{Number: 12345, SHA: "def456"}},
					},
					Agent: prowv1.KubernetesAgent,
					Job:   "my-little-job",
				},
				Status: prowv1.ProwJobStatus{
					State:     tc.state,
					StartTime: metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					PodName:   "some-pod",
					BuildID:   "123",
				},
			}

			err := reporter.reportStartedJob(ctx, logrus.NewEntry(logrus.StandardLogger()), pj)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			storagePath, _ := providers.StoragePath("kubernetes-jenkins", path.Join(subDir, "started.json"))
			content, err := io.ReadContent(ctx, logrus.WithContext(ctx), opener, storagePath)
			if err != nil {
				t.Fatalf("Failed reading started.json: %v", err)
			}

			var result metadata.Started
			if err := json.Unmarshal(content, &result); err != nil {
				t.Fatalf("Couldn't decode result as metadata.Started: %v", err)
			}
			if diff := cmp.Diff(tc.expect, result); diff != "" {
				t.Fatalf("Started.json mismatch. Want(-), got(+):\n%s", diff)
			}
		})
	}
}

func TestReportProwJob(t *testing.T) {
	ctx := context.Background()
	cfg := fca{c: config.Config{
		ProwConfig: config.ProwConfig{
			Plank: config.Plank{
				DefaultDecorationConfigs: config.DefaultDecorationMapToSliceTesting(
					map[string]*prowv1.DecorationConfig{"*": {
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket:       "kubernetes-jenkins",
							PathPrefix:   "some-prefix",
							PathStrategy: prowv1.PathStrategyLegacy,
							DefaultOrg:   "kubernetes",
							DefaultRepo:  "kubernetes",
						},
					}}),
			},
		},
	}}.Config
	fakeOpener := &fakeopener.FakeOpener{}
	reporter := New(cfg, fakeOpener, false)

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

	var content []byte
	var err error
	for p, b := range fakeOpener.Buffer {
		if strings.HasSuffix(p, prowv1.ProwJobFile) {
			content, err = stdio.ReadAll(b)
			if err != nil {
				t.Fatalf("Failed reading content: %v", err)
			}
			break
		}
	}

	var result prowv1.ProwJob
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("Couldn't unmarshal %s: %v", prowv1.ProwJobFile, err)
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
			gr := New(fca{}.Config, nil, false)
			result := gr.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), pj)
			if result != tc.shouldReport {
				t.Errorf("Got ShouldReport() returned %v, but expected %v", result, tc.shouldReport)
			}
		})
	}
}

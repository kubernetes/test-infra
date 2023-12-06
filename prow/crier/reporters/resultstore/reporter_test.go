/*
Copyright 2023 The Kubernetes Authors.

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

package resultstore

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io/fakeopener"
	"k8s.io/test-infra/prow/resultstore"
)

type fakeConfigGetter struct {
	c config.Config
}

func (fcg fakeConfigGetter) Config() *config.Config {
	return &fcg.c
}

func TestGetName(t *testing.T) {
	gr := New(fakeConfigGetter{}.Config, &fakeopener.FakeOpener{}, &resultstore.Uploader{}, false)
	want := "resultstorereporter"
	if got := gr.GetName(); got != want {
		t.Errorf("GetName() got %v, want %v", got, want)
	}
}

func TestShouldReport(t *testing.T) {
	tests := []struct {
		name         string
		job          *prowv1.ProwJob
		shouldReport bool
	}{
		{
			name: "Successful job reported",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:   prowv1.PostsubmitJob,
					Agent:  prowv1.KubernetesAgent,
					Job:    "prow-job",
					Report: true,
					Refs: &prowv1.Refs{
						Org:  "org",
						Repo: "repo",
					},
					DecorationConfig: &prowv1.DecorationConfig{
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket: "gs://bucket",
						},
					},
					ReporterConfig: &prowv1.ReporterConfig{
						ResultStore: &prowv1.ResultStoreReporter{
							ProjectID: "cloud-project-id",
						},
					},
				},
				Status: prowv1.ProwJobStatus{
					State:          prowv1.SuccessState,
					StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 25, time.UTC)},
					BuildID:        "build-id",
				},
			},
			shouldReport: true,
		},
		{
			name: "Failed job reported",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:   prowv1.PostsubmitJob,
					Agent:  prowv1.KubernetesAgent,
					Job:    "prow-job",
					Report: true,
					Refs: &prowv1.Refs{
						Org:  "org",
						Repo: "repo",
					},
					DecorationConfig: &prowv1.DecorationConfig{
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket: "gs://bucket",
						},
					},
					ReporterConfig: &prowv1.ReporterConfig{
						ResultStore: &prowv1.ResultStoreReporter{
							ProjectID: "cloud-project-id",
						},
					},
				},
				Status: prowv1.ProwJobStatus{
					State:          prowv1.FailureState,
					StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 25, time.UTC)},
					BuildID:        "build-id",
				},
			},
			shouldReport: true,
		},
		{
			name: "Bare bucket name okay",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:   prowv1.PostsubmitJob,
					Agent:  prowv1.KubernetesAgent,
					Job:    "prow-job",
					Report: true,
					Refs: &prowv1.Refs{
						Org:  "org",
						Repo: "repo",
					},
					DecorationConfig: &prowv1.DecorationConfig{
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket: "bucket",
						},
					},
					ReporterConfig: &prowv1.ReporterConfig{
						ResultStore: &prowv1.ResultStoreReporter{
							ProjectID: "cloud-project-id",
						},
					},
				},
				Status: prowv1.ProwJobStatus{
					State:          prowv1.SuccessState,
					StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 25, time.UTC)},
					BuildID:        "build-id",
				},
			},
			shouldReport: true,
		},
		{
			name: "Report: false job not reported",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:   prowv1.PostsubmitJob,
					Agent:  prowv1.KubernetesAgent,
					Job:    "prow-job",
					Report: false,
					Refs: &prowv1.Refs{
						Org:  "org",
						Repo: "repo",
					},
					DecorationConfig: &prowv1.DecorationConfig{
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket: "gs://bucket",
						},
					},
					ReporterConfig: &prowv1.ReporterConfig{
						ResultStore: &prowv1.ResultStoreReporter{
							ProjectID: "cloud-project-id",
						},
					},
				},
				Status: prowv1.ProwJobStatus{
					State:          prowv1.SuccessState,
					StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 25, time.UTC)},
					BuildID:        "build-id",
				},
			},
			shouldReport: false,
		},
		{
			name: "Report: no resultstore project id job not reported",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:   prowv1.PostsubmitJob,
					Agent:  prowv1.KubernetesAgent,
					Job:    "prow-job",
					Report: true,
					Refs: &prowv1.Refs{
						Org:  "org",
						Repo: "repo",
					},
					DecorationConfig: &prowv1.DecorationConfig{
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket: "gs://bucket",
						},
					},
					ReporterConfig: &prowv1.ReporterConfig{},
				},
				Status: prowv1.ProwJobStatus{
					State:          prowv1.SuccessState,
					StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 25, time.UTC)},
					BuildID:        "build-id",
				},
			},
			shouldReport: false,
		},
		{
			name: "Non-GCS job not reported",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:   prowv1.PostsubmitJob,
					Agent:  prowv1.KubernetesAgent,
					Job:    "prow-job",
					Report: true,
					Refs: &prowv1.Refs{
						Org:  "org",
						Repo: "repo",
					},
					DecorationConfig: &prowv1.DecorationConfig{
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket: "non-gcs://bucket",
						},
					},
					ReporterConfig: &prowv1.ReporterConfig{
						ResultStore: &prowv1.ResultStoreReporter{
							ProjectID: "cloud-project-id",
						},
					},
				},
				Status: prowv1.ProwJobStatus{
					State:          prowv1.SuccessState,
					StartTime:      metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 25, time.UTC)},
					BuildID:        "build-id",
				},
			},
			shouldReport: false,
		},
		{
			name: "Pending job not reported",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:   prowv1.PostsubmitJob,
					Agent:  prowv1.KubernetesAgent,
					Job:    "prow-job",
					Report: true,
					Refs: &prowv1.Refs{
						Org:  "org",
						Repo: "repo",
					},
					DecorationConfig: &prowv1.DecorationConfig{
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket: "gs://bucket",
						},
					},
					ReporterConfig: &prowv1.ReporterConfig{
						ResultStore: &prowv1.ResultStoreReporter{
							ProjectID: "cloud-project-id",
						},
					},
				},
				Status: prowv1.ProwJobStatus{
					State:     prowv1.PendingState,
					StartTime: metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					BuildID:   "build-id",
				},
			},
			shouldReport: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gr := New(fakeConfigGetter{}.Config, &fakeopener.FakeOpener{}, &resultstore.Uploader{}, false)
			result := gr.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), tc.job)
			if result != tc.shouldReport {
				t.Errorf("ShouldReport() got %v, want %v", result, tc.shouldReport)
			}
		})
	}
}

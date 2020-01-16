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

package reporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/googleapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

type fca struct {
	c config.Config
}

func (ca fca) Config() *config.Config {
	return &ca.c
}

func TestIsErrUnexpected(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		unexpected bool
	}{
		{
			name:       "standard errors are unexpected",
			err:        errors.New("this is just a normal error"),
			unexpected: true,
		},
		{
			name:       "nil errors are expected",
			err:        nil,
			unexpected: false,
		},
		{
			name:       "googleapi errors other than Precondition Failed are unexpected",
			err:        &googleapi.Error{Code: http.StatusNotFound},
			unexpected: true,
		},
		{
			name:       "Precondition Failed googleapi errors are expected",
			err:        &googleapi.Error{Code: http.StatusPreconditionFailed},
			unexpected: false,
		},
	}

	gr := newWithAuthor(fca{}.Config, nil, false)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gr.isErrUnexpected(tc.err)
			if result != tc.unexpected {
				t.Errorf("Expected isErrUnexpected() to return %v, got %v", tc.unexpected, result)
			}
		})
	}
}

func TestGetJobDestination(t *testing.T) {
	standardGcsConfig := &prowv1.GCSConfiguration{
		Bucket:       "kubernetes-jenkins",
		PathPrefix:   "some-prefix",
		PathStrategy: prowv1.PathStrategyLegacy,
		DefaultOrg:   "kubernetes",
		DefaultRepo:  "kubernetes",
	}
	standardRefs := &prowv1.Refs{
		Org:   "kubernetes",
		Repo:  "test-infra",
		Pulls: []prowv1.Pull{{Number: 12345}},
	}
	tests := []struct {
		name              string
		defaultGcsConfigs map[string]*prowv1.GCSConfiguration
		prowjobGcsConfig  *prowv1.GCSConfiguration
		prowjobType       prowv1.ProwJobType
		prowjobRefs       *prowv1.Refs
		expectBucket      string
		expectDir         string // tip: this will always end in "my-little-job/123"
		expectErr         bool
	}{
		{
			name:              "decorated prowjob uses inline config when default is empty",
			defaultGcsConfigs: nil,
			prowjobGcsConfig:  standardGcsConfig,
			prowjobType:       prowv1.PeriodicJob,
			expectBucket:      "kubernetes-jenkins",
			expectDir:         "some-prefix/logs/my-little-job/123",
		},
		{
			name: "decorated prowjob uses inline config over default config",
			defaultGcsConfigs: map[string]*prowv1.GCSConfiguration{
				"*": {
					Bucket:       "the-wrong-bucket",
					PathPrefix:   "",
					PathStrategy: prowv1.PathStrategyLegacy,
					DefaultOrg:   "kubernetes",
					DefaultRepo:  "kubernetes",
				},
			},
			prowjobGcsConfig: standardGcsConfig,
			prowjobType:      prowv1.PeriodicJob,
			expectBucket:     "kubernetes-jenkins",
			expectDir:        "some-prefix/logs/my-little-job/123",
		},
		{
			name:              "undecorated prowjob falls back to default config",
			defaultGcsConfigs: map[string]*prowv1.GCSConfiguration{"*": standardGcsConfig},
			prowjobGcsConfig:  nil,
			prowjobType:       prowv1.PeriodicJob,
			expectBucket:      "kubernetes-jenkins",
			expectDir:         "some-prefix/logs/my-little-job/123",
		},
		{
			name:              "undecorated prowjob with no default config is an error",
			defaultGcsConfigs: nil,
			prowjobGcsConfig:  nil,
			prowjobType:       prowv1.PeriodicJob,
			expectErr:         true,
		},
		{
			name: "undecorated prowjob uses the correct org config",
			defaultGcsConfigs: map[string]*prowv1.GCSConfiguration{
				"*": {
					Bucket:       "the-wrong-bucket",
					PathPrefix:   "",
					PathStrategy: prowv1.PathStrategyLegacy,
					DefaultOrg:   "the-wrong-org",
					DefaultRepo:  "the-wrong-repo",
				},
				"kubernetes": standardGcsConfig,
			},
			prowjobGcsConfig: nil,
			prowjobType:      prowv1.PeriodicJob,
			prowjobRefs:      standardRefs,
			expectBucket:     "kubernetes-jenkins",
			expectDir:        "some-prefix/logs/my-little-job/123",
		},
		{
			name:             "prowjob type is respected",
			prowjobGcsConfig: standardGcsConfig,
			prowjobRefs:      standardRefs,
			prowjobType:      prowv1.PresubmitJob,
			expectBucket:     "kubernetes-jenkins",
			expectDir:        "some-prefix/pr-logs/pull/test-infra/12345/my-little-job/123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pj := &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Type:             tc.prowjobType,
					Refs:             tc.prowjobRefs,
					Agent:            prowv1.KubernetesAgent,
					Job:              "my-little-job",
					DecorationConfig: &prowv1.DecorationConfig{GCSConfiguration: tc.prowjobGcsConfig},
				},
				Status: prowv1.ProwJobStatus{
					State:     prowv1.TriggeredState,
					StartTime: metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
					PodName:   "some-pod",
					BuildID:   "123",
				},
			}
			decorationConfigs := map[string]*prowv1.DecorationConfig{}
			for k, v := range tc.defaultGcsConfigs {
				decorationConfigs[k] = &prowv1.DecorationConfig{GCSConfiguration: v}
			}
			cfg := fca{c: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: decorationConfigs,
					},
				},
			}}.Config

			reporter := newWithAuthor(cfg, nil, false)
			bucket, dir, err := reporter.getJobDestination(pj)
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("Unexpected error: %v", err)
				}
			} else if tc.expectErr {
				t.Fatalf("Expected an error, but didn't get one; instead got gs://%q/%q", bucket, dir)
			}
			if bucket != tc.expectBucket {
				t.Errorf("Expected bucket %q, but got %q", tc.expectBucket, bucket)
			}
			if dir != tc.expectDir {
				t.Errorf("Expected dir %q, but got %q", tc.expectDir, dir)
			}
		})
	}
}

type testAuthor struct {
	alreadyUsed bool
	bucket      string
	path        string
	content     []byte
	overwrite   bool
	closed      bool
}

type testAuthorWriteCloser struct {
	author *testAuthor
}

func (wc *testAuthorWriteCloser) Write(p []byte) (int, error) {
	wc.author.content = append(wc.author.content, p...)
	return len(p), nil
}

func (wc *testAuthorWriteCloser) Close() error {
	wc.author.closed = true
	return nil
}

func (ta *testAuthor) NewWriter(ctx context.Context, bucket, path string, overwrite bool) io.WriteCloser {
	if ta.alreadyUsed {
		panic(fmt.Sprintf("NewWriter called on testAuthor twice: first for %q/%q, now for %q/%q", ta.bucket, ta.path, bucket, path))
	}
	ta.alreadyUsed = true
	ta.bucket = bucket
	ta.path = path
	ta.overwrite = overwrite
	return &testAuthorWriteCloser{author: ta}
}

func TestReportJobState(t *testing.T) {
	completionTime := &metav1.Time{Time: time.Date(2010, 10, 10, 19, 00, 0, 0, time.UTC)}
	tests := []struct {
		jobState       prowv1.ProwJobState
		expectedFile   string
		completionTime *metav1.Time
		passed         bool
	}{
		{
			jobState:     prowv1.TriggeredState,
			expectedFile: "started.json",
		},
		{
			jobState: prowv1.PendingState,
		},
		{
			jobState:       prowv1.SuccessState,
			expectedFile:   "finished.json",
			completionTime: completionTime,
			passed:         true,
		},
		{
			jobState:       prowv1.AbortedState,
			expectedFile:   "finished.json",
			completionTime: completionTime,
		},
		{
			jobState:       prowv1.ErrorState,
			expectedFile:   "finished.json",
			completionTime: completionTime,
		},
		{
			jobState:       prowv1.FailureState,
			expectedFile:   "finished.json",
			completionTime: completionTime,
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("report %s job", tc.jobState), func(t *testing.T) {
			ctx := context.Background()
			cfg := fca{c: config.Config{
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
			ta := &testAuthor{}
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

			err := reporter.reportJobState(ctx, pj)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !strings.HasSuffix(ta.path, tc.expectedFile) {
				t.Errorf("Expected file to be written to %q, but got %q", tc.expectedFile, ta.path)
			}
			if ta.overwrite {
				t.Errorf("Expected file to be written without overwriting, but overwrite was enabled")
			}

			if tc.expectedFile == "started.json" {
				var result metadata.Started
				if err := json.Unmarshal(ta.content, &result); err != nil {
					t.Errorf("Couldn't decode result as metadata.Started: %v", err)
				}
				if result.Timestamp != pj.Status.StartTime.Unix() {
					t.Errorf("Expected started.json timestamp to be %d, but got %d", pj.Status.StartTime.Unix(), result.Timestamp)
				}
			} else if tc.expectedFile == "finished.json" {
				var result metadata.Finished
				if err := json.Unmarshal(ta.content, &result); err != nil {
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
			}
		})
	}
}

func TestReportProwJob(t *testing.T) {
	ctx := context.Background()
	cfg := fca{c: config.Config{
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
	ta := &testAuthor{}
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

	if err := reporter.reportProwjob(ctx, pj); err != nil {
		t.Fatalf("Unexpected error calling reportProwjob: %v", err)
	}

	if !strings.HasSuffix(ta.path, "/prowjob.json") {
		t.Errorf("Expected prowjob to be written to prowjob.json, got %q", ta.path)
	}

	if !ta.overwrite {
		t.Errorf("Expected prowjob.json to be written with overwrite enabled, but it was not.")
	}

	var result prowv1.ProwJob
	if err := json.Unmarshal(ta.content, &result); err != nil {
		t.Fatalf("Couldn't unmarshal prowjob.json: %v", err)
	}
	if !cmp.Equal(*pj, result) {
		t.Fatalf("Input prowjob mismatches output prowjob:\n%s", cmp.Diff(*pj, result))
	}
}

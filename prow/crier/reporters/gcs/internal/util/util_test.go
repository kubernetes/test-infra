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

package util

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/internal/testutil"
)

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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isErrUnexpected(tc.err)
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
		buildID           string
		expectBucket      string
		expectDir         string // tip: this will always end in "my-little-job/[buildID]"
		expectErr         bool
	}{
		{
			name:              "decorated prowjob uses inline config when default is empty",
			defaultGcsConfigs: nil,
			prowjobGcsConfig:  standardGcsConfig,
			prowjobType:       prowv1.PeriodicJob,
			buildID:           "123",
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
			buildID:          "123",
			expectBucket:     "kubernetes-jenkins",
			expectDir:        "some-prefix/logs/my-little-job/123",
		},
		{
			name:              "undecorated prowjob falls back to default config",
			defaultGcsConfigs: map[string]*prowv1.GCSConfiguration{"*": standardGcsConfig},
			prowjobGcsConfig:  nil,
			prowjobType:       prowv1.PeriodicJob,
			buildID:           "123",
			expectBucket:      "kubernetes-jenkins",
			expectDir:         "some-prefix/logs/my-little-job/123",
		},
		{
			name:              "undecorated prowjob with no default config is an error",
			defaultGcsConfigs: nil,
			prowjobGcsConfig:  nil,
			prowjobType:       prowv1.PeriodicJob,
			buildID:           "123",
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
			buildID:          "123",
			expectBucket:     "kubernetes-jenkins",
			expectDir:        "some-prefix/logs/my-little-job/123",
		},
		{
			name:             "prowjob type is respected",
			prowjobGcsConfig: standardGcsConfig,
			prowjobRefs:      standardRefs,
			prowjobType:      prowv1.PresubmitJob,
			buildID:          "123",
			expectBucket:     "kubernetes-jenkins",
			expectDir:        "some-prefix/pr-logs/pull/test-infra/12345/my-little-job/123",
		},
		{
			name:              "reporting a prowjob with no BuildID is an error",
			defaultGcsConfigs: nil,
			prowjobGcsConfig:  standardGcsConfig,
			prowjobType:       prowv1.PeriodicJob,
			expectErr:         true,
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
					BuildID:   tc.buildID,
				},
			}
			decorationConfigs := map[string]*prowv1.DecorationConfig{}
			for k, v := range tc.defaultGcsConfigs {
				decorationConfigs[k] = &prowv1.DecorationConfig{GCSConfiguration: v}
			}
			cfg := testutil.Fca{C: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: config.DefaultDecorationMapToSliceTesting(decorationConfigs),
					},
				},
			}}.Config

			bucket, dir, err := GetJobDestination(cfg, pj)
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

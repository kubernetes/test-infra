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

package gcsupload

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

func TestOptions_AssembleTargets(t *testing.T) {
	var testCases = []struct {
		name     string
		jobType  prowapi.ProwJobType
		options  Options
		paths    []string
		extra    map[string]gcs.UploadFunc
		expected []string
		wantErr  bool
	}{
		{
			name:    "no extra paths should upload infra files for presubmits",
			jobType: prowapi.PresubmitJob,
			options: Options{
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "bucket",
				},
			},
			expected: []string{
				"pr-logs/directory/job/build.txt",
				"pr-logs/directory/job/latest-build.txt",
				"pr-logs/pull/org_repo/1/job/latest-build.txt",
			},
		},
		{
			name:    "no extra paths should upload infra files for postsubmits",
			jobType: prowapi.PostsubmitJob,
			options: Options{
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "bucket",
				},
			},
			expected: []string{
				"logs/job/latest-build.txt",
			},
		},
		{
			name:    "no extra paths should upload infra files for periodics",
			jobType: prowapi.PeriodicJob,
			options: Options{
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "bucket",
				},
			},
			expected: []string{
				"logs/job/latest-build.txt",
			},
		},
		{
			name:    "no extra paths should upload infra files for batches",
			jobType: prowapi.BatchJob,
			options: Options{
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "bucket",
				},
			},
			expected: []string{
				"pr-logs/directory/job/latest-build.txt",
			},
		},
		{
			name:    "extra paths should be uploaded under job dir",
			jobType: prowapi.PresubmitJob,
			options: Options{
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "bucket",
				},
			},
			extra: map[string]gcs.UploadFunc{
				"something": gcs.DataUpload(strings.NewReader("data")),
				"else":      gcs.DataUpload(strings.NewReader("data")),
			},
			expected: []string{
				"pr-logs/pull/org_repo/1/job/build/something",
				"pr-logs/pull/org_repo/1/job/build/else",
				"pr-logs/directory/job/build.txt",
				"pr-logs/directory/job/latest-build.txt",
				"pr-logs/pull/org_repo/1/job/latest-build.txt",
			},
		},
		{
			name:    "literal files should be uploaded under job dir",
			jobType: prowapi.PresubmitJob,
			options: Options{
				Items: []string{"something", "else"},
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "bucket",
				},
			},
			paths: []string{"something", "else", "notforupload"},
			expected: []string{
				"pr-logs/pull/org_repo/1/job/build/something",
				"pr-logs/pull/org_repo/1/job/build/else",
				"pr-logs/directory/job/build.txt",
				"pr-logs/directory/job/latest-build.txt",
				"pr-logs/pull/org_repo/1/job/latest-build.txt",
			},
		},
		{
			name:    "directories should be uploaded under job dir",
			jobType: prowapi.PresubmitJob,
			options: Options{
				Items: []string{"something"},
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "bucket",
				},
			},
			paths: []string{"something/", "something/else", "notforupload"},
			expected: []string{
				"pr-logs/pull/org_repo/1/job/build/something/else",
				"pr-logs/directory/job/build.txt",
				"pr-logs/directory/job/latest-build.txt",
				"pr-logs/pull/org_repo/1/job/latest-build.txt",
			},
		},
		{
			name:    "only job dir files should be output in local mode",
			jobType: prowapi.PresubmitJob,
			options: Options{
				Items: []string{"something", "more"},
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy:   prowapi.PathStrategyExplicit,
					LocalOutputDir: "/output",
				},
			},
			paths: []string{"something/", "something/else", "more", "notforupload"},
			expected: []string{
				"something/else",
				"more",
			},
		},
		{
			name:    "invalid bucket name",
			jobType: prowapi.PresubmitJob,
			options: Options{
				Items: []string{"something"},
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
					Bucket:       "://bucket",
				},
			},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			spec := &downwardapi.JobSpec{
				Job:  "job",
				Type: testCase.jobType,
				Refs: &prowapi.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []prowapi.Pull{
						{
							Number: 1,
						},
					},
				},
				BuildID: "build",
			}

			tmpDir, err := ioutil.TempDir("", testCase.name)
			if err != nil {
				t.Errorf("%s: error creating temp dir: %v", testCase.name, err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Errorf("%s: error cleaning up temp dir: %v", testCase.name, err)
				}
			}()

			for _, testPath := range testCase.paths {
				if strings.HasSuffix(testPath, "/") {
					if err := os.Mkdir(path.Join(tmpDir, testPath), 0755); err != nil {
						t.Errorf("%s: could not create test directory: %v", testCase.name, err)
					}
				} else if _, err := os.Create(path.Join(tmpDir, testPath)); err != nil {
					t.Errorf("%s: could not create test file: %v", testCase.name, err)
				}
			}

			// no way to configure this at compile-time since tmpdir is dynamic
			for i := range testCase.options.Items {
				testCase.options.Items[i] = path.Join(tmpDir, testCase.options.Items[i])
			}

			var uploadPaths []string
			targets, err := testCase.options.assembleTargets(spec, testCase.extra)
			if (err != nil) != testCase.wantErr {
				t.Errorf("assembleTargets() error = %v, wantErr %v", err, testCase.wantErr)
			}
			for uploadPath := range targets {
				uploadPaths = append(uploadPaths, uploadPath)
			}
			sort.Strings(uploadPaths)
			sort.Strings(testCase.expected)
			if actual, expected := uploadPaths, testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: did not assemble targets correctly:\n%s\n", testCase.name, diff.ObjectReflectDiff(expected, actual))
			}

		})
	}
}

func TestBuilderForStrategy(t *testing.T) {
	type info struct {
		org, repo string
	}
	var testCases = []struct {
		name          string
		strategy      string
		defaultOrg    string
		defaultRepo   string
		expectedPaths map[info]string
	}{
		{
			name:     "explicit",
			strategy: prowapi.PathStrategyExplicit,
			expectedPaths: map[info]string{
				{org: "org", repo: "repo"}: "org_repo",
			},
		},
		{
			name:        "single",
			strategy:    prowapi.PathStrategySingle,
			defaultOrg:  "org",
			defaultRepo: "repo",
			expectedPaths: map[info]string{
				{org: "org", repo: "repo"}:  "",
				{org: "org", repo: "repo2"}: "org_repo2",
				{org: "org2", repo: "repo"}: "org2_repo",
			},
		},
		{
			name:        "explicit",
			strategy:    prowapi.PathStrategyLegacy,
			defaultOrg:  "org",
			defaultRepo: "repo",
			expectedPaths: map[info]string{
				{org: "org", repo: "repo"}:  "",
				{org: "org", repo: "repo2"}: "repo2",
				{org: "org2", repo: "repo"}: "org2_repo",
			},
		},
	}

	for _, testCase := range testCases {
		builder := builderForStrategy(testCase.strategy, testCase.defaultOrg, testCase.defaultRepo)
		for sampleInfo, expectedPath := range testCase.expectedPaths {
			if actual, expected := builder(sampleInfo.org, sampleInfo.repo), expectedPath; actual != expected {
				t.Errorf("%s: expected (%s,%s) -> %s, got %s", testCase.name, sampleInfo.org, sampleInfo.repo, expected, actual)
			}
		}
	}
}

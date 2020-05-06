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

package main

import (
	"encoding/xml"
	"sort"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	"github.com/GoogleCloudPlatform/testgrid/resultstore"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
)

func TestProwJobName(t *testing.T) {
	cases := []struct {
		name            string
		url             string
		expectedJobName string
	}{
		{
			name:            "wrong url with only bucket name returns empty job name",
			url:             "gs://bucket",
			expectedJobName: "",
		},
		{
			name:            "wrong url path with no job name returns empty job name",
			url:             "gs://bucket/logs",
			expectedJobName: "",
		},
		{
			name:            "wrong url path with no job name and trailing slash returns empty job name",
			url:             "gs://bucket/logs/",
			expectedJobName: "",
		},
		{
			name:            "good job name",
			url:             "gs://bucket/logs/jobA/1234567890123456789",
			expectedJobName: "jobA",
		},
		{
			name:            "good job name with trailing slash",
			url:             "gs://bucket/logs/jobA/1234567890123456789/",
			expectedJobName: "jobA",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			urlPath, err := gcs.NewPath(tc.url)
			if err != nil {
				t.Errorf("incorrect url: %v", err)
			}

			job := prowJobName(*urlPath)
			if job != tc.expectedJobName {
				t.Errorf("incorrect job name: got %q, expected %q", job, tc.expectedJobName)
			}
		})
	}
}

func TestStartedReposToProperties(t *testing.T) {
	cases := []struct {
		name     string
		repos    map[string]string
		expected []resultstore.Property
	}{
		{
			name: "empty repos convert to no properties",
		},
		{
			name: "convert repos to properties",
			repos: map[string]string{
				"org1/repo1": "master",
				"org2/repo2": "branch1",
			},
			expected: []resultstore.Property{
				{Key: "Branch", Value: "branch1"},
				{Key: "Branch", Value: "master"},
				{Key: "Org", Value: "org1"},
				{Key: "Org", Value: "org2"},
				{Key: "Repo", Value: "org1/repo1"},
				{Key: "Repo", Value: "org1/repo1:master"},
				{Key: "Repo", Value: "org2/repo2"},
				{Key: "Repo", Value: "org2/repo2:branch1"},
				{Key: "Repo", Value: "repo1"},
				{Key: "Repo", Value: "repo2"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			properties := startedReposToProperties(tc.repos)
			sort.Slice(properties, func(i, j int) bool {
				return properties[i].Key+properties[i].Value < properties[j].Key+properties[j].Value
			})
			if !equality.Semantic.DeepEqual(properties, tc.expected) {
				t.Errorf(diff.ObjectReflectDiff(properties, tc.expected))
			}
		})
	}
}

func TestConvertProjectMetadataToResultStoreArtifacts(t *testing.T) {
	cases := []struct {
		name               string
		project            string
		details            string
		url                string
		result             downloadResult
		maxFiles           int
		expectedInvocation resultstore.Invocation
		expectedTarget     resultstore.Target
		expectedTest       resultstore.Test
	}{
		{
			name: "Convert empty project metadata",
			url:  "gs://bucket/logs/jobA/1234567890123456789",
			expectedInvocation: resultstore.Invocation{
				Files: []resultstore.File{
					{
						ID:          resultstore.InvocationLog,
						ContentType: "text/plain",
						URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
					},
				},
				Properties: []resultstore.Property{
					{Key: "Job", Value: "jobA"},
					{Key: "Pull", Value: ""},
				},
				Status:      resultstore.Running,
				Description: "In progress...",
			},
			expectedTarget: resultstore.Target{
				Status:      resultstore.Running,
				Description: "In progress...",
				Properties:  []resultstore.Property{},
			},
			expectedTest: resultstore.Test{
				Suite: resultstore.Suite{
					Name: "test",
					Files: []resultstore.File{
						{
							ID:          resultstore.TargetLog,
							ContentType: "text/plain",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
						},
					},
				},
				Action: resultstore.Action{
					Status:      resultstore.Running,
					Description: "In progress...",
				},
			},
		},
		{
			name:    "Convert multiple junit files",
			project: "projectX",
			details: "detailY",
			url:     "gs://bucket/logs/jobA/1234567890123456789",
			result: downloadResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 1234567890,
						Repos: map[string]string{
							"org/repoA": "branchB",
						},
						DeprecatedRepoVersion: "aadb2b88d190a38b59f512b4d8c508a88cf839e1",
					},
					Pending: false,
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Result:             "SUCCESS",
						DeprecatedRevision: "master",
					},
					Running: false,
				},
				artifactURLs: []string{
					"logs/jobA/1234567890123456789/artifacts/bar/junit_runner.xml",
					"logs/jobA/1234567890123456789/artifacts/foo/junit_runner.xml",
					"logs/jobA/1234567890123456789/build-log.txt",
				},
				suiteMetas: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							XMLName: xml.Name{},
							Suites: []junit.Suite{
								{
									XMLName:  xml.Name{Space: "testsuite", Local: ""},
									Time:     10.5,
									Failures: 0,
									Tests:    2,
									Results: []junit.Result{
										{
											Name:      "Result1",
											Time:      3.2,
											ClassName: "test1",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "p1", Value: "v1"},
												},
											},
										},
										{
											Name:      "Result2",
											Time:      7.3,
											ClassName: "test2",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "p2", Value: "v2"},
												},
											},
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"Context": "runner",
						},
						Path: "gs://bucket/logs/jobA/1234567890123456789/artifacts/bar/junit_runner.xml",
					},
					{
						Suites: junit.Suites{
							XMLName: xml.Name{},
							Suites: []junit.Suite{
								{
									XMLName:  xml.Name{Space: "testsuite", Local: ""},
									Time:     10.5,
									Failures: 0,
									Tests:    2,
									Results: []junit.Result{
										{
											Name:      "bar1",
											Time:      3.2,
											ClassName: "test1",
										},
										{
											Name:      "bar2",
											Time:      7.3,
											ClassName: "test2",
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"Context": "runner",
						},
						Path: "gs://bucket/logs/jobA/1234567890123456789/artifacts/foo/junit_runner.xml",
					},
				},
			},
			expectedInvocation: resultstore.Invocation{
				Project: "projectX",
				Details: "detailY",
				Files: []resultstore.File{
					{
						ID:          resultstore.InvocationLog,
						ContentType: "text/plain",
						URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
					},
				},
				Properties: []resultstore.Property{
					{Key: "Job", Value: "jobA"},
					{Key: "Pull", Value: ""},
					{Key: "Org", Value: "org"},
					{Key: "Branch", Value: "branchB"},
					{Key: "Repo", Value: "repoA"},
					{Key: "Repo", Value: "org/repoA"},
					{Key: "Repo", Value: "org/repoA:branchB"},
				},
				Start:       time.Unix(1234567890, 0),
				Status:      resultstore.Running,
				Description: "In progress...",
			},
			expectedTarget: resultstore.Target{
				Status:      resultstore.Running,
				Description: "In progress...",
				Start:       time.Unix(1234567890, 0),
				Properties: []resultstore.Property{
					{Key: "Result1:p1", Value: "v1"},
					{Key: "Result2:p2", Value: "v2"},
				},
			},
			expectedTest: resultstore.Test{
				Suite: resultstore.Suite{
					Name:  "test",
					Start: time.Unix(1234567890, 0),
					Files: []resultstore.File{
						{
							ID:          resultstore.TargetLog,
							ContentType: "text/plain",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
						},
						{
							ID:          "artifacts/bar/junit_runner.xml",
							ContentType: "text/xml",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/bar/junit_runner.xml",
						},
						{
							ID:          "artifacts/foo/junit_runner.xml",
							ContentType: "text/xml",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/foo/junit_runner.xml",
						},
					},
					Suites: []resultstore.Suite{
						{
							Name:     "junit_runner.xml",
							Duration: dur(10.5),
							Files: []resultstore.File{
								{
									ID:          "junit_runner.xml",
									ContentType: "text/xml",
									URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/bar/junit_runner.xml",
								},
							},
							Suites: []resultstore.Suite{
								{
									Cases: []resultstore.Case{
										{
											Name:     "Result1",
											Class:    "test1",
											Result:   resultstore.Completed,
											Duration: dur(3.2),
										},
										{
											Name:     "Result2",
											Class:    "test2",
											Result:   resultstore.Completed,
											Duration: dur(7.3),
										},
									},
									Duration: dur(10.5),
									Properties: []resultstore.Property{
										{Key: "Result1:p1", Value: "v1"},
										{Key: "Result2:p2", Value: "v2"},
									},
								},
							},
						},
						{
							Name:     "junit_runner.xml",
							Duration: dur(10.5),
							Files: []resultstore.File{
								{
									ID:          "junit_runner.xml",
									ContentType: "text/xml",
									URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/foo/junit_runner.xml",
								},
							},
							Suites: []resultstore.Suite{
								{
									Cases: []resultstore.Case{
										{
											Name:     "bar1",
											Class:    "test1",
											Result:   resultstore.Completed,
											Duration: dur(3.2),
										},
										{
											Name:     "bar2",
											Class:    "test2",
											Result:   resultstore.Completed,
											Duration: dur(7.3),
										},
									},
									Duration:   dur(10.5),
									Properties: nil,
								},
							},
						},
					},
				},
				Action: resultstore.Action{
					Start:       time.Unix(1234567890, 0),
					Status:      resultstore.Running,
					Description: "In progress...",
				},
			},
		},
		{
			name:     "Reject excessive artifacts",
			maxFiles: 3,
			project:  "projectX",
			details:  "detailY",
			url:      "gs://bucket/logs/jobA/1234567890123456789",
			result: downloadResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 1234567890,
						Repos: map[string]string{
							"org/repoA": "branchB",
						},
						DeprecatedRepoVersion: "aadb2b88d190a38b59f512b4d8c508a88cf839e1",
					},
					Pending: false,
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Result:             "SUCCESS",
						DeprecatedRevision: "master",
					},
					Running: false,
				},
				artifactURLs: []string{
					"logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
					"logs/jobA/1234567890123456789/artifacts/1",
					"logs/jobA/1234567890123456789/artifacts/2",
					"logs/jobA/1234567890123456789/artifacts/3",
					"logs/jobA/1234567890123456789/artifacts/4",
					"logs/jobA/1234567890123456789/artifacts/5",
					"logs/jobA/1234567890123456789/build-log.txt",
				},
				suiteMetas: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							XMLName: xml.Name{},
							Suites: []junit.Suite{
								{
									XMLName:  xml.Name{Space: "testsuite", Local: ""},
									Time:     10.5,
									Failures: 0,
									Tests:    2,
									Results: []junit.Result{
										{
											Name:      "Result1",
											Time:      3.2,
											ClassName: "test1",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "p1", Value: "v1"},
												},
											},
										},
										{
											Name:      "Result2",
											Time:      7.3,
											ClassName: "test2",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "p2", Value: "v2"},
												},
											},
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"Context": "runner",
						},
						Path: "gs://bucket/logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
					},
				},
			},
			expectedInvocation: resultstore.Invocation{
				Project: "projectX",
				Details: "detailY",
				Files: []resultstore.File{
					{
						ID:          resultstore.InvocationLog,
						ContentType: "text/plain",
						URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
					},
					{
						ID:          "exceeded 3 files",
						ContentType: "text/plain",
						URL:         "gs://bucket/logs/jobA/1234567890123456789/",
					},
				},
				Properties: []resultstore.Property{
					{Key: "Job", Value: "jobA"},
					{Key: "Pull", Value: ""},
					{Key: "Org", Value: "org"},
					{Key: "Branch", Value: "branchB"},
					{Key: "Repo", Value: "repoA"},
					{Key: "Repo", Value: "org/repoA"},
					{Key: "Repo", Value: "org/repoA:branchB"},
				},
				Start:       time.Unix(1234567890, 0),
				Status:      resultstore.Running,
				Description: "In progress...",
			},
			expectedTarget: resultstore.Target{
				Status:      resultstore.Running,
				Description: "In progress...",
				Start:       time.Unix(1234567890, 0),
				Properties: []resultstore.Property{
					{Key: "Result1:p1", Value: "v1"},
					{Key: "Result2:p2", Value: "v2"},
				},
			},
			expectedTest: resultstore.Test{
				Suite: resultstore.Suite{
					Name:  "test",
					Start: time.Unix(1234567890, 0),
					Files: []resultstore.File{
						{
							ID:          resultstore.TargetLog,
							ContentType: "text/plain",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
						},
						{
							ID:          "artifacts/junit_runner.xml",
							ContentType: "text/xml",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
						},
						{
							ID:          "artifacts/1",
							ContentType: "text/plain",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/1",
						},
						{
							ID:          "artifacts/2",
							ContentType: "text/plain",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/2",
						},
						{
							ID:          "artifacts/3",
							ContentType: "text/plain",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/3",
						},
					},
					Suites: []resultstore.Suite{
						{
							Name:     "junit_runner.xml",
							Duration: dur(10.5),
							Files: []resultstore.File{
								{
									ID:          "junit_runner.xml",
									ContentType: "text/xml",
									URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
								},
							},
							Suites: []resultstore.Suite{
								{
									Cases: []resultstore.Case{
										{
											Name:     "Result1",
											Class:    "test1",
											Result:   resultstore.Completed,
											Duration: dur(3.2),
										},
										{
											Name:     "Result2",
											Class:    "test2",
											Result:   resultstore.Completed,
											Duration: dur(7.3),
										},
									},
									Duration: dur(10.5),
									Properties: []resultstore.Property{
										{Key: "Result1:p1", Value: "v1"},
										{Key: "Result2:p2", Value: "v2"},
									},
								},
							},
						},
					},
				},
				Action: resultstore.Action{
					Start:       time.Unix(1234567890, 0),
					Status:      resultstore.Running,
					Description: "In progress...",
				},
			},
		},
		{
			name:    "Convert full project metadata",
			project: "projectX",
			details: "detailY",
			url:     "gs://bucket/logs/jobA/1234567890123456789",
			result: downloadResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 1234567890,
						Repos: map[string]string{
							"org/repoA": "branchB",
						},
						DeprecatedRepoVersion: "aadb2b88d190a38b59f512b4d8c508a88cf839e1",
					},
					Pending: false,
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Result:             "SUCCESS",
						DeprecatedRevision: "master",
					},
					Running: false,
				},
				artifactURLs: []string{
					"logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
					"logs/jobA/1234567890123456789/build-log.txt",
				},
				suiteMetas: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							XMLName: xml.Name{},
							Suites: []junit.Suite{
								{
									XMLName:  xml.Name{Space: "testsuite", Local: ""},
									Time:     10.5,
									Failures: 0,
									Tests:    2,
									Results: []junit.Result{
										{
											Name:      "Result1",
											Time:      3.2,
											ClassName: "test1",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "p1", Value: "v1"},
												},
											},
										},
										{
											Name:      "Result2",
											Time:      7.3,
											ClassName: "test2",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "p2", Value: "v2"},
												},
											},
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"Context": "runner",
						},
						Path: "gs://bucket/logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
					},
				},
			},
			expectedInvocation: resultstore.Invocation{
				Project: "projectX",
				Details: "detailY",
				Files: []resultstore.File{
					{
						ID:          resultstore.InvocationLog,
						ContentType: "text/plain",
						URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
					},
				},
				Properties: []resultstore.Property{
					{Key: "Job", Value: "jobA"},
					{Key: "Pull", Value: ""},
					{Key: "Org", Value: "org"},
					{Key: "Branch", Value: "branchB"},
					{Key: "Repo", Value: "repoA"},
					{Key: "Repo", Value: "org/repoA"},
					{Key: "Repo", Value: "org/repoA:branchB"},
				},
				Start:       time.Unix(1234567890, 0),
				Status:      resultstore.Running,
				Description: "In progress...",
			},
			expectedTarget: resultstore.Target{
				Status:      resultstore.Running,
				Description: "In progress...",
				Start:       time.Unix(1234567890, 0),
				Properties: []resultstore.Property{
					{Key: "Result1:p1", Value: "v1"},
					{Key: "Result2:p2", Value: "v2"},
				},
			},
			expectedTest: resultstore.Test{
				Suite: resultstore.Suite{
					Name:  "test",
					Start: time.Unix(1234567890, 0),
					Files: []resultstore.File{
						{
							ID:          resultstore.TargetLog,
							ContentType: "text/plain",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/build-log.txt",
						},
						{
							ID:          "artifacts/junit_runner.xml",
							ContentType: "text/xml",
							URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
						},
					},
					Suites: []resultstore.Suite{
						{
							Name:     "junit_runner.xml",
							Duration: dur(10.5),
							Files: []resultstore.File{
								{
									ID:          "junit_runner.xml",
									ContentType: "text/xml",
									URL:         "gs://bucket/logs/jobA/1234567890123456789/artifacts/junit_runner.xml",
								},
							},
							Suites: []resultstore.Suite{
								{
									Cases: []resultstore.Case{
										{
											Name:     "Result1",
											Class:    "test1",
											Result:   resultstore.Completed,
											Duration: dur(3.2),
										},
										{
											Name:     "Result2",
											Class:    "test2",
											Result:   resultstore.Completed,
											Duration: dur(7.3),
										},
									},
									Duration: dur(10.5),
									Properties: []resultstore.Property{
										{Key: "Result1:p1", Value: "v1"},
										{Key: "Result2:p2", Value: "v2"},
									},
								},
							},
						},
					},
				},
				Action: resultstore.Action{
					Start:       time.Unix(1234567890, 0),
					Status:      resultstore.Running,
					Description: "In progress...",
				},
			},
		},
	}

	for _, tc := range cases {
		cmpOption := cmp.AllowUnexported(resultstore.Invocation{})
		t.Run(tc.name, func(t *testing.T) {
			urlPath, err := gcs.NewPath(tc.url)
			if err != nil {
				t.Errorf("incorrect url: %v", err)
			}
			if tc.maxFiles == 0 {
				tc.maxFiles = 40000
			}
			invocation, target, test := convert(tc.project, tc.details, *urlPath, tc.result, tc.maxFiles)
			if diff := cmp.Diff(invocation, tc.expectedInvocation, cmpOption); diff != "" {
				t.Errorf("%s:%s mismatch (-got +want):\n%s", tc.name, "invocation", diff)
			}
			if diff := cmp.Diff(target, tc.expectedTarget, cmpOption); diff != "" {
				t.Errorf("%s:%s mismatch (-got +want):\n%s", tc.name, "target", diff)
			}
			if diff := cmp.Diff(test, tc.expectedTest, cmpOption); diff != "" {
				t.Errorf("%s:%s mismatch (-got +want):\n%s", tc.name, "test", diff)
			}
		})
	}
}

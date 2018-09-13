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

package reporter

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
)

const (
	testPubSubProjectName = "test-project"
	testPubSubTopicName   = "test-topic"
	testPubSubRunID       = "test-id"
)

type fgc struct {
	reportMessage string
	instance      string
}

func (f *fgc) SetReview(instance, id, revision, message string) error {
	if instance != f.instance {
		return fmt.Errorf("wrong instance: %s", instance)
	}
	f.reportMessage = message
	return nil
}

func TestReport(t *testing.T) {
	var testcases = []struct {
		name          string
		pj            *v1.ProwJob
		expectReport  bool
		expectError   bool
		reportMessage string
	}{
		{
			name: "unfinished pj",
			pj: &v1.ProwJob{
				Status: v1.ProwJobStatus{
					State: v1.PendingState,
				},
			},
		},
		{
			name: "finished non-gerrit pj",
			pj: &v1.ProwJob{
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "finished pj, missing gerrit id",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"gerrit-revision": "abc",
					},
					Annotations: map[string]string{
						"gerrit-instance": "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "finished pj, missing gerrit revision",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"gerrit-id":       "123-abc",
						"gerrit-instance": "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "finished pj, missing gerrit instance",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"gerrit-revision": "abc",
					},
					Annotations: map[string]string{
						"gerrit-id": "123-abc",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "finished pj, no spec",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"gerrit-revision": "abc",
					},
					Annotations: map[string]string{
						"gerrit-id":       "123-abc",
						"gerrit-instance": "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
			expectReport: true,
			expectError:  true,
		},
		{
			name: "finished pj",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"gerrit-revision": "abc",
					},
					Annotations: map[string]string{
						"gerrit-id":       "123-abc",
						"gerrit-instance": "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport:  true,
			reportMessage: "Job ci-foo finished with success\n Gubernator URL: guber/foo",
		},
		{
			name: "finished pj, slash in repo name",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"gerrit-revision": "abc",
					},
					Annotations: map[string]string{
						"gerrit-id":       "123-abc",
						"gerrit-instance": "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo/bar",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo/bar",
					},
					Job: "ci-foo-bar",
				},
			},
			expectReport:  true,
			reportMessage: "Job ci-foo-bar finished with success\n Gubernator URL: guber/foo/bar",
		},
	}

	for _, tc := range testcases {
		fgc := &fgc{instance: "gerrit"}
		reporter := &Client{gc: fgc}

		shouldReport := reporter.ShouldReport(tc.pj)
		if shouldReport != tc.expectReport {
			t.Errorf("test: %s: shouldReport: %v, expectReport: %v", tc.name, shouldReport, tc.expectReport)
		}

		if !shouldReport {
			continue
		}

		err := reporter.Report(tc.pj)
		if err == nil && tc.expectError {
			t.Errorf("test: %s: expect error but did not happen", tc.name)
		} else if err != nil && !tc.expectError {
			t.Errorf("test: %s: expect no error but got error %v", tc.name, err)
		}

		if err == nil {
			if fgc.reportMessage != tc.reportMessage {
				t.Errorf("test: %s: reported with : %s, expect: %s", tc.name, fgc.reportMessage, tc.reportMessage)
			}
		}
	}
}

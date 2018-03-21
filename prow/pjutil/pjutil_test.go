/*
Copyright 2017 The Kubernetes Authors.

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

package pjutil

import (
	"testing"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"

	"k8s.io/test-infra/prow/kube"
)

func TestProwJobToPod(t *testing.T) {
	tests := []struct {
		podName string
		buildID string
		labels  map[string]string
		pjSpec  kube.ProwJobSpec

		expected *v1.Pod
	}{
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: kube.ProwJobSpec{
				Type:  kube.PresubmitJob,
				Job:   "job-name",
				Agent: kube.KubernetesAgent,
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
				},
				PodSpec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "tester",
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},

			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod",
					Labels: map[string]string{
						kube.CreatedByProw:    "true",
						kube.ProwJobTypeLabel: "presubmit",
						"needstobe":           "inherited",
					},
					Annotations: map[string]string{
						kube.ProwJobAnnotation: "job-name",
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: "Never",
					Containers: []v1.Container{
						{
							Name:  "pod-0",
							Image: "tester",
							Env: []v1.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
								{Name: "BUILD_ID", Value: "blabla"},
								{Name: "BUILD_NUMBER", Value: "blabla"},
								{Name: "JOB_NAME", Value: "job-name"},
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","prowjobid":"pod","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}]}}`},
								{Name: "JOB_TYPE", Value: "presubmit"},
								{Name: "PROW_JOB_ID", Value: "pod"},
								{Name: "PULL_BASE_REF", Value: "base-ref"},
								{Name: "PULL_BASE_SHA", Value: "base-sha"},
								{Name: "PULL_NUMBER", Value: "1"},
								{Name: "PULL_PULL_SHA", Value: "pull-sha"},
								{Name: "PULL_REFS", Value: "base-ref:base-sha,1:pull-sha"},
								{Name: "REPO_NAME", Value: "repo-name"},
								{Name: "REPO_OWNER", Value: "org-name"},
							},
						},
					},
				},
			},
		},
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		pj := kube.ProwJob{ObjectMeta: metav1.ObjectMeta{Name: test.podName, Labels: test.labels}, Spec: test.pjSpec}
		got, err := ProwJobToPod(pj, test.buildID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !equality.Semantic.DeepEqual(got, test.expected) {
			t.Errorf("expected pod:\n%s", diff.ObjectReflectDiff(test.expected, got))
		}
	}
}

func TestPartitionActive(t *testing.T) {
	tests := []struct {
		pjs []kube.ProwJob

		pending   map[string]struct{}
		triggered map[string]struct{}
	}{
		{
			pjs: []kube.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bar",
					},
					Status: kube.ProwJobStatus{
						State: kube.PendingState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "baz",
					},
					Status: kube.ProwJobStatus{
						State: kube.SuccessState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "error",
					},
					Status: kube.ProwJobStatus{
						State: kube.ErrorState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bak",
					},
					Status: kube.ProwJobStatus{
						State: kube.PendingState,
					},
				},
			},
			pending: map[string]struct{}{
				"bar": {}, "bak": {},
			},
			triggered: map[string]struct{}{
				"foo": {},
			},
		},
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		pendingCh, triggeredCh := PartitionActive(test.pjs)
		for job := range pendingCh {
			if _, ok := test.pending[job.ObjectMeta.Name]; !ok {
				t.Errorf("didn't find pending job %#v", job)
			}
		}
		for job := range triggeredCh {
			if _, ok := test.triggered[job.ObjectMeta.Name]; !ok {
				t.Errorf("didn't find triggered job %#v", job)
			}
		}
	}
}

func TestGetLatestProwJobs(t *testing.T) {
	tests := []struct {
		name string

		pjs     []kube.ProwJob
		jobType string

		expected map[string]struct{}
	}{
		{
			pjs: []kube.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "831c7df0-baa4-11e7-a1a4-0a58ac10134a",
					},
					Spec: kube.ProwJobSpec{
						Type:  kube.PresubmitJob,
						Agent: kube.JenkinsAgent,
						Job:   "test_pull_request_origin_extended_networking_minimal",
						Refs: kube.Refs{
							Org:     "openshift",
							Repo:    "origin",
							BaseRef: "master",
							BaseSHA: "e92d5c525795eafb82cf16e3ab151b567b47e333",
							Pulls: []kube.Pull{
								{
									Number: 17061,
									Author: "enj",
									SHA:    "f94a3a51f59a693642e39084f03efa83af9442d3",
								},
							},
						},
						Report:       true,
						Context:      "ci/openshift-jenkins/extended_networking_minimal",
						RerunCommand: "/test extended_networking_minimal",
					},
					Status: kube.ProwJobStatus{
						StartTime:   metav1.Date(2017, time.October, 26, 23, 22, 19, 0, time.UTC),
						State:       kube.FailureState,
						Description: "Jenkins job failed.",
						URL:         "https://openshift-gce-devel.appspot.com/build/origin-ci-test/pr-logs/pull/17061/test_pull_request_origin_extended_networking_minimal/9756/",
						PodName:     "test_pull_request_origin_extended_networking_minimal-9756",
						BuildID:     "9756",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "0079d4d3-ba25-11e7-ae3f-0a58ac10123b",
					},
					Spec: kube.ProwJobSpec{
						Type:  kube.PresubmitJob,
						Agent: kube.JenkinsAgent,
						Job:   "test_pull_request_origin_extended_networking_minimal",
						Refs: kube.Refs{
							Org:     "openshift",
							Repo:    "origin",
							BaseRef: "master",
							BaseSHA: "e92d5c525795eafb82cf16e3ab151b567b47e333",
							Pulls: []kube.Pull{
								{
									Number: 17061,
									Author: "enj",
									SHA:    "f94a3a51f59a693642e39084f03efa83af9442d3",
								},
							},
						},
						Report:       true,
						Context:      "ci/openshift-jenkins/extended_networking_minimal",
						RerunCommand: "/test extended_networking_minimal",
					},
					Status: kube.ProwJobStatus{
						StartTime:   metav1.Date(2017, time.October, 26, 22, 22, 19, 0, time.UTC),
						State:       kube.FailureState,
						Description: "Jenkins job failed.",
						URL:         "https://openshift-gce-devel.appspot.com/build/origin-ci-test/pr-logs/pull/17061/test_pull_request_origin_extended_networking_minimal/9755/",
						PodName:     "test_pull_request_origin_extended_networking_minimal-9755",
						BuildID:     "9755",
					},
				},
			},
			jobType:  "presubmit",
			expected: map[string]struct{}{"831c7df0-baa4-11e7-a1a4-0a58ac10134a": {}},
		},
	}

	for _, test := range tests {
		got := GetLatestProwJobs(test.pjs, kube.ProwJobType(test.jobType))
		if len(got) != len(test.expected) {
			t.Errorf("expected jobs:\n%+v\ngot jobs:\n%+v", test.expected, got)
			continue
		}
		for name := range test.expected {
			if _, ok := got[name]; ok {
				t.Errorf("expected job: %s", name)
			}
		}
	}
}

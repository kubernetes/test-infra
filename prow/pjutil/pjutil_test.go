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
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

func TestPostsubmitSpec(t *testing.T) {
	tests := []struct {
		name     string
		p        config.Postsubmit
		refs     kube.Refs
		expected kube.ProwJobSpec
	}{
		{
			name: "can override path alias and cloneuri",
			p: config.Postsubmit{
				JobBase: config.JobBase{
					UtilityConfig: config.UtilityConfig{
						PathAlias: "foo",
						CloneURI:  "bar",
					},
				},
			},
			expected: kube.ProwJobSpec{
				Type: kube.PostsubmitJob,
				Refs: &kube.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
			},
		},
		{
			name: "controller can default path alias and cloneuri",
			refs: kube.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: kube.ProwJobSpec{
				Type: kube.PostsubmitJob,
				Refs: &kube.Refs{
					PathAlias: "fancy",
					CloneURI:  "cats",
				},
			},
		},
		{
			name: "job overrides take precedence over controller defaults",
			p: config.Postsubmit{
				JobBase: config.JobBase{
					UtilityConfig: config.UtilityConfig{
						PathAlias: "foo",
						CloneURI:  "bar",
					},
				},
			},
			refs: kube.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: kube.ProwJobSpec{
				Type: kube.PostsubmitJob,
				Refs: &kube.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
			},
		},
	}

	for _, tc := range tests {
		actual := PostsubmitSpec(tc.p, tc.refs)
		if expected := tc.expected; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: actual %#v != expected %#v", tc.name, actual, expected)
		}
	}
}

func TestPresubmitSpec(t *testing.T) {
	tests := []struct {
		name     string
		p        config.Presubmit
		refs     kube.Refs
		expected kube.ProwJobSpec
	}{
		{
			name: "can override path alias and cloneuri",
			p: config.Presubmit{
				JobBase: config.JobBase{
					UtilityConfig: config.UtilityConfig{
						PathAlias: "foo",
						CloneURI:  "bar",
					},
				},
			},
			expected: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Refs: &kube.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
				Report: true,
			},
		},
		{
			name: "controller can default path alias and cloneuri",
			refs: kube.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Refs: &kube.Refs{
					PathAlias: "fancy",
					CloneURI:  "cats",
				},
				Report: true,
			},
		},
		{
			name: "job overrides take precedence over controller defaults",
			p: config.Presubmit{
				JobBase: config.JobBase{
					UtilityConfig: config.UtilityConfig{
						PathAlias: "foo",
						CloneURI:  "bar",
					},
				},
			},
			refs: kube.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Refs: &kube.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
				Report: true,
			},
		},
	}

	for _, tc := range tests {
		actual := PresubmitSpec(tc.p, tc.refs)
		if expected := tc.expected; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: actual %#v != expected %#v", tc.name, actual, expected)
		}
	}
}

func TestBatchSpec(t *testing.T) {
	tests := []struct {
		name     string
		p        config.Presubmit
		refs     kube.Refs
		expected kube.ProwJobSpec
	}{
		{
			name: "can override path alias and cloneuri",
			p: config.Presubmit{
				JobBase: config.JobBase{
					UtilityConfig: config.UtilityConfig{
						PathAlias: "foo",
						CloneURI:  "bar",
					},
				},
			},
			expected: kube.ProwJobSpec{
				Type: kube.BatchJob,
				Refs: &kube.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
			},
		},
		{
			name: "controller can default path alias and cloneuri",
			refs: kube.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: kube.ProwJobSpec{
				Type: kube.BatchJob,
				Refs: &kube.Refs{
					PathAlias: "fancy",
					CloneURI:  "cats",
				},
			},
		},
		{
			name: "job overrides take precedence over controller defaults",
			p: config.Presubmit{
				JobBase: config.JobBase{
					UtilityConfig: config.UtilityConfig{
						PathAlias: "foo",
						CloneURI:  "bar",
					},
				},
			},
			refs: kube.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: kube.ProwJobSpec{
				Type: kube.BatchJob,
				Refs: &kube.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
			},
		},
	}

	for _, tc := range tests {
		actual := BatchSpec(tc.p, tc.refs)
		if expected := tc.expected; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: actual %#v != expected %#v", tc.name, actual, expected)
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
						Refs: &kube.Refs{
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
						Refs: &kube.Refs{
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

func TestNewProwJob(t *testing.T) {
	var testCases = []struct {
		name           string
		spec           kube.ProwJobSpec
		labels         map[string]string
		expectedLabels map[string]string
	}{
		{
			name: "periodic job, no extra labels",
			spec: kube.ProwJobSpec{
				Job:  "job",
				Type: kube.PeriodicJob,
			},
			labels: map[string]string{},
			expectedLabels: map[string]string{
				kube.CreatedByProw:     "true",
				kube.ProwJobAnnotation: "job",
				kube.ProwJobTypeLabel:  "periodic",
			},
		},
		{
			name: "periodic job, extra labels",
			spec: kube.ProwJobSpec{
				Job:  "job",
				Type: kube.PeriodicJob,
			},
			labels: map[string]string{
				"extra": "stuff",
			},
			expectedLabels: map[string]string{
				kube.CreatedByProw:     "true",
				kube.ProwJobAnnotation: "job",
				kube.ProwJobTypeLabel:  "periodic",
				"extra":                "stuff",
			},
		},
		{
			name: "presubmit job",
			spec: kube.ProwJobSpec{
				Job:  "job",
				Type: kube.PresubmitJob,
				Refs: &kube.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []kube.Pull{
						{Number: 1},
					},
				},
			},
			labels: map[string]string{},
			expectedLabels: map[string]string{
				kube.CreatedByProw:     "true",
				kube.ProwJobAnnotation: "job",
				kube.ProwJobTypeLabel:  "presubmit",
				kube.OrgLabel:          "org",
				kube.RepoLabel:         "repo",
				kube.PullLabel:         "1",
			},
		},
		{
			name: "non-github presubmit job",
			spec: kube.ProwJobSpec{
				Job:  "job",
				Type: kube.PresubmitJob,
				Refs: &kube.Refs{
					Org:  "https://some-gerrit-instance.foo.com",
					Repo: "some/invalid/repo",
					Pulls: []kube.Pull{
						{Number: 1},
					},
				},
			},
			labels: map[string]string{},
			expectedLabels: map[string]string{
				kube.CreatedByProw:     "true",
				kube.ProwJobAnnotation: "job",
				kube.ProwJobTypeLabel:  "presubmit",
				kube.OrgLabel:          "some-gerrit-instance.foo.com",
				kube.RepoLabel:         "repo",
				kube.PullLabel:         "1",
			},
		}, {
			name: "job with name too long to fit in a label",
			spec: kube.ProwJobSpec{
				Job:  "job-created-by-someone-who-loves-very-very-very-long-names-so-long-that-it-does-not-fit-into-the-Kubernetes-label-so-it-needs-to-be-truncated-to-63-characters",
				Type: kube.PresubmitJob,
				Refs: &kube.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []kube.Pull{
						{Number: 1},
					},
				},
			},
			labels: map[string]string{},
			expectedLabels: map[string]string{
				kube.CreatedByProw:     "true",
				kube.ProwJobAnnotation: "job-created-by-someone-who-loves-very-very-very-long-names-so-l",
				kube.ProwJobTypeLabel:  "presubmit",
				kube.OrgLabel:          "org",
				kube.RepoLabel:         "repo",
				kube.PullLabel:         "1",
			},
		},
	}

	for _, testCase := range testCases {
		pj := NewProwJob(testCase.spec, testCase.labels)
		if actual, expected := pj.Spec, testCase.spec; !equality.Semantic.DeepEqual(actual, expected) {
			t.Errorf("%s: incorrect ProwJobSpec created: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}
		if actual, expected := pj.Labels, testCase.expectedLabels; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: incorrect ProwJob labels created: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}
	}
}

func TestNewProwJobWithAnnotations(t *testing.T) {
	var testCases = []struct {
		name                string
		spec                kube.ProwJobSpec
		annotations         map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name: "job without annotation",
			spec: kube.ProwJobSpec{
				Job:  "job",
				Type: kube.PeriodicJob,
			},
			annotations: nil,
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
			},
		},
		{
			name: "job with annotation",
			spec: kube.ProwJobSpec{
				Job:  "job",
				Type: kube.PeriodicJob,
			},
			annotations: map[string]string{
				"annotation": "foo",
			},
			expectedAnnotations: map[string]string{
				"annotation":           "foo",
				kube.ProwJobAnnotation: "job",
			},
		},
	}

	for _, testCase := range testCases {
		pj := NewProwJobWithAnnotation(testCase.spec, nil, testCase.annotations)
		if actual, expected := pj.Spec, testCase.spec; !equality.Semantic.DeepEqual(actual, expected) {
			t.Errorf("%s: incorrect ProwJobSpec created: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}
		if actual, expected := pj.Annotations, testCase.expectedAnnotations; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: incorrect ProwJob labels created: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}
	}
}

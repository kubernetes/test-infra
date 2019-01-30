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
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

func TestPostsubmitSpec(t *testing.T) {
	tests := []struct {
		name     string
		p        config.Postsubmit
		refs     prowapi.Refs
		expected prowapi.ProwJobSpec
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
			expected: prowapi.ProwJobSpec{
				Type: prowapi.PostsubmitJob,
				Refs: &prowapi.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
			},
		},
		{
			name: "controller can default path alias and cloneuri",
			refs: prowapi.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: prowapi.ProwJobSpec{
				Type: prowapi.PostsubmitJob,
				Refs: &prowapi.Refs{
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
			refs: prowapi.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: prowapi.ProwJobSpec{
				Type: prowapi.PostsubmitJob,
				Refs: &prowapi.Refs{
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
		refs     prowapi.Refs
		expected prowapi.ProwJobSpec
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
			expected: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
				Report: true,
			},
		},
		{
			name: "controller can default path alias and cloneuri",
			refs: prowapi.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
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
			refs: prowapi.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
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
		refs     prowapi.Refs
		expected prowapi.ProwJobSpec
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
			expected: prowapi.ProwJobSpec{
				Type: prowapi.BatchJob,
				Refs: &prowapi.Refs{
					PathAlias: "foo",
					CloneURI:  "bar",
				},
			},
		},
		{
			name: "controller can default path alias and cloneuri",
			refs: prowapi.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: prowapi.ProwJobSpec{
				Type: prowapi.BatchJob,
				Refs: &prowapi.Refs{
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
			refs: prowapi.Refs{
				PathAlias: "fancy",
				CloneURI:  "cats",
			},
			expected: prowapi.ProwJobSpec{
				Type: prowapi.BatchJob,
				Refs: &prowapi.Refs{
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
		pjs []prowapi.ProwJob

		pending   map[string]struct{}
		triggered map[string]struct{}
	}{
		{
			pjs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bar",
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "baz",
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.SuccessState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "error",
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.ErrorState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bak",
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
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

		pjs     []prowapi.ProwJob
		jobType string

		expected map[string]struct{}
	}{
		{
			pjs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "831c7df0-baa4-11e7-a1a4-0a58ac10134a",
					},
					Spec: prowapi.ProwJobSpec{
						Type:  prowapi.PresubmitJob,
						Agent: prowapi.JenkinsAgent,
						Job:   "test_pull_request_origin_extended_networking_minimal",
						Refs: &prowapi.Refs{
							Org:     "openshift",
							Repo:    "origin",
							BaseRef: "master",
							BaseSHA: "e92d5c525795eafb82cf16e3ab151b567b47e333",
							Pulls: []prowapi.Pull{
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
					Status: prowapi.ProwJobStatus{
						StartTime:   metav1.Date(2017, time.October, 26, 23, 22, 19, 0, time.UTC),
						State:       prowapi.FailureState,
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
					Spec: prowapi.ProwJobSpec{
						Type:  prowapi.PresubmitJob,
						Agent: prowapi.JenkinsAgent,
						Job:   "test_pull_request_origin_extended_networking_minimal",
						Refs: &prowapi.Refs{
							Org:     "openshift",
							Repo:    "origin",
							BaseRef: "master",
							BaseSHA: "e92d5c525795eafb82cf16e3ab151b567b47e333",
							Pulls: []prowapi.Pull{
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
					Status: prowapi.ProwJobStatus{
						StartTime:   metav1.Date(2017, time.October, 26, 22, 22, 19, 0, time.UTC),
						State:       prowapi.FailureState,
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
		got := GetLatestProwJobs(test.pjs, prowapi.ProwJobType(test.jobType))
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
		spec           prowapi.ProwJobSpec
		labels         map[string]string
		expectedLabels map[string]string
	}{
		{
			name: "periodic job, no extra labels",
			spec: prowapi.ProwJobSpec{
				Job:  "job",
				Type: prowapi.PeriodicJob,
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
			spec: prowapi.ProwJobSpec{
				Job:  "job",
				Type: prowapi.PeriodicJob,
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
			spec: prowapi.ProwJobSpec{
				Job:  "job",
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []prowapi.Pull{
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
			spec: prowapi.ProwJobSpec{
				Job:  "job",
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:  "https://some-gerrit-instance.foo.com",
					Repo: "some/invalid/repo",
					Pulls: []prowapi.Pull{
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
			spec: prowapi.ProwJobSpec{
				Job:  "job-created-by-someone-who-loves-very-very-very-long-names-so-long-that-it-does-not-fit-into-the-Kubernetes-label-so-it-needs-to-be-truncated-to-63-characters",
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []prowapi.Pull{
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
		spec                prowapi.ProwJobSpec
		annotations         map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name: "job without annotation",
			spec: prowapi.ProwJobSpec{
				Job:  "job",
				Type: prowapi.PeriodicJob,
			},
			annotations: nil,
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
			},
		},
		{
			name: "job with annotation",
			spec: prowapi.ProwJobSpec{
				Job:  "job",
				Type: prowapi.PeriodicJob,
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

func TestJobURL(t *testing.T) {
	var testCases = []struct {
		name     string
		plank    config.Plank
		pj       prowapi.ProwJob
		expected string
	}{
		{
			name: "non-decorated job uses template",
			plank: config.Plank{
				Controller: config.Controller{
					JobURLTemplate: template.Must(template.New("test").Parse("{{.Spec.Type}}")),
				},
			},
			pj:       prowapi.ProwJob{Spec: prowapi.ProwJobSpec{Type: prowapi.PeriodicJob}},
			expected: "periodic",
		},
		{
			name: "non-decorated job with broken template gives empty string",
			plank: config.Plank{
				Controller: config.Controller{
					JobURLTemplate: template.Must(template.New("test").Parse("{{.Garbage}}")),
				},
			},
			pj:       prowapi.ProwJob{},
			expected: "",
		},
		{
			name: "decorated job without prefix uses template",
			plank: config.Plank{
				Controller: config.Controller{
					JobURLTemplate: template.Must(template.New("test").Parse("{{.Spec.Type}}")),
				},
			},
			pj:       prowapi.ProwJob{Spec: prowapi.ProwJobSpec{Type: prowapi.PeriodicJob}},
			expected: "periodic",
		},
		{
			name: "decorated job with prefix uses gcslib",
			plank: config.Plank{
				JobURLPrefix: "https://gubernator.com/build",
			},
			pj: prowapi.ProwJob{Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:   "org",
					Repo:  "repo",
					Pulls: []prowapi.Pull{{Number: 1}},
				},
				DecorationConfig: &prowapi.DecorationConfig{GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "bucket",
					PathStrategy: prowapi.PathStrategyExplicit,
				}},
			}},
			expected: "https://gubernator.com/build/bucket/pr-logs/pull/org_repo/1",
		},
	}

	logger := logrus.New()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := JobURL(testCase.plank, testCase.pj, logger.WithField("name", testCase.name)), testCase.expected; actual != expected {
				t.Errorf("%s: expected URL to be %q but got %q", testCase.name, expected, actual)
			}
		})
	}
}

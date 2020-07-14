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
	"errors"
	"fmt"
	"reflect"
	"testing"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
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
				Type: prowapi.PostsubmitJob,
				Refs: &prowapi.Refs{
					PathAlias: "fancy",
					CloneURI:  "cats",
				},
				Report: true,
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
				Report: true,
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

		pending   sets.String
		triggered sets.String
		aborted   sets.String
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aborted",
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.AbortedState,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aborted-and-completed",
					},
					Status: prowapi.ProwJobStatus{
						State:          prowapi.AbortedState,
						CompletionTime: &[]metav1.Time{metav1.Now()}[0],
					},
				},
			},
			pending:   sets.NewString("bar", "bak"),
			triggered: sets.NewString("foo"),
			aborted:   sets.NewString("aborted"),
		},
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		pendingCh, triggeredCh, abortedCh := PartitionActive(test.pjs)
		for job := range pendingCh {
			if !test.pending.Has(job.Name) {
				t.Errorf("didn't find pending job %#v", job)
			}
		}
		for job := range triggeredCh {
			if !test.triggered.Has(job.Name) {
				t.Errorf("didn't find triggered job %#v", job)
			}
		}
		for job := range abortedCh {
			if !test.aborted.Has(job.Name) {
				t.Errorf("didn't find aborted job %#v", job)
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
		name                string
		spec                prowapi.ProwJobSpec
		labels              map[string]string
		expectedLabels      map[string]string
		annotations         map[string]string
		expectedAnnotations map[string]string
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
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
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
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
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
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
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
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
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
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job-created-by-someone-who-loves-very-very-very-long-names-so-long-that-it-does-not-fit-into-the-Kubernetes-label-so-it-needs-to-be-truncated-to-63-characters",
			},
		},
		{
			name: "periodic job, extra labels, extra annotations",
			spec: prowapi.ProwJobSpec{
				Job:  "job",
				Type: prowapi.PeriodicJob,
			},
			labels: map[string]string{
				"extra": "stuff",
			},
			annotations: map[string]string{
				"extraannotation": "foo",
			},
			expectedLabels: map[string]string{
				kube.CreatedByProw:     "true",
				kube.ProwJobAnnotation: "job",
				kube.ProwJobTypeLabel:  "periodic",
				"extra":                "stuff",
			},
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
				"extraannotation":      "foo",
			},
		},
		{
			name: "job with podspec",
			spec: prowapi.ProwJobSpec{
				Job:     "job",
				Type:    prowapi.PeriodicJob,
				PodSpec: &corev1.PodSpec{}, // Needed to catch race
			},
			expectedLabels: map[string]string{
				kube.CreatedByProw:     "true",
				kube.ProwJobAnnotation: "job",
				kube.ProwJobTypeLabel:  "periodic",
			},
			expectedAnnotations: map[string]string{
				kube.ProwJobAnnotation: "job",
			},
		},
	}
	for _, testCase := range testCases {
		pj := NewProwJob(testCase.spec, testCase.labels, testCase.annotations)
		if actual, expected := pj.Spec, testCase.spec; !equality.Semantic.DeepEqual(actual, expected) {
			t.Errorf("%s: incorrect ProwJobSpec created: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}
		if actual, expected := pj.Labels, testCase.expectedLabels; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: incorrect ProwJob labels created: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}
		if actual, expected := pj.Annotations, testCase.expectedAnnotations; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: incorrect ProwJob annotations created: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}
		if pj.Spec.PodSpec != nil {
			futzWithPodSpec := func(spec *corev1.PodSpec, val string) {
				if spec == nil {
					return
				}
				if spec.NodeSelector == nil {
					spec.NodeSelector = map[string]string{}
				}
				spec.NodeSelector["foo"] = val
				for i := range spec.Containers {
					c := &spec.Containers[i]
					if c.Resources.Limits == nil {
						c.Resources.Limits = corev1.ResourceList{}
					}
					if c.Resources.Requests == nil {
						c.Resources.Requests = corev1.ResourceList{}
					}
					c.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(val)
					c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(val)
				}
			}
			go futzWithPodSpec(pj.Spec.PodSpec, "12M")
			futzWithPodSpec(testCase.spec.PodSpec, "34M")
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
		pj := NewProwJob(testCase.spec, nil, testCase.annotations)
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
		name        string
		plank       config.Plank
		pj          prowapi.ProwJob
		expected    string
		expectedErr string
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
			name: "decorated job with prefix uses gcsupload",
			plank: config.Plank{
				JobURLPrefixConfig:                       map[string]string{"*": "https://gubernator.com/build"},
				JobURLPrefixDisableAppendStorageProvider: true,
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
		{
			name: "decorated job with prefix uses gcsupload and new bucket format with gcs (deprecated job url format)",
			plank: config.Plank{
				JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/gcs"},
			},
			pj: prowapi.ProwJob{Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:   "org",
					Repo:  "repo",
					Pulls: []prowapi.Pull{{Number: 1}},
				},
				DecorationConfig: &prowapi.DecorationConfig{GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "gs://bucket",
					PathStrategy: prowapi.PathStrategyExplicit,
				}},
			}},
			expected: "https://prow.k8s.io/view/gs/bucket/pr-logs/pull/org_repo/1",
		},
		{
			name: "decorated job with prefix uses gcsupload and new bucket format with gcs (deprecated job url format with trailing slash)",
			plank: config.Plank{
				JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/gcs/"},
			},
			pj: prowapi.ProwJob{Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:   "org",
					Repo:  "repo",
					Pulls: []prowapi.Pull{{Number: 1}},
				},
				DecorationConfig: &prowapi.DecorationConfig{GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "gs://bucket",
					PathStrategy: prowapi.PathStrategyExplicit,
				}},
			}},
			expected: "https://prow.k8s.io/view/gs/bucket/pr-logs/pull/org_repo/1",
		},
		{
			name: "decorated job with prefix uses gcsupload and new bucket format with gcs (new job url format)",
			plank: config.Plank{
				JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/"},
			},
			pj: prowapi.ProwJob{Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:   "org",
					Repo:  "repo",
					Pulls: []prowapi.Pull{{Number: 1}},
				},
				DecorationConfig: &prowapi.DecorationConfig{GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "gs://bucket",
					PathStrategy: prowapi.PathStrategyExplicit,
				}},
			}},
			expected: "https://prow.k8s.io/view/gs/bucket/pr-logs/pull/org_repo/1",
		},
		{
			name: "decorated job with prefix uses gcsupload and new bucket format with s3",
			plank: config.Plank{
				JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/"},
			},
			pj: prowapi.ProwJob{Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:   "org",
					Repo:  "repo",
					Pulls: []prowapi.Pull{{Number: 1}},
				},
				DecorationConfig: &prowapi.DecorationConfig{GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "s3://bucket",
					PathStrategy: prowapi.PathStrategyExplicit,
				}},
			}},
			expected: "https://prow.k8s.io/view/s3/bucket/pr-logs/pull/org_repo/1",
		},
		{
			name: "decorated job with prefix uses gcsupload with valid bucket with multiple separators",
			plank: config.Plank{
				JobURLPrefixConfig: map[string]string{"*": "https://prow.k8s.io/view/"},
			},
			pj: prowapi.ProwJob{Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:   "org",
					Repo:  "repo",
					Pulls: []prowapi.Pull{{Number: 1}},
				},
				DecorationConfig: &prowapi.DecorationConfig{GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "gs://my-floppy-backup/a://doom2.wad.006",
					PathStrategy: prowapi.PathStrategyExplicit,
				}},
			}},
			expected: "https://prow.k8s.io/view/gs/my-floppy-backup/a:/doom2.wad.006/pr-logs/pull/org_repo/1",
		},
	}

	logger := logrus.New()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, actualErr := JobURL(testCase.plank, testCase.pj, logger.WithField("name", testCase.name))
			var actualErrStr string
			if actualErr != nil {
				actualErrStr = actualErr.Error()
			}

			if actualErrStr != testCase.expectedErr {
				t.Errorf("%s: expectedErr = %v, but got %v", testCase.name, testCase.expectedErr, actualErrStr)
			}
			if actual != testCase.expected {
				t.Errorf("%s: expected URL to be %q but got %q", testCase.name, testCase.expected, actual)
			}
		})
	}
}

func TestCreateRefs(t *testing.T) {
	pr := github.PullRequest{
		Number:  42,
		HTMLURL: "https://github.example.com/kubernetes/Hello-World/pull/42",
		Head: github.PullRequestBranch{
			SHA: "123456",
		},
		Base: github.PullRequestBranch{
			Ref: "master",
			Repo: github.Repo{
				Name:    "Hello-World",
				HTMLURL: "https://github.example.com/kubernetes/Hello-World",
				Owner: github.User{
					Login: "kubernetes",
				},
			},
		},
		User: github.User{
			Login:   "ibzib",
			HTMLURL: "https://github.example.com/ibzib",
		},
	}
	expected := prowapi.Refs{
		Org:      "kubernetes",
		Repo:     "Hello-World",
		RepoLink: "https://github.example.com/kubernetes/Hello-World",
		BaseRef:  "master",
		BaseSHA:  "abcdef",
		BaseLink: "https://github.example.com/kubernetes/Hello-World/commit/abcdef",
		Pulls: []prowapi.Pull{
			{
				Number:     42,
				Author:     "ibzib",
				SHA:        "123456",
				Link:       "https://github.example.com/kubernetes/Hello-World/pull/42",
				AuthorLink: "https://github.example.com/ibzib",
				CommitLink: "https://github.example.com/kubernetes/Hello-World/pull/42/commits/123456",
			},
		},
	}
	if actual := createRefs(pr, "abcdef"); !reflect.DeepEqual(expected, actual) {
		t.Errorf("diff between expected and actual refs:%s", diff.ObjectReflectDiff(expected, actual))
	}
}

func TestSpecFromJobBase(t *testing.T) {
	permittedGroups := []int{1234, 5678}
	permittedUsers := []string{"authorized_user", "another_authorized_user"}
	permittedOrgs := []string{"kubernetes", "kubernetes-sigs"}
	rerunAuthConfig := prowapi.RerunAuthConfig{
		AllowAnyone:   false,
		GitHubTeamIDs: permittedGroups,
		GitHubUsers:   permittedUsers,
		GitHubOrgs:    permittedOrgs,
	}
	testCases := []struct {
		name    string
		jobBase config.JobBase
		verify  func(prowapi.ProwJobSpec) error
	}{
		{
			name: "Verify reporter config gets copied",
			jobBase: config.JobBase{
				ReporterConfig: &prowapi.ReporterConfig{
					Slack: &prowapi.SlackReporterConfig{
						Channel: "my-channel",
					},
				},
			},
			verify: func(pj prowapi.ProwJobSpec) error {
				if pj.ReporterConfig == nil {
					return errors.New("Expected ReporterConfig to be non-nil")
				}
				if pj.ReporterConfig.Slack == nil {
					return errors.New("Expected ReporterConfig.Slack to be non-nil")
				}
				if pj.ReporterConfig.Slack.Channel != "my-channel" {
					return fmt.Errorf("Expected pj.ReporterConfig.Slack.Channel to be \"my-channel\", was %q",
						pj.ReporterConfig.Slack.Channel)
				}
				return nil
			},
		},
		{
			name: "Verify rerun permissions gets copied",
			jobBase: config.JobBase{
				RerunAuthConfig: &rerunAuthConfig,
			},
			verify: func(pj prowapi.ProwJobSpec) error {
				if pj.RerunAuthConfig.AllowAnyone {
					return errors.New("Expected RerunAuthConfig.AllowAnyone to be false")
				}
				if pj.RerunAuthConfig.GitHubTeamIDs == nil {
					return errors.New("Expected RerunAuthConfig.GitHubTeamIDs to be non-nil")
				}
				if pj.RerunAuthConfig.GitHubUsers == nil {
					return errors.New("Expected RerunAuthConfig.GitHubUsers to be non-nil")
				}
				if pj.RerunAuthConfig.GitHubOrgs == nil {
					return errors.New("Expected RerunAuthConfig.GitHubOrgs to be non-nil")
				}
				return nil
			},
		},
		{
			name: "Verify hidden property gets copied",
			jobBase: config.JobBase{
				Hidden: true,
			},
			verify: func(pj prowapi.ProwJobSpec) error {
				if !pj.Hidden {
					return errors.New("hidden property didnt get copied")
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pj := specFromJobBase(tc.jobBase)
			if err := tc.verify(pj); err != nil {
				t.Fatalf("Verification failed: %v", err)
			}
		})
	}
}

func TestPeriodicSpec(t *testing.T) {
	testCases := []struct {
		name   string
		config config.Periodic
		verify func(prowapi.ProwJobSpec) error
	}{
		{
			name:   "Report gets set to true",
			config: config.Periodic{},
			verify: func(p prowapi.ProwJobSpec) error {
				if !p.Report {
					return errors.New("report is not true")
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {
		if err := tc.verify(PeriodicSpec(tc.config)); err != nil {
			t.Error(err)
		}
	}
}

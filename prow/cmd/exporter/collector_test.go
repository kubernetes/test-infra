/*
Copyright 2019 The Kubernetes Authors.

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
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestKubeLabelsToPrometheusLabels(t *testing.T) {
	testcases := []struct {
		description         string
		labels              map[string]string
		expectedLabelKeys   []string
		expectedLabelValues []string
	}{
		{
			description:         "empty labels",
			labels:              map[string]string{},
			expectedLabelKeys:   []string{},
			expectedLabelValues: []string{},
		},
		{
			description: "labels with infra role",
			labels: map[string]string{
				"ci.openshift.io/role": "infra",
				"created-by-prow":      "true",
				"prow.k8s.io/build-id": "",
				"prow.k8s.io/id":       "35bca360-e085-11e9-8586-0a58ac104c36",
				"prow.k8s.io/job":      "periodic-prow-auto-config-brancher",
				"prow.k8s.io/type":     "periodic",
			},
			expectedLabelKeys: []string{
				"label_ci_openshift_io_role",
				"label_created_by_prow",
				"label_prow_k8s_io_build_id",
				"label_prow_k8s_io_id",
				"label_prow_k8s_io_job",
				"label_prow_k8s_io_type",
			},
			expectedLabelValues: []string{
				"infra",
				"true",
				"",
				"35bca360-e085-11e9-8586-0a58ac104c36",
				"periodic-prow-auto-config-brancher",
				"periodic",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.description, func(t *testing.T) {
			actualLabelKeys, actualLabelValues := kubeLabelsToPrometheusLabels(tc.labels, "label_")
			assertEqual(t, actualLabelKeys, tc.expectedLabelKeys)
			assertEqual(t, actualLabelValues, tc.expectedLabelValues)
		})
	}
}

func assertEqual(t *testing.T, actual, expected interface{}) {
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("actual differs from expected:\n%s", cmp.Diff(expected, actual))
	}
}

type fakeLister struct {
}

func (l fakeLister) List(selector labels.Selector) ([]*prowapi.ProwJob, error) {
	return []*prowapi.ProwJob{
		{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "pull-test-infra-bazel",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "7785d7a6-e601-11e9-8512-da8015665453",
				Labels: map[string]string{
					"created-by-prow":          "true",
					"event-GUID":               "770bab40-e601-11e9-8e50-08c45d902b6f",
					"preset-bazel-scratch-dir": "true",
					"preset-service-account":   "true",
					"prow.k8s.io/job":          "pull-test-infra-bazel",
					"prow.k8s.io/refs.org":     "kubernetes",
					"prow.k8s.io/refs.pull":    "14543",
					"prow.k8s.io/refs.repo":    "test-infra",
					"prow.k8s.io/type":         "presubmit",
				},
				Annotations: map[string]string{
					"prow.k8s.io/job":            "pull-test-infra-bazel",
					"testgrid-create-test-group": "true",
				},
			},
		},
		{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "branch-ci-openshift-release-master-config-updates",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "e44f91e5-e604-11e9-99c1-0a58ac10f9a6",
				Labels: map[string]string{
					"created-by-prow":       "true",
					"event-GUID":            "e4216820-e604-11e9-8cf0-295472589b4f",
					"prow.k8s.io/job":       "branch-ci-openshift-release-master-config-updates",
					"prow.k8s.io/refs.org":  "openshift",
					"prow.k8s.io/refs.repo": "release",
					"prow.k8s.io/type":      "postsubmit",
				},
				Annotations: map[string]string{
					"prow.k8s.io/job": "branch-ci-openshift-release-master-config-updates",
				},
			},
		},
	}, nil
}

type labelsAndValue struct {
	labels     []*dto.LabelPair
	gaugeValue float64
}

func TestProwJobCollector(t *testing.T) {
	expected := []labelsAndValue{
		{
			labels: []*dto.LabelPair{
				{
					Name:  stringPointer("job_agent"),
					Value: stringPointer("kubernetes"),
				},
				{
					Name:  stringPointer("job_name"),
					Value: stringPointer("pull-test-infra-bazel"),
				},
				{
					Name:  stringPointer("job_namespace"),
					Value: stringPointer("default"),
				},
				{
					Name:  stringPointer("label_event_GUID"),
					Value: stringPointer("770bab40-e601-11e9-8e50-08c45d902b6f"),
				},
				{
					Name:  stringPointer("label_preset_bazel_scratch_dir"),
					Value: stringPointer("true"),
				},
				{
					Name:  stringPointer("label_preset_service_account"),
					Value: stringPointer("true"),
				},
			},
			gaugeValue: float64(1),
		},
		{
			labels: []*dto.LabelPair{
				{
					Name:  stringPointer("annotation_prow_k8s_io_job"),
					Value: stringPointer("pull-test-infra-bazel"),
				},
				{
					Name:  stringPointer("annotation_testgrid_create_test_group"),
					Value: stringPointer("true"),
				},
				{
					Name:  stringPointer("job_agent"),
					Value: stringPointer("kubernetes"),
				},
				{
					Name:  stringPointer("job_name"),
					Value: stringPointer("pull-test-infra-bazel"),
				},
				{
					Name:  stringPointer("job_namespace"),
					Value: stringPointer("default"),
				},
			},
			gaugeValue: float64(1),
		},
		{
			labels: []*dto.LabelPair{
				{
					Name:  stringPointer("job_agent"),
					Value: stringPointer("kubernetes"),
				},
				{
					Name:  stringPointer("job_name"),
					Value: stringPointer("branch-ci-openshift-release-master-config-updates"),
				},
				{
					Name:  stringPointer("job_namespace"),
					Value: stringPointer("default"),
				},
				{
					Name:  stringPointer("label_event_GUID"),
					Value: stringPointer("e4216820-e604-11e9-8cf0-295472589b4f"),
				},
			},
			gaugeValue: float64(1),
		},
		{
			labels: []*dto.LabelPair{
				{
					Name:  stringPointer("annotation_prow_k8s_io_job"),
					Value: stringPointer("branch-ci-openshift-release-master-config-updates"),
				},
				{
					Name:  stringPointer("job_agent"),
					Value: stringPointer("kubernetes"),
				},
				{
					Name:  stringPointer("job_name"),
					Value: stringPointer("branch-ci-openshift-release-master-config-updates"),
				},
				{
					Name:  stringPointer("job_namespace"),
					Value: stringPointer("default"),
				},
			},
			gaugeValue: float64(1),
		},
	}

	pjc := prowJobCollector{
		lister: fakeLister{},
	}
	c := make(chan prometheus.Metric)
	go pjc.Collect(c)

	var metrics []prometheus.Metric

	for {
		select {
		case msg := <-c:
			metrics = append(metrics, msg)
			logrus.WithField("len(metrics)", len(metrics)).Infof("received a metric")
			if len(metrics) == 4 {
				// will panic when sending more metrics afterwards
				close(c)
				goto ExitForLoop
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout")
		}
	}

ExitForLoop:
	if len(metrics) != 4 {
		t.Fatalf("unexpected number '%d' of metrics sent by collector", len(metrics))
	}

	logrus.Info("get all 4 metrics")

	var actual []labelsAndValue
	for _, metric := range metrics {
		out := &dto.Metric{}
		if err := metric.Write(out); err != nil {
			t.Fatal("unexpected error occurred when writing")
		}
		actual = append(actual, labelsAndValue{labels: out.GetLabel(), gaugeValue: out.GetGauge().GetValue()})
	}
	if equalIgnoreOrder(expected, actual) != true {
		t.Fatalf("equalIgnoreOrder failed")
	}
}

func equalIgnoreOrder(values1 []labelsAndValue, values2 []labelsAndValue) bool {
	if len(values1) != len(values2) {
		return false
	}
	for _, v1 := range values1 {
		if !contains(values2, v1) {
			logrus.WithField("v1", v1).WithField("values2", values2).Errorf("v1 not in values2")
			return false
		}
	}
	for _, v2 := range values2 {
		if !contains(values1, v2) {
			logrus.WithField("v2", v2).WithField("values1", values1).Errorf("v2 not in values1")
			return false
		}
	}
	return true
}

func contains(values []labelsAndValue, value labelsAndValue) bool {
	for _, v := range values {
		if reflect.DeepEqual(v.gaugeValue, value.gaugeValue) && reflect.DeepEqual(v.labels, value.labels) {
			return true
		}
	}
	return false
}

func stringPointer(s string) *string {
	return &s
}

func TestFilterWithDenylist(t *testing.T) {
	testcases := []struct {
		description string
		labels      map[string]string
		expected    map[string]string
	}{
		{
			description: "nil labels",
			labels:      nil,
			expected:    nil,
		},
		{
			description: "empty labels",
			labels:      map[string]string{},
			expected:    map[string]string{},
		},
		{
			description: "normal labels",
			labels: map[string]string{
				"created-by-prow":       "true",
				"event-GUID":            "770bab40-e601-11e9-8e50-08c45d902b6f",
				"prow.k8s.io/refs.org":  "kubernetes",
				"prow.k8s.io/refs.pull": "14543",
				"prow.k8s.io/refs.repo": "test-infra",
				"prow.k8s.io/type":      "presubmit",
				"ci.openshift.io/role":  "infra",
			},
			expected: map[string]string{
				"event-GUID":           "770bab40-e601-11e9-8e50-08c45d902b6f",
				"ci.openshift.io/role": "infra",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.description, func(t *testing.T) {
			actual := filterWithDenylist(tc.labels)
			assertEqual(t, actual, tc.expected)
		})
	}
}

func TestGetLatest(t *testing.T) {
	time1 := time.Now()
	time2 := time1.Add(time.Minute)
	time3 := time2.Add(time.Minute)

	testcases := []struct {
		description string
		jobs        []*prowapi.ProwJob
		expected    map[string]*prowapi.ProwJob
	}{
		{
			description: "nil jobs",
			jobs:        nil,
			expected:    map[string]*prowapi.ProwJob{},
		},
		{
			description: "jobs with or without StartTime",
			jobs: []*prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						Job: "job0",
					},
					Status: prowapi.ProwJobStatus{},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "job1",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time1}},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "job1",
					},
					Status: prowapi.ProwJobStatus{},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "job2",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time1}},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "job2",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time3}},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "job2",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time2}},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Job: "job3",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time3}},
				},
			},
			expected: map[string]*prowapi.ProwJob{
				"job0": {
					Spec: prowapi.ProwJobSpec{
						Job: "job0",
					},
					Status: prowapi.ProwJobStatus{},
				},
				"job1": {
					Spec: prowapi.ProwJobSpec{
						Job: "job1",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time1}},
				},
				"job2": {
					Spec: prowapi.ProwJobSpec{
						Job: "job2",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time3}},
				},
				"job3": {
					Spec: prowapi.ProwJobSpec{
						Job: "job3",
					},
					Status: prowapi.ProwJobStatus{StartTime: metav1.Time{Time: time3}},
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.description, func(t *testing.T) {
			actual := getLatest(tc.jobs)
			assertEqual(t, actual, tc.expected)
		})
	}
}

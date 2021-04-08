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
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
)

type lister interface {
	List(selector labels.Selector) ([]*prowapi.ProwJob, error)
}

//https://godoc.org/github.com/prometheus/client_golang/prometheus#Collector
type prowJobCollector struct {
	lister lister
}

func (pjc prowJobCollector) Describe(ch chan<- *prometheus.Desc) {
	//prometheus.DescribeByCollect(pjc, ch)
	// Normally, we'd send descriptors into the channel. However, we cannot do so for these
	// metrics as their label sets are dynamic. This is a take-our-own-risk action and also a
	// compromise for implementing a metric with both dynamic keys and dynamic values in
	// the label set.
	// https://godoc.org/github.com/prometheus/client_golang/prometheus#hdr-Custom_Collectors_and_constant_Metrics
}

func (pjc prowJobCollector) Collect(ch chan<- prometheus.Metric) {
	logrus.Debug("ProwJobCollector collecting ...")
	prowJobs, err := pjc.lister.List(labels.Everything())
	if err != nil {
		logrus.WithError(err).Error("Failed to list prow jobs")
		return
	}
	//We need to filter out the latest jobs
	//because sending the same sample twice would lead to prometheus runtime error
	for _, pj := range getLatest(prowJobs) {
		agent := string(pj.Spec.Agent)
		pjLabelKeys, pjLabelValues := kubeLabelsToPrometheusLabels(filterWithDenylist(pj.Labels), "label_")
		pjLabelKeys = append([]string{"job_name", "job_namespace", "job_agent"}, pjLabelKeys...)
		pjLabelValues = append([]string{pj.Spec.Job, pj.Namespace, agent}, pjLabelValues...)
		labelDesc := prometheus.NewDesc(
			"prow_job_labels",
			"Kubernetes labels converted to Prometheus labels.",
			pjLabelKeys, nil,
		)
		ch <- prometheus.MustNewConstMetric(
			labelDesc,
			prometheus.GaugeValue,
			// See README.md for details
			float64(1),
			pjLabelValues...,
		)
		pjAnnotationKeys, pjAnnotationValues := kubeLabelsToPrometheusLabels(pj.Annotations, "annotation_")
		pjAnnotationKeys = append([]string{"job_name", "job_namespace", "job_agent"}, pjAnnotationKeys...)
		pjAnnotationValues = append([]string{pj.Spec.Job, pj.Namespace, agent}, pjAnnotationValues...)
		annotationDesc := prometheus.NewDesc(
			"prow_job_annotations",
			"Kubernetes annotations converted to Prometheus labels.",
			pjAnnotationKeys, nil,
		)
		ch <- prometheus.MustNewConstMetric(
			annotationDesc,
			prometheus.GaugeValue,
			float64(1),
			pjAnnotationValues...,
		)
	}
}

func getLatest(jobs []*prowapi.ProwJob) map[string]*prowapi.ProwJob {
	latest := map[string]time.Time{}
	latestJobs := map[string]*prowapi.ProwJob{}
	for _, job := range jobs {
		if _, ok := latest[job.Spec.Job]; !ok {
			latest[job.Spec.Job] = job.Status.StartTime.Time
			latestJobs[job.Spec.Job] = job
			continue
		}
		if job.Status.StartTime.Time.After(latest[job.Spec.Job]) {
			latest[job.Spec.Job] = job.Status.StartTime.Time
			latestJobs[job.Spec.Job] = job
		}
	}
	return latestJobs
}

var (
	labelKeyDenylist = sets.NewString(
		kube.CreatedByProw,
		kube.ProwJobTypeLabel,
		kube.ProwJobIDLabel,
		kube.ProwBuildIDLabel,
		kube.ProwJobAnnotation,
		kube.OrgLabel,
		kube.RepoLabel,
		kube.PullLabel,
	)
)

func filterWithDenylist(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	result := map[string]string{}
	for k, v := range labels {
		if !labelKeyDenylist.Has(k) {
			result[k] = v
		}
	}
	return result
}

var (
	invalidLabelCharRE    = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	escapeWithDoubleQuote = strings.NewReplacer("\\", `\\`, "\n", `\n`, "\"", `\"`)
)

// aligned with kube-state-metrics
// https://github.com/kubernetes/kube-state-metrics/blob/1d69c1e637564aec4591b5b03522fa8b5fca6597/internal/store/utils.go#L60
// kubeLabelsToPrometheusLabels ensures that the labels including key and value are accepted by prometheus
// We keep the function name (sanitizeLabelName and escapeString as well) the same as the one from kube-state-metrics for easy comparison
func kubeLabelsToPrometheusLabels(labels map[string]string, prefix string) ([]string, []string) {
	labelKeys := make([]string, 0, len(labels))
	for k := range labels {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)

	labelValues := make([]string, 0, len(labels))
	for i, k := range labelKeys {
		labelKeys[i] = fmt.Sprintf("%s%s", prefix, sanitizeLabelName(k))
		labelValues = append(labelValues, escapeString(labels[k]))
	}
	return labelKeys, labelValues
}

func sanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}

// https://github.com/kubernetes/kube-state-metrics/blob/1d69c1e637564aec4591b5b03522fa8b5fca6597/pkg/metric/metric.go#L96
func escapeString(v string) string {
	return escapeWithDoubleQuote.Replace(v)
}

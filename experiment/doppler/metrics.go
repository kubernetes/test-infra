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

package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

// The name of the metric that summarizes the chance that a PR
// has to hit a flake in a repository.
const total = "all"

type Metric struct {
	Name  string
	Gauge prometheus.Gauge
}

type Metrics []Metric

func (m Metrics) Set(repo, jobName string, value float64) {
	for i := range m {
		if m[i].Name == nameForMetric(repo, jobName) {
			m[i].Gauge.Set(value)
			return
		}
	}
}

func nameForMetric(repo, jobName string) string {
	return repo + "_" + jobName
}

// RegisterMetrics registers metrics for all presubmits that prow exposes.
func RegisterMetrics(url, repo string) (Metrics, error) {
	jobs, err := listProwJobs(url)
	if err != nil {
		return nil, err
	}

	var metrics Metrics
	presubmits := make(map[string]struct{})
	for _, pj := range jobs {
		// Register a metric for every presubmit job
		if pj.Type != "presubmit" {
			continue
		}
		if repo != "" && repo != pj.Repo {
			continue
		}
		metricName := nameForMetric(pj.Repo, pj.Job)
		if _, ok := presubmits[metricName]; !ok {
			metrics = append(metrics, newFlakeMetric(pj.Repo, pj.Job))
		}
		presubmits[metricName] = struct{}{}
	}
	metrics = append(metrics, newFlakeMetric(repo, total))
	return metrics, nil
}

// newFlakeMetric creates and registers a prometheus metric related
// to flakes.
func newFlakeMetric(repo, jobName string) Metric {
	labels := make(prometheus.Labels)
	labels["repository"] = repo
	labels["job_name"] = jobName

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "flake_percentage", ConstLabels: labels, Help: "Chance that a PR hits a flake",
	})
	prometheus.MustRegister(gauge)

	return Metric{
		Name:  nameForMetric(repo, jobName),
		Gauge: gauge,
	}
}

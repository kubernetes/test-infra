/*
Copyright 2022 The Kubernetes Authors.

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

// Package crier reports finished prowjob status to git providers.
package crier

import "github.com/prometheus/client_golang/prometheus"

const (
	ResultError   = "ERROR"
	ResultSuccess = "SUCCESS"
)

// Prometheus Metrics
var (
	crierMetrics = struct {
		latency *prometheus.HistogramVec
		// Count success/failures of reporting attempts.
		reportingResults *prometheus.CounterVec
	}{
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "crier_report_latency",
			Help:    "Histogram of time spent reporting, calculated by the time difference between job completion and end of reporting.",
			Buckets: []float64{1, 10, 20, 30, 60, 120, 180, 300, 600, 1200, 1800, 2700, 3600, 5400, 7200},
		}, []string{
			"reporter",
		}),
		reportingResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "crier_reporting_results",
			Help: "Count of successful and failed reporting attempts by reporter.",
		}, []string{
			"reporter",
			"result",
		}),
	}
)

func init() {
	prometheus.MustRegister(crierMetrics.latency)
	prometheus.MustRegister(crierMetrics.reportingResults)
}

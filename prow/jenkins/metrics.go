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

package jenkins

import "github.com/prometheus/client_golang/prometheus"

var (
	requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "jenkins_requests",
		Help: "Number of Jenkins requests made from prow.",
	}, []string{
		// http verb of the request
		"verb",
		// path of the request
		"handler",
		// http status code of the request
		"code",
	})
	requestRetries = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "jenkins_request_retries",
		Help: "Number of Jenkins request retries made from prow.",
	})
	requestLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "jenkins_request_latency",
		Help:    "Time for a request to roundtrip between prow and Jenkins.",
		Buckets: prometheus.DefBuckets,
	}, []string{
		// http verb of the request
		"verb",
		// path of the request
		"handler",
	})
	resyncPeriod = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "resync_period_seconds",
		Help:    "Time the controller takes to complete one reconciliation loop.",
		Buckets: prometheus.ExponentialBuckets(1, 3, 5),
	})
)

func init() {
	prometheus.MustRegister(requests)
	prometheus.MustRegister(requestRetries)
	prometheus.MustRegister(requestLatency)
	prometheus.MustRegister(resyncPeriod)
}

// ClientMetrics is a set of metrics gathered by the Jenkins client.
type ClientMetrics struct {
	Requests       *prometheus.CounterVec
	RequestRetries prometheus.Counter
	RequestLatency *prometheus.HistogramVec
}

// Metrics is a set of metrics gathered by the Jenkins operator.
// It includes client metrics and metrics related to the controller loop.
type Metrics struct {
	ClientMetrics *ClientMetrics
	ResyncPeriod  prometheus.Histogram
}

// NewMetrics creates a new set of metrics for the Jenkins operator.
func NewMetrics() *Metrics {
	return &Metrics{
		ClientMetrics: &ClientMetrics{
			Requests:       requests,
			RequestRetries: requestRetries,
			RequestLatency: requestLatency,
		},
		ResyncPeriod: resyncPeriod,
	}
}

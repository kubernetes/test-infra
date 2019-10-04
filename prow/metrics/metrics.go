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

// Package metrics contains utilities for working with metrics in prow.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/interrupts"
)

const metricsPort = 9090

// ExposeMetricsWithRegistry chooses whether to serve or push metrics for the service with the registry
func ExposeMetricsWithRegistry(component string, pushGateway config.PushGateway, reg *prometheus.Registry) {
	if pushGateway.Endpoint != "" {
		pushMetrics(component, pushGateway.Endpoint, pushGateway.Interval.Duration)
		if pushGateway.ServeMetrics {
			serveMetrics(reg)
		}
	} else {
		serveMetrics(reg)
	}
}

// ExposeMetrics chooses whether to serve or push metrics for the service
func ExposeMetrics(component string, pushGateway config.PushGateway) {
	ExposeMetricsWithRegistry(component, pushGateway, nil)
}

// serveMetrics serves prometheus metrics for the service
func serveMetrics(reg *prometheus.Registry) {
	var handler http.Handler
	if reg == nil {
		handler = promhttp.Handler()
	} else {
		handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", handler)
	server := &http.Server{Addr: ":" + strconv.Itoa(metricsPort), Handler: metricsMux}
	interrupts.ListenAndServe(server, 5*time.Second)
}

// pushMetrics is meant to run in a goroutine and continuously push
// metrics to the provided endpoint.
func pushMetrics(component, endpoint string, interval time.Duration) {
	interrupts.TickLiteral(func() {
		if err := push.FromGatherer(component, push.HostnameGroupingKey(), endpoint, prometheus.DefaultGatherer); err != nil {
			logrus.WithField("component", component).WithError(err).Error("Failed to push metrics.")
		}
	}, interval)
}

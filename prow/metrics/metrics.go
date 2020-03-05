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
	"github.com/sirupsen/logrus"
	ctrlruntimemetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/interrupts"
)

const metricsPort = 9090

type CreateServer func(http.Handler) interrupts.ListenAndServer

// ExposeMetricsWithRegistry chooses whether to serve or push metrics for the service with the registry
func ExposeMetricsWithRegistry(component string, pushGateway config.PushGateway, reg prometheus.Gatherer, createServer CreateServer) {
	if pushGateway.Endpoint != "" {
		pushMetrics(component, pushGateway.Endpoint, pushGateway.Interval.Duration)
		if !pushGateway.ServeMetrics {
			return
		}
	}

	if reg == nil {
		reg = prometheus.DefaultGatherer
	}
	handler := promhttp.HandlerFor(
		prometheus.Gatherers{reg, ctrlruntimemetrics.Registry},
		promhttp.HandlerOpts{},
	)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", handler)
	var server interrupts.ListenAndServer
	if createServer == nil {
		server = &http.Server{Addr: ":" + strconv.Itoa(metricsPort), Handler: metricsMux}
	} else {
		server = createServer(handler)
	}
	interrupts.ListenAndServe(server, 5*time.Second)
}

// ExposeMetrics chooses whether to serve or push metrics for the service
func ExposeMetrics(component string, pushGateway config.PushGateway) {
	ExposeMetricsWithRegistry(component, pushGateway, nil, nil)
}

// pushMetrics is meant to run in a goroutine and continuously push
// metrics to the provided endpoint.
func pushMetrics(component, endpoint string, interval time.Duration) {
	interrupts.TickLiteral(func() {
		if err := fromGatherer(component, hostnameGroupingKey(), endpoint, prometheus.DefaultGatherer); err != nil {
			logrus.WithField("component", component).WithError(err).Error("Failed to push metrics.")
		}
	}, interval)
}

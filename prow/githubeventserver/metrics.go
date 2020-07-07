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

package githubeventserver

import (
	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/test-infra/prow/plugins"
)

var (
	// Define all metrics for webhooks here.
	webhookCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "prow_webhook_counter",
		Help: "A counter of the webhooks made to prow.",
	}, []string{"event_type"})
	responseCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "prow_webhook_response_codes",
		Help: "A counter of the different responses hook has responded to webhooks with.",
	}, []string{"response_code"})
	pluginHandleDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "prow_plugin_handle_duration_seconds",
		Help:    "How long Prow took to handle an event by plugin, event type and action.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 40, 80, 160, 320, 640},
	}, []string{"event_type", "action", "plugin"})
	pluginHandleErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "prow_plugin_handle_errors",
		Help: "Prow errors handling an event by plugin, event type and action",
	}, []string{"event_type", "action", "plugin"})
)

func init() {
	prometheus.MustRegister(webhookCounter)
	prometheus.MustRegister(responseCounter)
	prometheus.MustRegister(pluginHandleDuration)
	prometheus.MustRegister(pluginHandleErrors)
}

// Metrics is a set of metrics gathered by hook.
type Metrics struct {
	WebhookCounter       *prometheus.CounterVec
	ResponseCounter      *prometheus.CounterVec
	PluginHandleDuration *prometheus.HistogramVec
	PluginHandleErrors   *prometheus.CounterVec
	*plugins.Metrics
}

// PluginMetrics is a set of metrics that are gathered by plugins.
// It is up the the consumers of these metrics to ensure that they
// update the values in a thread-safe manner.
type PluginMetrics struct {
	ConfigMapGauges *prometheus.GaugeVec
}

// NewMetrics creates a new set of metrics for the hook server.
func NewMetrics() *Metrics {
	return &Metrics{
		WebhookCounter:       webhookCounter,
		ResponseCounter:      responseCounter,
		PluginHandleDuration: pluginHandleDuration,
		PluginHandleErrors:   pluginHandleErrors,
		Metrics:              plugins.NewMetrics(),
	}
}

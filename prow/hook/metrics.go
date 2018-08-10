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

package hook

import (
	"github.com/prometheus/client_golang/prometheus"
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
)

func init() {
	prometheus.MustRegister(webhookCounter)
	prometheus.MustRegister(responseCounter)
}

type Metrics struct {
	WebhookCounter  *prometheus.CounterVec
	ResponseCounter *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		WebhookCounter:  webhookCounter,
		ResponseCounter: responseCounter,
	}
}

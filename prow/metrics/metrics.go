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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
)

// NewPusher creates a Pusher which dynamically reads its config from the
// ConfigAgent
func NewPusher(ca configGetter) Pusher {
	return Pusher{
		configAgent: ca,
	}
}

// Pusher is a struct that knows how to push metrics to prometheus
type Pusher struct {
	configAgent configGetter

	// gatherer and waiter are only on this struct to be faked out in the tests
	gatherer gathererFunc
	waiter   func(time.Duration) <-chan time.Time
}

// Start is meant to run in a goroutine and continuously push
// metrics to the provided endpoint.
func (pm Pusher) Start(component string) {
	if pm.gatherer == nil {
		pm.gatherer = push.FromGatherer
	}
	if pm.waiter == nil {
		pm.waiter = time.After
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		interval := pm.configAgent.Config().PushGateway.Interval
		waiter := pm.waiter(interval)

		select {
		case <-waiter:
			endpoint := pm.configAgent.Config().PushGateway.Endpoint
			if endpoint != "" {
				if err := pm.gatherer(component, push.HostnameGroupingKey(), endpoint, prometheus.DefaultGatherer); err != nil {
					logrus.WithField("component", component).WithError(err).Error("Failed to push metrics.")
				}
			}
		case <-sig:
			logrus.WithField("component", component).Infof("Metrics pusher shutting down...")
			return
		}
	}
}

type configGetter interface {
	Config() *config.Config
}
type gathererFunc func(job string, grouping map[string]string, url string, g prometheus.Gatherer) error

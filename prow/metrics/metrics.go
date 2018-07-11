/*
Copyright 2018 The Kubernetes Authors.

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

const (
	// minWaitDuration is the minimal interval allowed between two consecutive
	// Metrics Pusher loops. This guards against busy looping.
	minWaitDuration = 1 * time.Second
	// maxWaitDuration is the maximum interval allowed between two consecutive
	// Metrics Pusher loops. With this we make sure that potentially updated
	// configuration is at least queried once in some more or less reasonable
	// period.
	maxWaitDuration = 2 * time.Hour
)

// NewPusher creates a Pusher which dynamically reads its config from the
// ConfigAgent
func NewPusher(ca configGetter) Pusher {
	return Pusher{
		configAgent: ca,
		gatherer:    push.FromGatherer,
		waiter:      time.After,
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
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	getPusherConfig := func() (time.Duration, string) {
		c := pm.configAgent.Config().PushGateway

		e := c.Endpoint
		i := c.Interval

		if i < minWaitDuration {
			i = minWaitDuration
		}
		if i > maxWaitDuration {
			i = maxWaitDuration
		}

		return i, e
	}

	var endpoint string
	var interval time.Duration

	// get the initial sleep duration
	interval, _ = getPusherConfig()
	for {
		waiter := pm.waiter(interval)

		select {
		case <-waiter:
			// refresh both the interval and the endpoint from the config
			interval, endpoint = getPusherConfig()
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

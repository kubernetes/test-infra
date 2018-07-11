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

package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/test-infra/prow/config"
)

// TestMetricsPusherWaiterCalls tests that the waiter is actually called with
// the durations the config agent returns
func TestMetricsPusherWaiterCalls(t *testing.T) {
	waitForAfter := make(chan time.Duration)

	fakeAfter := func(d time.Duration) <-chan time.Time {
		newCh := make(chan time.Time, 1)
		newCh <- time.Time{}
		waitForAfter <- d
		return newCh
	}

	pusher := Pusher{
		waiter:   fakeAfter,
		gatherer: nullGatherer,
		configAgent: &fakeConfigAgent{
			fakeConfigs: []config.PushGateway{
				// Initial wait
				{Interval: 10 * time.Second},
				{Interval: 20 * time.Minute},
				// This interval is smaller then the min allowed duration
				{Interval: 0},
				{Interval: 30 * time.Second},
				// This interval is bigger then the max allowed duration
				{Interval: 30 * time.Hour},
				{Interval: 10 * time.Second},
			},
		},
	}
	go pusher.Start("")

	expectedIntervals := []time.Duration{
		10 * time.Second,
		20 * time.Minute,
		minWaitDuration,
		30 * time.Second,
		maxWaitDuration,
	}
	for i, expected := range expectedIntervals {
		if actual := <-waitForAfter; actual != expected {
			t.Fatalf("Expected interval %d to be '%s', got: '%s'", i, expected, actual)
		}
	}
}

// TestMetricsPusherGathererCalls tests that ...
// - the pusher does not run the gatherer function if the endpoint is not
//   configured (setting the endpoint to "" can be used to temporatily disable
//   the pushing/gathering of metrics)
// - keeps the pusher loop running, regardless if endpoint is configured or not
// - if the endpoint is set again, the gatherer function will be called again
func TestMetricsPusherGathererCalls(t *testing.T) {
	waitForURL := make(chan string)

	fakeGatherer := func(job string, grouping map[string]string, url string, g prometheus.Gatherer) error {
		waitForURL <- url
		return nil
	}

	pusher := Pusher{
		gatherer: fakeGatherer,
		waiter:   nullWaiter,
		configAgent: &fakeConfigAgent{
			fakeConfigs: []config.PushGateway{
				// The first time the Config() is requested is just to get the initial
				// sleep interval, subsequent calls will use both the interval and
				// the endpoint
				{Endpoint: "will be ignored"},
				{Endpoint: ""},
				{Endpoint: "endpoint one"},
				{Endpoint: ""},
				{Endpoint: ""},
				{Endpoint: "endpoint two"},
				{Endpoint: ""},
				{Endpoint: "endpoint three"},
			},
		},
	}
	go pusher.Start("")

	expectedEndpoints := []string{
		"endpoint one",
		"endpoint two",
		"endpoint three",
	}
	for i, expected := range expectedEndpoints {
		if actual := <-waitForURL; actual != expected {
			t.Fatalf("Expected endpoint %d to be '%s', got: '%s'", i, expected, actual)
		}
	}
}

func nullWaiter(wait time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func nullGatherer(job string, grouping map[string]string, url string, g prometheus.Gatherer) error {
	return nil
}

type fakeConfigAgent struct {
	fakeConfigs []config.PushGateway
	idx         int
}

func (ca *fakeConfigAgent) Config() *config.Config {

	c := &config.Config{}
	c.PushGateway = ca.fakeConfigs[ca.idx]

	if ca.idx < len(ca.fakeConfigs)-1 {
		ca.idx++
	}
	return c
}

/*
Copyright 2016 The Kubernetes Authors.

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
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/test-infra/prow/config"
)

func TestMetricsPusherIntervals(t *testing.T) {
	intervals := []time.Duration{15 * time.Millisecond, 0 * time.Millisecond, 10 * time.Millisecond}
	lastRunTime := time.Now()
	ranFuncCh := make(chan time.Time)

	intervalCheckGatherer := func(job string, grouping map[string]string, url string, g prometheus.Gatherer) error {
		ranFuncCh <- time.Now()
		return nil
	}

	pusher := Pusher{
		gatherer: intervalCheckGatherer,
		configAgent: &fakeConfigAgent{
			intervals: intervals,
		},
	}
	go pusher.Start("")

	for _, expectedInterval := range intervals {
		actualRunTime := <-ranFuncCh
		actualInterval := actualRunTime.Sub(lastRunTime)
		lastRunTime = actualRunTime
		maxAllowdDuration := expectedInterval * 2
		if maxAllowdDuration <= 0 {
			maxAllowdDuration = 10 * time.Millisecond
		}
		if actualInterval < expectedInterval {
			t.Errorf("Expected interval to be at least '%s', got '%s'", expectedInterval, actualInterval)
		}
		if actualInterval > maxAllowdDuration {
			t.Errorf("Expected interval to be at most '%s', got '%s'", expectedInterval*2, actualInterval)
		}
	}
}

func TestMetricsPusherEndpoints(t *testing.T) {
	endpoints := []string{"ep0", "ep1", "ep9000"}
	actualEndpoints := make(chan string)

	endpointCheckGatherer := func(job string, grouping map[string]string, url string, g prometheus.Gatherer) error {
		actualEndpoints <- url
		return nil
	}

	pusher := Pusher{
		gatherer: endpointCheckGatherer,
		configAgent: &fakeConfigAgent{
			endpoints: endpoints,
		},
	}
	go pusher.Start("")

	for _, expectedEndpoint := range endpoints {
		actualEndpoint := <-actualEndpoints
		if actualEndpoint != expectedEndpoint {
			t.Errorf("Expected endpoint to be '%s', got '%s'", expectedEndpoint, actualEndpoint)
		}
	}
}

func TestMetricsPusherWaiterCalls(t *testing.T) {
	intervals := []time.Duration{1 * time.Second, 2 * time.Second}

	fakeGatherer := func(job string, grouping map[string]string, url string, g prometheus.Gatherer) error {
		return nil
	}

	ranFuncCh := make(chan time.Duration)
	waiter := func(d time.Duration) <-chan time.Time {
		waiterCh := make(chan time.Time, 1)
		ranFuncCh <- d
		waiterCh <- time.Time{}
		return waiterCh
	}

	pusher := Pusher{
		gatherer: fakeGatherer,
		configAgent: &fakeConfigAgent{
			intervals: intervals,
		},
		waiter: waiter,
	}

	go pusher.Start("")

	for _, expectedInterval := range intervals {
		actualInterval := <-ranFuncCh
		if actualInterval != expectedInterval {
			t.Errorf("Expected interval was '%s', got '%s'", expectedInterval, actualInterval)
		}
	}
}

func TestMetricsPusherEmptyEndpoint(t *testing.T) {
	endpoints := []string{"", "ep0", "", "ep1"}

	ranFuncCh := make(chan string)
	fakeGatherer := func(job string, grouping map[string]string, url string, g prometheus.Gatherer) error {
		ranFuncCh <- url
		return nil
	}

	pusher := Pusher{
		gatherer: fakeGatherer,
		configAgent: &fakeConfigAgent{
			endpoints: endpoints,
		},
	}

	go pusher.Start("")

	collectedEndpoints := []string{}
	collectedEndpoints = append(collectedEndpoints, <-ranFuncCh)
	collectedEndpoints = append(collectedEndpoints, <-ranFuncCh)

	expectedEndpoints := []string{"ep0", "ep1"}

	if !reflect.DeepEqual(collectedEndpoints, expectedEndpoints) {
		t.Errorf("Expected collected endpoints to be '%v', got '%v'", expectedEndpoints, collectedEndpoints)
	}
}

type fakeConfigAgent struct {
	endpoints         []string
	intervals         []time.Duration
	calledForEndpoint bool
	endpointIdx       int
	intervalIdx       int
}

// Config returns a minimal *config.Config, filled only with the bits
// interesting for the metrics pusher.
// We also take advantage of the fact that Config() is *always* called twice
// per pusher iteration:
//   1st call to get the currently configured interval
//   2nd call to get the currently configured endpoint
// Therefore we will also return different configs per call. On every even call
// we will return the expected interval and a "default" endpoint, on every odd
// run we will return the expected endpoint and a "default" interval.
func (ca *fakeConfigAgent) Config() *config.Config {
	interval := 666 * time.Nanosecond
	endpoint := "should never be used"

	if ca.calledForEndpoint {
		if ca.endpointIdx < len(ca.endpoints) {
			endpoint = ca.endpoints[ca.endpointIdx]
		}
		ca.endpointIdx++
	} else {
		if ca.intervalIdx < len(ca.intervals) {
			interval = ca.intervals[ca.intervalIdx]
		}
		ca.intervalIdx++
	}

	ca.calledForEndpoint = !ca.calledForEndpoint

	fakeConfig := &config.Config{
		PushGateway: config.PushGateway{
			Endpoint: endpoint,
			Interval: interval,
		},
	}

	return fakeConfig
}

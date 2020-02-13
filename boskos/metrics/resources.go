/*
Copyright 2020 The Kubernetes Authors.

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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/ranch"
)

const (
	// ResourcesMetricName is the name of the Prometheus metric used to monitor Boskos resources.
	ResourcesMetricName = "boskos_resources"
	// ResourcesMetricDescription is the description for the Prometheus metric used to monitor Boskos resources.
	ResourcesMetricDescription = "Number of resources recorded in Boskos by resource type and state."
)

var (
	// ResourcesMetricLabels is the list of labels used for the Prometheus metric used to monitor Boskos resources.
	ResourcesMetricLabels = []string{"type", "state"}
)

type resourcesCollector struct {
	boskosResources *prometheus.Desc
	ranch           *ranch.Ranch
}

// NewResourcesCollector returns a collector which exports the current counts of
// Boskos resources, segmented by resource type and state.
func NewResourcesCollector(ranch *ranch.Ranch) prometheus.Collector {
	return resourcesCollector{
		boskosResources: prometheus.NewDesc(ResourcesMetricName, ResourcesMetricDescription, ResourcesMetricLabels, nil),
		ranch:           ranch,
	}
}

func (rc resourcesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- rc.boskosResources
}

func (rc resourcesCollector) Collect(ch chan<- prometheus.Metric) {
	metrics, err := rc.ranch.AllMetrics()
	if err != nil {
		logrus.WithError(err).Error("failed to get metrics")
	}
	NormalizeResourceMetrics(metrics, common.KnownStates, func(rtype, state string, count float64) {
		ch <- prometheus.MustNewConstMetric(rc.boskosResources, prometheus.GaugeValue, count, rtype, state)
	})
}

// NormalizeResourceMetrics "normalizes" the list of provided Metrics by
// bucketing any state not in states into the "Other" state, and by ensuring
// every state in states has some count (even if zero).
// It then applies the function for each combination of resource type and state.
func NormalizeResourceMetrics(metrics []common.Metric, states []string, updateFunc func(rtype, state string, count float64)) {
	knownStates := sets.NewString(states...)
	for _, metric := range metrics {
		countsByState := map[string]float64{}
		// Set default value of 0 for all known states
		for _, state := range states {
			countsByState[state] = 0
		}
		for state, value := range metric.Current {
			if !knownStates.Has(state) {
				state = common.Other
			}
			countsByState[state] += float64(value)
		}
		for state, count := range countsByState {
			updateFunc(metric.Type, state, count)
		}
	}
}

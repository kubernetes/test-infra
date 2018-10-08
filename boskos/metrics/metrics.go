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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
)

type prometheusMetrics struct {
	BoskosState map[string]map[string]prometheus.Gauge
}

var (
	promMetrics = prometheusMetrics{
		BoskosState: map[string]map[string]prometheus.Gauge{},
	}
	resources, states common.CommaSeparatedStrings
	defaultStates     = []string{
		common.Busy,
		common.Cleaning,
		common.Dirty,
		common.Free,
		common.Leased,
	}
)

func init() {
	flag.Var(&resources, "resource-type", "comma-separated list of resources need to have metrics collected")
	flag.Var(&states, "resource-state", "comma-separated list of states need to have metrics collected")
}

func initMetrics() {
	for _, resource := range resources {
		promMetrics.BoskosState[resource] = map[string]prometheus.Gauge{}
		for _, state := range states {
			promMetrics.BoskosState[resource][state] = prometheus.NewGauge(prometheus.GaugeOpts{
				Name: fmt.Sprintf("boskos_%s_%s", strings.Replace(resource, "-", "_", -1), state),
				Help: fmt.Sprintf("Number of %s %s", state, resource),
			})
		}
		// Adding other state for metrics that are not captured with existing state
		promMetrics.BoskosState[resource][common.Other] = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("boskos_%s_%s", strings.Replace(resource, "-", "_", -1), common.Other),
			Help: fmt.Sprintf("Number of %s %s", common.Other, resource),
		})
	}

	for _, gauges := range promMetrics.BoskosState {
		for _, gauge := range gauges {
			prometheus.MustRegister(gauge)
		}
	}
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	boskos := client.NewClient("Metrics", "http://boskos")
	logrus.Infof("Initialzied boskos client!")

	flag.Parse()
	if states == nil {
		states = defaultStates
	}

	initMetrics()

	http.Handle("/prometheus", promhttp.Handler())
	http.Handle("/", handleMetric(boskos))

	go func() {
		logTick := time.NewTicker(time.Minute).C
		for range logTick {
			if err := update(boskos); err != nil {
				logrus.WithError(err).Warning("[Boskos Metrics]Update failed!")
			}
		}
	}()

	logrus.Info("Start Service")
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

func filterMetrics(src map[string]int) map[string]int {
	metricStates := sets.NewString(states...)
	dest := map[string]int{}
	// Making sure all metrics are created
	for state := range metricStates {
		dest[state] = 0
	}
	dest[common.Other] = 0
	for state, value := range src {
		if state != common.Other && metricStates.Has(state) {
			dest[state] = value
		} else {
			dest[common.Other] += value
		}
	}
	return dest
}

func update(boskos *client.Client) error {
	for _, resource := range resources {
		metric, err := boskos.Metric(resource)
		if err != nil {
			return fmt.Errorf("fail to get metric for %s : %v", resource, err)
		}
		// Filtering metrics states
		for state, value := range filterMetrics(metric.Current) {
			promMetrics.BoskosState[resource][state].Set(float64(value))
		}
	}
	return nil
}

//  handleMetric: Handler for /
//  Method: GET
func handleMetric(boskos *client.Client) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		log := logrus.WithField("handler", "handleMetric")
		log.Infof("From %v", req.RemoteAddr)

		if req.Method != "GET" {
			log.Warningf("[BadRequest]method %v, expect GET", req.Method)
			http.Error(res, "only accepts GET request", http.StatusMethodNotAllowed)
			return
		}

		rtype := req.URL.Query().Get("type")
		if rtype == "" {
			msg := "type must be set in the request."
			log.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}

		log.Infof("Request for metric %v", rtype)

		metric, err := boskos.Metric(rtype)
		if err != nil {
			log.WithError(err).Errorf("Fail to get metic for %v", rtype)
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		metricJSON, err := json.Marshal(metric)
		if err != nil {
			log.WithError(err).Errorf("json.Marshal failed: %v", metricJSON)
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Infof("Metric query for %v: %v", rtype, string(metricJSON))
		fmt.Fprint(res, string(metricJSON))
	}
}

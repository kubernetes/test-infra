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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
)

var (
	resourceMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "boskos_resources",
		Help: "Number of resources recorded in Boskos",
	}, []string{"type", "state"})
	resources, states common.CommaSeparatedStrings
	defaultStates     = []string{
		common.Busy,
		common.Cleaning,
		common.Dirty,
		common.Free,
		common.Leased,
		common.ToBeDeleted,
		common.Tombstone,
	}
)

func init() {
	flag.Var(&resources, "resource-type", "comma-separated list of resources need to have metrics collected.")
	flag.Var(&states, "resource-state", "comma-separated list of states need to have metrics collected.")
	prometheus.MustRegister(resourceMetric)
}

func main() {
	logrusutil.ComponentInit("boskos-metrics")
	boskos := client.NewClient("Metrics", "http://boskos")
	logrus.Infof("Initialzied boskos client!")

	flag.Parse()
	if states == nil {
		states = defaultStates
	}

	metrics.ExposeMetrics("boskos", config.PushGateway{})

	go func() {
		logTick := time.NewTicker(30 * time.Second).C
		for range logTick {
			if err := update(boskos); err != nil {
				logrus.WithError(err).Warning("Update failed!")
			}
		}
	}()

	logrus.Info("Start Service")
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/", handleMetric(boskos))
	logrus.WithError(http.ListenAndServe(":8080", metricsMux)).Fatal("ListenAndServe returned.")
}

func update(boskos *client.Client) error {
	// initialize resources counted by type, then state
	resourcesByState := map[string]map[string]float64{}
	for _, resource := range resources {
		resourcesByState[resource] = map[string]float64{}
		for _, state := range states {
			resourcesByState[resource][state] = 0
		}
	}

	// record current states
	knownStates := sets.NewString(states...)
	for _, resource := range resources {
		metric, err := boskos.Metric(resource)
		if err != nil {
			return fmt.Errorf("fail to get metric for %s : %v", resource, err)
		}
		// Filtering metrics states
		for state, value := range metric.Current {
			if !knownStates.Has(state) {
				state = common.Other
			}
			resourcesByState[resource][state] = float64(value)
		}
	}

	// expose current states
	for resource, states := range resourcesByState {
		for state, amount := range states {
			resourceMetric.WithLabelValues(resource, state).Set(amount)
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

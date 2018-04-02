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
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
)

type prometheusMetrics struct {
	GceStats map[string]prometheus.Gauge
	GkeStats map[string]prometheus.Gauge
}

var (
	promMetrics = prometheusMetrics{
		GceStats: map[string]prometheus.Gauge{
			common.Free: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_free",
				Help: "Number of free gce-project",
			}),
			common.Busy: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_busy",
				Help: "Number of busy gce-project",
			}),
			common.Dirty: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_dirty",
				Help: "Number of dirty gce-project",
			}),
			common.Cleaning: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_cleaning",
				Help: "Number of cleaning gce-project",
			}),
		},
		GkeStats: map[string]prometheus.Gauge{
			common.Free: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gke_project_free",
				Help: "Number of free gke-project",
			}),
			common.Busy: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gke_project_busy",
				Help: "Number of busy gke-project",
			}),
			common.Dirty: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gke_project_dirty",
				Help: "Number of dirty gke-project",
			}),
			common.Cleaning: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gke_project_cleaning",
				Help: "Number of cleaning gke-project",
			}),
		},
	}
)

func init() {
	for _, gce := range promMetrics.GceStats {
		prometheus.MustRegister(gce)
	}

	for _, gke := range promMetrics.GkeStats {
		prometheus.MustRegister(gke)
	}
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	boskos := client.NewClient("Metrics", "http://boskos")
	logrus.Infof("Initialzied boskos client!")

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

func update(boskos *client.Client) error {
	gce, err := boskos.Metric("gce-project")
	if err != nil {
		return fmt.Errorf("fail to get metric for gce-project : %v", err)
	}

	promMetrics.GceStats[common.Free].Set(float64(gce.Current[common.Free]))
	promMetrics.GceStats[common.Busy].Set(float64(gce.Current[common.Busy]))
	promMetrics.GceStats[common.Dirty].Set(float64(gce.Current[common.Dirty]))
	promMetrics.GceStats[common.Cleaning].Set(float64(gce.Current[common.Cleaning]))

	gke, err := boskos.Metric("gke-project")
	if err != nil {
		return fmt.Errorf("fail to get metric for gke-project : %v", err)
	}

	promMetrics.GkeStats[common.Free].Set(float64(gke.Current[common.Free]))
	promMetrics.GkeStats[common.Busy].Set(float64(gke.Current[common.Busy]))
	promMetrics.GkeStats[common.Dirty].Set(float64(gke.Current[common.Dirty]))
	promMetrics.GkeStats[common.Cleaning].Set(float64(gke.Current[common.Cleaning]))

	return nil
}

//  handleMetric: Handler for /
//  Method: GET
func handleMetric(boskos *client.Client) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		log := logrus.WithField("handler", "handleMetric")
		log.Infof("From %v", req.RemoteAddr)

		if req.Method != "GET" {
			log.Warning("[BadRequest]method %v, expect GET", req.Method)
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

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

	"github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/test-infra/boskos/client"
)

type prometheusMetrics struct {
	GceStats map[string]prometheus.Gauge
	GkeStats map[string]prometheus.Gauge
}

var (
	promMetrics = prometheusMetrics{
		GceStats: map[string]prometheus.Gauge{
			"free": prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_free",
				Help: "Number of free gce-project",
			}),
			"busy": prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_busy",
				Help: "Number of busy gce-project",
			}),
			"dirty": prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_dirty",
				Help: "Number of dirty gce-project",
			}),
			"cleaning": prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gce_project_cleaning",
				Help: "Number of cleaning gce-project",
			}),
		},
		GkeStats: map[string]prometheus.Gauge{
			"free": prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gke_project_free",
				Help: "Number of free gke-project",
			}),
			"busy": prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gke_project_busy",
				Help: "Number of busy gke-project",
			}),
			"dirty": prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "boskos_gke_project_dirty",
				Help: "Number of dirty gke-project",
			}),
			"cleaning": prometheus.NewGauge(prometheus.GaugeOpts{
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
	boskos := client.NewClient("Reaper", "http://boskos")
	logrus.Infof("Initialzied boskos client!")

	http.Handle("/prometheus", promhttp.Handler())
	http.Handle("/", handleMetric(boskos))

	go func() {
		logTick := time.NewTicker(time.Minute).C
		for {
			select {
			case <-logTick:
				if err := update(boskos); err != nil {
					logrus.WithError(err).Warning("[Boskos Metrics]Update failed!")
				}
			}
		}
	}()

	logrus.Info("Start Service")
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

func update(boskos *client.Client) error {
	if gce, err := boskos.Metric("gce-project"); err != nil {
		return fmt.Errorf("fail to get metric for gce-project : %v", err)
	} else {
		promMetrics.GceStats["free"].Set(float64(gce.Current["free"]))
		promMetrics.GceStats["busy"].Set(float64(gce.Current["busy"]))
		promMetrics.GceStats["dirty"].Set(float64(gce.Current["dirty"]))
		promMetrics.GceStats["cleaning"].Set(float64(gce.Current["cleaning"]))
	}

	if gke, err := boskos.Metric("gke-project"); err != nil {
		return fmt.Errorf("fail to get metric for gke-project : %v", err)
	} else {
		promMetrics.GkeStats["free"].Set(float64(gke.Current["free"]))
		promMetrics.GkeStats["busy"].Set(float64(gke.Current["busy"]))
		promMetrics.GkeStats["dirty"].Set(float64(gke.Current["dirty"]))
		promMetrics.GkeStats["cleaning"].Set(float64(gke.Current["cleaning"]))
	}

	return nil
}

//  handleMetric: Handler for /metric
//  Method: GET
func handleMetric(boskos *client.Client) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		logrus.WithField("handler", "handleMetric").Infof("From %v", req.RemoteAddr)

		if req.Method != "GET" {
			logrus.Warning("[BadRequest]method %v, expect GET", req.Method)
			http.Error(res, "only accepts GET request", http.StatusMethodNotAllowed)
			return
		}

		rtype := req.URL.Query().Get("type")
		if rtype == "" {
			msg := "type must be set in the request."
			logrus.Warning(msg)
			http.Error(res, msg, http.StatusBadRequest)
			return
		}

		logrus.Infof("Request for metric %v", rtype)

		metric, err := boskos.Metric(rtype)
		if err != nil {
			logrus.WithError(err).Errorf("Fail to get metic for %v", rtype)
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		metricJSON, err := json.Marshal(metric)
		if err != nil {
			logrus.WithError(err).Errorf("json.Marshal failed: %v", metricJSON)
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
		logrus.Infof("Metric query for %v: %v", rtype, string(metricJSON))
		fmt.Fprint(res, string(metricJSON))
	}
}

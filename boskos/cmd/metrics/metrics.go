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

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/metrics"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/logrusutil"
	prowmetrics "k8s.io/test-infra/prow/metrics"
)

var (
	resourcesMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metrics.ResourcesMetricName,
		Help: metrics.ResourcesMetricDescription,
	}, metrics.ResourcesMetricLabels)
	boskosURL      string
	username       string
	passwordFile   string
	rtypes, states common.CommaSeparatedStrings
)

func init() {
	flag.StringVar(&boskosURL, "boskos-url", "http://boskos", "Boskos Server URL")
	flag.StringVar(&username, "username", "", "Username used to access the Boskos server")
	flag.StringVar(&passwordFile, "password-file", "", "The path to password file used to access the Boskos server")
	flag.Var(&rtypes, "resource-type", "comma-separated list of resources need to have metrics collected.")
	flag.Var(&states, "resource-state", "comma-separated list of states need to have metrics collected.")
	prometheus.MustRegister(resourcesMetric)
}

func main() {
	logrusutil.ComponentInit()

	flag.Parse()
	boskos, err := client.NewClient("Metrics", boskosURL, username, passwordFile)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create a Boskos client")
	}

	if states == nil {
		states = common.KnownStates
	}

	prowmetrics.ExposeMetrics("boskos", config.PushGateway{})

	go func() {
		tick := time.NewTicker(30 * time.Second).C
		for range tick {
			if err := updateResourcesMetric(boskos); err != nil {
				logrus.WithError(err).Warning("failed to update resource metrics")
			}
		}
	}()

	logrus.Info("Start Service")
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/", handleMetric(boskos))
	logrus.WithError(http.ListenAndServe(":8080", metricsMux)).Fatal("ListenAndServe returned.")
}

func updateResourcesMetric(boskos *client.Client) error {
	ms := []common.Metric{}
	for _, rtype := range rtypes {
		metric, err := boskos.Metric(rtype)
		if err != nil {
			logrus.WithError(err).Errorf("failed to get metric for %s", rtype)
			continue
		}
		ms = append(ms, metric)
	}
	metrics.NormalizeResourceMetrics(ms, states, func(rtype, state string, count float64) {
		resourcesMetric.WithLabelValues(rtype, state).Set(count)
	})
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

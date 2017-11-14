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
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/metrics"
)

var (
	prowJobs = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prowjobs",
		Help: "Number of prowjobs in the system",
	}, []string{
		// agent of the prowjob
		"agent",
		// name of the job
		"job",
		// state of the prowjob: triggered, pending, success, failure, aborted, error
		"state",
		// type of the prowjob: presubmit, postsubmit, periodic, batch
		"type",
	})
)

func init() {
	prometheus.MustRegister(prowJobs)
}

var (
	configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logger := logrus.WithField("component", "metrics-server")

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logger.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logger.WithError(err).Fatal("Error getting kube client.")
	}

	gateway := configAgent.Config().PushGateway
	if gateway.Endpoint != "" {
		go metrics.PushMetrics("metrics-server", gateway.Endpoint, gateway.Interval)
	}

	// Serve prometheus metrics.
	go serve()

	tick := time.Tick(30 * time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			if err := sync(kc); err != nil {
				logger.WithError(err).Error("Error syncing.")
			}
			logger.Infof("Sync time: %v", time.Since(start))
		case <-sig:
			logger.Infof("Metrics server is shutting down...")
			return
		}
	}
}

// serve starts a http server and serves prometheus metrics.
// Meant to be called inside a goroutine.
func serve() {
	http.Handle("/metrics", promhttp.Handler())
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

func sync(kc *kube.Client) error {
	pjs, err := kc.ListProwJobs(kube.EmptySelector)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}

	// map of agent to job to state to type to count
	metricMap := make(map[string]map[string]map[string]map[string]float64)

	for _, pj := range pjs {
		agent := string(pj.Spec.Agent)
		state := string(pj.Status.State)
		jobType := string(pj.Spec.Type)

		if metricMap[agent] == nil {
			metricMap[agent] = make(map[string]map[string]map[string]float64)
		}
		if metricMap[agent][pj.Spec.Job] == nil {
			metricMap[agent][pj.Spec.Job] = make(map[string]map[string]float64)
		}
		if metricMap[agent][pj.Spec.Job][state] == nil {
			metricMap[agent][pj.Spec.Job][state] = make(map[string]float64)
		}
		metricMap[agent][pj.Spec.Job][state][jobType]++
	}

	for agent, agentMap := range metricMap {
		for job, jobMap := range agentMap {
			for state, stateMap := range jobMap {
				for jobType, count := range stateMap {
					prowJobs.WithLabelValues(agent, job, state, jobType).Set(count)
				}
			}
		}
	}

	return nil
}

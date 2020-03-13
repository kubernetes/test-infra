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
	"runtime"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/handlers"
	"k8s.io/test-infra/boskos/metrics"
	"k8s.io/test-infra/boskos/ranch"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	prowmetrics "k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	defaultDynamicResourceUpdatePeriod = 10 * time.Minute
	defaultRequestTTL                  = 30 * time.Second
	defaultRequestGCPeriod             = time.Minute
)

var (
	configPath                  = flag.String("config", "config.yaml", "Path to init resource file")
	dynamicResourceUpdatePeriod = flag.Duration("dynamic-resource-update-period", defaultDynamicResourceUpdatePeriod,
		"Period at which to update dynamic resources. Set to 0 to disable.")
	requestTTL        = flag.Duration("request-ttl", defaultRequestTTL, "request TTL before losing priority in the queue")
	kubeClientOptions crds.KubernetesClientOptions
	logLevel          = flag.String("log-level", "info", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	namespace         = flag.String("namespace", corev1.NamespaceDefault, "namespace to install on")
)

var (
	httpRequestDuration = prowmetrics.HttpRequestDuration("boskos", 0.005, 1200)
	httpResponseSize    = prowmetrics.HttpResponseSize("boskos", 128, 65536)
	traceHandler        = prowmetrics.TraceHandler(handlers.NewBoskosSimplifier(), httpRequestDuration, httpResponseSize)
)

func init() {
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpResponseSize)
}

func main() {
	logrusutil.ComponentInit()
	kubeClientOptions.AddFlags(flag.CommandLine)
	flag.Parse()
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("invalid log level specified")
	}
	logrus.SetLevel(level)
	kubeClientOptions.Validate()

	// collect data on mutex holders and blocking profiles
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)

	defer interrupts.WaitForGracefulShutdown()
	pjutil.ServePProf()
	prowmetrics.ExposeMetrics("boskos", config.PushGateway{})
	// signal to the world that we are healthy
	// this needs to be in a separate port as we don't start the
	// main server with the main mux until we're ready
	health := pjutil.NewHealth()

	client, err := kubeClientOptions.CacheBackedClient(*namespace, &crds.ResourceObject{}, &crds.DRLCObject{})
	if err != nil {
		logrus.WithError(err).Fatal("unable to get client")
	}

	storage := ranch.NewStorage(interrupts.Context(), client, *namespace)

	r, err := ranch.NewRanch(*configPath, storage, *requestTTL)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to create ranch! Config: %v", *configPath)
	}

	boskos := &http.Server{
		Handler: traceHandler(handlers.NewBoskosHandler(r)),
		Addr:    ":8080",
	}

	// Viper defaults the configfile name to `config` and `SetConfigFile` only
	// has an effect when the configfile name is not an empty string, so we
	// just disable it entirely if there is no config.
	if *configPath != "" {
		v := viper.New()
		v.SetConfigFile(*configPath)
		v.SetConfigType("yaml")
		v.WatchConfig()
		v.OnConfigChange(func(in fsnotify.Event) {
			logrus.Infof("Updating Boskos Config")
			if err := r.SyncConfig(*configPath); err != nil {
				logrus.WithError(err).Errorf("Failed to update config")
			} else {
				logrus.Infof("Updated Boskos Config successfully")
			}
		})
	}

	prometheus.MustRegister(metrics.NewResourcesCollector(r))
	r.StartDynamicResourceUpdater(*dynamicResourceUpdatePeriod)
	r.StartRequestGC(defaultRequestGCPeriod)

	logrus.Info("Start Service")
	interrupts.ListenAndServe(boskos, 5*time.Second)

	// signal to the world that we're ready
	health.ServeReady()
}

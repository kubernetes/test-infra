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

package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/test-infra/prow/kube"
)

var (
	selector  = flag.String("label-selector", "", "Label selector to select prow pods for log tracing. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	namespace = flag.String("namespace", "", "Namespace where prow runs")
	headless  = flag.Bool("headless", false, "Run on headless mode")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logger := logrus.WithField("component", "tracer")

	if err := validateOptions(); err != nil {
		logrus.Fatal(err)
	}

	kc, err := kube.NewClientInCluster(*namespace)
	if err != nil {
		logger.WithError(err).Fatal("Error getting k8s client.")
	}

	mux := http.NewServeMux()
	if !*headless {
		mux.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir("/static"))))
	}
	mux.Handle("/trace", gziphandler.GzipHandler(handleTrace(*selector, kc)))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  2 * time.Minute,
		WriteTimeout: 2 * time.Minute,
	}
	logrus.Fatal(server.ListenAndServe())
}

func validateOptions() error {
	if *selector == "" {
		return fmt.Errorf("you need to specify a label selector")
	}
	if _, err := labels.Parse(*selector); err != nil {
		return err
	}
	if *namespace == "" {
		return fmt.Errorf("you need to specify a namespace")
	}
	return nil
}

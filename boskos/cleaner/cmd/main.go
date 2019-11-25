/*
Copyright 2019 The Kubernetes Authors.

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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/boskos/cleaner"
	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/ranch"
)

const (
	defaultCleanerCount      = 15
	defaultBoskosRetryPeriod = 15 * time.Second
	defaultOwner             = "cleaner"
)

var (
	kubeClientOptions crds.KubernetesClientOptions

	boskosURL    string
	cleanerCount int
)

func init() {
	flag.StringVar(&boskosURL, "boskos-url", "http://boskos", "Boskos Server URL")
	flag.IntVar(&cleanerCount, "cleaner-count", defaultCleanerCount, "Number of threads running cleanup")
	kubeClientOptions.AddFlags(flag.CommandLine)
}

func main() {
	flag.Parse()
	kubeClientOptions.Validate()

	logrus.SetFormatter(&logrus.JSONFormatter{})
	dc, err := kubeClientOptions.Client(crds.DRLCType)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create a Resource CRD client")
	}

	resStorage := crds.NewCRDStorage(dc)
	st, _ := ranch.NewStorage(nil, resStorage, "")

	logrus.SetFormatter(&logrus.JSONFormatter{})
	client := client.NewClient(defaultOwner, boskosURL)
	cleaner := cleaner.NewCleaner(cleanerCount, client, defaultBoskosRetryPeriod, st)

	cleaner.Start()
	defer cleaner.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

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
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/test-infra/boskos/cleaner"
	cleanerv2 "k8s.io/test-infra/boskos/cleaner/v2"
	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/ranch"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	prowmetrics "k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	defaultCleanerCount      = 15
	defaultBoskosRetryPeriod = 15 * time.Second
	defaultOwner             = "cleaner"
)

var (
	kubeClientOptions crds.KubernetesClientOptions

	boskosURL           string
	username            string
	passwordFile        string
	namespace           string
	cleanerCount        int
	logLevel            string
	useV2Implementation bool
)

func init() {
	flag.StringVar(&boskosURL, "boskos-url", "http://boskos", "Boskos Server URL")
	flag.StringVar(&username, "username", "", "Username used to access the Boskos server")
	flag.StringVar(&passwordFile, "password-file", "", "The path to password file used to access the Boskos server")
	flag.IntVar(&cleanerCount, "cleaner-count", defaultCleanerCount, "Number of threads running cleanup")
	flag.StringVar(&namespace, "namespace", corev1.NamespaceDefault, "namespace to install on")
	flag.StringVar(&logLevel, "log-level", "info", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	flag.BoolVar(&useV2Implementation, "use-v2-implementation", false, "Use the new controller-based v2 implementation. It is much faster and works directly based on the crds, but was used less")
	kubeClientOptions.AddFlags(flag.CommandLine)
}

func main() {
	logrusutil.ComponentInit()
	flag.Parse()
	kubeClientOptions.Validate()

	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("invalid log level specified")
	}
	logrus.SetLevel(level)

	client, err := client.NewClient(defaultOwner, boskosURL, username, passwordFile)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create a Boskos client")
	}

	defer interrupts.WaitForGracefulShutdown()
	prowmetrics.ExposeMetrics("boskos", config.PushGateway{})
	pjutil.ServePProf()

	if useV2Implementation {
		v2Main(client)
	} else {
		v1Main(client)
	}
}

func v1Main(client *client.Client) {

	kubeClient, err := kubeClientOptions.Client()
	if err != nil {
		logrus.WithError(err).Fatal("failed to construct kube client")
	}
	st := ranch.NewStorage(context.Background(), kubeClient, namespace)

	cleaner := cleaner.NewCleaner(cleanerCount, client, defaultBoskosRetryPeriod, st)

	cleaner.Start()
	defer cleaner.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

func v2Main(client *client.Client) {
	cfg, err := kubeClientOptions.Cfg()
	if err != nil {
		logrus.WithError(err).Fatal("failed to get kubeconfig.")
	}

	mgr, err := manager.New(cfg, manager.Options{
		LeaderElection:          true,
		LeaderElectionNamespace: namespace,
		LeaderElectionID:        "boskos-cleaner-leaderlock",
		Namespace:               namespace,
		MetricsBindAddress:      "0",
	})
	if err != nil {
		logrus.WithError(err).Fatal("failed to construct manager.")
	}

	if err := cleanerv2.Add(mgr, client, namespace); err != nil {
		logrus.WithError(err).Fatal("failed to add controller to manager.")
	}

	if err := mgr.Start(interrupts.Context().Done()); err != nil {
		logrus.WithError(err).Fatal("manager failed")
	} else {
		logrus.Info("manager ended gracefully.")
	}
}

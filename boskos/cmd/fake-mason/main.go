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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/mason"
	"k8s.io/test-infra/boskos/ranch"
)

const (
	defaultCleanerCount      = 15
	defaultBoskosRetryPeriod = 15 * time.Second
	defaultBoskosSyncPeriod  = 10 * time.Minute
	defaultOwner             = "mason"
	resourceName             = "FakeResource"
)

var (
	boskosURL         = flag.String("boskos-url", "http://boskos", "Boskos Server URL")
	username          = flag.String("username", "", "Username used to access the Boskos server")
	passwordFile      = flag.String("password-file", "", "The path to password file used to access the Boskos server")
	cleanerCount      = flag.Int("cleaner-count", defaultCleanerCount, "Number of threads running cleanup")
	namespace         = flag.String("namespace", corev1.NamespaceDefault, "namespace to install on")
	kubeClientOptions crds.KubernetesClientOptions
)

func configConverter(in string) (mason.Masonable, error) {
	return &fakeMasonAgent{}, nil
}

type fakeMasonAgent struct{}

func (m *fakeMasonAgent) Construct(context.Context, common.Resource, common.TypeToResources) (*common.UserData, error) {
	ud := map[string]string{"FakeResource": "fakeData"}
	return common.UserDataFromMap(ud), nil
}

func main() {
	kubeClientOptions.AddFlags(flag.CommandLine)
	flag.Parse()
	kubeClientOptions.Validate()

	logrus.SetFormatter(&logrus.JSONFormatter{})

	kubeClient, err := kubeClientOptions.Client()
	if err != nil {
		logrus.WithError(err).Fatal("failed to create kubeClient")
	}

	st := ranch.NewStorage(context.Background(), kubeClient, *namespace)

	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})
	client, err := client.NewClient(defaultOwner, *boskosURL, *username, *passwordFile)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create a Boskos client")
	}
	mason := mason.NewMason(*cleanerCount, client, defaultBoskosRetryPeriod, defaultBoskosSyncPeriod, st)

	// Registering Masonable Converters
	if err := mason.RegisterConfigConverter(resourceName, configConverter); err != nil {
		logrus.WithError(err).Fatalf("unable tp register config converter")
	}

	mason.Start()
	defer mason.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

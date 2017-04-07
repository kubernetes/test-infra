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
	"time"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plank"
)

var (
	totURL   = flag.String("tot-url", "http://tot", "Tot URL")
	crierURL = flag.String("crier-url", "http://crier", "Crier URL")
)

func main() {
	kc, err := kube.NewClientInCluster("default")
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}
	c := plank.NewController(kc, *crierURL, *totURL)
	for range time.Tick(30 * time.Second) {
		if err := c.Sync(); err != nil {
			logrus.WithError(err).Error("Error syncing.")
		}
	}
}

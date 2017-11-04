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
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

var configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	configAgent := config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	for now := range time.Tick(1 * time.Minute) {
		start := time.Now()
		if err := sync(kc, configAgent.Config(), now); err != nil {
			logrus.WithError(err).Error("Error syncing periodic jobs.")
		}
		logrus.Infof("Sync time: %v", time.Since(start))
	}
}

type kubeClient interface {
	ListProwJobs(string) ([]kube.ProwJob, error)
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

func sync(kc kubeClient, cfg *config.Config, now time.Time) error {
	jobs, err := kc.ListProwJobs(kube.EmptySelector)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	latestJobs := pjutil.GetLatestPeriodics(jobs)

	for _, p := range cfg.Periodics {
		j, ok := latestJobs[p.Name]
		if !ok || (j.Complete() && now.Sub(j.Status.StartTime) > p.GetInterval()) {
			if _, err := kc.CreateProwJob(pjutil.NewProwJob(pjutil.PeriodicSpec(p), p.Labels)); err != nil {
				return fmt.Errorf("error creating prow job: %v", err)
			}
		}
	}
	return nil
}

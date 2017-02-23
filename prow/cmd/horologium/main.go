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

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/line"
)

var configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	ca := config.ConfigAgent{}
	if err := ca.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster("default")
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	for now := range time.Tick(1 * time.Minute) {
		if err := sync(kc, ca.Config(), now); err != nil {
			logrus.WithError(err).Error("Error syncing periodic jobs.")
		}
	}
}

type kubeClient interface {
	ListJobs(map[string]string) ([]kube.Job, error)
	CreateJob(kube.Job) (kube.Job, error)
}

func sync(kc kubeClient, cfg *config.Config, now time.Time) error {
	jobs, err := kc.ListJobs(map[string]string{"type": "periodic"})
	if err != nil {
		return fmt.Errorf("error listing jobs: %v", err)
	}
	latestJobs := map[string]kube.Job{}
	for _, j := range jobs {
		name := j.Metadata.Labels["jenkins-job-name"]
		if j.Status.StartTime.After(latestJobs[name].Status.StartTime) {
			latestJobs[name] = j
		}
	}
	for _, p := range cfg.Periodics {
		j, ok := latestJobs[p.Name]
		if !ok || (j.Complete() && now.Sub(j.Status.StartTime) > p.GetInterval()) {
			if err := line.StartPeriodicJob(kc, p.Name); err != nil {
				return fmt.Errorf("error starting periodic job: %v", err)
			}
		}
	}
	return nil
}

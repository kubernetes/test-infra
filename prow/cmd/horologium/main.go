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
	"k8s.io/test-infra/prow/cron"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	configPath    string
	jobConfigPath string
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "horologium"}),
	)

	configAgent := config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	// start a cron
	cr := cron.New()
	cr.Start()

	for now := range time.Tick(1 * time.Minute) {
		start := time.Now()
		if err := sync(kc, configAgent.Config(), cr, now); err != nil {
			logrus.WithError(err).Error("Error syncing periodic jobs.")
		}
		logrus.Infof("Sync time: %v", time.Since(start))
	}
}

type kubeClient interface {
	ListProwJobs(string) ([]kube.ProwJob, error)
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

type cronClient interface {
	SyncConfig(cfg *config.Config) error
	QueuedJobs() []string
}

func sync(kc kubeClient, cfg *config.Config, cr cronClient, now time.Time) error {
	jobs, err := kc.ListProwJobs(kube.EmptySelector)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	latestJobs := pjutil.GetLatestProwJobs(jobs, kube.PeriodicJob)

	if err := cr.SyncConfig(cfg); err != nil {
		logrus.WithError(err).Error("Error syncing cron jobs.")
	}

	cronTriggers := map[string]bool{}
	for _, job := range cr.QueuedJobs() {
		cronTriggers[job] = true
	}

	errs := []error{}
	for _, p := range cfg.Periodics {
		j, ok := latestJobs[p.Name]

		if p.Cron == "" {
			if !ok || (j.Complete() && now.Sub(j.Status.StartTime.Time) > p.GetInterval()) {
				if _, err := kc.CreateProwJob(pjutil.NewProwJob(pjutil.PeriodicSpec(p), p.Labels)); err != nil {
					errs = append(errs, err)
				}
			}
		} else if _, exist := cronTriggers[p.Name]; exist {
			if !ok || j.Complete() {
				if _, err := kc.CreateProwJob(pjutil.NewProwJob(pjutil.PeriodicSpec(p), p.Labels)); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to create %d prowjobs: %v", len(errs), errs)
	}

	return nil
}

/*
Copyright 2016 The Kubernetes Authors.

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

type kubeClient interface {
	ListPods(selector string) ([]kube.Pod, error)
	DeletePod(name string) error

	ListProwJobs(selector string) ([]kube.ProwJob, error)
	DeleteProwJob(name string) error
}

type configAgent interface {
	Config() *config.Config
}

var (
	runOnce    = flag.Bool("run-once", false, "If true, run only once then quit.")
	configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	cluster    = flag.String("build-cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logger := logrus.WithField("component", "sinker")

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logger.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logger.WithError(err).Error("Error getting client.")
		return
	}

	var pkc *kube.Client
	if *cluster == "" {
		pkc = kc.Namespace(configAgent.Config().PodNamespace)
	} else {
		pkc, err = kube.NewClientFromFile(*cluster, configAgent.Config().PodNamespace)
		if err != nil {
			logger.WithError(err).Fatal("Error getting kube client.")
		}
	}

	c := controller{
		logger:      logger,
		kc:          kc,
		pkc:         pkc,
		configAgent: configAgent,
	}

	// Clean now and regularly from now on.
	for {
		start := time.Now()
		c.clean()
		logger.Infof("Sync time: %v", time.Since(start))
		if *runOnce {
			break
		}
		time.Sleep(configAgent.Config().Sinker.ResyncPeriod)
	}
}

type controller struct {
	logger      *logrus.Entry
	kc          kubeClient
	pkc         kubeClient
	configAgent configAgent
}

func (c *controller) clean() {
	// Clean up old prow jobs first.
	prowJobs, err := c.kc.ListProwJobs(kube.EmptySelector)
	if err != nil {
		c.logger.WithError(err).Error("Error listing prow jobs.")
		return
	}

	// Only delete pod if its prowjob is marked as finished
	isFinished := make(map[string]bool)

	maxProwJobAge := c.configAgent.Config().Sinker.MaxProwJobAge
	for _, prowJob := range prowJobs {
		// Handle periodics separately.
		if prowJob.Spec.Type == kube.PeriodicJob {
			continue
		}
		if prowJob.Complete() {
			isFinished[prowJob.Metadata.Name] = true
			if time.Since(prowJob.Status.StartTime) > maxProwJobAge {
				if err := c.kc.DeleteProwJob(prowJob.Metadata.Name); err == nil {
					c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
				} else {
					c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
				}
			}
		}
	}

	// Keep track of what periodic jobs are in the config so we will
	// not clean up their last prowjob.
	isActivePeriodic := make(map[string]bool)
	for _, p := range c.configAgent.Config().Periodics {
		isActivePeriodic[p.Name] = true
	}

	// Get the jobs that we need to retain so horologium can continue working
	// as intended.
	latestPeriodics := pjutil.GetLatestProwJobs(prowJobs, kube.PeriodicJob)
	for _, prowJob := range prowJobs {
		if prowJob.Spec.Type != kube.PeriodicJob {
			continue
		}

		latestPJ := latestPeriodics[prowJob.Spec.Job]
		if isActivePeriodic[prowJob.Spec.Job] && prowJob.Metadata.Name == latestPJ.Metadata.Name {
			// Ignore deleting this one.
			continue
		}
		if prowJob.Complete() {
			isFinished[prowJob.Metadata.Name] = true
			if time.Since(prowJob.Status.StartTime) > maxProwJobAge {
				if err := c.kc.DeleteProwJob(prowJob.Metadata.Name); err == nil {
					c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
				} else {
					c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
				}
			}
		}
	}

	// Now clean up old pods.
	selector := fmt.Sprintf("%s = %s", kube.CreatedByProw, "true")
	pods, err := c.pkc.ListPods(selector)
	if err != nil {
		c.logger.WithError(err).Error("Error listing pods.")
		return
	}
	maxPodAge := c.configAgent.Config().Sinker.MaxPodAge
	for _, pod := range pods {
		if _, ok := isFinished[pod.Metadata.Name]; !ok {
			continue
		}
		if (pod.Status.Phase == kube.PodSucceeded || pod.Status.Phase == kube.PodFailed) &&
			time.Since(pod.Status.StartTime) > maxPodAge {
			// Delete old completed pods. Don't quit if we fail to delete one.
			if err := c.pkc.DeletePod(pod.Metadata.Name); err == nil {
				c.logger.WithField("pod", pod.Metadata.Name).Info("Deleted old completed pod.")
			} else {
				c.logger.WithField("pod", pod.Metadata.Name).WithError(err).Error("Error deleting pod.")
			}
		}
	}
}

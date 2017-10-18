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
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

type kubeClient interface {
	ListPods(labels map[string]string) ([]kube.Pod, error)
	DeletePod(name string) error

	ListProwJobs(labels map[string]string) ([]kube.ProwJob, error)
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

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Error("Error getting client.")
		return
	}

	var pkc *kube.Client
	if *cluster == "" {
		pkc = kc.Namespace(configAgent.Config().PodNamespace)
	} else {
		pkc, err = kube.NewClientFromFile(*cluster, configAgent.Config().PodNamespace)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting kube client.")
		}
	}

	logger := logrus.StandardLogger()
	kc.Logger = logger.WithField("client", "kube")
	pkc.Logger = logger.WithField("client", "kube")

	// Clean now and regularly from now on.
	for {
		start := time.Now()
		clean(kc, pkc, configAgent)
		logrus.Infof("Sync time: %v", time.Since(start))
		if *runOnce {
			break
		}
		time.Sleep(configAgent.Config().Sinker.ResyncPeriod)
	}
}

func clean(kc, pkc kubeClient, configAgent configAgent) {
	// Clean up old prow jobs first.
	prowJobs, err := kc.ListProwJobs(nil)
	if err != nil {
		logrus.WithError(err).Error("Error listing prow jobs.")
		return
	}
	maxProwJobAge := configAgent.Config().Sinker.MaxProwJobAge
	for _, prowJob := range prowJobs {
		// Handle periodics separately.
		if prowJob.Spec.Type == kube.PeriodicJob {
			continue
		}
		if prowJob.Complete() && time.Since(prowJob.Status.StartTime) > maxProwJobAge {
			if err := kc.DeleteProwJob(prowJob.Metadata.Name); err == nil {
				logrus.WithField("prowjob", prowJob.Metadata.Name).Info("Deleted prowjob.")
			} else {
				logrus.WithField("prowjob", prowJob.Metadata.Name).WithError(err).Error("Error deleting prowjob.")
			}
		}
	}

	// Keep track of what periodic jobs are in the config so we will
	// not clean up their last prowjob.
	isActivePeriodic := make(map[string]bool)
	for _, p := range configAgent.Config().Periodics {
		isActivePeriodic[p.Name] = true
	}
	// Get the jobs that we need to retain so horologium can continue working
	// as intended.
	latestPeriodics := pjutil.GetLatestPeriodics(prowJobs)
	for _, prowJob := range prowJobs {
		if prowJob.Spec.Type != kube.PeriodicJob {
			continue
		}

		latestPJ := latestPeriodics[prowJob.Spec.Job]
		if isActivePeriodic[prowJob.Spec.Job] && prowJob.Metadata.Name == latestPJ.Metadata.Name {
			// Ignore deleting this one.
			continue
		}
		if prowJob.Complete() && time.Since(prowJob.Status.StartTime) > maxProwJobAge {
			if err := kc.DeleteProwJob(prowJob.Metadata.Name); err == nil {
				logrus.WithField("prowjob", prowJob.Metadata.Name).Info("Deleted prowjob.")
			} else {
				logrus.WithField("prowjob", prowJob.Metadata.Name).WithError(err).Error("Error deleting prowjob.")
			}
		}
	}

	// Now clean up old pods.
	labels := map[string]string{kube.CreatedByProw: "true"}
	pods, err := pkc.ListPods(labels)
	if err != nil {
		logrus.WithError(err).Error("Error listing pods.")
		return
	}
	maxPodAge := configAgent.Config().Sinker.MaxPodAge
	for _, pod := range pods {
		if (pod.Status.Phase == kube.PodSucceeded || pod.Status.Phase == kube.PodFailed) &&
			time.Since(pod.Status.StartTime) > maxPodAge {
			// Delete old completed pods. Don't quit if we fail to delete one.
			if err := pkc.DeletePod(pod.Metadata.Name); err == nil {
				logrus.WithField("pod", pod.Metadata.Name).Info("Deleted old completed pod.")
			} else {
				logrus.WithField("pod", pod.Metadata.Name).WithError(err).Error("Error deleting pod.")
			}
		}
	}
}

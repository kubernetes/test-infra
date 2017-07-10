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

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

const (
	period        = time.Hour
	maxProwJobAge = 7 * 24 * time.Hour
	maxPodAge     = 12 * time.Hour
)

type kubeClient interface {
	ListPods(labels map[string]string) ([]kube.Pod, error)
	DeletePod(name string) error

	ListProwJobs(labels map[string]string) ([]kube.ProwJob, error)
	DeleteProwJob(name string) error
}

var configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")

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
	pkc := kc.Namespace(configAgent.Config().PodNamespace)

	// Clean now and regularly from now on.
	clean(kc, pkc)
	t := time.Tick(period)
	for range t {
		clean(kc, pkc)
	}
}

func clean(kc, pkc kubeClient) {
	// Clean up old prow jobs first.
	prowJobs, err := kc.ListProwJobs(nil)
	if err != nil {
		logrus.WithError(err).Error("Error listing prow jobs.")
		return
	}
	for _, prowJob := range prowJobs {
		if prowJob.Complete() && time.Since(prowJob.Status.StartTime) > maxProwJobAge {
			if err := kc.DeleteProwJob(prowJob.Metadata.Name); err == nil {
				logrus.WithField("prowjob", prowJob.Metadata.Name).Info("Deleted prowjob.")
			} else {
				logrus.WithField("prowjob", prowJob.Metadata.Name).WithError(err).Error("Error deleting prowjob.")
			}
		}
	}

	// Now clean up old pods.
	pods, err := pkc.ListPods(nil)
	if err != nil {
		logrus.WithError(err).Error("Error listing pods.")
		return
	}
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

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
	"time"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
)

const (
	period    = time.Hour
	maxAge    = 24 * time.Hour
	namespace = "default"
)

type kubeClient interface {
	ListPods(labels map[string]string) ([]kube.Pod, error)
	DeletePod(name string) error

	ListJobs(labels map[string]string) ([]kube.Job, error)
	DeleteJob(name string) error
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})

	kc, err := kube.NewClientInCluster(namespace)
	if err != nil {
		logrus.WithError(err).Error("Error getting client.")
		return
	}

	// Clean now and regularly from now on.
	clean(kc)
	t := time.Tick(period)
	for range t {
		clean(kc)
	}
}

func clean(kc kubeClient) {
	// Clean up old jobs first.
	jobs, err := kc.ListJobs(nil)
	if err != nil {
		logrus.WithError(err).Error("Error listing jobs.")
		return
	}
	for _, job := range jobs {
		if job.Complete() && time.Since(job.Status.StartTime) > maxAge {
			// Delete successful jobs. Don't quit if we fail to delete one.
			if err := kc.DeleteJob(job.Metadata.Name); err == nil {
				logrus.WithField("job", job.Metadata.Name).Info("Deleted old completed job.")
			} else {
				logrus.WithField("job", job.Metadata.Name).WithError(err).Error("Error deleting job.")
				continue
			}
			pods, err := kc.ListPods(map[string]string{"job-name": job.Metadata.Name})
			if err != nil {
				logrus.WithField("job", job.Metadata.Name).WithError(err).Error("Error listing pods for job.")
				continue
			}
			for _, pod := range pods {
				if err := kc.DeletePod(pod.Metadata.Name); err == nil {
					logrus.WithField("pod", pod.Metadata.Name).Info("Deleted old pod for old job.")
				} else {
					logrus.WithField("pod", pod.Metadata.Name).WithError(err).Error("Error deleting old pod for old job.")
				}
			}
		}
	}

	// Now clean up old pods.
	pods, err := kc.ListPods(nil)
	if err != nil {
		logrus.WithError(err).Error("Error listing pods.")
		return
	}
	for _, pod := range pods {
		if (pod.Status.Phase == kube.PodSucceeded || pod.Status.Phase == kube.PodFailed) &&
			time.Since(pod.Status.StartTime) > maxAge {
			// Delete old completed pods. Don't quit if we fail to delete one.
			if err := kc.DeletePod(pod.Metadata.Name); err == nil {
				logrus.WithField("pod", pod.Metadata.Name).Info("Deleted old completed pod.")
			} else {
				logrus.WithField("pod", pod.Metadata.Name).WithError(err).Error("Error deleting pod.")
			}
		}
	}
}

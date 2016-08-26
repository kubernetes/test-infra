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
	"fmt"
	"log"
	"strconv"

	"github.com/kubernetes/test-infra/ciongke/kube"
)

// KubeAgent pulls BuildRequests off of the channel and turns them into
// Kubernetes jobs.
type KubeAgent struct {
	DryRun      bool
	TestPRImage string
	KubeClient  kubeClient
	Namespace   string

	BuildRequests <-chan BuildRequest
}

type kubeClient interface {
	ListPods(labels map[string]string) ([]kube.Pod, error)
	DeletePod(name string) error

	ListJobs(labels map[string]string) ([]kube.Job, error)
	CreateJob(j kube.Job) (kube.Job, error)
	DeleteJob(name string) error
}

// Cut off test-pr jobs after 10 hours.
const jobDeadlineSeconds = 60 * 60 * 10

func (ka *KubeAgent) Start() {
	go func() {
		for br := range ka.BuildRequests {
			if err := ka.deleteJob(br); err != nil {
				log.Printf("Error deleting job: %s", err)
			}
			if br.Create {
				if err := ka.createJob(br); err != nil {
					log.Printf("Error creating job: %s", err)
				}
			}
		}
	}()
}

func (ka *KubeAgent) createJob(br BuildRequest) error {
	name := fmt.Sprintf("%s-%s-pr-%d-%s", br.RepoOwner, br.RepoName, br.PR, br.JobName)
	job := kube.Job{
		Metadata: kube.ObjectMeta{
			Name:      name,
			Namespace: ka.Namespace,
			Labels: map[string]string{
				"owner":            br.RepoOwner,
				"repo":             br.RepoName,
				"pr":               strconv.Itoa(br.PR),
				"jenkins-job-name": br.JobName,
			},
		},
		Spec: kube.JobSpec{
			ActiveDeadlineSeconds: jobDeadlineSeconds,
			Template: kube.PodTemplateSpec{
				Spec: kube.PodSpec{
					RestartPolicy: "Never",
					Containers: []kube.Container{
						{
							Name:  "test-pr",
							Image: ka.TestPRImage,
							Args: []string{
								"--job-name=" + br.JobName,
								"--context=\"" + br.Context + "\"",
								"--repo-owner=" + br.RepoOwner,
								"--repo-name=" + br.RepoName,
								"--pr=" + strconv.Itoa(br.PR),
								"--branch=" + br.Branch,
								"--sha=" + br.SHA,
								"--dry-run=" + strconv.FormatBool(ka.DryRun),
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "oauth",
									ReadOnly:  true,
									MountPath: "/etc/github",
								},
								{
									Name:      "jenkins",
									ReadOnly:  true,
									MountPath: "/etc/jenkins",
								},
							},
						},
					},
					Volumes: []kube.Volume{
						{
							Name: "oauth",
							Secret: &kube.SecretSource{
								Name: "oauth-token",
							},
						},
						{
							Name: "jenkins",
							Secret: &kube.SecretSource{
								Name: "jenkins-token",
							},
						},
					},
				},
			},
		},
	}
	if _, err := ka.KubeClient.CreateJob(job); err != nil {
		return err
	}
	return nil
}

func (ka *KubeAgent) deleteJob(br BuildRequest) error {
	jobs, err := ka.KubeClient.ListJobs(map[string]string{
		"owner":            br.RepoOwner,
		"repo":             br.RepoName,
		"pr":               strconv.Itoa(br.PR),
		"jenkins-job-name": br.JobName,
	})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := ka.KubeClient.DeleteJob(job.Metadata.Name); err != nil {
			return err
		}
		pods, err := ka.KubeClient.ListPods(map[string]string{
			"job-name": job.Metadata.Name,
		})
		if err != nil {
			return err
		}
		for _, pod := range pods {
			if err = ka.KubeClient.DeletePod(pod.Metadata.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

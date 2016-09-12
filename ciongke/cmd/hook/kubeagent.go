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
	"github.com/Sirupsen/logrus"
	"github.com/satori/go.uuid"
	"strconv"
	"time"

	"github.com/kubernetes/test-infra/ciongke/kube"
)

// KubeAgent pulls KubeRequests off of the channel and turns them into
// Kubernetes jobs. The BuildRequests channel will create a new job, deleting
// the old if necessary, and the DeleteRequests channel will only delete.
type KubeAgent struct {
	DryRun      bool
	TestPRImage string
	KubeClient  kubeClient
	Namespace   string

	BuildRequests  <-chan KubeRequest
	DeleteRequests <-chan KubeRequest
}

// KubeRequest is the information required to start a job for a PR.
type KubeRequest struct {
	// The Jenkins job name, such as "kubernetes-pull-build-test-e2e-gce".
	JobName string
	// The context string for the GitHub status, such as "Jenkins GCE e2e".
	Context string

	RepoOwner string
	RepoName  string
	PR        int
	Branch    string
	SHA       string
}

type kubeClient interface {
	ListPods(labels map[string]string) ([]kube.Pod, error)
	DeletePod(name string) error

	ListJobs(labels map[string]string) ([]kube.Job, error)
	CreateJob(j kube.Job) (kube.Job, error)
	DeleteJob(name string) error
}

// Cut off test-pr jobs after 10 hours.
const jobDeadline = 10 * time.Hour

func fields(kr KubeRequest) logrus.Fields {
	return logrus.Fields{
		"job":    kr.JobName,
		"org":    kr.RepoOwner,
		"repo":   kr.RepoName,
		"pr":     kr.PR,
		"commit": kr.SHA,
		"branch": kr.Branch,
	}
}

// Start starts reading from the channels and does not block.
func (ka *KubeAgent) Start() {
	go func() {
		for kr := range ka.BuildRequests {
			if err := ka.deleteJob(kr); err != nil {
				logrus.WithFields(fields(kr)).Errorf("Error deleting job: %s", err)
			}
			if err := ka.createJob(kr); err != nil {
				logrus.WithFields(fields(kr)).Errorf("Error creating job: %s", err)
			}
		}
	}()
	go func() {
		for kr := range ka.DeleteRequests {
			if err := ka.deleteJob(kr); err != nil {
				logrus.WithFields(fields(kr)).Errorf("Error deleting job: %s", err)
			}
		}
	}()
}

func (ka *KubeAgent) createJob(kr KubeRequest) error {
	name := uuid.NewV1().String()
	job := kube.Job{
		Metadata: kube.ObjectMeta{
			Name:      name,
			Namespace: ka.Namespace,
			Labels: map[string]string{
				"owner":            kr.RepoOwner,
				"repo":             kr.RepoName,
				"pr":               strconv.Itoa(kr.PR),
				"jenkins-job-name": kr.JobName,
			},
		},
		Spec: kube.JobSpec{
			ActiveDeadlineSeconds: int(jobDeadline / time.Second),
			Template: kube.PodTemplateSpec{
				Spec: kube.PodSpec{
					RestartPolicy: "Never",
					Containers: []kube.Container{
						{
							Name:  "test-pr",
							Image: ka.TestPRImage,
							Args: []string{
								"--job-name=" + kr.JobName,
								"--context=" + kr.Context,
								"--repo-owner=" + kr.RepoOwner,
								"--repo-name=" + kr.RepoName,
								"--pr=" + strconv.Itoa(kr.PR),
								"--branch=" + kr.Branch,
								"--sha=" + kr.SHA,
								"--dry-run=" + strconv.FormatBool(ka.DryRun),
								"--jenkins-url=$(JENKINS_URL)",
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
							Env: []kube.EnvVar{
								{
									Name: "JENKINS_URL",
									ValueFrom: kube.EnvVarSource{
										ConfigMap: kube.ConfigMapKeySelector{
											Name: "jenkins-address",
											Key:  "jenkins-address",
										},
									},
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

func (ka *KubeAgent) deleteJob(kr KubeRequest) error {
	jobs, err := ka.KubeClient.ListJobs(map[string]string{
		"owner":            kr.RepoOwner,
		"repo":             kr.RepoName,
		"pr":               strconv.Itoa(kr.PR),
		"jenkins-job-name": kr.JobName,
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

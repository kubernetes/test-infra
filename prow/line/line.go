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

package line

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/satori/go.uuid"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
)

var (
	// Set by LINE_IMAGE environment variable.
	lineImage string
	// Set by DRY_RUN environment variable, defaults to true.
	dryRun bool
)

func init() {
	lineImage = os.Getenv("LINE_IMAGE")
	dry := os.Getenv("DRY_RUN")
	if dry == "" {
		dryRun = true
	} else if db, err := strconv.ParseBool(dry); err != nil {
		panic(fmt.Sprintf("DRY_RUN not parseable: %v", err))
	} else {
		dryRun = db
	}
}

// Cut off line jobs after 10 hours.
const jobDeadline = 10 * time.Hour

type startClient interface {
	CreateJob(kube.Job) (kube.Job, error)
}

type Pull struct {
	Number int
	Author string
	SHA    string
}

type BuildRequest struct {
	Org  string
	Repo string

	BaseRef string
	BaseSHA string

	Pulls []Pull
}

func (b BuildRequest) GetRefs() string {
	rs := []string{fmt.Sprintf("%s:%s", b.BaseRef, b.BaseSHA)}
	for _, pull := range b.Pulls {
		rs = append(rs, fmt.Sprintf("%d:%s", pull.Number, pull.SHA))
	}
	return strings.Join(rs, ",")
}

func StartPRJob(k *kube.Client, jobName, context string, pr github.PullRequest, baseSHA string) error {
	br := BuildRequest{
		Org:  pr.Base.Repo.Owner.Login,
		Repo: pr.Base.Repo.Name,

		BaseRef: pr.Base.Ref,
		BaseSHA: baseSHA,

		Pulls: []Pull{
			{
				Number: pr.Number,
				Author: pr.User.Login,
				SHA:    pr.Head.SHA,
			},
		},
	}
	return startJob(k, jobName, context, br)
}

func StartJob(k *kube.Client, jobName, context string, br BuildRequest) error {
	return startJob(k, jobName, context, br)
}

func StartPushJob(k *kube.Client, jobName string, pe github.PushEvent) error {
	br := BuildRequest{
		Org:     pe.Repo.Owner.Name,
		Repo:    pe.Repo.Name,
		BaseRef: pe.Branch(),
		BaseSHA: pe.After,
	}
	return startJob(k, jobName, "", br)
}

func startJob(k startClient, jobName, context string, br BuildRequest) error {
	refs := br.GetRefs()

	labels := map[string]string{
		"owner":            br.Org,
		"repo":             br.Repo,
		"jenkins-job-name": jobName,
	}
	annotations := map[string]string{
		"state":       "triggered",
		"description": "Build triggered.",
		"url":         "",
		"refs":        refs,
		"base-ref":    br.BaseRef,
		"base-sha":    br.BaseSHA,
	}
	args := []string{
		"--job-name=" + jobName,
		"--repo-owner=" + br.Org,
		"--repo-name=" + br.Repo,
		"--base-ref=" + br.BaseRef,
		"--base-sha=" + br.BaseSHA,
		"--refs=" + refs,
		"--dry-run=" + strconv.FormatBool(dryRun),
		"--jenkins-url=$(JENKINS_URL)",
	}
	if len(br.Pulls) == 0 {
		labels["type"] = "push"
		args = append(args, "--report=false")
	} else if len(br.Pulls) == 1 {
		labels["type"] = "pr"
		labels["pr"] = strconv.Itoa(br.Pulls[0].Number)
		annotations["context"] = context
		annotations["author"] = br.Pulls[0].Author
		annotations["pull-sha"] = br.Pulls[0].SHA
		args = append(args, "--pr="+strconv.Itoa(br.Pulls[0].Number))
		args = append(args, "--pull-sha="+br.Pulls[0].SHA)
		args = append(args, "--author="+br.Pulls[0].Author)
		args = append(args, "--report=true")
	} else if len(br.Pulls) > 1 {
		labels["type"] = "batch"
		annotations["context"] = context
		args = append(args, "--report=false")
	}

	name := uuid.NewV1().String()
	job := kube.Job{
		Metadata: kube.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: kube.JobSpec{
			ActiveDeadlineSeconds: int(jobDeadline / time.Second),
			Template: kube.PodTemplateSpec{
				Spec: kube.PodSpec{
					NodeSelector: map[string]string{
						"role": "build",
					},
					RestartPolicy: "Never",
					Containers: []kube.Container{
						{
							Name:  "line",
							Image: lineImage,
							Args:  args,
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
								{
									Name:      "labels",
									ReadOnly:  true,
									MountPath: "/etc/labels",
								},
								{
									Name:      "job-configs",
									ReadOnly:  true,
									MountPath: "/etc/jobs",
								},
							},
							Env: []kube.EnvVar{
								{
									Name: "JENKINS_URL",
									ValueFrom: &kube.EnvVarSource{
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
							Name: "labels",
							DownwardAPI: &kube.DownwardAPISource{
								Items: []kube.DownwardAPIFile{
									{
										Path: "labels",
										Field: kube.ObjectFieldSelector{
											FieldPath: "metadata.labels",
										},
									},
								},
							},
						},
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
						{
							Name: "job-configs",
							ConfigMap: &kube.ConfigMapSource{
								Name: "job-configs",
							},
						},
					},
				},
			},
		},
	}
	if _, err := k.CreateJob(job); err != nil {
		return err
	}
	return nil
}

type deleteClient interface {
	ListJobs(labels map[string]string) ([]kube.Job, error)
	GetJob(name string) (kube.Job, error)
	PatchJob(name string, job kube.Job) (kube.Job, error)
	PatchJobStatus(name string, job kube.Job) (kube.Job, error)
}

func DeletePRJob(k *kube.Client, jobName string, pr github.PullRequest) error {
	return deleteJob(k, jobName, pr)
}

func deleteJob(k deleteClient, jobName string, pr github.PullRequest) error {
	jobs, err := k.ListJobs(map[string]string{
		"owner":            pr.Base.Repo.Owner.Login,
		"repo":             pr.Base.Repo.Name,
		"pr":               strconv.Itoa(pr.Number),
		"jenkins-job-name": jobName,
	})
	if err != nil {
		return err
	}
	// Retry on conflict. This can happen if the job finishes and updates its
	// state right when we want to delete it.
	var job kube.Job
	for _, j := range jobs {
		job = j
		for i := 0; i < 3; i++ {
			if err := deleteKubeJob(k, job); err == nil {
				break
			} else {
				if _, ok := err.(kube.ConflictError); !ok {
					return err
				}
			}
			job, err = k.GetJob(j.Metadata.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteKubeJob(k deleteClient, job kube.Job) error {
	if job.Spec.Parallelism != nil && *job.Spec.Parallelism == 0 {
		// Already aborted this one.
		return nil
	} else if job.Status.Succeeded > 0 {
		// Already finished.
		return nil
	}
	// Delete the old job's pods by setting its parallelism to 0.
	parallelism := 0
	newStatus := job.Status
	newStatus.CompletionTime = time.Now()
	newAnnotations := job.Metadata.Annotations
	if newAnnotations == nil {
		newAnnotations = make(map[string]string)
	}
	newAnnotations["state"] = "aborted"
	newAnnotations["description"] = "Build aborted."
	newJob := kube.Job{
		Metadata: kube.ObjectMeta{
			Annotations: newAnnotations,
		},
		Spec: kube.JobSpec{
			Parallelism: &parallelism,
		},
		Status: newStatus,
	}
	// For some reason kubernetes makes you do this in two steps.
	if _, err := k.PatchJob(job.Metadata.Name, newJob); err != nil {
		return err
	}
	if _, err := k.PatchJobStatus(job.Metadata.Name, newJob); err != nil {
		return err
	}
	return nil
}

func SetJobStatus(k *kube.Client, podName, jobName, state, desc, url string) error {
	j, err := k.GetJob(jobName)
	if err != nil {
		return err
	}
	newAnnotations := j.Metadata.Annotations
	if newAnnotations == nil {
		newAnnotations = make(map[string]string)
	}
	newAnnotations["pod-name"] = podName
	newAnnotations["state"] = state
	newAnnotations["description"] = desc
	newAnnotations["url"] = url
	_, err = k.PatchJob(jobName, kube.Job{
		Metadata: kube.ObjectMeta{
			Annotations: newAnnotations,
		},
	})
	return err
}

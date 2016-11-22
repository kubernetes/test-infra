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
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
)

// TODO(spxtr): Improve the tests in here.

type kc struct {
	job kube.Job
}

func (c *kc) CreateJob(j kube.Job) (kube.Job, error) {
	c.job = j
	return j, nil
}

func (c *kc) ListJobs(labels map[string]string) ([]kube.Job, error) {
	return []kube.Job{c.job}, nil
}

func (c *kc) GetJob(name string) (kube.Job, error) {
	return c.job, nil
}

func (c *kc) PatchJob(name string, job kube.Job) (kube.Job, error) {
	c.job = job
	return job, nil
}

func (c *kc) PatchJobStatus(name string, job kube.Job) (kube.Job, error) {
	c.job = job
	return job, nil
}

// Make sure we set labels properly.
func TestStartJob(t *testing.T) {
	c := &kc{}
	br := BuildRequest{
		Org:     "owner",
		Repo:    "kube",
		BaseRef: "master",
		BaseSHA: "abc",
		Pulls: []Pull{
			{
				Number: 5,
				Author: "a",
				SHA:    "123",
			},
		},
	}
	if err := startJob(c, "job-name", br); err != nil {
		t.Fatalf("Didn't expect error starting job: %v", err)
	}
	labels := c.job.Metadata.Labels
	if labels["jenkins-job-name"] != "job-name" {
		t.Errorf("Jenkins job name label incorrect: %s", labels["jenkins-job-name"])
	}
	if labels["owner"] != "owner" {
		t.Errorf("Owner label incorrect: %s", labels["owner"])
	}
	if labels["repo"] != "kube" {
		t.Errorf("Repo label incorrect: %s", labels["kube"])
	}
	if labels["pr"] != "5" {
		t.Errorf("PR label incorrect: %s", labels["pr"])
	}
}

// Just make sure we set the parallelism to 0.
func TestDeleteJob(t *testing.T) {
	c := &kc{}
	if err := deleteJob(c, "job-name", github.PullRequest{}); err != nil {
		t.Fatalf("Didn't expect error deleting job: %v", err)
	}
	// The default kube.Job has nil parallelism and 0 succeeded pods. Ensure
	// that we explicitly zeroed parallelism.
	if c.job.Spec.Parallelism == nil || *c.job.Spec.Parallelism != 0 {
		t.Error("Didn't set parallelism to 0.")
	}
}

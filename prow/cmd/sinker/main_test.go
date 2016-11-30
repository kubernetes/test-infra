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
	"testing"
	"time"

	"k8s.io/test-infra/prow/kube"
)

type fakeClient struct {
	Pods []kube.Pod
	Jobs []kube.Job

	DeletedPods []kube.Pod
	DeletedJobs []kube.Job
}

func (c *fakeClient) ListPods(labels map[string]string) ([]kube.Pod, error) {
	pl := make([]kube.Pod, 0, len(c.Pods))
	for _, p := range c.Pods {
		if labelsMatch(labels, p.Metadata.Labels) {
			pl = append(pl, p)
		}
	}
	return pl, nil
}

func (c *fakeClient) ListJobs(labels map[string]string) ([]kube.Job, error) {
	jl := make([]kube.Job, 0, len(c.Jobs))
	for _, j := range c.Jobs {
		if labelsMatch(labels, j.Metadata.Labels) {
			jl = append(jl, j)
		}
	}
	return jl, nil
}

func (c *fakeClient) DeleteJob(name string) error {
	for i, j := range c.Jobs {
		if j.Metadata.Name == name {
			c.Jobs = append(c.Jobs[:i], c.Jobs[i+1:]...)
			c.DeletedJobs = append(c.DeletedJobs, j)
			return nil
		}
	}
	return fmt.Errorf("job %s not found", name)
}

func (c *fakeClient) DeletePod(name string) error {
	for i, p := range c.Pods {
		if p.Metadata.Name == name {
			c.Pods = append(c.Pods[:i], c.Pods[i+1:]...)
			c.DeletedPods = append(c.DeletedPods, p)
			return nil
		}
	}
	return fmt.Errorf("pod %s not found", name)
}

func labelsMatch(l1 map[string]string, l2 map[string]string) bool {
	for k1, v1 := range l1 {
		matched := false
		for k2, v2 := range l2 {
			if k1 == k2 && v1 == v2 {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func TestClean(t *testing.T) {
	pods := []kube.Pod{
		{
			Metadata: kube.ObjectMeta{
				Name: "old, failed",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, succeeded",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodSucceeded,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "new, failed",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: time.Now().Add(-10 * time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, running",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodRunning,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, running with job",
				Labels: map[string]string{
					"job-name": "old, aborted with pod",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodRunning,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
	}
	deletedPods := []string{
		"old, failed",
		"old, succeeded",
		"old, running with job",
	}
	zero := 0
	jobs := []kube.Job{
		{
			Metadata: kube.ObjectMeta{
				Name: "old, complete",
			},
			Status: kube.JobStatus{
				Active:    0,
				Succeeded: 1,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, deleted",
			},
			Spec: kube.JobSpec{
				Parallelism: &zero,
			},
			Status: kube.JobStatus{
				Active:    0,
				Succeeded: 0,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, active",
			},
			Status: kube.JobStatus{
				Active:    1,
				Succeeded: 0,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "new, active",
			},
			Status: kube.JobStatus{
				Active:    1,
				Succeeded: 0,
				StartTime: time.Now(),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "new, complete",
			},
			Status: kube.JobStatus{
				Active:    0,
				Succeeded: 1,
				StartTime: time.Now(),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "new, deleted",
			},
			Spec: kube.JobSpec{
				Parallelism: &zero,
			},
			Status: kube.JobStatus{
				Active:    0,
				Succeeded: 0,
				StartTime: time.Now(),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, aborted with pod",
			},
			Spec: kube.JobSpec{
				Parallelism: &zero,
			},
			Status: kube.JobStatus{
				Active:    0,
				Succeeded: 0,
				StartTime: time.Now().Add(-maxAge).Add(-time.Second),
			},
		},
	}
	deletedJobs := []string{
		"old, complete",
		"old, deleted",
		"old, aborted with pod",
	}
	kc := &fakeClient{
		Pods: pods,
		Jobs: jobs,
	}
	clean(kc)
	if len(deletedPods) != len(kc.DeletedPods) {
		t.Errorf("Deleted wrong number of pods: got %v expected %v", kc.DeletedPods, deletedPods)
	}
	for _, n := range deletedPods {
		found := false
		for _, p := range kc.DeletedPods {
			if p.Metadata.Name == n {
				found = true
			}
		}
		if !found {
			t.Errorf("Did not delete pod %s", n)
		}
	}
	if len(deletedJobs) != len(kc.DeletedJobs) {
		t.Errorf("Deleted wrong number of jobs: got %v expected %v", kc.DeletedJobs, deletedJobs)
	}
	for _, n := range deletedJobs {
		found := false
		for _, j := range kc.DeletedJobs {
			if j.Metadata.Name == n {
				found = true
			}
		}
		if !found {
			t.Errorf("Did not delete job %s", n)
		}
	}
}

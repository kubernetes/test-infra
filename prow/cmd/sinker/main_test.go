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
	"testing"
	"time"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/kube/fakekube"
)

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
	kc := &fakekube.FakeClient{
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

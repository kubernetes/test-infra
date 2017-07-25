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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

type fakeClient struct {
	Pods     []kube.Pod
	ProwJobs []kube.ProwJob

	DeletedPods     []kube.Pod
	DeletedProwJobs []kube.ProwJob
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

func (c *fakeClient) ListProwJobs(labels map[string]string) ([]kube.ProwJob, error) {
	jl := make([]kube.ProwJob, 0, len(c.ProwJobs))
	for _, j := range c.ProwJobs {
		if labelsMatch(labels, j.Metadata.Labels) {
			jl = append(jl, j)
		}
	}
	return jl, nil
}

func (c *fakeClient) DeleteProwJob(name string) error {
	for i, j := range c.ProwJobs {
		if j.Metadata.Name == name {
			c.ProwJobs = append(c.ProwJobs[:i], c.ProwJobs[i+1:]...)
			c.DeletedProwJobs = append(c.DeletedProwJobs, j)
			return nil
		}
	}
	return fmt.Errorf("prowjob %s not found", name)
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

const (
	maxProwJobAge = 2 * 24 * time.Hour
	maxPodAge     = 12 * time.Hour
)

type fca struct {
	c *config.Config
}

func newFakeConfigAgent() *fca {
	return &fca{
		c: &config.Config{
			Sinker: config.Sinker{
				MaxProwJobAge: maxProwJobAge,
				MaxPodAge:     maxPodAge,
			},
		},
	}

}

func (f *fca) Config() *config.Config {
	return f.c
}

func TestClean(t *testing.T) {
	pods := []kube.Pod{
		{
			Metadata: kube.ObjectMeta{
				Name: "old, failed",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, succeeded",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodSucceeded,
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
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
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
			},
		},
	}
	deletedPods := []string{
		"old, failed",
		"old, succeeded",
	}
	prowJobs := []kube.ProwJob{
		{
			Metadata: kube.ObjectMeta{
				Name: "old, complete",
			},
			Status: kube.ProwJobStatus{
				StartTime:      time.Now().Add(-maxProwJobAge).Add(-time.Second),
				CompletionTime: time.Now().Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old, incomplete",
			},
			Status: kube.ProwJobStatus{
				StartTime: time.Now().Add(-maxProwJobAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "new",
			},
			Status: kube.ProwJobStatus{
				StartTime: time.Now().Add(-time.Second),
			},
		},
	}
	deletedProwJobs := []string{
		"old, complete",
	}
	kc := &fakeClient{
		Pods:     pods,
		ProwJobs: prowJobs,
	}
	clean(kc, kc, newFakeConfigAgent())
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
	if len(deletedProwJobs) != len(kc.DeletedProwJobs) {
		t.Errorf("Deleted wrong number of prowjobs: got %v expected %v", kc.DeletedProwJobs, deletedProwJobs)
	}
	for _, n := range deletedProwJobs {
		found := false
		for _, j := range kc.DeletedProwJobs {
			if j.Metadata.Name == n {
				found = true
			}
		}
		if !found {
			t.Errorf("Did not delete prowjob %s", n)
		}
	}
}

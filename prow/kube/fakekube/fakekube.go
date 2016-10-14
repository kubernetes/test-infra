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

package fakekube

import (
	"fmt"

	"k8s.io/test-infra/prow/kube"
)

// FakeClient implements kube.Client.
type FakeClient struct {
	Pods []kube.Pod
	Jobs []kube.Job

	DeletedPods []kube.Pod
	DeletedJobs []kube.Job
}

func (c *FakeClient) ListPods(labels map[string]string) ([]kube.Pod, error) {
	pl := make([]kube.Pod, 0, len(c.Pods))
	for _, p := range c.Pods {
		if labelsMatch(labels, p.Metadata.Labels) {
			pl = append(pl, p)
		}
	}
	return pl, nil
}

func (c *FakeClient) DeletePod(name string) error {
	for i, p := range c.Pods {
		if p.Metadata.Name == name {
			c.Pods = append(c.Pods[:i], c.Pods[i+1:]...)
			c.DeletedPods = append(c.DeletedPods, p)
			return nil
		}
	}
	return fmt.Errorf("pod %s not found", name)
}

func (c *FakeClient) GetJob(name string) (kube.Job, error) {
	for _, j := range c.Jobs {
		if j.Metadata.Name == name {
			return j, nil
		}
	}
	return kube.Job{}, fmt.Errorf("job %s not found", name)
}

func (c *FakeClient) ListJobs(labels map[string]string) ([]kube.Job, error) {
	jl := make([]kube.Job, 0, len(c.Jobs))
	for _, j := range c.Jobs {
		if labelsMatch(labels, j.Metadata.Labels) {
			jl = append(jl, j)
		}
	}
	return jl, nil
}

func (c *FakeClient) CreateJob(j kube.Job) (kube.Job, error) {
	c.Jobs = append(c.Jobs, j)
	return j, nil
}

func (c *FakeClient) DeleteJob(name string) error {
	for i, j := range c.Jobs {
		if j.Metadata.Name == name {
			c.Jobs = append(c.Jobs[:i], c.Jobs[i+1:]...)
			c.DeletedJobs = append(c.DeletedJobs, j)
			return nil
		}
	}
	return fmt.Errorf("job %s not found", name)
}

func (c *FakeClient) PatchJob(name string, job kube.Job) (kube.Job, error) {
	for i, j := range c.Jobs {
		if j.Metadata.Name == name {
			c.Jobs[i].Metadata.Annotations = job.Metadata.Annotations
			c.Jobs[i].Spec = job.Spec
		}
	}
	return job, nil
}

func (c *FakeClient) PatchJobStatus(name string, job kube.Job) (kube.Job, error) {
	for i, j := range c.Jobs {
		if j.Metadata.Name == name {
			c.Jobs[i].Status = job.Status
		}
	}
	return job, nil
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

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
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/kube/labels"
)

type fakeClient struct {
	Pods     []kube.Pod
	ProwJobs []kube.ProwJob

	DeletedPods     []kube.Pod
	DeletedProwJobs []kube.ProwJob
}

func (c *fakeClient) ListPods(selector string) ([]kube.Pod, error) {
	s, err := labels.Parse(selector)
	if err != nil {
		return nil, err
	}
	pl := make([]kube.Pod, 0, len(c.Pods))
	for _, p := range c.Pods {
		if s.Matches(labels.Set(p.Metadata.Labels)) {
			pl = append(pl, p)
		}
	}
	return pl, nil
}

func (c *fakeClient) ListProwJobs(selector string) ([]kube.ProwJob, error) {
	s, err := labels.Parse(selector)
	if err != nil {
		return nil, err
	}
	jl := make([]kube.ProwJob, 0, len(c.ProwJobs))
	for _, j := range c.ProwJobs {
		if s.Matches(labels.Set(j.Metadata.Labels)) {
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
			Periodics: []config.Periodic{
				{Name: "retester"},
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
				Name: "old-failed",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old-succeeded",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodSucceeded,
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "new-failed",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: time.Now().Add(-10 * time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old-running",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodRunning,
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "unrelated-failed",
				Labels: map[string]string{
					kube.CreatedByProw: "not really",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "unrelated-complete",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodSucceeded,
				StartTime: time.Now().Add(-maxPodAge).Add(-time.Second),
			},
		},
	}
	deletedPods := []string{
		"old-failed",
		"old-succeeded",
	}
	prowJobs := []kube.ProwJob{
		{
			Metadata: kube.ObjectMeta{
				Name: "old-complete",
			},
			Status: kube.ProwJobStatus{
				StartTime:      time.Now().Add(-maxProwJobAge).Add(-time.Second),
				CompletionTime: time.Now().Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "old-incomplete",
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
		{
			Metadata: kube.ObjectMeta{
				Name: "newer-periodic",
			},
			Spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "retester",
			},
			Status: kube.ProwJobStatus{
				StartTime:      time.Now().Add(-maxProwJobAge).Add(-time.Second),
				CompletionTime: time.Now().Add(-time.Second),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "older-periodic",
			},
			Spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "retester",
			},
			Status: kube.ProwJobStatus{
				StartTime:      time.Now().Add(-maxProwJobAge).Add(-time.Minute),
				CompletionTime: time.Now().Add(-time.Minute),
			},
		},
		{
			Metadata: kube.ObjectMeta{
				Name: "oldest-periodic",
			},
			Spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "retester",
			},
			Status: kube.ProwJobStatus{
				StartTime:      time.Now().Add(-maxProwJobAge).Add(-time.Hour),
				CompletionTime: time.Now().Add(-time.Hour),
			},
		},
	}
	deletedProwJobs := []string{
		"old-complete",
		"older-periodic",
		"oldest-periodic",
	}
	kc := &fakeClient{
		Pods:     pods,
		ProwJobs: prowJobs,
	}
	// Run
	c := controller{
		logger:      logrus.WithField("component", "sinker"),
		kc:          kc,
		pkc:         kc,
		configAgent: newFakeConfigAgent(),
	}
	c.clean()
	// Check
	if len(deletedPods) != len(kc.DeletedPods) {
		var got []string
		for _, pj := range kc.DeletedPods {
			got = append(got, pj.Metadata.Name)
		}
		t.Errorf("Deleted wrong number of pods: got %d (%v), expected %d (%v)",
			len(got), strings.Join(got, ", "), len(deletedPods), strings.Join(deletedPods, ", "))
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
		var got []string
		for _, pj := range kc.DeletedProwJobs {
			got = append(got, pj.Metadata.Name)
		}
		t.Errorf("Deleted wrong number of prowjobs: got %d (%s), expected %d (%s)",
			len(got), strings.Join(got, ", "), len(deletedProwJobs), strings.Join(deletedProwJobs, ", "))
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

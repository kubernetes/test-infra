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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

type fakeClient struct {
	Pods []kube.Pod

	DeletedPods []kube.Pod
}

func (c *fakeClient) ListPods(selector string) ([]kube.Pod, error) {
	s, err := labels.Parse(selector)
	if err != nil {
		return nil, err
	}
	pl := make([]kube.Pod, 0, len(c.Pods))
	for _, p := range c.Pods {
		if s.Matches(labels.Set(p.ObjectMeta.Labels)) {
			pl = append(pl, p)
		}
	}
	return pl, nil
}

func (c *fakeClient) DeletePod(name string) error {
	for i, p := range c.Pods {
		if p.ObjectMeta.Name == name {
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
			ProwConfig: config.ProwConfig{
				Sinker: config.Sinker{
					MaxProwJobAge: maxProwJobAge,
					MaxPodAge:     maxPodAge,
				},
			},
			JobConfig: config.JobConfig{
				Periodics: []config.Periodic{
					{JobBase: config.JobBase{Name: "retester"}},
				},
			},
		},
	}

}

func (f *fca) Config() *config.Config {
	return f.c
}

func startTime(s time.Time) *metav1.Time {
	start := metav1.NewTime(s)
	return &start
}

func TestClean(t *testing.T) {

	pods := []kube.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-failed",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-succeeded",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-just-complete",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-pending",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodPending,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-pending-abort",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodPending,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "new-failed",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: startTime(time.Now().Add(-10 * time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-running",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodRunning,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unrelated-failed",
				Labels: map[string]string{
					kube.CreatedByProw: "not really",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unrelated-complete",
			},
			Status: kube.PodStatus{
				Phase:     kube.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
	}
	deletedPods := []string{
		"old-failed",
		"old-succeeded",
		"old-pending-abort",
	}
	setComplete := func(d time.Duration) *metav1.Time {
		completed := metav1.NewTime(time.Now().Add(d))
		return &completed
	}
	prowJobs := []runtime.Object{
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-failed",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-succeeded",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-just-complete",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-complete",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-incomplete",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-pending",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-pending-abort",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "newer-periodic",
				Namespace: "ns",
			},
			Spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "retester",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "older-periodic",
				Namespace: "ns",
			},
			Spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "retester",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Minute)),
				CompletionTime: setComplete(-time.Minute),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "oldest-periodic",
				Namespace: "ns",
			},
			Spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "retester",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Hour)),
				CompletionTime: setComplete(-time.Hour),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-failed-trusted",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
	}
	deletedProwJobs := []string{
		"old-failed",
		"old-succeeded",
		"old-complete",
		"old-pending-abort",
		"older-periodic",
		"oldest-periodic",
		"old-failed-trusted",
	}
	podsTrusted := []kube.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-failed-trusted",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: kube.PodStatus{
				Phase:     kube.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
	}
	deletedPodsTrusted := []string{"old-failed-trusted"}

	fpjc := fake.NewSimpleClientset(prowJobs...)
	kc := &fakeClient{
		Pods: pods,
	}
	kcTrusted := &fakeClient{
		Pods: podsTrusted,
	}
	// Run
	c := controller{
		logger:        logrus.WithField("component", "sinker"),
		prowJobClient: fpjc.ProwV1().ProwJobs("ns"),
		pkcs:          map[string]kubeClient{kube.DefaultClusterAlias: kc, "trusted": kcTrusted},
		configAgent:   newFakeConfigAgent(),
	}
	c.clean()
	var observedDeletedProwJobs []string
	for _, action := range fpjc.Fake.Actions() {
		switch action := action.(type) {
		case clienttesting.DeleteActionImpl:
			observedDeletedProwJobs = append(observedDeletedProwJobs, action.Name)
		}
	}
	if len(deletedProwJobs) != len(observedDeletedProwJobs) {
		t.Errorf("Deleted wrong number of prowjobs: got %d (%s), expected %d (%s)",
			len(observedDeletedProwJobs), strings.Join(observedDeletedProwJobs, ", "), len(deletedProwJobs), strings.Join(deletedProwJobs, ", "))
	}
	for _, n := range deletedProwJobs {
		found := false
		for _, job := range observedDeletedProwJobs {
			if job == n {
				found = true
			}
		}
		if !found {
			t.Errorf("Did not delete prowjob %s", n)
		}
	}
	// Check
	check := func(kc *fakeClient, deletedPods []string) {
		if len(deletedPods) != len(kc.DeletedPods) {
			var got []string
			for _, pj := range kc.DeletedPods {
				got = append(got, pj.ObjectMeta.Name)
			}
			t.Errorf("Deleted wrong number of pods: got %d (%v), expected %d (%v)",
				len(got), strings.Join(got, ", "), len(deletedPods), strings.Join(deletedPods, ", "))
		}
		for _, n := range deletedPods {
			found := false
			for _, p := range kc.DeletedPods {
				if p.ObjectMeta.Name == n {
					found = true
				}
			}
			if !found {
				t.Errorf("Did not delete pod %s", n)
			}
		}
	}
	check(kc, deletedPods)
	check(kcTrusted, deletedPodsTrusted)
}

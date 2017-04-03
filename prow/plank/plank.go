/*
Copyright 2017 The Kubernetes Authors.

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

package plank

import (
	"fmt"
	"time"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/line"
)

type kubeClient interface {
	ListJobs(map[string]string) ([]kube.Job, error)
	ListProwJobs(map[string]string) ([]kube.ProwJob, error)
	ReplaceProwJob(string, kube.ProwJob) (kube.ProwJob, error)
	CreateJob(kube.Job) (kube.Job, error)
}

type Controller struct {
	kc kubeClient
}

func NewController(kc *kube.Client) *Controller {
	return &Controller{
		kc: kc,
	}
}

func (c *Controller) Sync() error {
	pjs, err := c.kc.ListProwJobs(nil)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	js, err := c.kc.ListJobs(nil)
	if err != nil {
		return fmt.Errorf("error listing jobs: %v", err)
	}
	jm := map[string]*kube.Job{}
	for _, j := range js {
		jm[j.Metadata.Name] = &j
	}
	var errs []error
	for _, pj := range pjs {
		if err := c.syncJob(pj, jm); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	} else {
		return fmt.Errorf("errors syncing: %v", errs)
	}
}

func (c *Controller) syncJob(pj kube.ProwJob, jm map[string]*kube.Job) error {
	// Pass over completed prow jobs.
	if !pj.Status.CompletionTime.IsZero() {
		return nil
	}
	if pj.Status.KubeJobName == "" {
		// Start job.
		name, err := c.startJob(pj)
		if err != nil {
			return err
		}
		pj.Status.KubeJobName = name
		pj.Status.State = kube.PendingState
		pj.Status.StartTime = time.Now()
		if _, err := c.kc.ReplaceProwJob(pj.Metadata.Name, pj); err != nil {
			return err
		}
	} else if j, ok := jm[pj.Status.KubeJobName]; !ok {
		return fmt.Errorf("kube job %s not found", pj.Status.KubeJobName)
	} else if j.Complete() {
		// Kube job finished, update prow job.
		pj.Status.State = kube.ProwJobState(j.Metadata.Annotations["state"])
		pj.Status.CompletionTime = j.Status.CompletionTime
		if _, err := c.kc.ReplaceProwJob(pj.Metadata.Name, pj); err != nil {
			return err
		}
	} else {
		// Kube job still running. Nothing to do.
	}
	return nil
}

func (c *Controller) startJob(pj kube.ProwJob) (string, error) {
	//TODO(spxtr): All the rest.
	switch pj.Spec.Type {
	case kube.PresubmitJob:
	case kube.PostsubmitJob:
	case kube.PeriodicJob:
		return line.StartPeriodicJob(c.kc, pj.Spec.Job)
	case kube.BatchJob:
	}
	return "", fmt.Errorf("unhandled job type")
}

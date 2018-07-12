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

// Package pjutil contains helpers for working with ProwJobs.
package pjutil

import (
	"path/filepath"
	"strconv"

	"github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
)

const (
	jobNameLabel = "prow.k8s.io/job"
	jobTypeLabel = "prow.k8s.io/type"
	orgLabel     = "prow.k8s.io/refs.org"
	repoLabel    = "prow.k8s.io/refs.repo"
	pullLabel    = "prow.k8s.io/refs.pull"
)

// NewProwJob initializes a ProwJob out of a ProwJobSpec.
func NewProwJob(spec kube.ProwJobSpec, labels map[string]string) kube.ProwJob {
	allLabels := map[string]string{
		jobNameLabel: spec.Job,
		jobTypeLabel: string(spec.Type),
	}
	if spec.Type != kube.PeriodicJob {
		allLabels[orgLabel] = spec.Refs.Org
		allLabels[repoLabel] = spec.Refs.Repo
		if len(spec.Refs.Pulls) > 0 {
			allLabels[pullLabel] = strconv.Itoa(spec.Refs.Pulls[0].Number)
		}
	}
	for key, value := range labels {
		allLabels[key] = value
	}

	// let's validate labels
	for key, value := range allLabels {
		if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
			// try to use basename of a path, if path contains invalid //
			base := filepath.Base(value)
			if errs := validation.IsValidLabelValue(base); len(errs) == 0 {
				allLabels[key] = base
				continue
			}
			delete(allLabels, key)
			logrus.Warnf("Removing invalid label: key - %s, value - %s, error: %s", key, value, errs)
		}
	}

	return kube.ProwJob{
		APIVersion: "prow.k8s.io/v1",
		Kind:       "ProwJob",
		ObjectMeta: metav1.ObjectMeta{
			Name:   uuid.NewV1().String(),
			Labels: allLabels,
		},
		Spec: spec,
		Status: kube.ProwJobStatus{
			StartTime: metav1.Now(),
			State:     kube.TriggeredState,
		},
	}
}

// PresubmitSpec initializes a ProwJobSpec for a given presubmit job.
func PresubmitSpec(p config.Presubmit, refs kube.Refs) kube.ProwJobSpec {
	refs.PathAlias = p.PathAlias
	refs.CloneURI = p.CloneURI
	pjs := kube.ProwJobSpec{
		Type:      kube.PresubmitJob,
		Job:       p.Name,
		Refs:      &refs,
		ExtraRefs: p.ExtraRefs,

		Report:         !p.SkipReport,
		Context:        p.Context,
		RerunCommand:   p.RerunCommand,
		MaxConcurrency: p.MaxConcurrency,

		DecorationConfig: p.DecorationConfig,
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = p.Spec
		pjs.Cluster = p.Cluster
		if pjs.Cluster == "" {
			pjs.Cluster = kube.DefaultClusterAlias
		}
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PresubmitSpec(nextP, refs))
	}
	return pjs
}

// PostsubmitSpec initializes a ProwJobSpec for a given postsubmit job.
func PostsubmitSpec(p config.Postsubmit, refs kube.Refs) kube.ProwJobSpec {
	refs.PathAlias = p.PathAlias
	refs.CloneURI = p.CloneURI
	pjs := kube.ProwJobSpec{
		Type:      kube.PostsubmitJob,
		Job:       p.Name,
		Refs:      &refs,
		ExtraRefs: p.ExtraRefs,

		MaxConcurrency: p.MaxConcurrency,

		DecorationConfig: p.DecorationConfig,
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = p.Spec
		pjs.Cluster = p.Cluster
		if pjs.Cluster == "" {
			pjs.Cluster = kube.DefaultClusterAlias
		}
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PostsubmitSpec(nextP, refs))
	}
	return pjs
}

// PeriodicSpec initializes a ProwJobSpec for a given periodic job.
func PeriodicSpec(p config.Periodic) kube.ProwJobSpec {
	pjs := kube.ProwJobSpec{
		Type:      kube.PeriodicJob,
		Job:       p.Name,
		ExtraRefs: p.ExtraRefs,

		DecorationConfig: p.DecorationConfig,
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = p.Spec
		pjs.Cluster = p.Cluster
		if pjs.Cluster == "" {
			pjs.Cluster = kube.DefaultClusterAlias
		}
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PeriodicSpec(nextP))
	}
	return pjs
}

// BatchSpec initializes a ProwJobSpec for a given batch job and ref spec.
func BatchSpec(p config.Presubmit, refs kube.Refs) kube.ProwJobSpec {
	refs.PathAlias = p.PathAlias
	refs.CloneURI = p.CloneURI
	pjs := kube.ProwJobSpec{
		Type:      kube.BatchJob,
		Job:       p.Name,
		Refs:      &refs,
		ExtraRefs: p.ExtraRefs,
		Context:   p.Context, // The Submit Queue's getCompleteBatches needs this.

		DecorationConfig: p.DecorationConfig,
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = p.Spec
		pjs.Cluster = p.Cluster
		if pjs.Cluster == "" {
			pjs.Cluster = kube.DefaultClusterAlias
		}
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, BatchSpec(nextP, refs))
	}
	return pjs
}

// PartitionActive separates the provided prowjobs into pending and triggered
// and returns them inside channels so that they can be consumed in parallel
// by different goroutines. Complete prowjobs are filtered out. Controller
// loops need to handle pending jobs first so they can conform to maximum
// concurrency requirements that different jobs may have.
func PartitionActive(pjs []kube.ProwJob) (pending, triggered chan kube.ProwJob) {
	// Size channels correctly.
	pendingCount, triggeredCount := 0, 0
	for _, pj := range pjs {
		switch pj.Status.State {
		case kube.PendingState:
			pendingCount++
		case kube.TriggeredState:
			triggeredCount++
		}
	}
	pending = make(chan kube.ProwJob, pendingCount)
	triggered = make(chan kube.ProwJob, triggeredCount)

	// Partition the jobs into the two separate channels.
	for _, pj := range pjs {
		switch pj.Status.State {
		case kube.PendingState:
			pending <- pj
		case kube.TriggeredState:
			triggered <- pj
		}
	}
	close(pending)
	close(triggered)
	return pending, triggered
}

// GetLatestProwJobs filters through the provided prowjobs and returns
// a map of jobType jobs to their latest prowjobs.
func GetLatestProwJobs(pjs []kube.ProwJob, jobType kube.ProwJobType) map[string]kube.ProwJob {
	latestJobs := make(map[string]kube.ProwJob)
	for _, j := range pjs {
		if j.Spec.Type != jobType {
			continue
		}
		name := j.Spec.Job
		if j.Status.StartTime.After(latestJobs[name].Status.StartTime.Time) {
			latestJobs[name] = j
		}
	}
	return latestJobs
}

// ProwJobFields extracts logrus fields from a prowjob useful for logging.
func ProwJobFields(pj *kube.ProwJob) logrus.Fields {
	fields := make(logrus.Fields)
	fields["name"] = pj.ObjectMeta.Name
	fields["job"] = pj.Spec.Job
	fields["type"] = pj.Spec.Type
	if len(pj.ObjectMeta.Labels[github.EventGUID]) > 0 {
		fields[github.EventGUID] = pj.ObjectMeta.Labels[github.EventGUID]
	}
	if pj.Spec.Refs != nil && len(pj.Spec.Refs.Pulls) == 1 {
		fields[github.PrLogField] = pj.Spec.Refs.Pulls[0].Number
		fields[github.RepoLogField] = pj.Spec.Refs.Repo
		fields[github.OrgLogField] = pj.Spec.Refs.Org
	}
	return fields
}

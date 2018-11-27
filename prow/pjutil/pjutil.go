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
	"github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
)

// NewProwJobWithAnnotation initializes a ProwJob out of a ProwJobSpec with annotations.
func NewProwJobWithAnnotation(spec kube.ProwJobSpec, labels, annotations map[string]string) kube.ProwJob {
	return newProwJob(spec, labels, annotations)
}

// NewProwJob initializes a ProwJob out of a ProwJobSpec.
func NewProwJob(spec kube.ProwJobSpec, labels map[string]string) kube.ProwJob {
	return newProwJob(spec, labels, nil)
}

func newProwJob(spec kube.ProwJobSpec, extraLabels, extraAnnotations map[string]string) kube.ProwJob {
	labels, annotations := decorate.LabelsAndAnnotationsForSpec(spec, extraLabels, extraAnnotations)

	return kube.ProwJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "prow.k8s.io/v1",
			Kind:       "ProwJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewV1().String(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
		Status: kube.ProwJobStatus{
			StartTime: metav1.Now(),
			State:     kube.TriggeredState,
		},
	}
}

// NewPresubmit converts a config.Presubmit into a kube.ProwJob.
// The kube.Refs are configured correctly per the pr, baseSHA.
// The eventGUID becomes a github.EventGUID label.
func NewPresubmit(pr github.PullRequest, baseSHA string, job config.Presubmit, eventGUID string) kube.ProwJob {
	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	number := pr.Number
	kr := kube.Refs{
		Org:     org,
		Repo:    repo,
		BaseRef: pr.Base.Ref,
		BaseSHA: baseSHA,
		Pulls: []kube.Pull{
			{
				Number: number,
				Author: pr.User.Login,
				SHA:    pr.Head.SHA,
			},
		},
	}
	labels := make(map[string]string)
	for k, v := range job.Labels {
		labels[k] = v
	}
	labels[github.EventGUID] = eventGUID
	return NewProwJob(PresubmitSpec(job, kr), labels)
}

// PresubmitSpec initializes a ProwJobSpec for a given presubmit job.
func PresubmitSpec(p config.Presubmit, refs kube.Refs) kube.ProwJobSpec {
	pjs := specFromJobBase(p.JobBase)
	pjs.Type = kube.PresubmitJob
	pjs.Context = p.Context
	pjs.Report = !p.SkipReport
	pjs.RerunCommand = p.RerunCommand
	pjs.Refs = completePrimaryRefs(refs, p.JobBase)

	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PresubmitSpec(nextP, refs))
	}
	return pjs
}

// PostsubmitSpec initializes a ProwJobSpec for a given postsubmit job.
func PostsubmitSpec(p config.Postsubmit, refs kube.Refs) kube.ProwJobSpec {
	pjs := specFromJobBase(p.JobBase)
	pjs.Type = kube.PostsubmitJob
	pjs.Refs = completePrimaryRefs(refs, p.JobBase)

	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PostsubmitSpec(nextP, refs))
	}
	return pjs
}

// PeriodicSpec initializes a ProwJobSpec for a given periodic job.
func PeriodicSpec(p config.Periodic) kube.ProwJobSpec {
	pjs := specFromJobBase(p.JobBase)
	pjs.Type = kube.PeriodicJob

	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PeriodicSpec(nextP))
	}
	return pjs
}

// BatchSpec initializes a ProwJobSpec for a given batch job and ref spec.
func BatchSpec(p config.Presubmit, refs kube.Refs) kube.ProwJobSpec {
	pjs := specFromJobBase(p.JobBase)
	pjs.Type = kube.BatchJob
	pjs.Context = p.Context
	pjs.Refs = completePrimaryRefs(refs, p.JobBase)

	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, BatchSpec(nextP, refs))
	}
	return pjs
}

func specFromJobBase(jb config.JobBase) kube.ProwJobSpec {
	var namespace string
	if jb.Namespace != nil {
		namespace = *jb.Namespace
	}
	return kube.ProwJobSpec{
		Job:             jb.Name,
		Agent:           kube.ProwJobAgent(jb.Agent),
		Cluster:         jb.Cluster,
		Namespace:       namespace,
		MaxConcurrency:  jb.MaxConcurrency,
		ErrorOnEviction: jb.ErrorOnEviction,

		ExtraRefs:        jb.ExtraRefs,
		DecorationConfig: jb.DecorationConfig,

		PodSpec:   jb.Spec,
		BuildSpec: jb.BuildSpec,
	}
}

func completePrimaryRefs(refs kube.Refs, jb config.JobBase) *kube.Refs {
	if jb.PathAlias != "" {
		refs.PathAlias = jb.PathAlias
	}
	if jb.CloneURI != "" {
		refs.CloneURI = jb.CloneURI
	}
	refs.SkipSubmodules = jb.SkipSubmodules
	return &refs
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

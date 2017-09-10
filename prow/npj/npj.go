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

// Package npj contains helpers for creating ProwJobs.
package npj

import (
	"fmt"
	"strconv"
	"time"

	uuid "github.com/satori/go.uuid"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

// NewProwJob initializes a ProwJob out of a ProwJobSpec.
func NewProwJob(spec kube.ProwJobSpec) kube.ProwJob {
	return kube.ProwJob{
		APIVersion: "prow.k8s.io/v1",
		Kind:       "ProwJob",
		Metadata: kube.ObjectMeta{
			Name: uuid.NewV1().String(),
		},
		Spec: spec,
		Status: kube.ProwJobStatus{
			StartTime: time.Now(),
			State:     kube.TriggeredState,
		},
	}
}

// PresubmitSpec initializes a ProwJobSpec for a given presubmit job.
func PresubmitSpec(p config.Presubmit, refs kube.Refs) kube.ProwJobSpec {
	pjs := kube.ProwJobSpec{
		Type: kube.PresubmitJob,
		Job:  p.Name,
		Refs: refs,

		Report:         !p.SkipReport,
		Context:        p.Context,
		RerunCommand:   p.RerunCommand,
		MaxConcurrency: p.MaxConcurrency,
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = *p.Spec
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PresubmitSpec(nextP, refs))
	}
	return pjs
}

// PostsubmitSpec initializes a ProwJobSpec for a given postsubmit job.
func PostsubmitSpec(p config.Postsubmit, refs kube.Refs) kube.ProwJobSpec {
	pjs := kube.ProwJobSpec{
		Type:           kube.PostsubmitJob,
		Job:            p.Name,
		Refs:           refs,
		MaxConcurrency: p.MaxConcurrency,
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = *p.Spec
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PostsubmitSpec(nextP, refs))
	}
	return pjs
}

// PeriodicSpec initializes a ProwJobSpec for a given periodic job.
func PeriodicSpec(p config.Periodic) kube.ProwJobSpec {
	pjs := kube.ProwJobSpec{
		Type: kube.PeriodicJob,
		Job:  p.Name,
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = *p.Spec
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, PeriodicSpec(nextP))
	}
	return pjs
}

// BatchSpec initializes a ProwJobSpec for a given batch job and ref spec.
func BatchSpec(p config.Presubmit, refs kube.Refs) kube.ProwJobSpec {
	pjs := kube.ProwJobSpec{
		Type:    kube.BatchJob,
		Job:     p.Name,
		Refs:    refs,
		Context: p.Context, // The Submit Queue's getCompleteBatches needs this.
	}
	pjs.Agent = kube.ProwJobAgent(p.Agent)
	if pjs.Agent == kube.KubernetesAgent {
		pjs.PodSpec = *p.Spec
	}
	for _, nextP := range p.RunAfterSuccess {
		pjs.RunAfterSuccess = append(pjs.RunAfterSuccess, BatchSpec(nextP, refs))
	}
	return pjs
}

// ProwJobToPod converts a ProwJob to a Pod that will run the tests.
func ProwJobToPod(pj kube.ProwJob, buildID string) *kube.Pod {
	env := EnvForSpec(pj.Spec)
	env["BUILD_NUMBER"] = buildID

	spec := pj.Spec.PodSpec
	spec.RestartPolicy = "Never"

	// Set environment variables in each container in the pod spec. We don't
	// want to update the spec in place, since that will update the ProwJob
	// spec. Instead, create a copy.
	spec.Containers = []kube.Container{}
	for i := range pj.Spec.PodSpec.Containers {
		spec.Containers = append(spec.Containers, pj.Spec.PodSpec.Containers[i])
		spec.Containers[i].Name = fmt.Sprintf("%s-%d", pj.Metadata.Name, i)
		spec.Containers[i].Env = append(spec.Containers[i].Env, kubeEnv(env)...)
	}
	return &kube.Pod{
		Metadata: kube.ObjectMeta{
			Name: pj.Metadata.Name,
			Labels: map[string]string{
				kube.CreatedByProw: "true",
				"type":             string(pj.Spec.Type),
				"job":              pj.Spec.Job,
			},
		},
		Spec: spec,
	}
}

// kubeEnv transforms a mapping of environment variables
// into their serialized form for a PodSpec
func kubeEnv(environment map[string]string) []kube.EnvVar {
	var kubeEnvironment []kube.EnvVar
	for key, value := range environment {
		kubeEnvironment = append(kubeEnvironment, kube.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	return kubeEnvironment
}

// EnvForSpec returns a mapping of environment variables
// to their values that should be available for a job spec
func EnvForSpec(spec kube.ProwJobSpec) map[string]string {
	env := map[string]string{
		"JOB_NAME": spec.Job,
	}

	if spec.Type == kube.PeriodicJob {
		return env
	}
	env["REPO_OWNER"] = spec.Refs.Org
	env["REPO_NAME"] = spec.Refs.Repo
	env["PULL_BASE_REF"] = spec.Refs.BaseRef
	env["PULL_BASE_SHA"] = spec.Refs.BaseSHA
	env["PULL_REFS"] = spec.Refs.String()

	if spec.Type == kube.PostsubmitJob || spec.Type == kube.BatchJob {
		return env
	}
	env["PULL_NUMBER"] = strconv.Itoa(spec.Refs.Pulls[0].Number)
	env["PULL_PULL_SHA"] = spec.Refs.Pulls[0].SHA
	return env
}

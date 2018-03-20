/*
Copyright 2018 The Kubernetes Authors.

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

package decorate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/test-infra/prow/kube"
)

const (
	JobNameEnv   = "JOB_NAME"
	JobSpecEnv   = "JOB_SPEC"
	JobTypeEnv   = "JOB_TYPE"
	ProwJobIdEnv = "PROW_JOB_ID"

	BuildIdEnv = "BUILD_ID"
	// Deprecated, will be removed in the future.
	ProwBuildIdEnv = "BUILD_NUMBER"
	// Deprecated, will be removed in the future.
	JenkinsBuildIdEnv = "buildId"

	RepoOwnerEnv   = "REPO_OWNER"
	RepoNameEnv    = "REPO_NAME"
	PullBaseRefEnv = "PULL_BASE_REF"
	PullBaseShaEnv = "PULL_BASE_SHA"
	PullRefsEnv    = "PULL_REFS"
	PullNumberEnv  = "PULL_NUMBER"
	PullPullShaEnv = "PULL_PULL_SHA"
)

// EnvForSpec returns a mapping of environment variables
// to their values that should be available for a job spec
func EnvForSpec(spec JobSpec) (map[string]string, error) {
	env := map[string]string{
		JobNameEnv:   spec.Job,
		BuildIdEnv:   spec.BuildId,
		ProwJobIdEnv: spec.ProwJobId,
		JobTypeEnv:   string(spec.Type),
	}

	// for backwards compatibility, we provide the build ID
	// in both $BUILD_ID and $BUILD_NUMBER for Prow agents
	// and in both $buildId and $BUILD_NUMBER for Jenkins
	if spec.agent == kube.KubernetesAgent {
		env[ProwBuildIdEnv] = spec.BuildId
	} else if spec.agent == kube.JenkinsAgent {
		env[JenkinsBuildIdEnv] = spec.BuildId
	}

	raw, err := json.Marshal(spec)
	if err != nil {
		return env, fmt.Errorf("failed to marshal job spec: %v", err)
	}
	env[JobSpecEnv] = string(raw)

	if spec.Type == kube.PeriodicJob {
		return env, nil
	}
	env[RepoOwnerEnv] = spec.Refs.Org
	env[RepoNameEnv] = spec.Refs.Repo
	env[PullBaseRefEnv] = spec.Refs.BaseRef
	env[PullBaseShaEnv] = spec.Refs.BaseSHA
	env[PullRefsEnv] = spec.Refs.String()

	if spec.Type == kube.PostsubmitJob || spec.Type == kube.BatchJob {
		return env, nil
	}
	env[PullNumberEnv] = strconv.Itoa(spec.Refs.Pulls[0].Number)
	env[PullPullShaEnv] = spec.Refs.Pulls[0].SHA
	return env, nil
}

func EnvForType(jobType kube.ProwJobType) []string {
	baseEnv := []string{JobNameEnv, JobSpecEnv, JobTypeEnv, ProwJobIdEnv, BuildIdEnv, ProwBuildIdEnv, JenkinsBuildIdEnv}
	refsEnv := []string{RepoOwnerEnv, RepoNameEnv, PullBaseRefEnv, PullBaseShaEnv, PullRefsEnv}
	pullEnv := []string{PullNumberEnv, PullPullShaEnv}

	switch jobType {
	case kube.PeriodicJob:
		return baseEnv
	case kube.PostsubmitJob, kube.BatchJob:
		return append(baseEnv, refsEnv...)
	case kube.PresubmitJob:
		return append(append(baseEnv, refsEnv...), pullEnv...)
	default:
		return []string{}
	}
}

// ProwJobToPod converts a ProwJob to a Pod that will run the tests.
func ProwJobToPod(pj kube.ProwJob, buildID string) (*v1.Pod, error) {
	if pj.Spec.PodSpec == nil {
		return nil, fmt.Errorf("prowjob %q lacks a pod spec", pj.Name)
	}

	env, err := EnvForSpec(NewJobSpec(pj.Spec, buildID, pj.Name))
	if err != nil {
		return nil, err
	}

	spec := pj.Spec.PodSpec.DeepCopy()
	spec.RestartPolicy = "Never"

	for i := range spec.InitContainers {
		if spec.InitContainers[i].Name == "" {
			spec.InitContainers[i].Name = fmt.Sprintf("%s-%d", pj.ObjectMeta.Name, i)
		}
		spec.InitContainers[i].Env = append(spec.InitContainers[i].Env, kubeEnv(env)...)
	}
	for i := range spec.Containers {
		if spec.Containers[i].Name == "" {
			spec.Containers[i].Name = fmt.Sprintf("%s-%d", pj.ObjectMeta.Name, i)
		}
		spec.Containers[i].Env = append(spec.Containers[i].Env, kubeEnv(env)...)
	}
	podLabels := make(map[string]string)
	for k, v := range pj.ObjectMeta.Labels {
		podLabels[k] = v
	}
	podLabels[kube.CreatedByProw] = "true"
	podLabels[kube.ProwJobTypeLabel] = string(pj.Spec.Type)
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pj.ObjectMeta.Name,
			Labels: podLabels,
			Annotations: map[string]string{
				kube.ProwJobAnnotation: pj.Spec.Job,
			},
		},
		Spec: *spec,
	}, nil
}

// kubeEnv transforms a mapping of environment variables
// into their serialized form for a PodSpec, sorting by
// the name of the env vars
func kubeEnv(environment map[string]string) []v1.EnvVar {
	var keys []string
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var kubeEnvironment []v1.EnvVar
	for _, key := range keys {
		kubeEnvironment = append(kubeEnvironment, v1.EnvVar{
			Name:  key,
			Value: environment[key],
		})
	}

	return kubeEnvironment
}

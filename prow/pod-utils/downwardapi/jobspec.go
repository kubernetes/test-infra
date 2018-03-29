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

package downwardapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"k8s.io/test-infra/prow/kube"
)

// JobSpec is the full downward API that we expose to
// jobs that realize a ProwJob. We will provide this
// data to jobs with environment variables in two ways:
//  - the full spec, in serialized JSON in one variable
//  - individual fields of the spec in their own variables
type JobSpec struct {
	Type      kube.ProwJobType `json:"type,omitempty"`
	Job       string           `json:"job,omitempty"`
	BuildId   string           `json:"buildid,omitempty"`
	ProwJobId string           `json:"prowjobid,omitempty"`

	Refs kube.Refs `json:"refs,omitempty"`

	// we need to keep track of the agent until we
	// migrate everyone away from using the $BUILD_NUMBER
	// environment variable
	agent kube.ProwJobAgent
}

func NewJobSpec(spec kube.ProwJobSpec, buildId, prowJobId string) JobSpec {
	refs := kube.Refs{}
	if spec.Refs != nil {
		refs = *spec.Refs
	}

	return JobSpec{
		Type:      spec.Type,
		Job:       spec.Job,
		BuildId:   buildId,
		ProwJobId: prowJobId,
		Refs:      refs,
		agent:     spec.Agent,
	}
}

// ResolveSpecFromEnv will determine the Refs being
// tested in by parsing Prow environment variable contents
func ResolveSpecFromEnv() (*JobSpec, error) {
	specEnv, ok := os.LookupEnv(JobSpecEnv)
	if !ok {
		return nil, fmt.Errorf("$%s unset", JobSpecEnv)
	}

	spec := &JobSpec{}
	if err := json.Unmarshal([]byte(specEnv), spec); err != nil {
		return nil, fmt.Errorf("malformed $%s: %v", JobSpecEnv, err)
	}

	return spec, nil
}

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

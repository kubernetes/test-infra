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

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

// JobSpec is the full downward API that we expose to
// jobs that realize a ProwJob. We will provide this
// data to jobs with environment variables in two ways:
//  - the full spec, in serialized JSON in one variable
//  - individual fields of the spec in their own variables
type JobSpec struct {
	Type      prowapi.ProwJobType `json:"type,omitempty"`
	Job       string              `json:"job,omitempty"`
	BuildID   string              `json:"buildid,omitempty"`
	ProwJobID string              `json:"prowjobid,omitempty"`

	// refs & extra_refs from the full spec
	Refs      *prowapi.Refs  `json:"refs,omitempty"`
	ExtraRefs []prowapi.Refs `json:"extra_refs,omitempty"`

	DecorationConfig *prowapi.DecorationConfig `json:"decoration_config,omitempty"`

	// we need to keep track of the agent until we
	// migrate everyone away from using the $BUILD_NUMBER
	// environment variable
	agent prowapi.ProwJobAgent
}

// NewJobSpec converts a prowapi.ProwJobSpec invocation into a JobSpec
func NewJobSpec(spec prowapi.ProwJobSpec, buildID, prowJobID string) JobSpec {
	return JobSpec{
		Type:             spec.Type,
		Job:              spec.Job,
		BuildID:          buildID,
		ProwJobID:        prowJobID,
		Refs:             spec.Refs,
		ExtraRefs:        spec.ExtraRefs,
		DecorationConfig: spec.DecorationConfig,
		agent:            spec.Agent,
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
	// ci represents whether the current environment is a CI environment
	ci = "CI"

	// JobSpecEnv is the name that contains JobSpec marshaled into a string.
	JobSpecEnv = "JOB_SPEC"

	jobNameEnv   = "JOB_NAME"
	jobTypeEnv   = "JOB_TYPE"
	prowJobIDEnv = "PROW_JOB_ID"

	buildIDEnv     = "BUILD_ID"
	prowBuildIDEnv = "BUILD_NUMBER" // Deprecated, will be removed in the future.

	repoOwnerEnv   = "REPO_OWNER"
	repoNameEnv    = "REPO_NAME"
	pullBaseRefEnv = "PULL_BASE_REF"
	pullBaseShaEnv = "PULL_BASE_SHA"
	pullRefsEnv    = "PULL_REFS"
	pullNumberEnv  = "PULL_NUMBER"
	pullPullShaEnv = "PULL_PULL_SHA"
)

// EnvForSpec returns a mapping of environment variables
// to their values that should be available for a job spec
func EnvForSpec(spec JobSpec) (map[string]string, error) {
	env := map[string]string{
		ci:           "true",
		jobNameEnv:   spec.Job,
		buildIDEnv:   spec.BuildID,
		prowJobIDEnv: spec.ProwJobID,
		jobTypeEnv:   string(spec.Type),
	}

	// for backwards compatibility, we provide the build ID
	// in both $BUILD_ID and $BUILD_NUMBER for Prow agents
	// and in both $buildId and $BUILD_NUMBER for Jenkins
	if spec.agent == prowapi.KubernetesAgent {
		env[prowBuildIDEnv] = spec.BuildID
	}

	raw, err := json.Marshal(spec)
	if err != nil {
		return env, fmt.Errorf("failed to marshal job spec: %v", err)
	}
	env[JobSpecEnv] = string(raw)

	if spec.Type == prowapi.PeriodicJob {
		return env, nil
	}

	env[repoOwnerEnv] = spec.Refs.Org
	env[repoNameEnv] = spec.Refs.Repo
	env[pullBaseRefEnv] = spec.Refs.BaseRef
	env[pullBaseShaEnv] = spec.Refs.BaseSHA
	env[pullRefsEnv] = spec.Refs.String()

	if spec.Type == prowapi.PostsubmitJob || spec.Type == prowapi.BatchJob {
		return env, nil
	}

	env[pullNumberEnv] = strconv.Itoa(spec.Refs.Pulls[0].Number)
	env[pullPullShaEnv] = spec.Refs.Pulls[0].SHA
	return env, nil
}

// EnvForType returns the slice of environment variables to export for jobType
func EnvForType(jobType prowapi.ProwJobType) []string {
	baseEnv := []string{ci, jobNameEnv, JobSpecEnv, jobTypeEnv, prowJobIDEnv, buildIDEnv, prowBuildIDEnv}
	refsEnv := []string{repoOwnerEnv, repoNameEnv, pullBaseRefEnv, pullBaseShaEnv, pullRefsEnv}
	pullEnv := []string{pullNumberEnv, pullPullShaEnv}

	switch jobType {
	case prowapi.PeriodicJob:
		return baseEnv
	case prowapi.PostsubmitJob, prowapi.BatchJob:
		return append(baseEnv, refsEnv...)
	case prowapi.PresubmitJob:
		return append(append(baseEnv, refsEnv...), pullEnv...)
	default:
		return []string{}
	}
}

// getRevisionFromRef returns a ref or sha from a refs object
func getRevisionFromRef(refs *prowapi.Refs) string {
	if len(refs.Pulls) > 0 {
		return refs.Pulls[0].SHA
	}

	if refs.BaseSHA != "" {
		return refs.BaseSHA
	}

	return refs.BaseRef
}

// GetRevisionFromSpec returns a main ref or sha from a spec object
func GetRevisionFromSpec(spec *JobSpec) string {
	if spec.Refs != nil {
		return getRevisionFromRef(spec.Refs)
	} else if len(spec.ExtraRefs) > 0 {
		return getRevisionFromRef(&spec.ExtraRefs[0])
	}
	return ""
}

// MainRefs determines the main refs under test, if there are any
func (s *JobSpec) MainRefs() *prowapi.Refs {
	if s.Refs != nil {
		return s.Refs
	}
	if len(s.ExtraRefs) > 0 {
		return &s.ExtraRefs[0]
	}
	return nil
}

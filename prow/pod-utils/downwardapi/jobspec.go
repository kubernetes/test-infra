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
	"reflect"
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

// JobSpec is the full downward API that we expose to
// jobs that realize a ProwJob. We will provide this
// data to jobs with environment variables in two ways:
//   - the full spec, in serialized JSON in one variable
//   - individual fields of the spec in their own variables
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
		return nil, fmt.Errorf("malformed $%s: %w", JobSpecEnv, err)
	}

	return spec, nil
}

const (
	// ci represents whether the current environment is a CI environment
	CI = "CI"

	// JobSpecEnv is the name that contains JobSpec marshaled into a string.
	JobSpecEnv = "JOB_SPEC"

	JobNameEnv   = "JOB_NAME"
	JobTypeEnv   = "JOB_TYPE"
	ProwJobIDEnv = "PROW_JOB_ID"

	BuildIDEnv     = "BUILD_ID"
	ProwBuildIDEnv = "BUILD_NUMBER" // Deprecated, will be removed in the future.

	RepoOwnerEnv   = "REPO_OWNER"
	RepoNameEnv    = "REPO_NAME"
	PullBaseRefEnv = "PULL_BASE_REF"
	PullBaseShaEnv = "PULL_BASE_SHA"
	PullRefsEnv    = "PULL_REFS"
	PullNumberEnv  = "PULL_NUMBER"
	PullPullShaEnv = "PULL_PULL_SHA"
	PullHeadRefEnv = "PULL_HEAD_REF"
	PullTitleEnv   = "PULL_TITLE"
)

// EnvForSpec returns a mapping of environment variables
// to their values that should be available for a job spec
func EnvForSpec(spec JobSpec) (map[string]string, error) {
	env := map[string]string{
		CI:           "true",
		JobNameEnv:   spec.Job,
		BuildIDEnv:   spec.BuildID,
		ProwJobIDEnv: spec.ProwJobID,
		JobTypeEnv:   string(spec.Type),
	}

	// for backwards compatibility, we provide the build ID
	// in both $BUILD_ID and $BUILD_NUMBER for Prow agents
	// and in both $buildId and $BUILD_NUMBER for Jenkins
	if spec.agent == prowapi.KubernetesAgent {
		env[ProwBuildIDEnv] = spec.BuildID
	}

	raw, err := json.Marshal(spec)
	if err != nil {
		return env, fmt.Errorf("failed to marshal job spec: %w", err)
	}
	env[JobSpecEnv] = string(raw)

	if spec.Type == prowapi.PeriodicJob {
		return env, nil
	}

	env[RepoOwnerEnv] = spec.Refs.Org
	env[RepoNameEnv] = spec.Refs.Repo
	env[PullBaseRefEnv] = spec.Refs.BaseRef
	env[PullBaseShaEnv] = spec.Refs.BaseSHA
	env[PullRefsEnv] = spec.Refs.String()

	if spec.Type == prowapi.PostsubmitJob || spec.Type == prowapi.BatchJob {
		return env, nil
	}

	env[PullNumberEnv] = strconv.Itoa(spec.Refs.Pulls[0].Number)
	env[PullPullShaEnv] = spec.Refs.Pulls[0].SHA
	env[PullHeadRefEnv] = spec.Refs.Pulls[0].HeadRef
	env[PullTitleEnv] = spec.Refs.Pulls[0].Title

	return env, nil
}

// EnvForType returns the slice of environment variables to export for jobType
func EnvForType(jobType prowapi.ProwJobType) []string {
	baseEnv := []string{CI, JobNameEnv, JobSpecEnv, JobTypeEnv, ProwJobIDEnv, BuildIDEnv, ProwBuildIDEnv}
	refsEnv := []string{RepoOwnerEnv, RepoNameEnv, PullBaseRefEnv, PullBaseShaEnv, PullRefsEnv}
	pullEnv := []string{PullNumberEnv, PullPullShaEnv, PullHeadRefEnv, PullTitleEnv}

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
	if refs == nil {
		return ""
	}
	if len(refs.Pulls) > 0 {
		return refs.Pulls[0].SHA
	}

	if refs.BaseSHA != "" {
		return refs.BaseSHA
	}

	return refs.BaseRef
}

// GetRevisionFromSpec returns a main ref or sha from a spec object
func GetRevisionFromSpec(jobSpec *JobSpec) string {
	return GetRevisionFromRefs(jobSpec.Refs, jobSpec.ExtraRefs)
}

func GetRevisionFromRefs(refs *prowapi.Refs, extra []prowapi.Refs) string {
	return getRevisionFromRef(mainRefs(refs, extra))
}

func mainRefs(refs *prowapi.Refs, extra []prowapi.Refs) *prowapi.Refs {
	if refs != nil {
		return refs
	}
	if len(extra) > 0 {
		return &extra[0]
	}
	return nil
}

func PjToStarted(pj *prowapi.ProwJob, cloneRecords []clone.Record) metadata.Started {
	return refsToStarted(pj.Spec.Refs, pj.Spec.ExtraRefs, cloneRecords, pj.Status.StartTime.Unix())
}

func SpecToStarted(spec *JobSpec, cloneRecords []clone.Record) metadata.Started {
	return refsToStarted(spec.Refs, spec.ExtraRefs, cloneRecords, time.Now().Unix())
}

// refsToStarted translate refs into a Started struct
// optionally overwrite RepoVersion with provided cloneRecords
func refsToStarted(refs *prowapi.Refs, extraRefs []prowapi.Refs, cloneRecords []clone.Record, startTime int64) metadata.Started {
	var version string

	started := metadata.Started{
		Timestamp: startTime,
	}

	if mainRefs := mainRefs(refs, extraRefs); mainRefs != nil {
		version = shaForRefs(*mainRefs, cloneRecords)
	}

	if version == "" {
		version = GetRevisionFromRefs(refs, extraRefs)
	}

	started.DeprecatedRepoVersion = version
	started.RepoCommit = version

	if refs != nil && len(refs.Pulls) > 0 {
		started.Pull = strconv.Itoa(refs.Pulls[0].Number)
	}

	started.Repos = map[string]string{}

	if refs != nil {
		started.Repos[refs.Org+"/"+refs.Repo] = refs.String()
	}
	for _, ref := range extraRefs {
		started.Repos[ref.Org+"/"+ref.Repo] = ref.String()
	}

	return started
}

// shaForRefs finds the resolved SHA after cloning and merging for the given refs
func shaForRefs(refs prowapi.Refs, cloneRecords []clone.Record) string {
	for _, record := range cloneRecords {
		if reflect.DeepEqual(refs, record.Refs) {
			return record.FinalSHA
		}
	}
	return ""
}

// InCI returns true if the CI environment variable is not empty
func InCI() bool {
	return os.Getenv(CI) != ""
}

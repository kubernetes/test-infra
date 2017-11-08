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

package pjutil

import (
	"encoding/json"
	"strconv"

	"fmt"

	"k8s.io/test-infra/prow/kube"
)

// JobSpec is the full downward API that we expose to
// jobs that realize a ProwJob. We will provide this
// data to jobs with environment variables in two ways:
//  - the full spec, in serialized JSON in one variable
//  - individual fields of the spec in their own variables
type JobSpec struct {
	Type    kube.ProwJobType `json:"type,omitempty"`
	Job     string           `json:"job,omitempty"`
	BuildId string           `json:"buildid,omitempty"`

	Refs kube.Refs `json:"refs,omitempty"`

	// we need to keep track of the agent until we
	// migrate everyone away from using the $BUILD_NUMBER
	// environment variable
	agent kube.ProwJobAgent
}

func NewJobSpec(spec kube.ProwJobSpec, buildId string) JobSpec {
	return JobSpec{
		Type:    spec.Type,
		Job:     spec.Job,
		BuildId: buildId,
		Refs:    spec.Refs,
		agent:   spec.Agent,
	}
}

// EnvForSpec returns a mapping of environment variables
// to their values that should be available for a job spec
func EnvForSpec(spec JobSpec) (map[string]string, error) {
	env := map[string]string{
		"JOB_NAME": spec.Job,
		"BUILD_ID": spec.BuildId,
		"JOB_TYPE": string(spec.Type),
	}

	// for backwards compatibility, we provide the build ID
	// in both $BUILD_ID and $BUILD_NUMBER for Prow agents
	// and in both $buildId and $BUILD_NUMBER for Jenkins
	if spec.agent == kube.KubernetesAgent {
		env["BUILD_NUMBER"] = spec.BuildId
	} else if spec.agent == kube.JenkinsAgent {
		env["buildId"] = spec.BuildId
	}

	raw, err := json.Marshal(spec)
	if err != nil {
		return env, fmt.Errorf("failed to marshal job spec: %v", err)
	}
	env["JOB_SPEC"] = string(raw)

	if spec.Type == kube.PeriodicJob {
		return env, nil
	}
	env["REPO_OWNER"] = spec.Refs.Org
	env["REPO_NAME"] = spec.Refs.Repo
	env["PULL_BASE_REF"] = spec.Refs.BaseRef
	env["PULL_BASE_SHA"] = spec.Refs.BaseSHA
	env["PULL_REFS"] = spec.Refs.String()

	if spec.Type == kube.PostsubmitJob || spec.Type == kube.BatchJob {
		return env, nil
	}
	env["PULL_NUMBER"] = strconv.Itoa(spec.Refs.Pulls[0].Number)
	env["PULL_PULL_SHA"] = spec.Refs.Pulls[0].SHA
	return env, nil
}

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

package decorate

import (
	"encoding/json"
	"fmt"
	"os"

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

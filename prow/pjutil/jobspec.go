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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"

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
	return JobSpec{
		Type:      spec.Type,
		Job:       spec.Job,
		BuildId:   buildId,
		ProwJobId: prowJobId,
		Refs:      spec.Refs,
		agent:     spec.Agent,
	}
}

// GetBuildID calls out to `tot` in order
// to vend build identifier for the job
func GetBuildID(name, totURL string) (string, error) {
	var err error
	url, err := url.Parse(totURL)
	if err != nil {
		return "", fmt.Errorf("invalid tot url: %v", err)
	}
	url.Path = path.Join(url.Path, "vend", name)
	sleep := 100 * time.Millisecond
	for retries := 0; retries < 10; retries++ {
		if retries > 0 {
			time.Sleep(sleep)
			sleep = sleep * 2
		}
		var resp *http.Response
		resp, err = http.Get(url.String())
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = fmt.Errorf("got unexpected response from tot: %v", resp.Status)
			continue
		}
		var buf []byte
		buf, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			return string(buf), nil
		}
		return "", err
	}
	return "", err
}

// EnvForSpec returns a mapping of environment variables
// to their values that should be available for a job spec
func EnvForSpec(spec JobSpec) (map[string]string, error) {
	env := map[string]string{
		"JOB_NAME":    spec.Job,
		"BUILD_ID":    spec.BuildId,
		"PROW_JOB_ID": spec.ProwJobId,
		"JOB_TYPE":    string(spec.Type),
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

// ResolveSpecFromEnv will determine the Refs being
// tested in by parsing Prow environment variable contents
func ResolveSpecFromEnv() (*JobSpec, error) {
	specEnv, ok := os.LookupEnv("JOB_SPEC")
	if !ok {
		return nil, errors.New("$JOB_SPEC unset")
	}

	spec := &JobSpec{}
	if err := json.Unmarshal([]byte(specEnv), spec); err != nil {
		return nil, fmt.Errorf("malformed $JOB_SPEC: %v", err)
	}

	return spec, nil
}

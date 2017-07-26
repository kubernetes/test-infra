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

package kube

import (
	"fmt"
	"strings"
	"time"
)

type ProwJobType string
type ProwJobState string
type ProwJobAgent string

const (
	PresubmitJob  ProwJobType = "presubmit"
	PostsubmitJob             = "postsubmit"
	PeriodicJob               = "periodic"
	BatchJob                  = "batch"
)

const (
	TriggeredState ProwJobState = "triggered"
	PendingState                = "pending"
	SuccessState                = "success"
	FailureState                = "failure"
	AbortedState                = "aborted"
	ErrorState                  = "error"
)

const (
	KubernetesAgent ProwJobAgent = "kubernetes"
	JenkinsAgent                 = "jenkins"
)

type ProwJob struct {
	APIVersion string        `json:"apiVersion,omitempty"`
	Kind       string        `json:"kind,omitempty"`
	Metadata   ObjectMeta    `json:"metadata,omitempty"`
	Spec       ProwJobSpec   `json:"spec,omitempty"`
	Status     ProwJobStatus `json:"status,omitempty"`
}

type ProwJobSpec struct {
	Type  ProwJobType  `json:"type,omitempty"`
	Agent ProwJobAgent `json:"agent,omitempty"`
	Job   string       `json:"job,omitempty"`
	Refs  Refs         `json:"refs,omitempty"`

	Report         bool   `json:"report,omitempty"`
	Context        string `json:"context,omitempty"`
	RerunCommand   string `json:"rerun_command,omitempty"`
	MaxConcurrency int    `json:"max_concurrency,omitempty"`

	PodSpec PodSpec `json:"pod_spec,omitempty"`

	RunAfterSuccess []ProwJobSpec `json:"run_after_success,omitempty"`
}

type ProwJobStatus struct {
	StartTime       time.Time    `json:"startTime,omitempty"`
	CompletionTime  time.Time    `json:"completionTime,omitempty"`
	State           ProwJobState `json:"state,omitempty"`
	Description     string       `json:"description,omitempty"`
	URL             string       `json:"url,omitempty"`
	PodName         string       `json:"pod_name,omitempty"`
	BuildID         string       `json:"build_id,omitempty"`
	JenkinsQueueURL string       `json:"jenkins_queue_url,omitempty"`
	JenkinsEnqueued bool         `json:"jenkins_enqueued,omitempty"`
	// TODO(spxtr): Drop this in favor of just BuildID.
	JenkinsBuildID string `json:"jenkins_build_id,omitempty"`
}

func (j *ProwJob) Complete() bool {
	return !j.Status.CompletionTime.IsZero()
}

type Pull struct {
	Number int    `json:"number,omitempty"`
	Author string `json:"author,omitempty"`
	SHA    string `json:"sha,omitempty"`
}

type Refs struct {
	Org  string `json:"org,omitempty"`
	Repo string `json:"repo,omitempty"`

	BaseRef string `json:"base_ref,omitempty"`
	BaseSHA string `json:"base_sha,omitempty"`

	Pulls []Pull `json:"pulls,omitempty"`
}

func (r Refs) String() string {
	rs := []string{fmt.Sprintf("%s:%s", r.BaseRef, r.BaseSHA)}
	for _, pull := range r.Pulls {
		rs = append(rs, fmt.Sprintf("%d:%s", pull.Number, pull.SHA))
	}
	return strings.Join(rs, ",")
}

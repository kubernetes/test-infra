/*
Copyright 2023 The Kubernetes Authors.

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

package resultstore

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
)

type Payload struct {
	Job       *v1.ProwJob
	Started   *metadata.Started
	Finished  *metadata.Finished
	Files     []*resultstore.File
	ProjectID string
}

func (p *Payload) invocation() (*resultstore.Invocation, error) {
	if p.Job == nil {
		return nil, errors.New("internal error: Payload.Job is nil")
	}
	i := &resultstore.Invocation{
		Id:                   p.invocationID(),
		StatusAttributes:     p.invocationStatusAttributes(),
		Timing:               p.invocationTiming(),
		InvocationAttributes: p.invocationAttributes(),
		WorkspaceInfo:        p.workspaceInfo(),
		Properties:           p.invocationProperties(),
		Files:                p.Files,
	}
	return i, nil
}

func (p *Payload) invocationID() *resultstore.Invocation_Id {
	return &resultstore.Invocation_Id{
		// Name is a v4 UUID set in pjutil.go.
		InvocationId: p.Job.Name,
	}
}

func (p *Payload) invocationStatusAttributes() *resultstore.StatusAttributes {
	status := resultstore.Status_TOOL_FAILED
	switch p.Job.Status.State {
	case v1.SuccessState:
		status = resultstore.Status_PASSED
	case v1.FailureState:
		status = resultstore.Status_FAILED
	case v1.AbortedState:
		status = resultstore.Status_CANCELLED
	case v1.ErrorState:
		status = resultstore.Status_INCOMPLETE
	}
	return &resultstore.StatusAttributes{
		Status: status,
	}
}

func (p *Payload) invocationTiming() *resultstore.Timing {
	start := p.Job.Status.StartTime.Time
	var duration time.Duration
	if p.Job.Status.CompletionTime != nil {
		duration = p.Job.Status.CompletionTime.Time.Sub(start)
	}
	return &resultstore.Timing{
		StartTime: &timestamppb.Timestamp{
			Seconds: start.Unix(),
		},
		Duration: &durationpb.Duration{
			Seconds: int64(duration.Seconds()),
		},
	}
}

func (p *Payload) invocationAttributes() *resultstore.InvocationAttributes {
	return &resultstore.InvocationAttributes{
		// TODO: ProjectID might be assigned directly from the GCS
		// BucketAttrs.ProjectNumber; requires a raw GCS client.
		ProjectId:   p.ProjectID,
		Labels:      []string{"prow"},
		Description: descriptionFromLabels(p.Job.Labels),
	}
}

func descriptionFromLabels(labels map[string]string) string {
	jt := labels[kube.ProwJobTypeLabel]
	repo := labels[kube.RepoLabel]
	pull := labels[kube.PullLabel]
	if ps := labels[kube.GerritPatchset]; ps != "" {
		pull += "/" + ps
	}
	buildID := labels[kube.ProwBuildIDLabel]
	job := labels[kube.ContextAnnotation]
	return fmt.Sprintf("%s for %s/%s/%s/%s", jt, repo, pull, buildID, job)
}

func (p *Payload) workspaceInfo() *resultstore.WorkspaceInfo {
	return &resultstore.WorkspaceInfo{
		CommandLines: commandLines(p.Job),
	}
}

func commandLines(pj *v1.ProwJob) []*resultstore.CommandLine {
	var cl []*resultstore.CommandLine
	if pj.Spec.PodSpec != nil {
		for _, c := range pj.Spec.PodSpec.Containers {
			cl = append(cl, &resultstore.CommandLine{
				Label:   c.Name,
				Args:    c.Args,
				Command: strings.Join(c.Command, " "),
			})
		}
	}
	return cl
}

func (p *Payload) invocationProperties() []*resultstore.Property {
	ps := []*resultstore.Property{
		{
			Key:   "Instance",
			Value: p.Job.Status.BuildID,
		},
		{
			Key:   "Job",
			Value: p.Job.Spec.Job,
		},
		{
			Key:   "Prow_Dashboard_URL",
			Value: p.Job.Status.URL,
		},
	}
	if p.Job.Spec.Refs != nil {
		ps = append(ps, &resultstore.Property{
			Key:   "Repo",
			Value: p.Job.Spec.Refs.RepoLink,
		})
	}
	if p.Started != nil {
		ps = append(ps, &resultstore.Property{
			Key:   "Commit",
			Value: p.Started.RepoCommit,
		})
		for _, branch := range p.Started.Repos {
			ps = append(ps, &resultstore.Property{
				Key:   "Branch",
				Value: branch,
			})
		}
	}
	if p.Job.Spec.PodSpec != nil {
		for _, c := range p.Job.Spec.PodSpec.Containers {
			for _, e := range c.Env {
				ps = append(ps, &resultstore.Property{
					Key:   "Env",
					Value: e.Name + "=" + e.Value,
				})
			}
		}
	}
	return ps
}

const defaultConfigurationId = "default"

func (p *Payload) defaultConfiguration() *resultstore.Configuration {
	return &resultstore.Configuration{
		Id: &resultstore.Configuration_Id{
			ConfigurationId: defaultConfigurationId,
		},
	}
}

func (p *Payload) targetID() string {
	if p.Job == nil {
		return "Unknown"
	}
	return p.Job.Spec.Job
}

func (p *Payload) overallTarget() *resultstore.Target {
	return &resultstore.Target{
		Id: &resultstore.Target_Id{
			TargetId: p.targetID(),
		},
		TargetAttributes: &resultstore.TargetAttributes{
			Type: resultstore.TargetType_TEST,
		},
		Visible: true,
	}
}

func (p *Payload) configuredTarget() *resultstore.ConfiguredTarget {
	return &resultstore.ConfiguredTarget{
		Id: &resultstore.ConfiguredTarget_Id{
			TargetId:        p.targetID(),
			ConfigurationId: defaultConfigurationId,
		},
	}
}

func (p *Payload) overallAction() *resultstore.Action {
	return &resultstore.Action{
		Id: &resultstore.Action_Id{
			TargetId:        p.targetID(),
			ConfigurationId: defaultConfigurationId,
			ActionId:        "overall",
		},
		Timing: p.metadataTiming(),
		// TODO: What else if anything is required here?
		ActionType: &resultstore.Action_TestAction{},
	}
}

func (p *Payload) metadataTiming() *resultstore.Timing {
	if p.Started == nil {
		return nil
	}
	start := p.Started.Timestamp
	var duration int64
	switch {
	case p.Finished != nil:
		duration = *p.Finished.Timestamp - start
	case p.Job != nil && p.Job.Status.CompletionTime != nil:
		duration = p.Job.Status.CompletionTime.Unix() - start
	default:
		return nil
	}
	return &resultstore.Timing{
		StartTime: &timestamppb.Timestamp{
			Seconds: start,
		},
		Duration: &durationpb.Duration{
			Seconds: duration,
		},
	}
}

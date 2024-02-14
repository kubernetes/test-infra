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
	"slices"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	gerrit "k8s.io/test-infra/prow/gerrit/source"
	"k8s.io/test-infra/prow/kube"
)

type Payload struct {
	Job       *v1.ProwJob
	Started   *metadata.Started
	Finished  *metadata.Finished
	Files     []*resultstore.File
	ProjectID string
}

// InvocationID returns the ResultStore InvocationId.
func (p *Payload) InvocationID() (string, error) {
	if p.Job == nil {
		return "", errors.New("internal error: pj is nil")
	}
	// Name is a v4 UUID set in pjutil.go.
	return p.Job.Name, nil
}

// Invocation returns an Invocation suitable to upload to ResultStore.
func (p *Payload) Invocation() (*resultstore.Invocation, error) {
	if p.Job == nil {
		return nil, errors.New("internal error: pj is nil")
	}
	i := &resultstore.Invocation{
		StatusAttributes:     invocationStatusAttributes(p.Job),
		Timing:               invocationTiming(p.Job),
		InvocationAttributes: invocationAttributes(p.ProjectID, p.Job),
		WorkspaceInfo:        workspaceInfo(p.Job),
		Properties:           invocationProperties(p.Job, p.Started),
		Files:                p.Files,
	}
	return i, nil
}

func invocationStatusAttributes(job *v1.ProwJob) *resultstore.StatusAttributes {
	status := resultstore.Status_TOOL_FAILED
	if job != nil {
		switch job.Status.State {
		case v1.SuccessState:
			status = resultstore.Status_PASSED
		case v1.FailureState:
			status = resultstore.Status_FAILED
		case v1.AbortedState:
			status = resultstore.Status_CANCELLED
		case v1.ErrorState:
			status = resultstore.Status_INCOMPLETE
		}
	}
	return &resultstore.StatusAttributes{
		Status: status,
	}
}

func invocationTiming(pj *v1.ProwJob) *resultstore.Timing {
	if pj == nil {
		return nil
	}
	start := pj.Status.StartTime.Time
	var duration time.Duration
	if pj.Status.CompletionTime != nil {
		duration = pj.Status.CompletionTime.Time.Sub(start)
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

func invocationAttributes(projectID string, pj *v1.ProwJob) *resultstore.InvocationAttributes {
	var labels map[string]string
	if pj != nil {
		labels = pj.Labels
	}
	return &resultstore.InvocationAttributes{
		// TODO: ProjectID might be assigned directly from the GCS
		// BucketAttrs.ProjectNumber; requires a raw GCS client.
		ProjectId:   projectID,
		Labels:      []string{"prow"},
		Description: descriptionFromLabels(labels),
	}
}

func descriptionFromLabels(labels map[string]string) string {
	jt := labels[kube.ProwJobTypeLabel]
	parts := []string{
		labels[kube.RepoLabel],
	}
	if pull := labels[kube.PullLabel]; pull != "" {
		parts = append(parts, pull)
		if ps := labels[kube.GerritPatchset]; ps != "" {
			parts = append(parts, ps)
		}
	}
	parts = append(parts, labels[kube.ProwBuildIDLabel], labels[kube.ProwJobAnnotation])
	return fmt.Sprintf("%s for %s", jt, strings.Join(parts, "/"))
}

func workspaceInfo(job *v1.ProwJob) *resultstore.WorkspaceInfo {
	return &resultstore.WorkspaceInfo{
		CommandLines: commandLines(job),
	}
}

// Per the ResultStore maintainers, the CommandLine Label must be
// populated, and should be either "original" or "canonical". (To be
// documented by them: if the original value contains placeholders,
// the final values should be added as "canonical".)
const commandLineLabel = "original"

func commandLines(pj *v1.ProwJob) []*resultstore.CommandLine {
	var cl []*resultstore.CommandLine
	if pj != nil && pj.Spec.PodSpec != nil {
		for _, c := range pj.Spec.PodSpec.Containers {
			cl = append(cl, &resultstore.CommandLine{
				Label: commandLineLabel,
				Tool:  strings.Join(c.Command, " "),
				Args:  c.Args,
			})
		}
	}
	return cl
}

func invocationProperties(pj *v1.ProwJob, started *metadata.Started) []*resultstore.Property {
	var ps []*resultstore.Property
	ps = append(ps, jobProperties(pj)...)
	ps = append(ps, startedProperties(started)...)
	return ps
}

func jobProperties(pj *v1.ProwJob) []*resultstore.Property {
	if pj == nil {
		return nil
	}
	ps := []*resultstore.Property{
		{
			Key:   "Instance",
			Value: pj.Status.BuildID,
		},
		{
			Key:   "Job",
			Value: pj.Spec.Job,
		},
		{
			Key:   "Prow_Dashboard_URL",
			Value: pj.Status.URL,
		},
	}
	ps = append(ps, podSpecProperties(pj.Spec.PodSpec)...)
	return ps
}

func podSpecProperties(podSpec *corev1.PodSpec) []*resultstore.Property {
	if podSpec == nil {
		return nil
	}
	var ps []*resultstore.Property
	seenEnv := map[string]bool{}
	for _, c := range podSpec.Containers {
		for _, e := range c.Env {
			if e.Name == "" {
				continue
			}
			v := e.Name + "=" + e.Value
			if !seenEnv[v] {
				seenEnv[v] = true
				ps = append(ps, &resultstore.Property{
					Key:   "Env",
					Value: v,
				})
			}
		}
	}
	return ps
}

func startedProperties(started *metadata.Started) []*resultstore.Property {
	if started == nil {
		return nil
	}
	ps := []*resultstore.Property{{
		Key:   "Commit",
		Value: started.RepoCommit,
	}}

	var branches, repos []string
	seenBranch := map[string]bool{}
	for repo, branch := range started.Repos {
		if !seenBranch[branch] {
			seenBranch[branch] = true
			branches = append(branches, branch)
		}
		repos = append(repos, repo)
	}
	slices.Sort(branches)
	for _, b := range branches {
		ps = append(ps, &resultstore.Property{
			Key:   "Branch",
			Value: b,
		})
	}
	slices.Sort(repos)
	for _, r := range repos {
		ps = append(ps, &resultstore.Property{
			Key:   "Repo",
			Value: gerrit.EnsureCodeURL(r),
		})
	}
	return ps
}

const defaultConfigurationId = "default"

func (p *Payload) DefaultConfiguration() *resultstore.Configuration {
	return &resultstore.Configuration{
		Id: &resultstore.Configuration_Id{
			ConfigurationId: defaultConfigurationId,
		},
	}
}

func targetID(pj *v1.ProwJob) string {
	if pj == nil {
		return "Unknown"
	}
	return pj.Spec.Job

}

func (p *Payload) OverallTarget() *resultstore.Target {
	return &resultstore.Target{
		Id: &resultstore.Target_Id{
			TargetId: targetID(p.Job),
		},
		TargetAttributes: &resultstore.TargetAttributes{
			Type: resultstore.TargetType_TEST,
		},
		Visible: true,
	}
}

func (p *Payload) ConfiguredTarget() *resultstore.ConfiguredTarget {
	return &resultstore.ConfiguredTarget{
		Id: &resultstore.ConfiguredTarget_Id{
			TargetId:        targetID(p.Job),
			ConfigurationId: defaultConfigurationId,
		},
		StatusAttributes: invocationStatusAttributes(p.Job),
		Timing:           metadataTiming(p.Job, p.Started, p.Finished),
	}
}

func (p *Payload) OverallAction() *resultstore.Action {
	return &resultstore.Action{
		Id: &resultstore.Action_Id{
			TargetId:        targetID(p.Job),
			ConfigurationId: defaultConfigurationId,
			ActionId:        "overall",
		},
		StatusAttributes: invocationStatusAttributes(p.Job),
		Timing:           metadataTiming(p.Job, p.Started, p.Finished),
		// TODO: What else if anything is required here?
		ActionType: &resultstore.Action_TestAction{},
	}
}

func metadataTiming(job *v1.ProwJob, started *metadata.Started, finished *metadata.Finished) *resultstore.Timing {
	if started == nil {
		return nil
	}
	start := started.Timestamp
	var duration int64
	switch {
	case finished != nil:
		duration = *finished.Timestamp - start
	case job != nil && job.Status.CompletionTime != nil:
		duration = job.Status.CompletionTime.Unix() - start
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

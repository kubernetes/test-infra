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
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
)

func int64Pointer(v int64) *int64 {
	return &v
}

func TestInvocation(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		payload *Payload
		want    *resultstore.Invocation
		wantErr bool
	}{
		{
			desc: "complete",
			payload: &Payload{
				Job: &v1.ProwJob{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job-name",
						Labels: map[string]string{
							kube.ProwJobTypeLabel:  "job-type-label",
							kube.RepoLabel:         "repo-label",
							kube.PullLabel:         "pull-label",
							kube.GerritPatchset:    "gerrit-patchset-label",
							kube.ProwBuildIDLabel:  "build-id-label",
							kube.ContextAnnotation: "context-annotation-label",
						},
					},
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
						PodSpec: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:    "container-1",
									Args:    []string{"arg-1", "arg-2"},
									Command: []string{"command"},
									Env: []corev1.EnvVar{
										{
											Name:  "env1",
											Value: "env1-value",
										},
										{
											Name:  "env2",
											Value: "env2-value",
										},
									},
								},
							},
						},
						Refs: &v1.Refs{
							RepoLink: "repo-link",
						},
					},
					Status: v1.ProwJobStatus{
						StartTime: metav1.Time{
							Time: time.Unix(100, 0),
						},
						CompletionTime: &metav1.Time{
							Time: time.Unix(300, 0),
						},
						State:   v1.SuccessState,
						URL:     "https://prow/url",
						BuildID: "build-id",
					},
				},
				Started: &metadata.Started{
					Timestamp:  150,
					RepoCommit: "repo-commit",
					Repos: map[string]string{
						"repo-key": "repo-value",
					},
				},
				Finished: &metadata.Finished{
					Timestamp: int64Pointer(250),
				},
				ProjectID: "project-id",
			},
			want: &resultstore.Invocation{
				Id: &resultstore.Invocation_Id{
					InvocationId: "job-name",
				},
				InvocationAttributes: &resultstore.InvocationAttributes{
					ProjectId: "project-id",
					Labels: []string{
						"prow",
					},
					Description: "job-type-label for repo-label/pull-label/gerrit-patchset-label/build-id-label/context-annotation-label",
				},
				Properties: []*resultstore.Property{
					{
						Key:   "Instance",
						Value: "build-id",
					},
					{
						Key:   "Job",
						Value: "spec-job",
					},
					{
						Key:   "Prow_Dashboard_URL",
						Value: "https://prow/url",
					},
					{
						Key:   "Repo",
						Value: "repo-link",
					},
					{
						Key:   "Commit",
						Value: "repo-commit",
					},
					{
						Key:   "Branch",
						Value: "repo-value",
					},
					{
						Key:   "Env",
						Value: "env1=env1-value",
					},
					{
						Key:   "Env",
						Value: "env2=env2-value",
					},
				},
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_PASSED,
				},
				Timing: &resultstore.Timing{
					StartTime: &timestamppb.Timestamp{
						Seconds: 100,
					},
					Duration: &durationpb.Duration{
						Seconds: 200,
					},
				},
				WorkspaceInfo: &resultstore.WorkspaceInfo{
					CommandLines: []*resultstore.CommandLine{
						{
							Label:   "container-1",
							Args:    []string{"arg-1", "arg-2"},
							Command: "command",
						},
					},
				},
			},
		},
		{
			desc: "podspec refs nil",
			payload: &Payload{
				Job: &v1.ProwJob{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job-name",
						Labels: map[string]string{
							kube.ProwJobTypeLabel:  "job-type-label",
							kube.RepoLabel:         "repo-label",
							kube.PullLabel:         "pull-label",
							kube.GerritPatchset:    "gerrit-patchset-label",
							kube.ProwBuildIDLabel:  "build-id-label",
							kube.ContextAnnotation: "context-annotation-label",
						},
					},
					Spec: v1.ProwJobSpec{
						Job:     "spec-job",
						PodSpec: nil,
						Refs:    nil,
					},
					Status: v1.ProwJobStatus{
						StartTime: metav1.Time{
							Time: time.Unix(100, 0),
						},
						CompletionTime: &metav1.Time{
							Time: time.Unix(300, 0),
						},
						State:   v1.SuccessState,
						URL:     "https://prow/url",
						BuildID: "build-id",
					},
				},
				Started: &metadata.Started{
					Timestamp:  150,
					RepoCommit: "repo-commit",
					Repos: map[string]string{
						"repo-key": "repo-value",
					},
				},
				Finished: &metadata.Finished{
					Timestamp: int64Pointer(250),
				},
				ProjectID: "project-id",
			},
			want: &resultstore.Invocation{
				Id: &resultstore.Invocation_Id{
					InvocationId: "job-name",
				},
				InvocationAttributes: &resultstore.InvocationAttributes{
					ProjectId: "project-id",
					Labels: []string{
						"prow",
					},
					Description: "job-type-label for repo-label/pull-label/gerrit-patchset-label/build-id-label/context-annotation-label",
				},
				Properties: []*resultstore.Property{
					{
						Key:   "Instance",
						Value: "build-id",
					},
					{
						Key:   "Job",
						Value: "spec-job",
					},
					{
						Key:   "Prow_Dashboard_URL",
						Value: "https://prow/url",
					},
					{
						Key:   "Commit",
						Value: "repo-commit",
					},
					{
						Key:   "Branch",
						Value: "repo-value",
					},
				},
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_PASSED,
				},
				Timing: &resultstore.Timing{
					StartTime: &timestamppb.Timestamp{
						Seconds: 100,
					},
					Duration: &durationpb.Duration{
						Seconds: 200,
					},
				},
				WorkspaceInfo: &resultstore.WorkspaceInfo{},
			},
		},
		{
			desc: "completiontime started finished nil",
			payload: &Payload{
				Job: &v1.ProwJob{
					ObjectMeta: metav1.ObjectMeta{
						Name: "job-name",
						Labels: map[string]string{
							kube.ProwJobTypeLabel:  "job-type-label",
							kube.RepoLabel:         "repo-label",
							kube.PullLabel:         "pull-label",
							kube.GerritPatchset:    "gerrit-patchset-label",
							kube.ProwBuildIDLabel:  "build-id-label",
							kube.ContextAnnotation: "context-annotation-label",
						},
					},
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
						PodSpec: &corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:    "container-1",
									Args:    []string{"arg-1", "arg-2"},
									Command: []string{"command"},
									Env: []corev1.EnvVar{
										{
											Name:  "env1",
											Value: "env1-value",
										},
										{
											Name:  "env2",
											Value: "env2-value",
										},
									},
								},
							},
						},
						Refs: &v1.Refs{
							RepoLink: "repo-link",
						},
					},
					Status: v1.ProwJobStatus{
						StartTime: metav1.Time{
							Time: time.Unix(100, 0),
						},
						CompletionTime: nil,
						State:          v1.SuccessState,
						URL:            "https://prow/url",
						BuildID:        "build-id",
					},
				},
				Started:   nil,
				Finished:  nil,
				ProjectID: "project-id",
			},
			want: &resultstore.Invocation{
				Id: &resultstore.Invocation_Id{
					InvocationId: "job-name",
				},
				InvocationAttributes: &resultstore.InvocationAttributes{
					ProjectId: "project-id",
					Labels: []string{
						"prow",
					},
					Description: "job-type-label for repo-label/pull-label/gerrit-patchset-label/build-id-label/context-annotation-label",
				},
				Properties: []*resultstore.Property{
					{
						Key:   "Instance",
						Value: "build-id",
					},
					{
						Key:   "Job",
						Value: "spec-job",
					},
					{
						Key:   "Prow_Dashboard_URL",
						Value: "https://prow/url",
					},
					{
						Key:   "Repo",
						Value: "repo-link",
					},
					{
						Key:   "Env",
						Value: "env1=env1-value",
					},
					{
						Key:   "Env",
						Value: "env2=env2-value",
					},
				},
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_PASSED,
				},
				Timing: &resultstore.Timing{
					StartTime: &timestamppb.Timestamp{
						Seconds: 100,
					},
					Duration: &durationpb.Duration{
						Seconds: 0,
					},
				},
				WorkspaceInfo: &resultstore.WorkspaceInfo{
					CommandLines: []*resultstore.CommandLine{
						{
							Label:   "container-1",
							Args:    []string{"arg-1", "arg-2"},
							Command: "command",
						},
					},
				},
			},
		},
		{
			desc:    "job nil",
			payload: &Payload{},
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.payload.invocation()
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatalf("got unexpected error: %v", err)
			}
			if tc.wantErr {
				t.Fatal("wanted error, got nil")
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("invocation differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStatusAttributes(t *testing.T) {
	for _, tc := range []struct {
		prowState v1.ProwJobState
		want      resultstore.Status
	}{
		{
			prowState: v1.SuccessState,
			want:      resultstore.Status_PASSED,
		},
		{
			prowState: v1.FailureState,
			want:      resultstore.Status_FAILED,
		},
		{
			prowState: v1.AbortedState,
			want:      resultstore.Status_CANCELLED,
		},
		{
			prowState: v1.ErrorState,
			want:      resultstore.Status_INCOMPLETE,
		},
		{
			prowState: v1.PendingState,
			want:      resultstore.Status_TOOL_FAILED,
		},
	} {
		t.Run(string(tc.prowState), func(t *testing.T) {
			p := &Payload{
				Job: &v1.ProwJob{
					Status: v1.ProwJobStatus{
						State: tc.prowState,
					},
				},
			}
			want := &resultstore.StatusAttributes{
				Status: tc.want,
			}
			if diff := cmp.Diff(want, p.invocationStatusAttributes(), protocmp.Transform()); diff != "" {
				t.Errorf("invocationStatusAttributes differs (-want +got):\n%s", diff)
			}

		})
	}
}

func TestDefaultconfiguration(t *testing.T) {
	p := &Payload{}
	got := p.defaultConfiguration()
	want := &resultstore.Configuration{
		Id: &resultstore.Configuration_Id{
			ConfigurationId: "default",
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("defaultConfiguration differs (-want +got):\n%s", diff)
	}
}

func TestOverallTarget(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		payload *Payload
		want    *resultstore.Target
	}{
		{
			desc: "success",
			payload: &Payload{
				Job: &v1.ProwJob{
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
					},
				},
			},
			want: &resultstore.Target{
				Id: &resultstore.Target_Id{
					TargetId: "spec-job",
				},
				TargetAttributes: &resultstore.TargetAttributes{
					Type: resultstore.TargetType_TEST,
				},
				Visible: true,
			},
		},
		{
			desc:    "nil job",
			payload: &Payload{},
			want: &resultstore.Target{
				Id: &resultstore.Target_Id{
					TargetId: "Unknown",
				},
				TargetAttributes: &resultstore.TargetAttributes{
					Type: resultstore.TargetType_TEST,
				},
				Visible: true,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tc.payload.overallTarget(), protocmp.Transform()); diff != "" {
				t.Errorf("overallTarget differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfiguredTarget(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		payload *Payload
		want    *resultstore.ConfiguredTarget
	}{
		{
			desc: "success",
			payload: &Payload{
				Job: &v1.ProwJob{
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
					},
				},
			},
			want: &resultstore.ConfiguredTarget{
				Id: &resultstore.ConfiguredTarget_Id{
					TargetId:        "spec-job",
					ConfigurationId: "default",
				},
			},
		},
		{
			desc:    "nil job",
			payload: &Payload{},
			want: &resultstore.ConfiguredTarget{
				Id: &resultstore.ConfiguredTarget_Id{
					TargetId:        "Unknown",
					ConfigurationId: "default",
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tc.payload.configuredTarget(), protocmp.Transform()); diff != "" {
				t.Errorf("configuredTarget differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOverallAction(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		payload *Payload
		want    *resultstore.Action
	}{
		{
			desc: "success",
			payload: &Payload{
				Job: &v1.ProwJob{
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
					},
				},
				Started: &metadata.Started{
					Timestamp:  150,
					RepoCommit: "repo-commit",
					Repos: map[string]string{
						"repo-key": "repo-value",
					},
				},
				Finished: &metadata.Finished{
					Timestamp: int64Pointer(250),
				},
			},
			want: &resultstore.Action{
				Id: &resultstore.Action_Id{
					TargetId:        "spec-job",
					ConfigurationId: "default",
					ActionId:        "overall",
				},
				ActionType: &resultstore.Action_TestAction{},
				Timing: &resultstore.Timing{
					StartTime: &timestamppb.Timestamp{
						Seconds: 150,
					},
					Duration: &durationpb.Duration{
						Seconds: 100,
					},
				},
			},
		},
		{
			desc: "started nil",
			payload: &Payload{
				Job: &v1.ProwJob{
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
					},
				},
				Finished: &metadata.Finished{
					Timestamp: int64Pointer(250),
				},
			},
			want: &resultstore.Action{
				Id: &resultstore.Action_Id{
					TargetId:        "spec-job",
					ConfigurationId: "default",
					ActionId:        "overall",
				},
				ActionType: &resultstore.Action_TestAction{},
			},
		},
		{
			desc: "finished nil use completion time",
			payload: &Payload{
				Job: &v1.ProwJob{
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
					},
					Status: v1.ProwJobStatus{
						CompletionTime: &metav1.Time{
							Time: time.Unix(250, 0),
						},
					},
				},
				Started: &metadata.Started{
					Timestamp:  150,
					RepoCommit: "repo-commit",
					Repos: map[string]string{
						"repo-key": "repo-value",
					},
				},
			},
			want: &resultstore.Action{
				Id: &resultstore.Action_Id{
					TargetId:        "spec-job",
					ConfigurationId: "default",
					ActionId:        "overall",
				},
				ActionType: &resultstore.Action_TestAction{},
				Timing: &resultstore.Timing{
					StartTime: &timestamppb.Timestamp{
						Seconds: 150,
					},
					Duration: &durationpb.Duration{
						Seconds: 100,
					},
				},
			},
		},
		{
			desc: "finished and job completion time nil",
			payload: &Payload{
				Job: &v1.ProwJob{
					Spec: v1.ProwJobSpec{
						Job: "spec-job",
					},
					Status: v1.ProwJobStatus{
						CompletionTime: nil,
					},
				},
				Started: &metadata.Started{
					Timestamp:  150,
					RepoCommit: "repo-commit",
					Repos: map[string]string{
						"repo-key": "repo-value",
					},
				},
			},
			want: &resultstore.Action{
				Id: &resultstore.Action_Id{
					TargetId:        "spec-job",
					ConfigurationId: "default",
					ActionId:        "overall",
				},
				ActionType: &resultstore.Action_TestAction{},
			},
		},
		{
			desc: "job nil",
			payload: &Payload{
				Started: &metadata.Started{
					Timestamp:  150,
					RepoCommit: "repo-commit",
					Repos: map[string]string{
						"repo-key": "repo-value",
					},
				},
				Finished: &metadata.Finished{
					Timestamp: int64Pointer(250),
				},
			},
			want: &resultstore.Action{
				Id: &resultstore.Action_Id{
					TargetId:        "Unknown",
					ConfigurationId: "default",
					ActionId:        "overall",
				},
				ActionType: &resultstore.Action_TestAction{},
				Timing: &resultstore.Timing{
					StartTime: &timestamppb.Timestamp{
						Seconds: 150,
					},
					Duration: &durationpb.Duration{
						Seconds: 100,
					},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tc.payload.overallAction(), protocmp.Transform()); diff != "" {
				t.Errorf("overallAction differs (-want +got):\n%s", diff)
			}
		})
	}
}

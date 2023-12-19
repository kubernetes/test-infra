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
							kube.ProwJobAnnotation: "job-label",
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
									},
								},
							},
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
						"https://started-review.repo": "started-branch",
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
					Description: "job-type-label for repo-label/pull-label/gerrit-patchset-label/build-id-label/job-label",
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
						Key:   "Env",
						Value: "env1=env1-value",
					},
					{
						Key:   "Commit",
						Value: "repo-commit",
					},
					{
						Key:   "Branch",
						Value: "started-branch",
					},
					{
						Key:   "Repo",
						Value: "https://started.repo",
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
							Label: "original",
							Tool:  "command",
							Args:  []string{"arg-1", "arg-2"},
						},
					},
				},
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
							kube.ProwBuildIDLabel:  "build-id-label",
							kube.ProwJobAnnotation: "job-label",
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
					Description: "job-type-label for repo-label/build-id-label/job-label",
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
							Label: "original",
							Tool:  "command",
							Args:  []string{"arg-1", "arg-2"},
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
			got, err := tc.payload.Invocation()
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

func TestInvocationId(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		job     *v1.ProwJob
		want    *resultstore.Invocation_Id
		wantErr bool
	}{
		{
			desc: "success",
			job: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "job-name",
				},
			},
			want: &resultstore.Invocation_Id{
				InvocationId: "job-name",
			},
		},
		{
			desc:    "nil",
			job:     nil,
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := invocationID(tc.job)
			if err != nil {
				if tc.wantErr {
					t.Logf("got expected error: %v", err)
					return
				}
				t.Fatal("got unexpected error")
			}
			if tc.wantErr {
				t.Fatal("want error, got nil")
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("invocation id differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInvocationTiming(t *testing.T) {
	for _, tc := range []struct {
		desc string
		job  *v1.ProwJob
		want *resultstore.Timing
	}{
		{
			desc: "success",
			job: &v1.ProwJob{
				Status: v1.ProwJobStatus{
					StartTime: metav1.Time{
						Time: time.Unix(100, 0),
					},
					CompletionTime: &metav1.Time{
						Time: time.Unix(300, 0),
					},
				},
			},
			want: &resultstore.Timing{
				StartTime: &timestamppb.Timestamp{
					Seconds: 100,
				},
				Duration: &durationpb.Duration{
					Seconds: 200,
				},
			},
		},
		{
			desc: "completion nil",
			job: &v1.ProwJob{
				Status: v1.ProwJobStatus{
					StartTime: metav1.Time{
						Time: time.Unix(100, 0),
					},
					CompletionTime: nil,
				},
			},
			want: &resultstore.Timing{
				StartTime: &timestamppb.Timestamp{
					Seconds: 100,
				},
				Duration: &durationpb.Duration{
					Seconds: 0,
				},
			},
		},
		{
			desc: "job nil",
			job:  nil,
			want: nil,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := invocationTiming(tc.job)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("timing differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInvocationProperties(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		job     *v1.ProwJob
		started *metadata.Started
		want    []*resultstore.Property
	}{
		{
			desc: "success",
			job: &v1.ProwJob{
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
								},
							},
						},
					},
				},
				Status: v1.ProwJobStatus{
					URL:     "https://prow/url",
					BuildID: "build-id",
				},
			},
			started: &metadata.Started{
				Timestamp:  150,
				RepoCommit: "repo-commit",
				Repos: map[string]string{
					"https://started.repo": "started-branch",
				},
			},
			want: []*resultstore.Property{
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
					Key:   "Env",
					Value: "env1=env1-value",
				},
				{
					Key:   "Commit",
					Value: "repo-commit",
				},
				{
					Key:   "Branch",
					Value: "started-branch",
				},
				{
					Key:   "Repo",
					Value: "https://started.repo",
				},
			},
		},
		{
			desc: "job nil",
			job:  nil,
			started: &metadata.Started{
				Timestamp:  150,
				RepoCommit: "repo-commit",
				Repos: map[string]string{
					"https://started.repo": "started-branch",
				},
			},
			want: []*resultstore.Property{
				{
					Key:   "Commit",
					Value: "repo-commit",
				},
				{
					Key:   "Branch",
					Value: "started-branch",
				},
				{
					Key:   "Repo",
					Value: "https://started.repo",
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := invocationProperties(tc.job, tc.started)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("properties differ (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStartedProperties(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		started *metadata.Started
		want    []*resultstore.Property
	}{
		{
			desc: "single repo",
			started: &metadata.Started{
				Timestamp:  150,
				RepoCommit: "repo-commit",
				Repos: map[string]string{
					"https://repo1.com": "branch1",
				},
			},
			want: []*resultstore.Property{
				{
					Key:   "Commit",
					Value: "repo-commit",
				},
				{
					Key:   "Branch",
					Value: "branch1",
				},
				{
					Key:   "Repo",
					Value: "https://repo1.com",
				},
			},
		},
		{
			desc: "multi repo",
			started: &metadata.Started{
				Timestamp:  150,
				RepoCommit: "repo-commit",
				Repos: map[string]string{
					"repo/two":                 "branch2",
					"https://repo1-review.com": "branch1",
				},
			},
			want: []*resultstore.Property{
				{
					Key:   "Commit",
					Value: "repo-commit",
				},
				{
					Key:   "Branch",
					Value: "branch1",
				},
				{
					Key:   "Branch",
					Value: "branch2",
				},
				{
					Key:   "Repo",
					Value: "https://repo1.com",
				},
				{
					Key:   "Repo",
					Value: "repo/two",
				},
			},
		},
		{
			desc: "non gerrit",
			started: &metadata.Started{
				Timestamp:  150,
				RepoCommit: "repo-commit",
				Repos: map[string]string{
					"https://repo1.other-review.com": "branch1",
				},
			},
			want: []*resultstore.Property{
				{
					Key:   "Commit",
					Value: "repo-commit",
				},
				{
					Key:   "Branch",
					Value: "branch1",
				},
				{
					Key:   "Repo",
					Value: "https://repo1.other-review.com",
				},
			},
		},
		{
			desc:    "nil",
			started: nil,
			want:    nil,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := startedProperties(tc.started)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("properties differ (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPodSpecProperties(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		podSpec *corev1.PodSpec
		want    []*resultstore.Property
	}{
		{
			desc: "success",
			podSpec: &corev1.PodSpec{
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
							{
								Name:  "",
								Value: "skip empty Name",
							},
						},
					},
					{
						Name:    "container-2",
						Args:    []string{"arg-3", "arg-4"},
						Command: []string{"command2"},
						Env: []corev1.EnvVar{
							{
								Name:  "env1",
								Value: "env1-value",
							},
							{
								Name:  "env3",
								Value: "env3-value",
							},
						},
					},
				},
			},
			want: []*resultstore.Property{
				{
					Key:   "Env",
					Value: "env1=env1-value",
				},
				{
					Key:   "Env",
					Value: "env2=env2-value",
				},
				{
					Key:   "Env",
					Value: "env3=env3-value",
				},
			},
		},
		{
			desc:    "nil podspec",
			podSpec: nil,
			want:    nil,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := podSpecProperties(tc.podSpec)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("properties differ (-want +got):\n%s", diff)
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
			job := &v1.ProwJob{
				Status: v1.ProwJobStatus{
					State: tc.prowState,
				},
			}
			want := &resultstore.StatusAttributes{
				Status: tc.want,
			}
			if diff := cmp.Diff(want, invocationStatusAttributes(job), protocmp.Transform()); diff != "" {
				t.Errorf("invocationStatusAttributes differs (-want +got):\n%s", diff)
			}

		})
	}
}

func TestDefaultconfiguration(t *testing.T) {
	p := &Payload{}
	got := p.DefaultConfiguration()
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
			if diff := cmp.Diff(tc.want, tc.payload.OverallTarget(), protocmp.Transform()); diff != "" {
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
			if diff := cmp.Diff(tc.want, tc.payload.ConfiguredTarget(), protocmp.Transform()); diff != "" {
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
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
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
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_PASSED,
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
					Status: v1.ProwJobStatus{
						State: v1.ErrorState,
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
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_INCOMPLETE,
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
						State: v1.FailureState,
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
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_FAILED,
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
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_TOOL_FAILED,
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
				StatusAttributes: &resultstore.StatusAttributes{
					Status: resultstore.Status_TOOL_FAILED,
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
			if diff := cmp.Diff(tc.want, tc.payload.OverallAction(), protocmp.Transform()); diff != "" {
				t.Errorf("overallAction differs (-want +got):\n%s", diff)
			}
		})
	}
}

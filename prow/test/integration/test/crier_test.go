/*
Copyright 2020 The Kubernetes Authors.

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
package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/github"
)

func TestReportGHStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state prowjobv1.ProwJobState
		want  string
	}{
		{
			name:  "pending",
			state: prowjobv1.PendingState,
			want:  "pending",
		},
		{
			name:  "success",
			state: prowjobv1.SuccessState,
			want:  "success",
		},
		{
			name:  "failed",
			state: prowjobv1.FailureState,
			want:  "failure",
		},
		{
			name:  "aborted",
			state: prowjobv1.AbortedState,
			want:  "failure",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterContext := getClusterContext()
			t.Logf("Creating client for cluster: %s", clusterContext)

			kubeClient, err := NewClients("", clusterContext)
			if err != nil {
				t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
			}

			githubClient := github.NewClient(func() []byte { return nil }, func([]byte) []byte { return nil }, github.DefaultGraphQLEndpoint, "http://localhost/fakeghserver")

			ctx := context.Background()

			podName := fmt.Sprintf("crier-%s-%s", tt.name, RandomString(t))
			sha := RandomString(t)

			t.Logf("Creating CRD %s for sha %s", podName, sha)
			prowjob := prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"prow.k8s.io/job":       podName,
						"prow.k8s.io/refs.org":  "fake-org",
						"prow.k8s.io/refs.pull": "0",
						"prow.k8s.io/refs.repo": "fake-repo",
						"prow.k8s.io/type":      "presubmit",
					},
					Labels: map[string]string{
						"created-by-prow":  "true",
						"prow.k8s.io/type": "presubmit",
					},
					Name:      podName,
					Namespace: defaultNamespace,
				},
				Spec: prowjobv1.ProwJobSpec{
					Type:      prowjobv1.PresubmitJob,
					Namespace: "test-pods",
					Job:       podName,
					Refs: &prowjobv1.Refs{
						Org:     "fake-org",
						Repo:    "fake-repo",
						BaseRef: "master",
						BaseSHA: "49e0442008f963eb77963213b85fb53c345e0632",
						Pulls: []prowjobv1.Pull{
							{
								Author: "fake_author",
								Number: 0,
								SHA:    sha,
							},
						},
					},
					Report: true,
				},
				Status: prowjobv1.ProwJobStatus{
					State:     tt.state,
					StartTime: v1.NewTime(time.Now().Add(-1 * time.Second)),
				},
			}
			if tt.state == prowjobv1.SuccessState || tt.state == prowjobv1.FailureState {
				prowjob.Status.CompletionTime = &v1.Time{Time: time.Now()}
			}

			t.Cleanup(func() {
				if err := kubeClient.Delete(ctx, &prowjob); err != nil {
					t.Logf("Failed cleanup resource %q: %v", prowjob.Name, err)
				}
			})

			t.Logf("Creating prowjob: %s", podName)
			if err := kubeClient.Create(ctx, &prowjob); err != nil {
				t.Fatalf("Failed creating prowjob: %v", err)
			}
			t.Logf("Finished creating prowjob: %s", podName)

			if err := wait.Poll(200*time.Millisecond, 5*time.Minute, func() (bool, error) {
				ss, err := githubClient.GetCombinedStatus("fake-org", "fake-repo", sha)
				if err != nil {
					return false, fmt.Errorf("failed listing statues: %w", err)
				}
				if want, got := 1, len(ss.Statuses); want != got {
					// Wait until it's ready
					return false, nil
				}
				return tt.want == ss.Statuses[0].State, nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

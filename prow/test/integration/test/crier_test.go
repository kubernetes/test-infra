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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowgh "k8s.io/test-infra/prow/github"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReportGHStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		states []prowjobv1.ProwJobState
		want   []string
	}{
		{
			name: "triggered",
			want: []string{"pending"},
		},
		{
			name:   "pending",
			states: []prowjobv1.ProwJobState{prowjobv1.PendingState},
			want:   []string{"pending", "pending"},
		},
		{
			name:   "pending-success",
			states: []prowjobv1.ProwJobState{prowjobv1.SuccessState},
			want:   []string{"pending", "success"},
		},
		{
			name:   "pending-failed",
			states: []prowjobv1.ProwJobState{prowjobv1.FailureState},
			want:   []string{"pending", "failure"},
		},
		{
			name:   "pending-aborted",
			states: []prowjobv1.ProwJobState{prowjobv1.AbortedState},
			want:   []string{"pending", "failure"},
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

			ctx := context.Background()

			podName := fmt.Sprintf("crier-test-pod-%s", RandomString(t))
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
						"name":             podName,
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
					State: prowjobv1.TriggeredState,
				},
			}

			t.Cleanup(func() {
				kubeClient.Delete(ctx, &prowjob)
			})

			t.Logf("Creating prowjob: %s", podName)
			if err := kubeClient.Create(ctx, &prowjob); err != nil {
				t.Fatalf("Failed creating prowjob: %v", err)
			}
			t.Logf("Finished creating prowjob: %s", podName)

			var waitStatus func(*testing.T, string)
			waitStatus = func(t *testing.T, s string) {
				if err := wait.Poll(200*time.Microsecond, 10*time.Second, func() (bool, error) {
					e := fmt.Sprintf("http://localhost/fakeghserver/repos/fake-org/fake-repo/statuses/%s", sha)
					resp, err := http.Get(e)
					if err != nil {
						return false, fmt.Errorf("Failed query endpoint %q: %v", e, err)
					}
					var ss []prowgh.Status
					d := json.NewDecoder(resp.Body)
					d.DisallowUnknownFields()
					if err := d.Decode(&ss); err != nil {
						return false, fmt.Errorf("Failed unmarshal response: %v", err)
					}
					if len(ss) == 0 { // Keep waiting for status
						return false, nil
					}
					if want, got := 1, len(ss); want != got {
						return false, fmt.Errorf("Number of contexts mismatch, want: %d: got: %d", want, got)
					}
					return s == ss[0].State, nil
				}); err != nil {
					t.Fatal(err)
				}
			}

			waitStatus(t, tt.want[0])

			var patchJobStatus = func(t *testing.T, s prowjobv1.ProwJobState) {
				t.Logf("Patching to status %s", s)
				var changes []string
				changes = append(changes, fmt.Sprintf("\"state\": \"%s\"", s))
				if s == prowjobv1.SuccessState || s == prowjobv1.FailureState {
					changes = append(changes, fmt.Sprintf("\"completionTime\": \"%s\"", time.Now().Format(time.RFC3339)))
				}
				patch := []byte(fmt.Sprintf("{\"status\": {%s}}", strings.Join(changes, ", ")))
				if err := kubeClient.Patch(ctx, &prowjob, client.RawPatch(types.MergePatchType, patch)); err != nil {
					t.Fatalf("Failed patching prowjob: %v", err)
				}
			}

			for i, s := range tt.states {
				patchJobStatus(t, s)
				waitStatus(t, tt.want[i+1])
			}
		})
	}
}

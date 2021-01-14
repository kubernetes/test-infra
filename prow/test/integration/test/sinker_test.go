// +build e2etest
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

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDeletePod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hasCR      bool
		wantDelete bool
	}{
		{
			"running-pod",
			true,
			false,
		},
		{
			"orphaned-pod",
			false,
			true,
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

			prowjob := prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"prow.k8s.io/job": tt.name,
					},
					Labels: map[string]string{
						"created-by-prow":  "true",
						"prow.k8s.io/type": "periodic",
						"name":             tt.name,
					},
					Name:      tt.name,
					Namespace: defaultNamespace,
				},
				Spec: prowjobv1.ProwJobSpec{
					Type:      prowjobv1.PeriodicJob,
					Namespace: testpodNamespace,
					Job:       tt.name,
				},
				Status: prowjobv1.ProwJobStatus{
					State: prowjobv1.TriggeredState,
				},
			}
			pod := corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:      tt.name,
					Namespace: testpodNamespace,
					Labels: map[string]string{
						"name":            tt.name,
						"created-by-prow": "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "busybox",
							Image: "localhost:5000/busybox",
							Args: []string{
								"sleep",
								"1000000",
							},
						},
					},
				},
			}

			t.Cleanup(func() {
				kubeClient.Delete(ctx, &prowjob)
				kubeClient.Delete(ctx, &pod)
			})

			if tt.hasCR {
				t.Logf("Creating prowjob: %s", tt.name)
				if err := kubeClient.Create(ctx, &prowjob); err != nil {
					t.Fatalf("Failed creating prowjob: %v", err)
				}
				t.Logf("Finished creating prowjob: %s", tt.name)
			}
			// Create pod
			t.Logf("Creating pod: %s", tt.name)
			if err := kubeClient.Create(ctx, &pod); err != nil {
				t.Fatalf("Failed creating pod: %v", err)
			}
			t.Logf("Finished creating pod: %s", tt.name)

			// Make sure pod is running
			t.Logf("Make sure pod is running: %s", tt.name)
			if err = wait.Poll(time.Second, time.Minute, func() (bool, error) {
				p := &corev1.Pod{}
				if err := kubeClient.Get(ctx, client.ObjectKey{
					Name:      tt.name,
					Namespace: testpodNamespace,
				}, p); err != nil {
					return false, fmt.Errorf("Failed getting pod: %v", err)
				}
				return (p.Status.Phase == corev1.PodRunning), nil
			}); err != nil {
				t.Fatalf("Pod was not created successfully: %v", err)
			}
			t.Logf("Pod is running: %s", tt.name)

			// Make sure pod is deleted, it'll take roughly 2 minutes
			// Don't care about the outcome, will check later
			t.Logf("Wait for sinker deleting pod or timeout in 2 minutes: %s", tt.name)
			var exist bool
			wait.Poll(time.Second, 2*time.Minute, func() (bool, error) {
				exist = false
				pods := &corev1.PodList{}
				err = kubeClient.List(ctx, pods, ctrlruntimeclient.InNamespace(testpodNamespace))
				if err != nil {
					return false, err
				}
				for _, p := range pods.Items {
					if p.Name == tt.name {
						exist = true
					}
				}
				return !exist, nil
			})
			// Check for the error of `List` call.
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Pod %s exist: %v", tt.name, exist)
			if want, got := tt.wantDelete, !exist; want != got {
				t.Fatalf("Want deleted: %v. Got deleted: %v", want, got)
			}
		})
	}
}

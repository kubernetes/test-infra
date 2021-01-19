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
	"log"
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
		name             string
		notCreatedByProw bool
		jobStartTime     v1.Time
		jobFinishTime    v1.Time
		hasCR            bool
		wantJobDeleted   bool
		wantPodDeleted   bool
	}{
		{
			name:           "running-pod",
			jobStartTime:   v1.NewTime(time.Now().Add(-1 * time.Minute)),
			hasCR:          true,
			wantPodDeleted: false,
		},
		{
			name:           "orphaned-pod",
			jobStartTime:   v1.NewTime(time.Now().Add(-1 * time.Minute)),
			hasCR:          false,
			wantPodDeleted: true,
		},
		{
			name:           "ttl-deleted",
			jobStartTime:   v1.NewTime(time.Now().Add(-32 * time.Minute)),
			jobFinishTime:  v1.NewTime(time.Now().Add(-31 * time.Minute)),
			hasCR:          true,
			wantPodDeleted: true,
		},
		{
			name:           "max-prow-job-age",
			jobStartTime:   v1.NewTime(time.Now().Add(-49 * time.Hour)),
			jobFinishTime:  v1.NewTime(time.Now().Add(-48 * time.Hour)),
			hasCR:          true,
			wantJobDeleted: true,
			wantPodDeleted: true,
		},
		{
			name:             "orphaned-pod-not-prowpod",
			notCreatedByProw: true,
			wantPodDeleted:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resourceName := fmt.Sprintf("%s-%s", tt.name, RandomString(t))

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
						"prow.k8s.io/job": resourceName,
					},
					Labels: map[string]string{
						"created-by-prow":  "true",
						"prow.k8s.io/type": "periodic",
						"name":             resourceName,
					},
					Name:      resourceName,
					Namespace: defaultNamespace,
				},
				Spec: prowjobv1.ProwJobSpec{
					Type:      prowjobv1.PeriodicJob,
					Namespace: testpodNamespace,
					Job:       resourceName,
				},
				Status: prowjobv1.ProwJobStatus{
					State: prowjobv1.TriggeredState,
				},
			}

			pod := corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:      resourceName,
					Namespace: testpodNamespace,
					Labels: map[string]string{
						"name": resourceName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "alpine",
							Image: "localhost:5000/alpine",
							Args: []string{
								"sleep",
								"1000000",
							},
						},
					},
				},
			}
			if !tt.notCreatedByProw {
				pod.ObjectMeta.Labels["created-by-prow"] = "true"
			}

			t.Cleanup(func() {
				kubeClient.Delete(ctx, &prowjob)
				kubeClient.Delete(ctx, &pod)
			})

			if !tt.notCreatedByProw {
				t.Logf("Creating prowjob: %s", resourceName)
				if err := kubeClient.Create(ctx, &prowjob); err != nil {
					t.Fatalf("Failed creating prowjob: %v", err)
				}
				t.Logf("Finished creating prowjob: %s", resourceName)
				// Make sure pod is running
				p := &prowjobv1.ProwJob{}
				if err := kubeClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: defaultNamespace,
				}, p); err != nil {
					log.Fatalf("Prowjob %q not exist: %v", resourceName, err)
				}
			}

			// Create pod
			t.Logf("Creating pod: %s", resourceName)
			if err := kubeClient.Create(ctx, &pod); err != nil {
				t.Fatalf("Failed creating pod: %v", err)
			}
			t.Logf("Finished creating pod: %s", resourceName)

			// Make sure pod is running
			t.Logf("Make sure pod is running: %s", resourceName)
			if err = wait.Poll(time.Second, time.Minute, func() (bool, error) {
				p := &corev1.Pod{}
				if err := kubeClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: testpodNamespace,
				}, p); err != nil {
					return false, fmt.Errorf("Failed getting pod: %v", err)
				}
				return (p.Status.Phase == corev1.PodRunning), nil
			}); err != nil {
				t.Fatalf("Pod was not created successfully: %v", err)
			}
			t.Logf("Pod is running: %s", resourceName)

			if !tt.notCreatedByProw {
				// Patch prowjob to make it stale if needed
				prowjob.Status.StartTime = tt.jobStartTime
				prowjob.Status.CompletionTime = &tt.jobFinishTime
				if err := kubeClient.Update(ctx, &prowjob); err != nil {
					t.Fatalf("Failed updating prowjob %q: %v", resourceName, err)
				}
				// Delete prowjob if CR isn't supposed to have existed
				if !tt.hasCR {
					if err := kubeClient.Delete(ctx, &prowjob); err != nil {
						t.Fatalf("Failed deleting prowjob: %v", err)
					}
				}
			}

			// Make sure pod is deleted, it'll take roughly 2 minutes
			// Don't care about the outcome, will check later
			t.Logf("Wait for sinker deleting pod or timeout in 2 minutes: %s", resourceName)
			var exist bool
			wait.Poll(time.Second, 2*time.Minute, func() (bool, error) {
				exist = false
				pods := &corev1.PodList{}
				err = kubeClient.List(ctx, pods, ctrlruntimeclient.InNamespace(testpodNamespace))
				if err != nil {
					return false, err
				}
				for _, p := range pods.Items {
					if p.Name == resourceName {
						exist = true
					}
				}
				return !exist, nil
			})
			// Check for the error of `List` call.
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Pod %s exist: %v", resourceName, exist)
			if want, got := tt.wantPodDeleted, !exist; want != got {
				t.Fatalf("Want deleted: %v. Got deleted: %v", want, got)
			}

			// Check for prowjob deletion.
			{
				exist = false
				pjs := &prowjobv1.ProwJobList{}
				err = kubeClient.List(ctx, pjs, ctrlruntimeclient.InNamespace(defaultNamespace))
				if err != nil {
					log.Fatalf("Failed listing prowjobs: %v", err)
				}
				for _, pj := range pjs.Items {
					if pj.Name == resourceName {
						exist = true
					}
				}
				if tt.wantJobDeleted && exist {
					t.Fatalf("Pod exisentce mismatch. Want: %v, got: %v", tt.wantJobDeleted, exist)
				}
			}
		})
	}
}

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
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDeletePod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		prowjob        *prowjobv1.ProwJob
		pod            *corev1.Pod
		prowjobDeleted bool
		wantJobDeleted bool
		wantPodDeleted bool
	}{
		{
			name: "running-pod",
			prowjob: &prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: defaultNamespace,
				},
				Spec: prowjobv1.ProwJobSpec{
					Namespace: testpodNamespace,
				},
				Status: prowjobv1.ProwJobStatus{
					State:     prowjobv1.TriggeredState,
					StartTime: v1.NewTime(time.Now().Add(-1 * time.Minute)),
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: testpodNamespace,
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
			},
			prowjobDeleted: false,
			wantPodDeleted: false,
		},
		{
			name: "orphaned-pod",
			prowjob: &prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: defaultNamespace,
				},
				Spec: prowjobv1.ProwJobSpec{
					Namespace: testpodNamespace,
				},
				Status: prowjobv1.ProwJobStatus{
					State:     prowjobv1.TriggeredState,
					StartTime: v1.NewTime(time.Now().Add(-1 * time.Minute)),
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: testpodNamespace,
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
			},
			prowjobDeleted: true,
			wantPodDeleted: true,
		},
		{
			name: "ttl-deleted",
			prowjob: &prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: defaultNamespace,
				},
				Spec: prowjobv1.ProwJobSpec{
					Namespace: testpodNamespace,
				},
				Status: prowjobv1.ProwJobStatus{
					State:          prowjobv1.TriggeredState,
					StartTime:      v1.NewTime(time.Now().Add(-32 * time.Minute)),
					CompletionTime: &v1.Time{Time: time.Now().Add(-31 * time.Minute)},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: testpodNamespace,
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
			},
			prowjobDeleted: false,
			wantJobDeleted: false,
			wantPodDeleted: true,
		},
		{
			name: "max-prow-job-age",
			prowjob: &prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: defaultNamespace,
				},
				Spec: prowjobv1.ProwJobSpec{
					Namespace: testpodNamespace,
				},
				Status: prowjobv1.ProwJobStatus{
					State:          prowjobv1.TriggeredState,
					StartTime:      v1.NewTime(time.Now().Add(-50 * time.Hour)),
					CompletionTime: &v1.Time{Time: time.Now().Add(-49 * time.Hour)},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"created-by-prow": "true",
					},
					Namespace: testpodNamespace,
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
			},
			prowjobDeleted: false,
			wantPodDeleted: true,
		},
		{
			name:    "orphaned-pod-not-prowjob",
			prowjob: nil,
			pod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Namespace: testpodNamespace,
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
			},
			prowjobDeleted: false,
			wantPodDeleted: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prowjob, pod := tt.prowjob, tt.pod
			// The name of prowjob and pod is derived from the test name,
			// doing it here to avoid repeated declaration in tests.
			resourceName := fmt.Sprintf("%s-%s", tt.name, RandomString(t))
			if prowjob != nil {
				prowjob.ObjectMeta.Labels["name"] = resourceName
				prowjob.ObjectMeta.Name = resourceName
			}
			pod.ObjectMeta.Name = resourceName
			clusterContext := getClusterContext()
			t.Logf("Creating client for cluster: %s", clusterContext)

			kubeClient, err := NewClients("", clusterContext)
			if err != nil {
				t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
			}

			ctx := context.Background()

			t.Cleanup(func() {
				if prowjob != nil {
					kubeClient.Delete(ctx, prowjob)
				}
				kubeClient.Delete(ctx, pod)
			})

			if tt.prowjob != nil {
				t.Logf("Creating prowjob: %s", prowjob.Name)
				if err := kubeClient.Create(ctx, prowjob); err != nil {
					t.Fatalf("Failed creating prowjob: %v", err)
				}
				t.Logf("Finished creating prowjob: %s", prowjob.Name)
			}

			// Create pod
			t.Logf("Creating pod: %s", pod.Name)
			if err := kubeClient.Create(ctx, pod); err != nil {
				t.Fatalf("Failed creating pod: %v", err)
			}
			t.Logf("Finished creating pod: %s", pod.Name)

			// Delete prowjob to make pod orphan
			if tt.prowjobDeleted {
				t.Logf("Deleting prowjob %s to make the pod %s orphan", prowjob.Name, pod.Name)
				if err := kubeClient.Delete(ctx, prowjob); err != nil {
					t.Fatalf("Failed deleting prowjob %s: %v", prowjob.Name, err)
				}
			}

			// Make sure pod is deleted, it'll take roughly 3 minutes
			// Don't care about the outcome, will check later
			t.Logf("Wait for sinker deleting pod or timeout in 3 minutes: %s", pod.Name)
			var exist bool
			wait.Poll(time.Second, 2*time.Minute, func() (bool, error) {
				exist = false
				pods := &corev1.PodList{}
				err = kubeClient.List(ctx, pods, ctrlruntimeclient.InNamespace(testpodNamespace))
				if err != nil {
					return false, err
				}
				for _, p := range pods.Items {
					if p.Name == pod.Name {
						exist = true
					}
				}
				return !exist, nil
			})
			// Check for the error of `List` call.
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Pod %s exist: %v", pod.Name, exist)
			if want, got := tt.wantPodDeleted, !exist; want != got {
				t.Fatalf("Want pod deleted: %v. Got deleted: %v", want, got)
			}

			// Check for prowjob deletion.
			if prowjob != nil {
				exist = false
				pjs := &prowjobv1.ProwJobList{}
				err = kubeClient.List(ctx, pjs, ctrlruntimeclient.InNamespace(defaultNamespace))
				if err != nil {
					t.Fatalf("Failed listing prowjobs: %v", err)
				}
				for _, pj := range pjs.Items {
					if pj.Name == prowjob.Name {
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

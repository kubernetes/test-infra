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

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestDeletePod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hasCRD     bool
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
		name := tt.name
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterContext := getClusterContext()
			t.Logf("Creating client for cluster: %s", clusterContext)

			// defaultKubeconfig := os.Getenv("KUBECONFIG")
			// stat, err := os.Lstat(defaultKubeconfig)
			// if err != nil {
			// 	t.Fatalf("Failed stat %q: %v", defaultKubeconfig, err)
			// }
			// t.Logf("Stat of %q: %v\n\n%v\n\n%v", defaultKubeconfig, stat.Mode(), stat.Sys(), stat)
			kubeClient, prowjobClient, err := NewClients("", clusterContext)
			if err != nil {
				t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
			}

			ctx := context.Background()

			t.Cleanup(func() {
				prowjobClient.ProwV1().ProwJobs("default").
					Delete(ctx, name, v1.DeleteOptions{})
				kubeClient.CoreV1().Pods(testpodNamespace).
					Delete(ctx, name, v1.DeleteOptions{})
			})

			if tt.hasCRD {
				t.Logf("Creating CRD: %s", name)
				prowjob := prowjobv1.ProwJob{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"prow.k8s.io/job": name,
						},
						Labels: map[string]string{
							"created-by-prow":  "true",
							"prow.k8s.io/type": "periodic",
							"name":             name,
						},
						Name: name,
					},
					Spec: prowjobv1.ProwJobSpec{
						Type:      prowjobv1.PeriodicJob,
						Namespace: testpodNamespace,
						Job:       name,
					},
					Status: prowjobv1.ProwJobStatus{
						State: prowjobv1.TriggeredState,
					},
				}
				if _, err := prowjobClient.ProwV1().ProwJobs("default").
					Create(ctx, &prowjob, v1.CreateOptions{}); err != nil {
					t.Fatalf("Failed creating prowjob: %v", err)
				}
				t.Logf("Finished creating CRD: %s", name)
			}
			// Create pod
			t.Logf("Creating pod: %s", name)
			pod := corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name: name,
					Labels: map[string]string{
						"name":            name,
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
			if _, err := kubeClient.CoreV1().Pods(testpodNamespace).Create(ctx, &pod, v1.CreateOptions{}); err != nil {
				t.Fatalf("Failed creating pod: %v", err)
			}
			t.Logf("Finished creating pod: %s", name)

			// Make sure pod is running
			t.Logf("Make sure pod is running: %s", name)
			if err = wait.Poll(time.Second, time.Minute, func() (bool, error) {
				p, err := kubeClient.CoreV1().Pods(testpodNamespace).Get(ctx, name, v1.GetOptions{})
				if err != nil {
					return false, fmt.Errorf("Failed listing pods: %v", err)
				}
				return (p.Status.Phase == corev1.PodRunning), nil
			}); err != nil {
				t.Fatalf("Pod was not created successfully: %v", err)
			}
			t.Logf("Pod is running: %s", name)

			// Make sure pod is deleted, it'll take roughly 2 minutes
			// Don't care about the outcome, will check later
			t.Logf("Wait for sinker deleting pod or timeout in 2 minutes: %s", name)
			wait.Poll(time.Second, 2*time.Minute, func() (bool, error) {
				pods, err := kubeClient.CoreV1().Pods(testpodNamespace).List(ctx, v1.ListOptions{})
				if err != nil {
					return false, err
				}
				var exist bool
				for _, p := range pods.Items {
					if p.Name == name {
						exist = true
					}
				}
				return !exist, nil
			})
			pods, err := kubeClient.CoreV1().Pods(testpodNamespace).List(ctx, v1.ListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var exist bool
			for _, p := range pods.Items {
				if p.Name == name {
					exist = true
				}
			}
			t.Logf("Pod %s exist: %v", name, exist)
			if want, got := tt.wantDelete, !exist; want != got {
				t.Fatalf("Want deleted: %v. Got deleted: %v", want, got)
			}
		})
	}
}

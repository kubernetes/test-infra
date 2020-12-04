package e2e

import (
	"context"
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
			kubeClient, prowjobClient, err := NewClients("", clusterContext)
			if err != nil {
				t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
			}

			ctx := context.Background()

			t.Cleanup(func() {
				prowjobClient.ProwV1().ProwJobs("default").
					Delete(ctx, name, v1.DeleteOptions{})
				kubeClient.CoreV1().Pods("test-pods").
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
						Namespace: "test-pods",
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
			if _, err := kubeClient.CoreV1().Pods("test-pods").Create(ctx, &pod, v1.CreateOptions{}); err != nil {
				t.Fatalf("Failed creating pod: %v", err)
			}
			t.Logf("Finished creating pod: %s", name)

			// Make sure pod is running
			t.Logf("Make sure pod is running: %s", name)
			if err = wait.Poll(time.Second, time.Minute, func() (bool, error) {
				p, err := kubeClient.CoreV1().Pods("test-pods").Get(ctx, name, v1.GetOptions{})
				if err != nil {
					t.Fatalf("Failed listing pods: %v", err)
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
				pods, err := kubeClient.CoreV1().Pods("test-pods").List(ctx, v1.ListOptions{})
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
			pods, err := kubeClient.CoreV1().Pods("test-pods").List(ctx, v1.ListOptions{})
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

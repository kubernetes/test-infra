/*
Copyright 2021 The Kubernetes Authors.

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

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestLaunchProwJob(t *testing.T) {
	const existJobName = "horologium-schedule-test-job"
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "periodic-job-must-run",
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

			pjs := &prowjobv1.ProwJobList{}
			if err := wait.Poll(200*time.Millisecond, 1*time.Minute, func() (bool, error) {
				err = kubeClient.List(ctx, pjs, &ctrlruntimeclient.ListOptions{
					LabelSelector: labels.SelectorFromSet(map[string]string{kube.ProwJobAnnotation: existJobName}),
					Namespace:     defaultNamespace,
				})
				if err != nil {
					return false, fmt.Errorf("failed listing prow jobs: %w", err)
				}
				return len(pjs.Items) > 0, nil
			}); err != nil {
				t.Fatalf("Failed waiting for job %q: %v", existJobName, err)
			}
		})
	}
}

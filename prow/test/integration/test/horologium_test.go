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
	"sort"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

			getNextRunOrFail := func(t *testing.T, jobName string, lastRun *v1.Time) *prowjobv1.ProwJob {
				var res *prowjobv1.ProwJob
				if err := wait.Poll(time.Second, 70*time.Second, func() (bool, error) {
					pjs := &prowjobv1.ProwJobList{}
					err = kubeClient.List(ctx, pjs, &ctrlruntimeclient.ListOptions{
						LabelSelector: labels.SelectorFromSet(map[string]string{kube.ProwJobAnnotation: existJobName}),
						Namespace:     defaultNamespace,
					})
					if err != nil {
						return false, fmt.Errorf("failed listing prow jobs: %w", err)
					}
					sort.Slice(pjs.Items, func(i, j int) bool {
						return pjs.Items[i].Status.StartTime.After(pjs.Items[j].Status.StartTime.Time)
					})
					for _, pj := range pjs.Items {
						if lastRun != nil && pj.CreationTimestamp.Before(lastRun) {
							return false, nil
						}
						res = &pj
						break
					}
					return res != nil, nil
				}); err != nil {
					t.Fatalf("Failed waiting for job %q: %v", jobName, err)
				}
				return res
			}

			t.Logf("Wait for the next run of %q", existJobName)
			pj := getNextRunOrFail(t, existJobName, nil)
			// Enforce that the previous run was created 30-60 seconds ago, so
			// that the next run won't happen in the following 30 seconds.
			if pj.CreationTimestamp.Add(30 * time.Second).After(time.Now()) {
				pj = getNextRunOrFail(t, existJobName, &pj.CreationTimestamp)
			}

			// Wait for 15 seconds, the next run kicked off by horologium should
			// be no more than 45 seconds later, unless there is manual
			// interruption as what the next section is going to do.
			time.Sleep(15 * time.Second)

			timeBeforeNewJob := time.Now()
			pjToBe := pj.DeepCopy()
			pjToBe.ResourceVersion = ""
			pjToBe.Name = uuid.NewV1().String()
			pjToBe.Status.StartTime = v1.NewTime(time.Now().Add(1 * time.Second))
			t.Log("Creating prowjob again, and the next run should happen 60 seconds later.")
			if err := kubeClient.Create(ctx, pjToBe); err != nil {
				t.Fatalf("Failed creating prowjob: %v", err)
			}
			t.Logf("Finished creating prowjob")

			cutoff := v1.NewTime(time.Now().Add(1 * time.Second))
			t.Logf("Finding job after: %v", cutoff)
			nextPj := getNextRunOrFail(t, existJobName, &cutoff)
			if nextPj.CreationTimestamp.Add(-60 * time.Second).Before(timeBeforeNewJob) {
				t.Fatalf("New job was created too early. Want: 60 seconds after %v, got: %v", timeBeforeNewJob, nextPj.CreationTimestamp)
			}
		})
	}
}

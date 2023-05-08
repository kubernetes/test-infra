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

	uuid "github.com/google/uuid"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const horologiumJobConfigFile = "horologium-test.yaml"

var horologiumJobConfig = `periodics:
- interval: 1m
  name: horologium-schedule-test-job
  spec:
    containers:
    - command:
      - echo
      args:
      - "Hello World!"
      image: localhost:5001/alpine
`

func TestLaunchProwJob(t *testing.T) {
	const existJobName = "horologium-schedule-test-job"
	t.Parallel()

	clusterContext := getClusterContext()
	t.Logf("Creating client for cluster: %s", clusterContext)
	kubeClient, err := NewClients("", clusterContext)
	if err != nil {
		t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
	}

	if err := updateJobConfig(context.Background(), kubeClient, horologiumJobConfigFile, []byte(horologiumJobConfig)); err != nil {
		t.Fatalf("Failed update job config: %v", err)
	}

	// Horologium itself is pretty good at handling the configmap update, but
	// not kubelet, accoriding to
	// https://github.com/kubernetes/kubernetes/issues/30189 kubelet syncs
	// configmap updates on existing pods every minute, which is a long wait.
	// The proposed fix in the issue was updating the deployment, which imo
	// should be better handled by just refreshing pods.
	// So here comes forcing restart of horologium pods.
	if err := refreshProwPods(kubeClient, context.Background(), "horologium"); err != nil {
		t.Fatalf("Failed refreshing horologium pods: %v", err)
	}

	t.Cleanup(func() {
		if err := updateJobConfig(context.Background(), kubeClient, horologiumJobConfigFile, []byte{}); err != nil {
			t.Logf("ERROR CLEANUP: %v", err)
		}
		labels, _ := labels.Parse("prow.k8s.io/job = horologium-schedule-test-job")
		if err := kubeClient.DeleteAllOf(context.Background(), &prowjobv1.ProwJob{}, &ctrlruntimeclient.DeleteAllOfOptions{
			ListOptions: ctrlruntimeclient.ListOptions{LabelSelector: labels},
		}); err != nil {
			t.Logf("ERROR CLEANUP: %v", err)
		}
	})

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
			ctx := context.Background()

			// getNextRunOrFail is a helper function getting the latest run
			// after lastRun, and fail if there is none found.
			getNextRunOrFail := func(t *testing.T, jobName string, lastRun *v1.Time) *prowjobv1.ProwJob {
				var res *prowjobv1.ProwJob
				if err := wait.Poll(time.Second, 90*time.Second, func() (bool, error) {
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
					if len(pjs.Items) > 0 {
						if lastRun != nil && pjs.Items[0].CreationTimestamp.Before(lastRun) {
							return false, nil
						}
						res = &pjs.Items[0]
					}
					return res != nil, nil
				}); err != nil {
					t.Fatalf("Failed waiting for job %q: %v", jobName, err)
				}
				return res
			}

			t.Logf("Ensure there is at least one run of %q", existJobName)
			pj := getNextRunOrFail(t, existJobName, nil)

			// TODO(mpherman): Make a better test for this.
			// Hijacking Horlogium Integration test to make sure PJ is created with a tenantID (specifically the default one)
			if pj.Spec.ProwJobDefault == nil || pj.Spec.ProwJobDefault.TenantID != config.DefaultTenantID {
				t.Fatalf("ProwJob Not created with TenantID")
			}

			// Now examines that 'interval' respects the last run instead of a
			// fixed schedule. For example, if the previous pj started at 00:00:00,
			// the next one will be scheduled to run at 00:01:00. But if this
			// job was triggered out of schedule for whatever reason, for
			// example at 00:00:39, then the next run will be expected to run at
			// 00:01:39 instead of 00:01:00.

			// First make sure that the previous run was created more than 30
			// seconds ago, this will enforce the next scheduled run to be 30-60
			// seconds later.
			if pj.CreationTimestamp.Add(30 * time.Second).After(time.Now()) {
				pj = getNextRunOrFail(t, existJobName, &pj.CreationTimestamp)
			}

			// Wait for 15 seconds, so the next scheduled run is 15-45 seconds
			// later.
			time.Sleep(15 * time.Second)

			// Now kick off this job manually, which should alter the next run
			// to be scheduled after 60 seconds instead of 15-45 seconds.
			timeBeforeNewJob := time.Now()
			uuidV1 := uuid.New()
			pjToBe := pj.DeepCopy()
			pjToBe.ResourceVersion = ""
			pjToBe.Name = uuidV1.String()
			pjToBe.Status.StartTime = v1.NewTime(time.Now().Add(1 * time.Second))
			t.Log("Creating prowjob again, and the next run should happen 60 seconds later.")
			if err := kubeClient.Create(ctx, pjToBe); err != nil {
				t.Fatalf("Failed creating prowjob: %v", err)
			}
			t.Logf("Finished creating prowjob")

			// Assert the new run is 60 seconds later.
			cutoff := v1.NewTime(time.Now().Add(1 * time.Second))
			t.Logf("Finding job after: %v", cutoff)
			nextPj := getNextRunOrFail(t, existJobName, &cutoff)
			if nextPj.CreationTimestamp.Add(-60 * time.Second).Before(timeBeforeNewJob) {
				t.Fatalf("New job was created too early. Want: 60 seconds after %v, got: %v", timeBeforeNewJob, nextPj.CreationTimestamp)
			}
		})
	}
}

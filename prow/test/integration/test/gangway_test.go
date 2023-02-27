/*
Copyright 2022 The Kubernetes Authors.

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

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gangway"
	gangwayGoogleClient "k8s.io/test-infra/prow/gangway/client/google"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/test/integration/internal/fakegitserver"
)

// TestGangway makes gRPC calls to gangway.
func TestGangway(t *testing.T) {
	t.Parallel()

	const (
		UidLabel         = "integration-test/uid"
		ProwJobDecorated = `
presubmits:
  - name: trigger-inrepoconfig-presubmit-via-gangway%s
    always_run: false
    decorate: true
    spec:
      containers:
      - image: localhost:5001/alpine
        command:
        - sh
        args:
        - -c
        - |
          set -eu
          echo "hello from trigger-inrepoconfig-presubmit-via-gangway-repo%s"
          cat README.txt
`
	)

	// createGerritRepo creates Gerrit-style Git refs for changes (PRs). The
	// revision is always named "refs/changes/00/123/1" where 123 is the change
	// number (PR number) and 1 is the first revision (version) of the same
	// change.
	CreateRepo1 := createGerritRepo("TestGangway1", fmt.Sprintf(ProwJobDecorated, "1", "1"))

	tests := []struct {
		name          string
		repoSetups    []fakegitserver.RepoSetup
		projectNumber string
		msg           *gangway.CreateJobExecutionRequest
		want          string
	}{
		{
			name: "inrepoconfig-presubmit",
			repoSetups: []fakegitserver.RepoSetup{
				{
					Name:      "some/org/gangway-test-repo-1",
					Script:    CreateRepo1,
					Overwrite: true,
				},
			},
			projectNumber: "123",
			msg: &gangway.CreateJobExecutionRequest{
				JobName:          "trigger-inrepoconfig-presubmit-via-gangway1",
				JobExecutionType: gangway.JobExecutionType_PRESUBMIT,
				// Define where the job definition lives from inrepoconfig.
				Refs: &gangway.Refs{
					Org:      "https://fakegitserver.default/repo/some/org",
					Repo:     "gangway-test-repo-1",
					CloneUri: "https://fakegitserver.default/repo/some/org/gangway-test-repo-1",
					BaseRef:  "master",
					BaseSha:  "f1267354a7bbc5ce7d0458cdf4d0d36e8d35d8b3",
					Pulls: []*gangway.Pull{
						{
							Number: 1,
							Sha:    "458b96a96a74689447530035f5a71c426bacb505",
						},
					},
				},
				PodSpecOptions: &gangway.PodSpecOptions{
					Envs: map[string]string{
						"FOO_VAR": "value-of-foo-var",
					},
					Labels: map[string]string{
						kube.GerritRevision: "123",
					},
					Annotations: map[string]string{
						"foo_annotation": "value-of-foo-annotation",
					},
				},
			},
			want: `hello from trigger-inrepoconfig-presubmit-via-gangway-repo1
this-is-from-repoTestGangway1
`,
		},
		{
			name:          "mainconfig-periodic",
			projectNumber: "123",
			msg: &gangway.CreateJobExecutionRequest{
				JobName:          "trigger-mainconfig-periodic-via-gangway1",
				JobExecutionType: gangway.JobExecutionType_PERIODIC,
				PodSpecOptions: &gangway.PodSpecOptions{
					Envs: map[string]string{
						"FOO_VAR": "value-of-foo-var",
					},
					Labels: map[string]string{
						kube.GerritRevision: "123",
					},
					Annotations: map[string]string{
						"foo_annotation": "value-of-foo-annotation",
					},
				},
			},
			want: `hello from main config periodic
`,
		},
	}

	// Ensure that all repos are named uniquely, because otherwise they clobber
	// each other when we create them against fakegitserver. This prevents
	// programmer error when writing new tests.
	allRepoDirs := []string{}
	for _, tt := range tests {
		for _, repoSetup := range tt.repoSetups {

			allRepoDirs = append(allRepoDirs, repoSetup.Name)
		}
	}
	if err := enforceUniqueRepoDirs(allRepoDirs); err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set up a connection to gangway.
			c, err := gangwayGoogleClient.NewInsecure(":32000", tt.projectNumber)
			if err != nil {
				t.Fatal(err)
			}

			defer c.Close()

			ctx := context.Background()
			ctx = c.EmbedProjectNumber(ctx)

			var prowjob prowjobv1.ProwJob

			clusterContext := getClusterContext()
			t.Logf("Creating client for cluster: %s", clusterContext)

			restConfig, err := NewRestConfig("", clusterContext)
			if err != nil {
				t.Fatalf("could not create restConfig: %v", err)
			}

			kubeClient, err := ctrlruntimeclient.New(restConfig, ctrlruntimeclient.Options{})
			if err != nil {
				t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
			}

			// Set up repos on FGS for just this test case.
			fgsClient := fakegitserver.NewClient("http://localhost/fakegitserver", 5*time.Second)
			for _, repoSetup := range tt.repoSetups {
				err := fgsClient.SetupRepo(repoSetup)
				if err != nil {
					t.Fatalf("FGS repo setup failed: %v", err)
				}
			}

			// Create a unique test case ID (UID) for this particular test
			// invocation. This makes it easier to check from this code whether
			// sub actually received the exact same message we just published.
			uid := RandomString(t)
			tt.msg.PodSpecOptions.Labels = make(map[string]string)
			tt.msg.PodSpecOptions.Labels[UidLabel] = uid

			// Use Prow API to create the job through gangway. This is a gRPC
			// call.
			jobExecution, err := c.GRPC.CreateJobExecution(ctx, tt.msg)
			if err != nil {
				t.Fatalf("Failed to create job execution: %v", err)
			}
			fmt.Println(jobExecution)

			// We expect the job to have succeeded.
			timeout := 120 * time.Second
			pollInterval := 500 * time.Millisecond
			expectedStatus := gangway.JobExecutionStatus_SUCCESS

			if err := c.WaitForJobExecutionStatus(ctx, jobExecution.Id, pollInterval, timeout, expectedStatus); err != nil {
				t.Fatal(err)
			} else {
				// Only clean up the ProwJob if it succeeded (save the ProwJob for debugging if it failed).
				t.Cleanup(func() {
					if err := kubeClient.Delete(ctx, &prowjob); err != nil {
						t.Logf("Failed cleanup resource %q: %v", prowjob.Name, err)
					}
				})
			}
		})
	}
}

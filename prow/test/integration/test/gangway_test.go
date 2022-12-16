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
	"log"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gangway"
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
		name       string
		repoSetups []fakegitserver.RepoSetup
		metadata   []string
		msg        *gangway.CreateJobExecutionRequest
		want       string
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
			metadata: []string{
				"x-endpoint-api-consumer-type", "PROJECT",
				"x-endpoint-api-consumer-number", "123"},
			msg: &gangway.CreateJobExecutionRequest{
				JobName:          "trigger-inrepoconfig-presubmit-via-gangway1",
				JobExecutionType: gangway.JobExecutionType_PRESUBMIT,
				// Define where the job definition lives from inrepoconfig.
				Refs: &gangway.Refs{
					Org:     "https://fakegitserver.default/repo/some/org",
					Repo:    "gangway-test-repo-1",
					BaseRef: "master",
					BaseSha: "f1267354a7bbc5ce7d0458cdf4d0d36e8d35d8b3",
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
			name: "mainconfig-periodic",
			metadata: []string{
				"x-endpoint-api-consumer-type", "PROJECT",
				"x-endpoint-api-consumer-number", "123"},
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
			conn, err := grpc.Dial(":32000", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			defer conn.Close()

			prowClient := gangway.NewProwClient(conn)

			ctx := context.Background()
			ctx = metadata.NewOutgoingContext(
				ctx,
				metadata.Pairs(tt.metadata...),
			)

			var prowjob prowjobv1.ProwJob
			var prowjobList prowjobv1.ProwJobList
			var podName *string

			clusterContext := getClusterContext()
			t.Logf("Creating client for cluster: %s", clusterContext)

			restConfig, err := NewRestConfig("", clusterContext)
			if err != nil {
				t.Fatalf("could not create restConfig: %v", err)
			}

			clientset, err := kubernetes.NewForConfig(restConfig)
			if err != nil {
				t.Fatalf("could not create Clientset: %v", err)
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
			jobExecution, err := prowClient.CreateJobExecution(ctx, tt.msg)
			if err != nil {
				t.Fatalf("Failed to create job execution: %v", err)
			}
			fmt.Println(jobExecution)

			// We expect the job to have succeeded. This is mostly copy/pasted
			// from the pod-utils_test.go file next to this file.
			//
			// Testing that the job has succeeded is useful because if there are
			// any refs defined, those refs need to be cloned as well. So it
			// tests more components (clonerefs, initupload, etc). In this
			// sense, the tests here can be thought of as a superset of the
			// TestClonerefs test in pod-utils_test.go.
			expectJobSuccess := func() (bool, error) {
				err = kubeClient.List(ctx, &prowjobList, ctrlruntimeclient.MatchingLabels{"integration-test/uid": uid})
				if err != nil {
					t.Logf("failed getting prow job with label: %s", uid)
					return false, nil
				}
				if len(prowjobList.Items) != 1 {
					t.Logf("unexpected number of matching prow jobs: %d", len(prowjobList.Items))
					return false, nil
				}
				prowjob = prowjobList.Items[0]
				// The pod name should match the job name (this is another UID,
				// distinct from the uid we generated above).
				podName = &prowjob.Name
				switch prowjob.Status.State {
				case prowjobv1.SuccessState:
					got, err := getPodLogs(clientset, "test-pods", *podName, &coreapi.PodLogOptions{Container: "test"})
					if err != nil {
						t.Errorf("failed getting logs for clonerefs")
						return false, nil
					}
					if diff := cmp.Diff(got, tt.want); diff != "" {
						return false, fmt.Errorf("actual logs differ from expected: %s", diff)
					}
					return true, nil
				case prowjobv1.FailureState:
					return false, fmt.Errorf("possible programmer error: prow job %s failed", *podName)
				default:
					return false, nil
				}
			}

			// This is also mostly copy/pasted from pod-utils_test.go. The logic
			// here deals with the case where the test fails (where we want to
			// log as much as possible to make it easier to see why a test
			// failed), or when the test succeeds (where we clean up the ProwJob
			// that was created by sub).
			timeout := 120 * time.Second
			pollInterval := 500 * time.Millisecond
			if waitErr := wait.Poll(pollInterval, timeout, expectJobSuccess); waitErr != nil {
				if podName == nil {
					t.Fatal("could not find test pod")
				}
				// Retrieve logs from clonerefs.
				podLogs, err := getPodLogs(clientset, "test-pods", *podName, &coreapi.PodLogOptions{Container: "clonerefs"})
				if err != nil {
					t.Errorf("failed getting logs for clonerefs")
				}
				t.Logf("logs for clonerefs:\n\n%s\n\n", podLogs)

				// If we got an error, show the failing prow job's test
				// container (our test case's "got" and "expected" shell code).
				pjPod := &coreapi.Pod{}
				err = kubeClient.Get(ctx, ctrlruntimeclient.ObjectKey{
					Namespace: "test-pods",
					Name:      *podName,
				}, pjPod)
				if err != nil {
					t.Errorf("failed getting prow job's pod %v; unable to determine why the test failed", *podName)
					t.Error(err)
					t.Fatal(waitErr)
				}
				// Error messages from clonerefs, initupload, entrypoint, or sidecar.
				for _, containerStatus := range pjPod.Status.InitContainerStatuses {
					terminated := containerStatus.State.Terminated
					if terminated != nil && len(terminated.Message) > 0 {
						t.Errorf("InitContainer %q: %s", containerStatus.Name, terminated.Message)
					}
				}
				// Error messages from the test case's shell script (showing the
				// git SHAs that we expected vs what we got).
				for _, containerStatus := range pjPod.Status.ContainerStatuses {
					terminated := containerStatus.State.Terminated
					if terminated != nil && len(terminated.Message) > 0 {
						t.Errorf("Container %s: %s", containerStatus.Name, terminated.Message)
					}
				}

				t.Fatal(waitErr)
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

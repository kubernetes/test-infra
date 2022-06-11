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

	"github.com/google/go-cmp/cmp"
	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pubsub/subscriber"
	"k8s.io/test-infra/prow/test/integration/internal/fakegitserver"
	"k8s.io/test-infra/prow/test/integration/internal/fakepubsub"
)

func TestPubSubSubscriptions(t *testing.T) {
	t.Parallel()

	const (
		PubsubEmulatorHost = "localhost:30303"
		UidLabel           = "integration-test/uid"
		Repo1HEADsha       = "8c5dc6fe1b5a63200f23a2364011e8270f0f7cd0"
		CreateRepoRepo1    = `
echo this-is-from-repo1 > README.txt
git add README.txt
git commit -m "commit 1"
`
	)

	tests := []struct {
		name       string
		repoSetups []fakegitserver.RepoSetup
		msg        fakepubsub.PubSubMessageForSub
		expected   string
	}{
		{
			name: "staticconfig-postsubmit",
			repoSetups: []fakegitserver.RepoSetup{
				{
					Name:      "repo1",
					Script:    CreateRepoRepo1,
					Overwrite: true,
				},
			},
			msg: fakepubsub.PubSubMessageForSub{
				Attributes: map[string]string{
					subscriber.ProwEventType: subscriber.PostsubmitProwJobEvent,
				},
				Data: subscriber.ProwJobEvent{
					Name: "trigger-postsubmit-via-pubsub1", // This job is defined in the static config.
					Refs: &prowjobv1.Refs{
						Org:      "org1",
						Repo:     "repo1",
						BaseSHA:  Repo1HEADsha,
						BaseRef:  "master",
						CloneURI: "http://fakegitserver.default/repo/repo1",
					},
				},
			},
			expected: `hello from trigger-postsubmit-via-pubsub1
this-is-from-repo1
`,
		},
	}

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

	fpsClient, err := fakepubsub.NewClient("project1", PubsubEmulatorHost)
	if err != nil {
		t.Fatalf("Failed creating fakepubsub client")
	}

	fgsClient := fakegitserver.NewClient("http://localhost/fakegitserver", 5*time.Second)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			var prowjob prowjobv1.ProwJob
			var prowjobList prowjobv1.ProwJobList
			var podName *string

			// Set up repos on FGS for just this test case.
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
			tt.msg.Data.Labels = make(map[string]string)
			tt.msg.Data.Labels[UidLabel] = uid

			// Publish the message to the topic being watched by sub. This topic
			// is defined in the integration tests's config/prow/config.yaml.
			err := fpsClient.PublishMessage(ctx, tt.msg, "topic1")
			if err != nil {
				t.Fatalf("Failed to publish message to topic1: %v", err)
			}

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
					if diff := cmp.Diff(got, tt.expected); diff != "" {
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
			timeout := 30 * time.Second
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

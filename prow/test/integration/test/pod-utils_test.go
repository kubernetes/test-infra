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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/test/integration/lib"

	coreapi "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// fooHEADsha is the HEAD commit SHA of the "foo" repo that fakegitserver
	// always initializes on startup (see fakegitserver's
	// `-populate-sample-repos` option, which is set to true by default).
	fooHEADsha                    = "e7d762376b714fdc2d6493ecd798a9d128e4b88f"
	fooPR1sha                     = "b343a11088668210321ca76712e9bc3b8aa1f2f7"
	fooPR1MergeSha                = "a081e60423aea6d3bdde2a50f8c435546529a932"
	fooPR2sha                     = "89bb95ef23c663d1b9ad97ff4351de516f25496c"
	fooPR3sha                     = "2049df35eda98a9068ddfbc7bc6ce0ab5ef0d0ba"
	fooMultiMergeSha              = "bc75802040886a627d4315b1c32635ab37ca0bba"
	barHEADsha                    = "c2740e40972392a7c8954b389c71733e1f900561"
	barPR2sha                     = "62d5ed7cbccce940421b7e234de28591472b6955"
	barPR3sha                     = "f8387fe902fcb0c50d6d9e67f68b9802d591fd45"
	barMultiMergeWithSubmoduleSha = "c6a86219ebf1917a0a53317067b6ad01a6ed4799"
)

// TestClonerefs tests the "clonerefs" binary by creating a ProwJob with
// DecorationConfig. Because of the DecorationConfig, clonerefs gets added as an
// "InitContainer" to the pod that gets scheduled for the ProwJob. We tweak
// CloneURI so that it clones from the fakegitserver instead of GitHub (or
// elsewhere). Inside the test container in the pod, we check that we can indeed
// see the cloned contents by checking the HEAD SHA against an expected value
// (this is possible because fakegitserver always populates the same "foo" Git
// repo with the same SHA on startup).
//
// We test a basic postsubmit job as well as a presubmit job. The main
// difference with the presubmit job is that clonerefs needs to clone additional
// PR commits; otherwise they are the same.
func TestClonerefs(t *testing.T) {
	t.Parallel()

	createRepoFoo := `
echo hello > README.txt
git add README.txt
git commit -m "commit 1"

echo "hello world!" > README.txt
git add README.txt
git commit -m "commit 2"

for num in 1 2 3; do
	git checkout -d master
	echo "${num}" > "${num}"
	git add "${num}"
	git commit -m "PR${num}"
	git update-ref "refs/pull/${num}/head" HEAD
done

git checkout master
`
	createRepoBar := `
echo bar > bar.txt
git add bar.txt
git commit -m "commit 1"

echo "hello world!" > bar.txt
git add bar.txt
git commit -m "commit 2"

for num in 1 2 3; do
	git checkout -d master
	echo "${num}" > "${num}"
	git add "${num}"
	git commit -m "PR${num}"
	git update-ref "refs/pull/${num}/head" HEAD
done

git checkout master

git submodule add http://fakegitserver.default/repo/foo4
git commit -m "add submodule"
`

	tests := []struct {
		name       string
		repoSetups []lib.FGSRepoSetup
		prowjob    prowjobv1.ProwJob
	}{
		{
			// Check if we got a properly cloned repo.
			name: "postsubmit",
			repoSetups: []lib.FGSRepoSetup{
				{
					Name:      "foo1",
					Script:    createRepoFoo,
					Overwrite: true,
				},
			},
			prowjob: prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"prow.k8s.io/type": "postsubmit",
					},
					Labels: map[string]string{
						"prow.k8s.io/type": "postsubmit",
					},
				},
				Spec: prowjobv1.ProwJobSpec{
					Type: prowjobv1.PostsubmitJob,
					Refs: &prowjobv1.Refs{
						Repo:    "foo1",
						BaseSHA: fooHEADsha,
						// According to
						// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/,
						// each Service in a Kubernetes cluster has a hostname
						// assigned to it, with the form
						// <ServiceName>.<Namespace>. So in our case, we have
						// the "fakegitserver" in the "default" namespace, so
						// the hostname is "fakegitserver.default".
						//
						// The "/foo" at the end is simply to refer to the Git
						// repo named "foo" in the "/git-repo/foo" directory in
						// the fakegitserver container.
						CloneURI: "http://fakegitserver.default/repo/foo1",
					},
					PodSpec: &coreapi.PodSpec{
						Containers: []coreapi.Container{
							{
								Args: []string{
									"sh",
									"-c",
									fmt.Sprintf(`
expected=%s
got=$(git rev-parse HEAD)
if [ $got != $expected ]; then
	echo >&2 "[FAIL] got $got, expected $expected"
	>&2 git log
	exit 1
fi
`, fooHEADsha),
								},
							},
						},
					},
				},
			},
		},
		{
			// Check that the PR has been merged.
			name: "presubmit-single-pr",
			repoSetups: []lib.FGSRepoSetup{
				{
					Name:      "foo2",
					Script:    createRepoFoo,
					Overwrite: true,
				},
			},
			prowjob: prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"prow.k8s.io/type": "presubmit",
					},
					Labels: map[string]string{
						"prow.k8s.io/type": "presubmit",
					},
				},
				Spec: prowjobv1.ProwJobSpec{
					Type: prowjobv1.PresubmitJob,
					Refs: &prowjobv1.Refs{
						Repo:     "foo2",
						BaseSHA:  fooHEADsha,
						CloneURI: "http://fakegitserver.default/repo/foo2",
						Pulls: []prowjobv1.Pull{
							{
								Number: 1,
								SHA:    fooPR1sha,
							},
						},
					},
					PodSpec: &coreapi.PodSpec{
						Containers: []coreapi.Container{
							{
								Args: []string{
									"sh",
									"-c",
									fmt.Sprintf(`
# Check if PR1's commit SHA is in our HEAD's history (that we've merged it in).
if ! git merge-base --is-ancestor %s HEAD; then
	echo >&2 "[FAIL] PR1 was not merged in"
	exit 1
fi

# For good measure, check that we have a proper Git repo that the git client command can recognize.
expected=%s
got=$(git rev-parse HEAD)
if [ $got != $expected ]; then
	echo >&2 "[FAIL] got $got, expected $expected"
	>&2 git log
	exit 1
fi
`, fooPR1sha, fooPR1MergeSha),
								},
							},
						},
					},
				},
			},
		},
		{
			// Check that all 3 PRs have been merged.
			name: "presubmit-multi-pr",
			repoSetups: []lib.FGSRepoSetup{
				{
					Name:      "foo3",
					Script:    createRepoFoo,
					Overwrite: true,
				},
			},
			prowjob: prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"prow.k8s.io/type": "presubmit",
					},
					Labels: map[string]string{
						"prow.k8s.io/type": "presubmit",
					},
				},
				Spec: prowjobv1.ProwJobSpec{
					Type: prowjobv1.PresubmitJob,
					Refs: &prowjobv1.Refs{
						Repo:     "foo3",
						BaseSHA:  fooHEADsha,
						CloneURI: "http://fakegitserver.default/repo/foo3",
						Pulls: []prowjobv1.Pull{
							// For wider code coverage, we define the first Pull
							// with only a Ref, not a SHA.
							{
								Number: 1,
								Ref:    "pull/1/head",
							},
							{
								Number: 2,
								SHA:    fooPR2sha,
							},
							{
								Number: 3,
								SHA:    fooPR3sha,
							},
						},
					},
					PodSpec: &coreapi.PodSpec{
						Containers: []coreapi.Container{
							{
								Args: []string{
									"sh",
									"-c",
									fmt.Sprintf(`
# Check if *all* PR commit SHAs are in our HEAD's history.
if ! git merge-base --is-ancestor %s HEAD; then
	echo >&2 "[FAIL] PR1 was not merged in"
	exit 1
fi
if ! git merge-base --is-ancestor %s HEAD; then
	echo >&2 "[FAIL] PR2 was not merged in"
	exit 1
fi
if ! git merge-base --is-ancestor %s HEAD; then
	echo >&2 "[FAIL] PR3 was not merged in"
	exit 1
fi

# For good measure, check that we have a proper Git repo that the git client command can recognize.
expected=%s
got=$(git rev-parse HEAD)
if [ $got != $expected ]; then
	echo >&2 "[FAIL] got $got, expected $expected"
	>&2 git log
	exit 1
fi
`, fooPR1sha, fooPR2sha, fooPR3sha, fooMultiMergeSha),
								},
							},
						},
					},
				},
			},
		},
		{
			// Check that all 3 PRs have been merged and also that the submodule has been cloned.
			name: "presubmit-multi-pr-submodule",
			repoSetups: []lib.FGSRepoSetup{
				{
					Name:      "foo4",
					Script:    createRepoFoo,
					Overwrite: true,
				},
				{
					Name:      "bar",
					Script:    createRepoBar,
					Overwrite: true,
				},
			},
			prowjob: prowjobv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"prow.k8s.io/type": "presubmit",
					},
					Labels: map[string]string{
						"prow.k8s.io/type": "presubmit",
					},
				},
				Spec: prowjobv1.ProwJobSpec{
					Type: prowjobv1.PresubmitJob,
					Refs: &prowjobv1.Refs{
						Repo:     "bar",
						BaseSHA:  barHEADsha,
						CloneURI: "http://fakegitserver.default/repo/bar",
						Pulls: []prowjobv1.Pull{
							{
								Number: 1,
								Ref:    "pull/1/head",
							},
							{
								Number: 2,
								SHA:    barPR2sha,
							},
							{
								Number: 3,
								SHA:    barPR3sha,
							},
						},
					},
					PodSpec: &coreapi.PodSpec{
						Containers: []coreapi.Container{
							{
								Args: []string{
									"sh",
									"-c",
									fmt.Sprintf(`
expected=%s
got=$(git rev-parse HEAD)
if [ $got != $expected ]; then
	echo >&2 "[FAIL] (bar HEAD post-merge) got $got, expected $expected"
	exit 1
fi
# Check that the submodule is also populated (that it is set to the HEAD sha of
# foo).
cd foo4
expected=%s
got=$(git rev-parse HEAD)
if [ $got != $expected ]; then
	echo >&2 "[FAIL] (submodule foo4 HEAD) got $got, expected $expected"
	>&2 git log
	exit 1
fi
`, barMultiMergeWithSubmoduleSha, fooHEADsha),
								},
							},
						},
					},
				},
			},
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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a Prow Job and make it run.
			ctx := context.Background()

			// Create a unique podName. It is also used as the name of the
			// ProwJob CR we will create in the cluster to get a pod (with the
			// same name) scheduled. Uniqueness is important because we check
			// to see if this particular ProwJob finished successfully.
			podName := fmt.Sprintf("test-clonerefs-%s-%s", tt.name, RandomString(t))
			tt.prowjob.ObjectMeta.Annotations["prow.k8s.io/job"] = podName
			tt.prowjob.ObjectMeta.Name = podName
			tt.prowjob.Spec.Job = podName

			// We need to use DecorationConfig, because CloneRefs only gets
			// called if we have a decorated config.
			tt.prowjob.Spec.DecorationConfig = &prowjobv1.DecorationConfig{
				UtilityImages: &prowjobv1.UtilityImages{
					// These images get created by the integration test.
					CloneRefs:  "localhost:5001/clonerefs:latest",
					InitUpload: "localhost:5001/initupload:latest",
					Entrypoint: "localhost:5001/entrypoint:latest",
					Sidecar:    "localhost:5001/sidecar:latest",
				},
				GCSConfiguration: &prowjobv1.GCSConfiguration{
					PathStrategy: prowjobv1.PathStrategyExplicit,
					// This field makes initupload skip the upload to GCS.
					LocalOutputDir: "/output",
				},
			}

			// Set common miscellaneous fields shared by test cases.
			tt.prowjob.ObjectMeta.Namespace = defaultNamespace
			tt.prowjob.Spec.Namespace = defaultNamespace
			tt.prowjob.Spec.Agent = prowjobv1.KubernetesAgent
			tt.prowjob.Spec.Refs.Org = "some-org"
			tt.prowjob.Spec.Refs.BaseRef = "master"
			tt.prowjob.Spec.Refs.WorkDir = true
			// Use fakegitserver because it ships with sh and git, allowing us
			// to write self-documenting shell code to run as the test (to see
			// that clonerefs did actually clone things for us).
			tt.prowjob.Spec.PodSpec.Containers[0].Image = "localhost:5001/fakegitserver"

			tt.prowjob.Status = prowjobv1.ProwJobStatus{
				// Setting the state to TriggeredState tells Prow to schedule
				// (i.e., create a pod) and kick things off.
				State:     prowjobv1.TriggeredState,
				StartTime: v1.NewTime(time.Now().Add(-1 * time.Second)),
			}

			// Set up repos on FGS for just this test case.
			for _, repoSetup := range tt.repoSetups {
				buf, err := json.Marshal(repoSetup)
				if err != nil {
					t.Fatalf("could not marshal %v", repoSetup)
				}
				// Notice that this odd-looking URL is required because we (this
				// test) is not inside the KIND cluster and so we need to send
				// the packets to KIND (running on localhost). KIND will then
				// reroute the packets to fakegitserver.
				req, err := http.NewRequest("POST", "http://localhost/fakegitserver/setup-repo", bytes.NewBuffer(buf))
				if err != nil {
					t.Fatalf("failed to create POST request: %v", err)
				}
				req.Header.Set("Content-Type", "application/json; charset=UTF-8")
				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("FGS repo setup failed")
				}
				if resp.StatusCode != 200 {
					t.Fatalf("got %v response", resp.StatusCode)
				}
			}

			t.Logf("Creating prowjob: %s", podName)
			if err := kubeClient.Create(ctx, &tt.prowjob); err != nil {
				t.Fatalf("Failed creating prowjob: %v", err)
			}
			t.Logf("Finished creating prowjob: %s", podName)

			expectJobSuccess := func() (bool, error) {
				err = kubeClient.Get(ctx, ctrlruntimeclient.ObjectKey{Namespace: defaultNamespace, Name: podName}, &tt.prowjob)
				if err != nil {
					t.Logf("failed getting prow job: %s", podName)
					return false, nil
				}
				switch tt.prowjob.Status.State {
				case prowjobv1.SuccessState:
					return true, nil
				// If we observe a FailureState because the pod finished, exit
				// early with an error. This way we don't have to wait until the
				// timeout expires to see that the test failed.
				case prowjobv1.FailureState:
					return false, fmt.Errorf("prow job %s failed", podName)
				default:
					return false, nil
				}
			}

			// Wait up to 30 seconds to observe that this test passed.
			timeout := 30 * time.Second
			pollInterval := 500 * time.Millisecond
			if waitErr := wait.Poll(pollInterval, timeout, expectJobSuccess); waitErr != nil {
				// Retrieve logs from clonerefs.
				podLogs, err := getPodLogs(clientset, "test-pods", podName, &coreapi.PodLogOptions{Container: "clonerefs"})
				if err != nil {
					t.Errorf("failed getting logs for clonerefs")
				}
				t.Logf("logs for clonerefs:\n\n%s\n\n", podLogs)

				// If we got an error, show the failing prow job's test
				// container (our test case's "got" and "expected" shell code).
				pjPod := &coreapi.Pod{}
				err = kubeClient.Get(ctx, ctrlruntimeclient.ObjectKey{
					Namespace: "test-pods",
					Name:      podName,
				}, pjPod)
				if err != nil {
					t.Errorf("failed getting prow job's pod %v; unable to determine why the test failed", podName)
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
					if err := kubeClient.Delete(ctx, &tt.prowjob); err != nil {
						t.Logf("Failed cleanup resource %q: %v", tt.prowjob.Name, err)
					}
				})
			}
		})
	}
}

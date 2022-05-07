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

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"

	coreapi "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// fooHEADsha is the HEAD commit SHA of the "foo" repo that fakegitserver
	// always initializes on startup (see fakegitserver's
	// `-populate-sample-repos` option, which is set to true by default).
	fooHEADsha                    = "d502f45dd6b258da4d3002540ffe90b8858d3d37"
	fooPR1sha                     = "4f8372a904835cd9f3475620ea1495365a9ec511"
	fooPR1MergeSha                = "3fd823c8015fcb5dd6a7a47036b2921a8d38aef2"
	fooPR2sha                     = "abb72af1c44933257f1b7fedb7bcc22394941866"
	fooPR3sha                     = "51294238706cc5ba47835edb52bf31de51276d76"
	fooMultiMergeSha              = "7288ffdf3eb79d9e6b8b7ebec8920cefc6ac979f"
	barHEADsha                    = "98208bb2c686607dac8fe94ef42066b3954d7ca3"
	barPR2sha                     = "ec42e7c9cc483530cd2ee6feb89b714b80372884"
	barPR3sha                     = "ccdad555b1f5ca855908ab49e66fb5d9b5e2d453"
	barMultiMergeWithSubmoduleSha = "18319ef2ed035229f9b9f85661d5e5d8dbc8a5eb"
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

	tests := []struct {
		name    string
		prowjob prowjobv1.ProwJob
	}{
		{
			// Check if we got a properly cloned repo.
			name: "postsubmit",
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
						Repo:    "foo",
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
						CloneURI: "http://fakegitserver.default/repo/foo",
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
						Repo:     "foo",
						BaseSHA:  fooHEADsha,
						CloneURI: "http://fakegitserver.default/repo/foo",
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
						Repo:     "foo",
						BaseSHA:  fooHEADsha,
						CloneURI: "http://fakegitserver.default/repo/foo",
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
	echo >&2 "[FAIL] (bar HEAD) got $got, expected $expected"
	exit 1
fi
# Check that the submodule is also populated (that it is set to the HEAD sha of
# foo).
cd foo
expected=%s
got=$(git rev-parse HEAD)
if [ $got != $expected ]; then
	echo >&2 "[FAIL] (submodule foo HEAD) got $got, expected $expected"
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

	kubeClient, err := NewClients("", clusterContext)
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
			if err := wait.Poll(pollInterval, timeout, expectJobSuccess); err != nil {
				t.Fatal(err)
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

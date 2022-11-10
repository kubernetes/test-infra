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
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/test/integration/internal/fakegitserver"

	coreapi "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fooHEADsha = "e7d762376b714fdc2d6493ecd798a9d128e4b88f"
	fooPR1sha  = "b343a11088668210321ca76712e9bc3b8aa1f2f7"
	fooPR2sha  = "89bb95ef23c663d1b9ad97ff4351de516f25496c"
	fooPR3sha  = "2049df35eda98a9068ddfbc7bc6ce0ab5ef0d0ba"

	barHEADsha = "c2740e40972392a7c8954b389c71733e1f900561"
	barPR2sha  = "62d5ed7cbccce940421b7e234de28591472b6955"
	barPR3sha  = "f8387fe902fcb0c50d6d9e67f68b9802d591fd45"
)

// TestClonerefs tests the "clonerefs" binary by creating a ProwJob with
// DecorationConfig. Because of the DecorationConfig, clonerefs gets added as an
// "InitContainer" to the pod that gets scheduled for the ProwJob. We tweak
// CloneURI so that it clones from the fakegitserver instead of GitHub (or
// elsewhere). Inside the test container in the pod, we check that we can indeed
// see the cloned contents by checking various things like the SHAs and tree
// hashes.
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
		repoSetups []fakegitserver.RepoSetup
		prowjob    prowjobv1.ProwJob
		expected   string
	}{
		{
			// Check if we got a properly cloned repo.
			name: "postsubmit",
			repoSetups: []fakegitserver.RepoSetup{
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
						// The "/foo1" at the end is simply to refer to the Git
						// repo named "foo" in the "/git-repo/foo1" directory in
						// the fakegitserver container.
						CloneURI: "http://fakegitserver.default/repo/foo1",
					},
					PodSpec: &coreapi.PodSpec{
						Containers: []coreapi.Container{
							{
								Args: []string{
									"sh",
									"-c",
									`
cat <<EOF

HEAD: $(git rev-parse HEAD)

ls-tree:
$(git ls-tree HEAD)
EOF
`,
								},
							},
						},
					},
				},
			},
			// Use `git-ls-tree` to check that the repo __contents__
			// (disregarding commit metadata information) are exactly what we
			// expect. These tree SHA values should be stable even if we change
			// the name of the GIT_USER or other fields during clonerefs' "git
			// merge" invocations.
			expected: `
HEAD: e7d762376b714fdc2d6493ecd798a9d128e4b88f

ls-tree:
100644 blob a0423896973644771497bdc03eb99d5281615b51	README.txt
`,
		},
		{
			// Check that the PR has been merged.
			name: "presubmit-single-pr",
			repoSetups: []fakegitserver.RepoSetup{
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
cat <<EOF

HEAD: $(git rev-parse HEAD)

ls-tree:
$(git ls-tree HEAD)

PRs:
EOF

pr="%s"
if git merge-base --is-ancestor $pr HEAD; then
	echo $pr was merged into HEAD
else
	echo $pr was NOT merged into HEAD
fi
`, fooPR1sha),
								},
							},
						},
					},
				},
			},
			// Check if PR1's commit SHA is in our HEAD's history (that we've
			// merged it in).
			expected: `
HEAD: a081e60423aea6d3bdde2a50f8c435546529a932

ls-tree:
100644 blob d00491fd7e5bb6fa28c517a0bb32b8b506539d4d	1
100644 blob a0423896973644771497bdc03eb99d5281615b51	README.txt

PRs:
b343a11088668210321ca76712e9bc3b8aa1f2f7 was merged into HEAD
`,
		},
		{
			// Check that all 3 PRs have been merged.
			name: "presubmit-multi-pr",
			repoSetups: []fakegitserver.RepoSetup{
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
cat <<EOF

HEAD: $(git rev-parse HEAD)

ls-tree:
$(git ls-tree HEAD)

PRs:
EOF

for pr in %s %s %s; do
	if git merge-base --is-ancestor $pr HEAD; then
		echo $pr was merged into HEAD
	else
		echo $pr was NOT merged into HEAD
	fi
done
`, fooPR1sha, fooPR2sha, fooPR3sha),
								},
							},
						},
					},
				},
			},
			expected: `
HEAD: bc75802040886a627d4315b1c32635ab37ca0bba

ls-tree:
100644 blob d00491fd7e5bb6fa28c517a0bb32b8b506539d4d	1
100644 blob 0cfbf08886fca9a91cb753ec8734c84fcbe52c9f	2
100644 blob 00750edc07d6415dcc07ae0351e9397b0222b7ba	3
100644 blob a0423896973644771497bdc03eb99d5281615b51	README.txt

PRs:
b343a11088668210321ca76712e9bc3b8aa1f2f7 was merged into HEAD
89bb95ef23c663d1b9ad97ff4351de516f25496c was merged into HEAD
2049df35eda98a9068ddfbc7bc6ce0ab5ef0d0ba was merged into HEAD
`,
		},
		{
			// Check that all 3 PRs have been merged and also that the submodule has been cloned.
			name: "presubmit-multi-pr-submodule",
			repoSetups: []fakegitserver.RepoSetup{
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
									`
cat <<EOF

HEAD: $(git rev-parse HEAD)

ls-tree:
$(git ls-tree HEAD)

ls-tree (submodule):
$(cd foo4 && git ls-tree HEAD)
EOF
`,
								},
							},
						},
					},
				},
			},
			expected: `
HEAD: c6a86219ebf1917a0a53317067b6ad01a6ed4799

ls-tree:
100644 blob 04a95473544a5229662d133e42754b940fe06735	.gitmodules
100644 blob d00491fd7e5bb6fa28c517a0bb32b8b506539d4d	1
100644 blob 0cfbf08886fca9a91cb753ec8734c84fcbe52c9f	2
100644 blob 00750edc07d6415dcc07ae0351e9397b0222b7ba	3
100644 blob a0423896973644771497bdc03eb99d5281615b51	bar.txt
160000 commit e7d762376b714fdc2d6493ecd798a9d128e4b88f	foo4

ls-tree (submodule):
100644 blob a0423896973644771497bdc03eb99d5281615b51	README.txt
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

	fgsClient := fakegitserver.NewClient("http://localhost/fakegitserver", 5*time.Second)

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
					InitUpload: "localhost:5001/initupload-fakegcsserver:latest",
					Entrypoint: "localhost:5001/entrypoint:latest",
					Sidecar:    "localhost:5001/sidecar:latest",
				},
				GCSConfiguration: &prowjobv1.GCSConfiguration{
					PathStrategy: prowjobv1.PathStrategyExplicit,
					Bucket:       "bucket-foo",
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
				err := fgsClient.SetupRepo(repoSetup)
				if err != nil {
					t.Fatalf("FGS repo setup failed: %v", err)
				}
			}

			t.Logf("Creating prowjob: %s", podName)
			if err := kubeClient.Create(ctx, &tt.prowjob); err != nil {
				t.Fatalf("Failed creating prowjob: %v", err)
			}
			t.Logf("Finished creating prowjob: %s", podName)

			var lastErr error
			expectJobSuccess := func() (bool, error) {
				lastErr = nil
				// Check pod status instead of prowjob, to reduce the dependency
				// on prow-controller-manager.
				var pod coreapi.Pod
				lastErr = kubeClient.Get(ctx, ctrlruntimeclient.ObjectKey{Namespace: testpodNamespace, Name: podName}, &pod)
				if lastErr != nil {
					return false, nil
				}
				var finished bool
				for _, c := range pod.Status.ContainerStatuses {
					if c.Name != "test" {
						continue
					}
					if c.State.Terminated == nil {
						// Not finished yet
						continue
					}
					if c.State.Terminated.ExitCode != 0 {
						// If we observe a FailureState because the pod finished, exit
						// early with an error. This way we don't have to wait until the
						// timeout expires to see that the test failed.
						//
						// This should only happen for programmer errors (if there were
						// unintended errors in the shell script (`sh -c ...`) that runs
						// in the test case').
						lastErr = fmt.Errorf("possible programmer error: clonerefs %s failed with exit code '%d', message: '%s'", podName, c.State.Terminated.ExitCode, c.State.Terminated.Message)
						return false, lastErr
					}
					finished = true
				}

				if !finished {
					return false, nil
				}

				// Check logs of the finished ProwJob. We simply diff these
				// logs against what we expect in the test case. This is
				// much simpler than running this sort of comparison check
				// inside the test pod itself, because here we can get all
				// the pretty-printing facilities of cmp.Diff().
				got, err := getPodLogs(clientset, "test-pods", podName, &coreapi.PodLogOptions{Container: "test"})
				if err != nil {
					t.Errorf("failed getting logs for clonerefs")
					return false, nil
				}
				if diff := cmp.Diff(got, tt.expected); diff != "" {
					lastErr = fmt.Errorf("actual logs differ from expected: %s", diff)
					return true, lastErr
				}

				return true, nil
			}

			// Wait up to 90 seconds to observe that this test passed.
			// These 90 seconds are spent waiting on serial execution of
			// clonerefs, initupload, sidecar, place-entrypoint then test. Based
			// on observation from kind logs these could take ~60 seconds.
			timeout := 90 * time.Second
			pollInterval := 500 * time.Millisecond
			if waitErr := wait.Poll(pollInterval, timeout, expectJobSuccess); waitErr != nil {
				if lastErr != nil {
					t.Logf("The last wait error is: %v", lastErr)
				}
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

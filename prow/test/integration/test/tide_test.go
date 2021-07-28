/*
Copyright 2020 The Kubernetes Authors.

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
	"strconv"
	"testing"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/github"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTide(t *testing.T) {
	const (
		baseRef = "master"
		// matches tide deployment configurating in //prow/test/integration/prow/config.yaml
		// for tide to query the right org/repo
		org  = "fake-org-tide"
		repo = "fake-repo-tide"
	)

	clusterContext := getClusterContext()
	t.Logf("Creating client for cluster: %s", clusterContext)

	kubeClient, err := NewClients("", clusterContext)
	if err != nil {
		t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
	}

	githubClient := github.NewClient(func() []byte { return nil }, func(b []byte) []byte { return b }, "", "http://localhost/fakeghserver")

	t.Run("merge single job", func(t *testing.T) {
		ctx := context.Background()
		title := RandomString(t)
		sha := RandomString(t)
		baseSHA := RandomString(t)

		prowJob, err := createPR(ctx, githubClient, kubeClient, org, repo, title, sha, baseSHA, baseRef)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			if err := kubeClient.Delete(ctx, prowJob); err != nil {
				t.Logf("Failed cleanup resource %q: %v", prowJob.Name, err)
			}
		})

		// TODO: Test tide handling
		// Currently a PR is being created on fakeghserver, and a successful presubmit prowjob.
		// tide manages to query it. The fakeghserver gets a correct query for fake-org-tide/fake-repo-tide
		// and sends a response accordingly, with the already created pull request info.
		// But from some reason it is being returned to `tide` empty,
		// so tide does nothing with an empty response.
	})
}

func createPR(ctx context.Context, githubClient github.Client, kubeClient ctrlruntimeclient.Client,
	org, repo, title, sha, baseSHA, baseRef string) (*prowjobv1.ProwJob, error) {
	prID, err := githubClient.CreatePullRequest(org, repo, title, "body", baseSHA, baseRef, true)
	if err != nil {
		return nil, fmt.Errorf("Failed creating pull request: %v", err)
	}

	podName := fmt.Sprintf("tide-%s", title)

	prowJob := prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				"prow.k8s.io/job":       podName,
				"prow.k8s.io/refs.org":  org,
				"prow.k8s.io/refs.pull": strconv.Itoa(prID),
				"prow.k8s.io/refs.repo": repo,
				"prow.k8s.io/type":      "presubmit",
			},
			Labels: map[string]string{
				"created-by-prow":  "true",
				"prow.k8s.io/type": "presubmit",
			},
			Name:      podName,
			Namespace: defaultNamespace,
		},
		Spec: prowjobv1.ProwJobSpec{
			Type:      prowjobv1.PresubmitJob,
			Namespace: "test-pods",
			Job:       podName,
			Refs: &prowjobv1.Refs{
				Org:     org,
				Repo:    repo,
				BaseRef: baseRef,
				BaseSHA: baseSHA,
				Pulls: []prowjobv1.Pull{
					{
						Author: "fake_author",
						Number: prID,
						SHA:    sha,
					},
				},
			},
			Report: true,
		},
		Status: prowjobv1.ProwJobStatus{
			State:          "success",
			StartTime:      v1.NewTime(time.Now().Add(-1 * time.Second)),
			CompletionTime: &v1.Time{Time: time.Now()},
		},
	}

	if err := kubeClient.Create(ctx, &prowJob); err != nil {
		return nil, fmt.Errorf("Failed creating prowjob: %v", err)
	}

	return &prowJob, nil
}

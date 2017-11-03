/*
Copyright 2017 The Kubernetes Authors.

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

package pjutil

import (
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestProwJobToPod(t *testing.T) {
	tests := []struct {
		podName string
		buildID string
		labels  map[string]string
		pjSpec  kube.ProwJobSpec

		expected *kube.Pod
	}{
		{
			podName: "pod",
			buildID: "blabla",
			labels:  map[string]string{"needstobe": "inherited"},
			pjSpec: kube.ProwJobSpec{
				Type:  kube.PresubmitJob,
				Job:   "job-name",
				Agent: kube.KubernetesAgent,
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
				},
				PodSpec: kube.PodSpec{
					Containers: []kube.Container{
						{
							Image: "tester",
							Env: []kube.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
							},
						},
					},
				},
			},

			expected: &kube.Pod{
				Metadata: kube.ObjectMeta{
					Name: "pod",
					Labels: map[string]string{
						kube.CreatedByProw:    "true",
						kube.ProwJobTypeLabel: "presubmit",
						"needstobe":           "inherited",
					},
					Annotations: map[string]string{
						kube.ProwJobAnnotation: "job-name",
					},
				},
				Spec: kube.PodSpec{
					RestartPolicy: "Never",
					Containers: []kube.Container{
						{
							Name:  "pod-0",
							Image: "tester",
							Env: []kube.EnvVar{
								{Name: "MY_ENV", Value: "rocks"},
								{Name: "BUILD_NUMBER", Value: "blabla"},
								{Name: "JOB_NAME", Value: "job-name"},
								{Name: "JOB_TYPE", Value: "presubmit"},
								{Name: "BUILD_ID", Value: "blabla"},
								{Name: "JOB_SPEC", Value: `{"type":"presubmit","job":"job-name","buildid":"blabla","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}]}}`},
								{Name: "PULL_BASE_REF", Value: "base-ref"},
								{Name: "REPO_OWNER", Value: "org-name"},
								{Name: "REPO_NAME", Value: "repo-name"},
								{Name: "PULL_BASE_SHA", Value: "base-sha"},
								{Name: "PULL_REFS", Value: "base-ref:base-sha,1:pull-sha"},
								{Name: "PULL_NUMBER", Value: "1"},
								{Name: "PULL_PULL_SHA", Value: "pull-sha"},
							},
						},
					},
				},
			},
		},
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		pj := kube.ProwJob{Metadata: kube.ObjectMeta{Name: test.podName, Labels: test.labels}, Spec: test.pjSpec}
		got, err := ProwJobToPod(pj, test.buildID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// TODO: For now I am just comparing fields manually, eventually we
		// should port the semantic.DeepEqual helper from the api-machinery
		// repo, which is basically a fork of the reflect package.
		// if !semantic.DeepEqual(got, test.expected) {
		//	 t.Errorf("got pod:\n%#v\n\nexpected pod:\n%#v\n", got, test.expected)
		// }
		var foundCreatedByLabel, foundTypeLabel, foundJobAnnotation bool
		for key, value := range got.Metadata.Labels {
			if key == kube.CreatedByProw && value == "true" {
				foundCreatedByLabel = true
			}
			if key == kube.ProwJobTypeLabel && value == string(pj.Spec.Type) {
				foundTypeLabel = true
			}
			var match bool
			for k, v := range test.expected.Metadata.Labels {
				if k == key && v == value {
					match = true
					break
				}
			}
			if !match {
				t.Errorf("expected labels: %v, got: %v", test.expected.Metadata.Labels, got.Metadata.Labels)
			}
		}
		for key, value := range got.Metadata.Annotations {
			if key == kube.ProwJobAnnotation && value == pj.Spec.Job {
				foundJobAnnotation = true
			}
		}
		if !foundCreatedByLabel {
			t.Errorf("expected a created-by-prow=true label in %v", got.Metadata.Labels)
		}
		if !foundTypeLabel {
			t.Errorf("expected a %s=%s label in %v", kube.ProwJobTypeLabel, pj.Spec.Type, got.Metadata.Labels)
		}
		if !foundJobAnnotation {
			t.Errorf("expected a %s=%s annotation in %v", kube.ProwJobAnnotation, pj.Spec.Job, got.Metadata.Annotations)
		}

		expectedContainer := test.expected.Spec.Containers[i]
		gotContainer := got.Spec.Containers[i]

		dumpGotEnv := false
		for _, expectedEnv := range expectedContainer.Env {
			found := false
			for _, gotEnv := range gotContainer.Env {
				if expectedEnv.Name == gotEnv.Name && expectedEnv.Value == gotEnv.Value {
					found = true
					break
				}
			}
			if !found {
				dumpGotEnv = true
				t.Errorf("could not find expected env %s=%s", expectedEnv.Name, expectedEnv.Value)
			}
		}
		if dumpGotEnv {
			t.Errorf("expected env:\n%#v\ngot:\n%#v\n", expectedContainer.Env, gotContainer.Env)
		}
		if expectedContainer.Image != gotContainer.Image {
			t.Errorf("expected image: %s, got: %s", expectedContainer.Image, gotContainer.Image)
		}
		if test.expected.Spec.RestartPolicy != got.Spec.RestartPolicy {
			t.Errorf("expected restart policy: %s, got: %s", test.expected.Spec.RestartPolicy, got.Spec.RestartPolicy)
		}
	}
}

func TestPartitionPending(t *testing.T) {
	tests := []struct {
		pjs []kube.ProwJob

		pending    map[string]struct{}
		nonPending map[string]struct{}
	}{
		{
			pjs: []kube.ProwJob{
				{
					Metadata: kube.ObjectMeta{
						Name: "foo",
					},
					Status: kube.ProwJobStatus{
						State: kube.TriggeredState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "bar",
					},
					Status: kube.ProwJobStatus{
						State: kube.PendingState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "baz",
					},
					Status: kube.ProwJobStatus{
						State: kube.SuccessState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "error",
					},
					Status: kube.ProwJobStatus{
						State: kube.ErrorState,
					},
				},
				{
					Metadata: kube.ObjectMeta{
						Name: "bak",
					},
					Status: kube.ProwJobStatus{
						State: kube.PendingState,
					},
				},
			},
			pending: map[string]struct{}{
				"bar": {}, "bak": {},
			},
			nonPending: map[string]struct{}{
				"foo": {}, "baz": {}, "error": {},
			},
		},
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		pendingCh, nonPendingCh := PartitionPending(test.pjs)
		for job := range pendingCh {
			if _, ok := test.pending[job.Metadata.Name]; !ok {
				t.Errorf("didn't find pending job %#v", job)
			}
		}
		for job := range nonPendingCh {
			if _, ok := test.nonPending[job.Metadata.Name]; !ok {
				t.Errorf("didn't find non-pending job %#v", job)
			}
		}
	}
}

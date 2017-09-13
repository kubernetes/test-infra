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
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestEnvironmentForSpec(t *testing.T) {
	var tests = []struct {
		name     string
		spec     kube.ProwJobSpec
		expected map[string]string
	}{
		{
			name: "periodic job",
			spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "job-name",
			},
			expected: map[string]string{
				"JOB_NAME": "job-name",
			},
		},
		{
			name: "postsubmit job",
			spec: kube.ProwJobSpec{
				Type: kube.PostsubmitJob,
				Job:  "job-name",
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
				},
			},
			expected: map[string]string{
				"JOB_NAME":      "job-name",
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha",
			},
		},
		{
			name: "batch job",
			spec: kube.ProwJobSpec{
				Type: kube.BatchJob,
				Job:  "job-name",
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}, {
						Number: 2,
						Author: "other-author-name",
						SHA:    "second-pull-sha",
					}},
				},
			},
			expected: map[string]string{
				"JOB_NAME":      "job-name",
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha,1:pull-sha,2:second-pull-sha",
			},
		},
		{
			name: "presubmit job",
			spec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  "job-name",
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
			},
			expected: map[string]string{
				"JOB_NAME":      "job-name",
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha,1:pull-sha",
				"PULL_NUMBER":   "1",
				"PULL_PULL_SHA": "pull-sha",
			},
		},
	}

	for _, test := range tests {
		if actual, expected := EnvForSpec(test.spec), test.expected; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: got environment:\n\t%v\n\tbut expected:\n\t%v", test.name, actual, expected)
		}
	}
}

func TestProwJobToPod(t *testing.T) {
	tests := []struct {
		podName string
		buildID string
		pjSpec  kube.ProwJobSpec

		expected *kube.Pod
	}{
		{
			podName: "pod",
			buildID: "blabla",
			pjSpec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  "job-name",
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
						kube.CreatedByProw: "true",
						"type":             "presubmit",
						"job":              "job-name",
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
		pj := kube.ProwJob{Metadata: kube.ObjectMeta{Name: test.podName}, Spec: test.pjSpec}
		got := ProwJobToPod(pj, test.buildID)
		// TODO: For now I am just comparing fields manually, eventually we
		// should port the semantic.DeepEqual helper from the api-machinery
		// repo, which is basically a fork of the reflect package.
		// if !semantic.DeepEqual(got, test.expected) {
		//	 t.Errorf("got pod:\n%#v\n\nexpected pod:\n%#v\n", got, test.expected)
		// }
		var foundCreatedByLabel, foundTypeLabel, foundJobLabel bool
		for key, value := range got.Metadata.Labels {
			if key == kube.CreatedByProw && value == "true" {
				foundCreatedByLabel = true
			}
			if key == "type" && value == string(pj.Spec.Type) {
				foundTypeLabel = true
			}
			if key == "job" && value == pj.Spec.Job {
				foundJobLabel = true
			}
		}
		if !foundCreatedByLabel {
			t.Errorf("expected a created-by-prow=true label in %v", got.Metadata.Labels)
		}
		if !foundTypeLabel {
			t.Errorf("expected a type=%s label in %v", pj.Spec.Type, got.Metadata.Labels)
		}
		if !foundJobLabel {
			t.Errorf("expected a job=%s label in %v", pj.Spec.Job, got.Metadata.Labels)
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

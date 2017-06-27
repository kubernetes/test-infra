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

package plank

import (
	"bytes"
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestURLTemplate(t *testing.T) {
	testcases := []struct {
		name    string
		jobType kube.ProwJobType
		org     string
		repo    string
		job     string
		build   string
		expect  string
	}{
		{
			name:    "k8s presubmit",
			jobType: kube.PresubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/0/k8s-pre-1/1/",
		},
		{
			name:    "k8s/test-infra presubmit",
			jobType: kube.PresubmitJob,
			org:     "kubernetes",
			repo:    "test-infra",
			job:     "ti-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/test-infra/0/ti-pre-1/1/",
		},
		{
			name:    "foo/k8s presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/foo_kubernetes/0/k8s-pre-1/1/",
		},
		{
			name:    "foo-bar presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "bar",
			job:     "foo-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/foo_bar/0/foo-pre-1/1/",
		},
		{
			name:    "k8s postsubmit",
			jobType: kube.PostsubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-post-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/k8s-post-1/1/",
		},
		{
			name:    "k8s periodic",
			jobType: kube.PeriodicJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-peri-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/k8s-peri-1/1/",
		},
		{
			name:    "empty periodic",
			jobType: kube.PeriodicJob,
			org:     "",
			repo:    "",
			job:     "nan-peri-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/nan-peri-1/1/",
		},
		{
			name:    "k8s batch",
			jobType: kube.BatchJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-batch-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/batch/k8s-batch-1/1/",
		},
	}

	for _, tc := range testcases {
		var pj = kube.ProwJob{
			Metadata: kube.ObjectMeta{Name: tc.name},
			Spec: kube.ProwJobSpec{
				Type: tc.jobType,
				Job:  tc.job,
				Refs: kube.Refs{
					Pulls: []kube.Pull{{}},
					Org:   tc.org,
					Repo:  tc.repo,
				},
			},
			Status: kube.ProwJobStatus{
				BuildID: tc.build,
			},
		}

		var b bytes.Buffer
		if err := urlTmpl.Execute(&b, &pj); err != nil {
			t.Fatalf("Error executing template: %v", err)
		}
		res := b.String()
		if res != tc.expect {
			t.Errorf("tc: %s, Expect URL: %s, got %s", tc.name, tc.expect, res)
		}
	}
}

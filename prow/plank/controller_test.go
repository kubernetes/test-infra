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
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestGuberURL(t *testing.T) {

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
			name:    "foo-bar presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "bar",
			job:     "foo-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/foo_bar/0/foo-pre-1/1/",
		},
		{
			name:    "nan-bar presubmit",
			jobType: kube.PresubmitJob,
			org:     "",
			repo:    "bar",
			job:     "bar-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/bar/0/bar-pre-1/1/",
		},
		{
			name:    "foo-nan presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "",
			job:     "foo-pre-2",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/foo_/0/foo-pre-2/1/",
		},
		{
			name:    "nan presubmit",
			jobType: kube.PresubmitJob,
			org:     "",
			repo:    "",
			job:     "nan-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/0/nan-pre-1/1/",
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
		}

		res := guberURL(pj, tc.build)
		if res != tc.expect {
			t.Errorf("tc: %s, Expect URL: %s, got %s", tc.name, tc.expect, res)
		}
	}
}

/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"testing"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/kube/fakekube"
)

// Make sure the job we build has the proper spec.
func TestCreateJob(t *testing.T) {
	c := &fakekube.FakeClient{}
	ka := &KubeAgent{
		Namespace:  "ns",
		LineImage:  "im:tag",
		KubeClient: c,
	}
	br := KubeRequest{
		JobName: "kubernetes-e2e-gce",
		Context: "GCE e2e",

		RepoOwner: "kubernetes",
		RepoName:  "kubernetes",
		PR:        123,
		BaseRef:   "master",
		PullSHA:   "12345abcde",
	}
	if err := ka.createJob(br); err != nil {
		t.Fatalf("Didn't expect error: %s", err)
	}
	if len(c.Jobs) != 1 {
		t.Errorf("Wrong number of jobs after create: %d", len(c.Jobs))
	}
	j := c.Jobs[0]
	if _, ok := j.Metadata.Labels["repo"]; !ok {
		t.Errorf("No repo label: %+v", j)
	} else if j.Metadata.Labels["repo"] != "kubernetes" {
		t.Errorf("Wrong repo label: %+v", j)
	}
	if _, ok := j.Metadata.Labels["pr"]; !ok {
		t.Errorf("No pr label: %+v", j)
	} else if j.Metadata.Labels["pr"] != "123" {
		t.Errorf("Wrong pr label: %+v", j)
	}
	if _, ok := j.Metadata.Labels["jenkins-job-name"]; !ok {
		t.Errorf("No jenkins-job-name label: %+v", j)
	} else if j.Metadata.Labels["jenkins-job-name"] != "kubernetes-e2e-gce" {
		t.Errorf("Wrong jenkins-job-name label: %+v", j)
	}
	if j.Metadata.Namespace != "ns" {
		t.Errorf("Wrong namespace: %+v", j)
	}
}

func TestDeletePR(t *testing.T) {
	c := &fakekube.FakeClient{
		Jobs: []kube.Job{
			{
				// Delete this one.
				Metadata: kube.ObjectMeta{
					Name: "12345",
					Labels: map[string]string{
						"owner":            "o",
						"pr":               "3",
						"repo":             "r",
						"jenkins-job-name": "job",
					},
				},
			},
			{
				// Different PR.
				Metadata: kube.ObjectMeta{
					Name: "abcde",
					Labels: map[string]string{
						"owner":            "o",
						"pr":               "4",
						"repo":             "r",
						"jenkins-job-name": "job",
					},
				},
			},
			{
				// Different repo.
				Metadata: kube.ObjectMeta{
					Name: "qwerty",
					Labels: map[string]string{
						"owner":            "o",
						"pr":               "3",
						"repo":             "q",
						"jenkins-job-name": "job",
					},
				},
			},
			{
				// Different job name.
				Metadata: kube.ObjectMeta{
					Name: "o-r-pr-3-abcd-otherjob",
					Labels: map[string]string{
						"owner":            "o",
						"pr":               "3",
						"repo":             "r",
						"jenkins-job-name": "otherjob",
					},
				},
			},
		},
	}
	s := &KubeAgent{
		KubeClient: c,
	}
	br := KubeRequest{
		JobName:   "job",
		PR:        3,
		RepoOwner: "o",
		RepoName:  "r",
	}
	s.deleteJob(br)
	for _, j := range c.Jobs {
		if j.Metadata.Name == "12345" && j.Spec.Parallelism == nil {
			t.Error("Job not deleted.")
		} else if j.Metadata.Name != "12345" && j.Spec.Parallelism != nil {
			t.Errorf("Deleted wrong job: %s", j.Metadata.Name)
		}
	}
}

/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestPickLatest(t *testing.T) {
	jobs := []kube.ProwJob{
		// We're using Cluster as a simple way to distinguish runs
		{Spec: kube.ProwJobSpec{Job: "glob-1", Cluster: "1"}},
		{Spec: kube.ProwJobSpec{Job: "glob-1", Cluster: "2"}},
		{Spec: kube.ProwJobSpec{Job: "glob-2", Cluster: "1"}},
		{Spec: kube.ProwJobSpec{Job: "job-a", Cluster: "1"}},
		{Spec: kube.ProwJobSpec{Job: "job-ab", Cluster: "1"}},
	}
	expected := []kube.ProwJob{
		{Spec: kube.ProwJobSpec{Job: "glob-1", Cluster: "1"}},
		{Spec: kube.ProwJobSpec{Job: "glob-2", Cluster: "1"}},
		{Spec: kube.ProwJobSpec{Job: "job-a", Cluster: "1"}},
	}
	result := pickLatestJobs(jobs, "glob-*,job-a")
	if !reflect.DeepEqual(result, expected) {
		fmt.Println("expected:")
		for _, job := range expected {
			fmt.Printf("  job: %s cluster: %s,", job.Spec.Job, job.Spec.Cluster)
		}
		fmt.Println("got:")
		for _, job := range result {
			fmt.Printf("  job: %s cluster: %s,", job.Spec.Job, job.Spec.Cluster)
		}
	}
}

func TestRenderBadge(t *testing.T) {
	for _, tc := range []struct {
		jobStates      []string
		expectedColor  string
		expectedStatus string
	}{
		{nil, "darkgrey", "no results"},
		{[]string{"success"}, "brightgreen", "passing"},
		{[]string{"success", "failure"}, "red", "failing 2"},
		{[]string{"success", "failure", "failure", "failure", "failure"}, "red", "failing 2, 3, 4, ..."},
	} {
		jobs := []kube.ProwJob{}
		for i, state := range tc.jobStates {
			jobs = append(jobs, kube.ProwJob{
				Spec:   kube.ProwJobSpec{Job: fmt.Sprintf("%d", i+1)},
				Status: kube.ProwJobStatus{State: kube.ProwJobState(state)},
			})
		}
		status, color, _ := renderBadge(jobs)
		if color != tc.expectedColor {
			t.Errorf("unexpected color for %v: got %s instead of %s", tc.jobStates, color, tc.expectedColor)
		}
		if status != tc.expectedStatus {
			t.Errorf("unexpected status for %v: got %s instead of %s", tc.jobStates, status, tc.expectedStatus)
		}
	}
}

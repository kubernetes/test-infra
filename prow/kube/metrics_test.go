/*
Copyright 2019 The Kubernetes Authors.

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

package kube

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestGetJobLabelMap(t *testing.T) {
	pjs := []prowapi.ProwJob{
		{
			Spec: prowapi.ProwJobSpec{
				Job:  "test-job-1",
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:     "org1",
					Repo:    "repo1",
					BaseRef: "master",
				},
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.PendingState,
			},
		},
		{
			Spec: prowapi.ProwJobSpec{
				Job:  "test-job-1",
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:     "org1",
					Repo:    "repo1",
					BaseRef: "master",
				},
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.PendingState,
			},
		},
		{
			Spec: prowapi.ProwJobSpec{
				Job:  "test-job-2",
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:     "org1",
					Repo:    "repo1",
					BaseRef: "master",
				},
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.PendingState,
			},
		},
		{
			Spec: prowapi.ProwJobSpec{
				Job:  "test-job-2",
				Type: prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org:     "org1",
					Repo:    "repo1",
					BaseRef: "release-4.1",
				},
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.PendingState,
			},
		},
		{
			Spec: prowapi.ProwJobSpec{
				Job:  "test-job-3",
				Type: prowapi.PresubmitJob,
				Refs: nil,
				ExtraRefs: []prowapi.Refs{
					{
						Org:     "org1",
						Repo:    "repo1",
						BaseRef: "release-4.2",
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				State: prowapi.FailureState,
			},
		},
	}

	jobLabelMap := getJobLabelMap(pjs)

	expected := map[jobLabel]float64{
		{jobName: "test-job-1", jobType: string(prowapi.PresubmitJob), org: "org1", repo: "repo1", baseRef: "master", state: string(prowapi.PendingState)}:      2,
		{jobName: "test-job-2", jobType: string(prowapi.PresubmitJob), org: "org1", repo: "repo1", baseRef: "master", state: string(prowapi.PendingState)}:      1,
		{jobName: "test-job-2", jobType: string(prowapi.PresubmitJob), org: "org1", repo: "repo1", baseRef: "release-4.1", state: string(prowapi.PendingState)}: 1,
		{jobName: "test-job-3", jobType: string(prowapi.PresubmitJob), org: "org1", repo: "repo1", baseRef: "release-4.2", state: string(prowapi.FailureState)}: 1,
	}

	if !reflect.DeepEqual(expected, jobLabelMap) {
		t.Errorf("Unexpected mis-match: %s", diff.ObjectReflectDiff(expected, jobLabelMap))
	}
}

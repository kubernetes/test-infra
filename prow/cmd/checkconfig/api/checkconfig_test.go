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

package checkconfig

import (
	"testing"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

func TestValidatePresubmitJob(t *testing.T) {

	testCases := []struct {
		name        string
		agent       v1.ProwJobAgent
		jobName     string
		errExpected bool
	}{
		{
			name:    "Kubernetes presubmit with short name, no err",
			agent:   v1.KubernetesAgent,
			jobName: "my-valid-job",
		},
		{
			name:        "Kubernetes presubmit with 64 char name, err",
			agent:       v1.KubernetesAgent,
			jobName:     "Very long job name because short ones can be done by everyone!!!",
			errExpected: true,
		},
		{
			name:    "Build presubmit with 64 char name, no err",
			agent:   v1.KnativeBuildAgent,
			jobName: "Very long job name because short ones can be done by everyone!!!",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ps := config.Presubmit{
				JobBase: config.JobBase{
					Name:  tc.jobName,
					Agent: string(tc.agent),
				}}

			if err := ValidatePresubmitJob("", ps); err != nil != tc.errExpected {
				t.Errorf("Did expected err: %t but wasnt the case", tc.errExpected)
			}
		})
	}
}

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
)

type fakeJobResult struct {
	err error
}

func Test_resultForJob(t *testing.T) {
	type args struct {
		pj       prowapi.ProwJob
		selector string
	}
	testcases := []struct {
		name             string
		args             args
		expected         prowjobResult
		expectToContinue bool
		expectedErr      error
	}{
		{
			name: "Prowjob still executing",
			args: args{
				pj: prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "winwin",
						Namespace: "prowjobs",
					},
					Spec: prowapi.ProwJobSpec{
						Job: "test-job",
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
				selector: "test-job",
			},
			expected: prowjobResult{
				Status: "pending",
				URL:    "result.com",
			},
			expectToContinue: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fakeProwJobClient := fake.NewSimpleClientset(&tc.args.pj).ProwV1().ProwJobs("prowjobs")
			pjr, shouldContinue, err := resultForJob(fakeProwJobClient, tc.args.selector)
			if !reflect.DeepEqual(pjr, tc.expected) {
				t.Errorf("resultForJob() got = %v, want %v", pjr, tc.expected)
			}
			if tc.expectToContinue != shouldContinue {
				t.Errorf("resultForJob() got = %v, want %v", shouldContinue, tc.expectToContinue)
			}
			if !reflect.DeepEqual(tc.expectedErr, err) {
				t.Errorf("resultForJob() got = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

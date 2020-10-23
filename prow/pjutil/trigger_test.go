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
	"k8s.io/apimachinery/pkg/watch"
	coretesting "k8s.io/client-go/testing"
	pjapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
)

type fakeJobResult struct {
	err error
}

func Test_resultForJob(t *testing.T) {
	type args struct {
		pj           prowapi.ProwJob
		watchResults []prowapi.ProwJob
		selector     string
	}
	testcases := []struct {
		name             string
		args             args
		expected         pjapi.ProwJobStatus
		expectToContinue bool
		expectedErr      error
	}{
		{
			name: "Prowjob completed successfully",
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
						State: prowapi.TriggeredState,
					},
				},
				watchResults: []prowapi.ProwJob{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "winwin",
							Namespace: "prowjobs",
						},
						Spec: prowapi.ProwJobSpec{
							Job: "test-job",
						},
						Status: prowapi.ProwJobStatus{
							State: prowapi.SuccessState,
						},
					},
				},
				selector: "metadata.name=winwin",
			},
			expected: pjapi.ProwJobStatus{
				State: prowapi.SuccessState,
			},
			expectToContinue: false,
		},
		{
			name: "Longer running prowjob",
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
						State: prowapi.TriggeredState,
					},
				},
				watchResults: []prowapi.ProwJob{
					{
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
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "winwin",
							Namespace: "prowjobs",
						},
						Spec: prowapi.ProwJobSpec{
							Job: "test-job",
						},
						Status: prowapi.ProwJobStatus{
							State: prowapi.SuccessState,
						},
					},
				},
				selector: "metadata.name=winwin",
			},
			expected: pjapi.ProwJobStatus{
				State: prowapi.SuccessState,
			},
			expectToContinue: false,
		},
		{
			name: "Prowjob failed",
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
						State: prowapi.TriggeredState,
					},
				},
				watchResults: []prowapi.ProwJob{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "winwin",
							Namespace: "prowjobs",
						},
						Spec: prowapi.ProwJobSpec{
							Job: "test-job",
						},
						Status: prowapi.ProwJobStatus{
							State: prowapi.FailureState,
						},
					},
				},
				selector: "metadata.name=winwin",
			},
			expected: pjapi.ProwJobStatus{
				State: prowapi.FailureState,
			},
			expectToContinue: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset(&tc.args.pj)
			cs.Fake.PrependWatchReactor("prowjobs", func(action coretesting.Action) (bool, watch.Interface, error) {
				ret := watch.NewFakeWithChanSize(len(tc.args.watchResults), true)
				for _, res := range tc.args.watchResults {
					ret.Modify(&res)
				}
				return true, ret, nil
			})
			pjr, shouldContinue, err := resultForJob(cs.ProwV1().ProwJobs("prowjobs"), tc.args.selector)
			if !reflect.DeepEqual(pjr.State, tc.expected.State) {
				t.Errorf("resultForJob() ProwJobStatus got = %v, want %v", pjr, tc.expected)
			}
			if tc.expectToContinue != shouldContinue {
				t.Errorf("resultForJob() ShouldContinue got = %v, want %v", shouldContinue, tc.expectToContinue)
			}
			if !reflect.DeepEqual(tc.expectedErr, err) {
				t.Errorf("resultForJob() error got = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

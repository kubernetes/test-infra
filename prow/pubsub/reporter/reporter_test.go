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

package reporter

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/test-infra/prow/kube"
)

const (
	testPubSubProjectName = "test-project"
	testPubSubTopicName   = "test-topic"
	testPubSubRunID       = "test-id"
)

func TestGenerateMessageFromPJ(t *testing.T) {
	var testcases = []struct {
		name            string
		pj              *kube.ProwJob
		expectedMessage *ReportMessage
		expectedError   error
	}{
		{
			name: "Prowjob with all information should work with no error",
			pj: &kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						pubsubProjectLabel: testPubSubProjectName,
						pubsubTopicLabel:   testPubSubTopicName,
						pubsubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.SuccessState,
					URL:   "guber/test1",
				},
			},
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   testPubSubRunID,
				Status:  kube.SuccessState,
				URL:     "guber/test1",
			},
		},
		{
			name: "Prowjob has no pubsub runID label, should return a message with runid empty",
			pj: &kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-runID",
					Labels: map[string]string{
						pubsubProjectLabel: testPubSubProjectName,
						pubsubTopicLabel:   testPubSubTopicName,
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.SuccessState,
				},
			},
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   "",
				Status:  kube.SuccessState,
			},
		},
	}

	for _, tc := range testcases {
		m := generateMessageFromPJ(tc.pj)

		if !reflect.DeepEqual(m, tc.expectedMessage) {
			t.Errorf("Unexpected result from test: %s.\nExpected: %v\nGot: %v",
				tc.name, tc.expectedMessage, m)
		}
	}
}

func TestShouldReport(t *testing.T) {
	var testcases = []struct {
		name           string
		pj             *kube.ProwJob
		expectedResult bool
	}{
		{
			name: "Prowjob with all pubsub information should return",
			pj: &kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						pubsubProjectLabel: testPubSubProjectName,
						pubsubTopicLabel:   testPubSubTopicName,
						pubsubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.SuccessState,
				},
			},
			expectedResult: true,
		},
		{
			name: "Prowjob has no pubsub project label, should not report",
			pj: &kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-project",
					Labels: map[string]string{
						pubsubTopicLabel: testPubSubTopicName,
						pubsubRunIDLabel: testPubSubRunID,
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.SuccessState,
				},
			},
			expectedResult: false,
		},
		{
			name: "Prowjob has no pubsub topic label, should not report",
			pj: &kube.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-topic",
					Labels: map[string]string{
						pubsubProjectLabel: testPubSubProjectName,
						pubsubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: kube.ProwJobStatus{
					State: kube.SuccessState,
				},
			},
			expectedResult: false,
		},
	}

	c := NewReporter()

	for _, tc := range testcases {
		r := c.ShouldReport(tc.pj)

		if r != tc.expectedResult {
			t.Errorf("Unexpected result from test: %s.\nExpected: %v\nGot: %v",
				tc.name, tc.expectedResult, r)
		}
	}
}

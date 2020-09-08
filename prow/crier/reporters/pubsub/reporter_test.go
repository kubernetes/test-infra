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

package pubsub

import (
	"reflect"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

const (
	testPubSubProjectName = "test-project"
	testPubSubTopicName   = "test-topic"
	testPubSubRunID       = "test-id"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

func (f *fca) Config() *config.Config {
	f.Lock()
	defer f.Unlock()
	return f.c
}

func TestGenerateMessageFromPJ(t *testing.T) {
	var testcases = []struct {
		name            string
		pj              *prowapi.ProwJob
		jobURLPrefix    string
		expectedMessage *ReportMessage
		expectedError   error
	}{
		// tests with gubernator job URLs
		{
			name: "Prowjob with all information for presubmit jobs should work with no error",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
					URL:   "guber/test1",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PresubmitJob,
					Job:  "test1",
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{{Number: 123}},
					},
				},
			},
			jobURLPrefix: "guber/",
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   testPubSubRunID,
				Status:  prowapi.SuccessState,
				URL:     "guber/test1",
				GCSPath: "gs://test1",
				Refs: []prowapi.Refs{
					{
						Pulls: []prowapi.Pull{{Number: 123}},
					},
				},
				JobType: prowapi.PresubmitJob,
				JobName: "test1",
			},
		},
		{
			name: "Prowjob with all information for periodic jobs should work with no error",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
					URL:   "guber/test1",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PeriodicJob,
					Job:  "test1",
				},
			},
			jobURLPrefix: "guber/",
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   testPubSubRunID,
				Status:  prowapi.SuccessState,
				URL:     "guber/test1",
				GCSPath: "gs://test1",
				JobType: prowapi.PeriodicJob,
				JobName: "test1",
			},
		},
		{
			name: "Prowjob has no pubsub runID label, should return a message with runid empty",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-runID",
					Labels: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   "",
				Status:  prowapi.SuccessState,
			},
		},
		{
			name: "Prowjob with all information annotations should work with no error",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
					URL:   "guber/test1",
				},
			},
			jobURLPrefix: "guber/",
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   testPubSubRunID,
				Status:  prowapi.SuccessState,
				URL:     "guber/test1",
				GCSPath: "gs://test1",
			},
		},
		{
			name: "Prowjob has no pubsub runID annotation, should return a message with runid empty",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-runID",
					Annotations: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   "",
				Status:  prowapi.SuccessState,
			},
		},

		// tests with regular job URLs
		{
			name: "Prowjob with all information for presubmit jobs should work with no error",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
					URL:   "https://prow.k8s.io/view/gcs/test1",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PresubmitJob,
					Job:  "test1",
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{{Number: 123}},
					},
				},
			},
			jobURLPrefix: "https://prow.k8s.io/view/gcs/",
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   testPubSubRunID,
				Status:  prowapi.SuccessState,
				URL:     "https://prow.k8s.io/view/gcs/test1",
				GCSPath: "gs://test1",
				Refs: []prowapi.Refs{
					{
						Pulls: []prowapi.Pull{{Number: 123}},
					},
				},
				JobType: prowapi.PresubmitJob,
				JobName: "test1",
			},
		},
		{
			name: "Prowjob with all information for periodic jobs should work with no error",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
					URL:   "https://prow.k8s.io/view/gcs/test1",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PeriodicJob,
					Job:  "test1",
				},
			},
			jobURLPrefix: "https://prow.k8s.io/view/gcs/",
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   testPubSubRunID,
				Status:  prowapi.SuccessState,
				URL:     "https://prow.k8s.io/view/gcs/test1",
				GCSPath: "gs://test1",
				JobType: prowapi.PeriodicJob,
				JobName: "test1",
			},
		},
		{
			name: "Prowjob with all information annotations should work with no error",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
					URL:   "https://prow.k8s.io/view/gcs/test1",
				},
			},
			jobURLPrefix: "https://prow.k8s.io/view/gcs/",
			expectedMessage: &ReportMessage{
				Project: testPubSubProjectName,
				Topic:   testPubSubTopicName,
				RunID:   testPubSubRunID,
				Status:  prowapi.SuccessState,
				URL:     "https://prow.k8s.io/view/gcs/test1",
				GCSPath: "gs://test1",
			},
		},
	}

	for _, tc := range testcases {
		fca := &fca{
			c: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						JobURLPrefixConfig: map[string]string{"*": tc.jobURLPrefix},
					},
				},
			},
		}

		c := &Client{
			config: fca.Config,
		}

		m := c.generateMessageFromPJ(tc.pj)

		if !reflect.DeepEqual(m, tc.expectedMessage) {
			t.Errorf("Unexpected result from test: %s.\nExpected: %v\nGot: %v",
				tc.name, tc.expectedMessage, m)
		}
	}
}

func TestShouldReport(t *testing.T) {
	var testcases = []struct {
		name           string
		pj             *prowapi.ProwJob
		expectedResult bool
	}{
		{
			name: "Prowjob with all pubsub information labels should return",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedResult: true,
		},
		{
			name: "Prowjob has no pubsub project label, should not report",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-project",
					Labels: map[string]string{
						PubSubTopicLabel: testPubSubTopicName,
						PubSubRunIDLabel: testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedResult: false,
		},
		{
			name: "Prowjob has no pubsub topic label, should not report",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-topic",
					Labels: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedResult: false,
		},
		{
			name: "Prowjob with all pubsub information annotations should return",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Annotations: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubTopicLabel:   testPubSubTopicName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedResult: true,
		},
		{
			name: "Prowjob has no pubsub project annotation, should not report",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-project",
					Annotations: map[string]string{
						PubSubTopicLabel: testPubSubTopicName,
						PubSubRunIDLabel: testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedResult: false,
		},
		{
			name: "Prowjob has no pubsub topic annotation, should not report",
			pj: &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-no-topic",
					Annotations: map[string]string{
						PubSubProjectLabel: testPubSubProjectName,
						PubSubRunIDLabel:   testPubSubRunID,
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.SuccessState,
				},
			},
			expectedResult: false,
		},
	}

	var fakeConfigAgent fca
	c := NewReporter(fakeConfigAgent.Config)

	for _, tc := range testcases {
		r := c.ShouldReport(logrus.NewEntry(logrus.StandardLogger()), tc.pj)

		if r != tc.expectedResult {
			t.Errorf("Unexpected result from test: %s.\nExpected: %v\nGot: %v",
				tc.name, tc.expectedResult, r)
		}
	}
}

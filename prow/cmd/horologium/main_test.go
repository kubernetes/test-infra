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

package main

import (
	"testing"
	"time"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

type fakeKube struct {
	jobs    []kube.Job
	created bool
}

func (fk *fakeKube) ListJobs(ls map[string]string) ([]kube.Job, error) {
	return fk.jobs, nil
}

func (fk *fakeKube) CreateJob(j kube.Job) (kube.Job, error) {
	fk.created = true
	return j, nil
}

// Assumes there is one periodic job called "p" with an interval of one minute.
func TestSync(t *testing.T) {
	testcases := []struct {
		testName string

		jobName         string
		jobComplete     bool
		jobStartTimeAgo time.Duration

		shouldStart bool
	}{
		{
			testName:    "no job",
			shouldStart: true,
		},
		{
			testName:        "job with other name",
			jobName:         "not-j",
			jobComplete:     true,
			jobStartTimeAgo: time.Hour,
			shouldStart:     true,
		},
		{
			testName:        "old, complete job",
			jobName:         "j",
			jobComplete:     true,
			jobStartTimeAgo: time.Hour,
			shouldStart:     true,
		},
		{
			testName:        "old, incomplete job",
			jobName:         "j",
			jobComplete:     false,
			jobStartTimeAgo: time.Hour,
			shouldStart:     false,
		},
		{
			testName:        "new, complete job",
			jobName:         "j",
			jobComplete:     true,
			jobStartTimeAgo: time.Second,
			shouldStart:     false,
		},
		{
			testName:        "new, incomplete job",
			jobName:         "j",
			jobComplete:     false,
			jobStartTimeAgo: time.Second,
			shouldStart:     false,
		},
	}
	for _, tc := range testcases {
		cfg := config.Config{
			Periodics: []config.Periodic{{Name: "j"}},
		}
		cfg.Periodics[0].SetInterval(time.Minute)

		var jobs []kube.Job
		now := time.Now()
		if tc.jobName != "" {
			jobs = []kube.Job{{
				Metadata: kube.ObjectMeta{
					Labels: map[string]string{"jenkins-job-name": tc.jobName},
				},
				Status: kube.JobStatus{
					StartTime: now.Add(-tc.jobStartTimeAgo),
				},
			}}
			if tc.jobComplete {
				jobs[0].Status.Succeeded = 1
			}
		}
		kc := &fakeKube{jobs: jobs}
		if err := sync(kc, &cfg, now); err != nil {
			t.Fatalf("For case %s, didn't expect error: %v", tc.testName, err)
		}
		if tc.shouldStart != kc.created {
			t.Errorf("For case %s, did the wrong thing.", tc.testName)
		}
	}
}

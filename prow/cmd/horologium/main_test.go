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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

type fakeKube struct {
	jobs    []kube.ProwJob
	created bool
}

func (fk *fakeKube) ListProwJobs(s string) ([]kube.ProwJob, error) {
	return fk.jobs, nil
}

func (fk *fakeKube) CreateProwJob(j kube.ProwJob) (kube.ProwJob, error) {
	fk.created = true
	return j, nil
}

type fakeCron struct {
	jobs []string
}

func (fc *fakeCron) SyncConfig(cfg *config.Config) error {
	for _, p := range cfg.Periodics {
		if p.Cron != "" {
			fc.jobs = append(fc.jobs, p.Name)
		}
	}

	return nil
}

func (fc *fakeCron) QueuedJobs() []string {
	res := []string{}
	for _, job := range fc.jobs {
		res = append(res, job)
	}
	fc.jobs = []string{}
	return res
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
			JobConfig: config.JobConfig{
				Periodics: []config.Periodic{{JobBase: config.JobBase{Name: "j"}}},
			},
		}
		cfg.Periodics[0].SetInterval(time.Minute)

		var jobs []kube.ProwJob
		now := time.Now()
		if tc.jobName != "" {
			jobs = []kube.ProwJob{{
				Spec: kube.ProwJobSpec{
					Type: kube.PeriodicJob,
					Job:  tc.jobName,
				},
				Status: kube.ProwJobStatus{
					StartTime: metav1.NewTime(now.Add(-tc.jobStartTimeAgo)),
				},
			}}
			complete := metav1.NewTime(now.Add(-time.Millisecond))
			if tc.jobComplete {
				jobs[0].Status.CompletionTime = &complete
			}
		}
		kc := &fakeKube{jobs: jobs}
		fc := &fakeCron{}
		if err := sync(kc, &cfg, fc, now); err != nil {
			t.Fatalf("For case %s, didn't expect error: %v", tc.testName, err)
		}
		if tc.shouldStart != kc.created {
			t.Errorf("For case %s, did the wrong thing.", tc.testName)
		}
	}
}

// Test sync periodic job scheduled by cron.
func TestSyncCron(t *testing.T) {
	testcases := []struct {
		testName    string
		jobName     string
		jobComplete bool
		shouldStart bool
	}{
		{
			testName:    "no job",
			shouldStart: true,
		},
		{
			testName:    "job with other name",
			jobName:     "not-j",
			jobComplete: true,
			shouldStart: true,
		},
		{
			testName:    "job still running",
			jobName:     "j",
			jobComplete: false,
			shouldStart: false,
		},
		{
			testName:    "job finished",
			jobName:     "j",
			jobComplete: true,
			shouldStart: true,
		},
	}
	for _, tc := range testcases {
		cfg := config.Config{
			JobConfig: config.JobConfig{
				Periodics: []config.Periodic{{JobBase: config.JobBase{Name: "j"}, Cron: "@every 1m"}},
			},
		}

		var jobs []kube.ProwJob
		now := time.Now()
		if tc.jobName != "" {
			jobs = []kube.ProwJob{{
				Spec: kube.ProwJobSpec{
					Type: kube.PeriodicJob,
					Job:  tc.jobName,
				},
				Status: kube.ProwJobStatus{
					StartTime: metav1.NewTime(now.Add(-time.Hour)),
				},
			}}
			complete := metav1.NewTime(now.Add(-time.Millisecond))
			if tc.jobComplete {
				jobs[0].Status.CompletionTime = &complete
			}
		}
		kc := &fakeKube{jobs: jobs}
		fc := &fakeCron{}
		if err := sync(kc, &cfg, fc, now); err != nil {
			t.Fatalf("For case %s, didn't expect error: %v", tc.testName, err)
		}
		if tc.shouldStart != kc.created {
			t.Errorf("For case %s, did the wrong thing.", tc.testName)
		}
	}
}

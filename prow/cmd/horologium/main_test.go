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
	"context"
	"flag"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
)

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
			ProwConfig: config.ProwConfig{
				ProwJobNamespace: "prowjobs",
			},
			JobConfig: config.JobConfig{
				Periodics: []config.Periodic{{JobBase: config.JobBase{Name: "j"}}},
			},
		}
		cfg.Periodics[0].SetInterval(time.Minute)

		var jobs []runtime.Object
		now := time.Now()
		if tc.jobName != "" {
			job := &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-interval",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PeriodicJob,
					Job:  tc.jobName,
				},
				Status: prowapi.ProwJobStatus{
					StartTime: metav1.NewTime(now.Add(-tc.jobStartTimeAgo)),
				},
			}
			complete := metav1.NewTime(now.Add(-time.Millisecond))
			if tc.jobComplete {
				job.Status.CompletionTime = &complete
			}
			jobs = append(jobs, job)
		}
		fakeProwJobClient := &createTrackingClient{Client: fakectrlruntimeclient.NewFakeClient(jobs...)}
		fc := &fakeCron{}
		if err := sync(fakeProwJobClient, &cfg, fc, now); err != nil {
			t.Fatalf("For case %s, didn't expect error: %v", tc.testName, err)
		}

		sawCreation := fakeProwJobClient.sawCreate
		if tc.shouldStart != sawCreation {
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
			ProwConfig: config.ProwConfig{
				ProwJobNamespace: "prowjobs",
			},
			JobConfig: config.JobConfig{
				Periodics: []config.Periodic{{JobBase: config.JobBase{Name: "j"}, Cron: "@every 1m"}},
			},
		}

		var jobs []runtime.Object
		now := time.Now()
		if tc.jobName != "" {
			job := &prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-cron",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PeriodicJob,
					Job:  tc.jobName,
				},
				Status: prowapi.ProwJobStatus{
					StartTime: metav1.NewTime(now.Add(-time.Hour)),
				},
			}
			complete := metav1.NewTime(now.Add(-time.Millisecond))
			if tc.jobComplete {
				job.Status.CompletionTime = &complete
			}
			jobs = append(jobs, job)
		}
		fakeProwJobClient := &createTrackingClient{Client: fakectrlruntimeclient.NewFakeClient(jobs...)}
		fc := &fakeCron{}
		if err := sync(fakeProwJobClient, &cfg, fc, now); err != nil {
			t.Fatalf("For case %s, didn't expect error: %v", tc.testName, err)
		}

		sawCreation := fakeProwJobClient.sawCreate
		if tc.shouldStart != sawCreation {
			t.Errorf("For case %s, did the wrong thing.", tc.testName)
		}
	}
}

func TestFlags(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]string
		del      sets.String
		expected func(*options)
		err      bool
	}{
		{
			name: "minimal flags work",
		},
		{
			name: "explicitly set --config-path",
			args: map[string]string{
				"--config-path": "/random/value",
			},
			expected: func(o *options) {
				o.config.ConfigPath = "/random/value"
			},
		},
		{
			name: "expicitly set --dry-run=false",
			args: map[string]string{
				"--dry-run": "false",
			},
			expected: func(o *options) {
				o.dryRun = false
			},
		},
		{
			name: "--dry-run=true requires --deck-url",
			args: map[string]string{
				"--dry-run":  "true",
				"--deck-url": "",
			},
			err: true,
		},
		{
			name: "explicitly set --dry-run=true",
			args: map[string]string{
				"--dry-run":  "true",
				"--deck-url": "http://whatever",
			},
			expected: func(o *options) {
				o.dryRun = true
			},
		},
		{
			name: "dry run defaults to true",
			expected: func(o *options) {
				o.dryRun = true
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := &options{
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:              "config-path",
					JobConfigPathFlagName:           "job-config-path",
					ConfigPath:                      "yo",
					SupplementalProwConfigsFileName: "_prowconfig.yaml",
				},
				dryRun:                 true,
				instrumentationOptions: flagutil.DefaultInstrumentationOptions(),
			}
			expected.kubernetes.DeckURI = "http://whatever"
			if tc.expected != nil {
				tc.expected(expected)
			}

			argMap := map[string]string{
				"--config-path": "yo",
				"--deck-url":    "http://whatever",
			}
			for k, v := range tc.args {
				argMap[k] = v
			}
			for k := range tc.del {
				delete(argMap, k)
			}

			var args []string
			for k, v := range argMap {
				args = append(args, k+"="+v)
			}
			fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			actual := gatherOptions(fs, args...)
			switch err := actual.Validate(); {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(*expected, actual):
				t.Errorf("%#v != expected %#v", actual, *expected)
			}
		})
	}
}

type createTrackingClient struct {
	ctrlruntimeclient.Client
	sawCreate bool
}

func (ct *createTrackingClient) Create(ctx context.Context, obj ctrlruntimeclient.Object, opts ...ctrlruntimeclient.CreateOption) error {
	ct.sawCreate = true
	return ct.Client.Create(ctx, obj, opts...)
}

/*
Copyright 2016 The Kubernetes Authors.

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
	"os"
	"regexp"
	"strings"
	"testing"
)

const testThis = "@k8s-bot test this"

// Make sure that our rerun commands match our triggers.
func TestJobTriggers(t *testing.T) {
	ja := &JobAgent{}
	if err := ja.load("../../jobs.yaml"); err != nil {
		t.Fatalf("Could not load job configs: %v", err)
	}
	if len(ja.jobs) == 0 {
		t.Fatalf("No jobs found in jobs.yaml.")
	}
	for _, jobs := range ja.jobs {
		for i, job := range jobs {
			if job.Name == "" {
				t.Errorf("Job %v needs a name.", job)
				continue
			}
			if job.Context == "" {
				t.Errorf("Job %s needs a context.", job.Name)
			}
			if job.RerunCommand == "" || job.Trigger == "" {
				t.Errorf("Job %s needs a trigger and a rerun command.", job.Name)
				continue
			}
			// Check that the merge bot will run AlwaysRun jobs, otherwise it
			// will attempt to rerun forever.
			if job.AlwaysRun && !job.re.MatchString(testThis) {
				t.Errorf("AlwaysRun job %s: \"%s\" does not match regex \"%v\".", job.Name, testThis, job.Trigger)
			}
			// Check that the rerun command actually runs the job.
			if !job.re.MatchString(job.RerunCommand) {
				t.Errorf("For job %s: RerunCommand \"%s\" does not match regex \"%v\".", job.Name, job.RerunCommand, job.Trigger)
			}
			// Next check that the rerun command doesn't run any other jobs.
			for j, job2 := range jobs {
				if i == j {
					continue
				}
				if job2.re.MatchString(job.RerunCommand) {
					t.Errorf("RerunCommand \"%s\" from job %s matches \"%v\" from job %s but shouldn't.", job.RerunCommand, job.Name, job2.Trigger, job2.Name)
				}
			}
			// Ensure that bootstrap jobs have a shell script of the same name.
			if strings.HasPrefix(job.Name, "pull-") {
				if _, err := os.Stat(fmt.Sprintf("../../../jobs/%s.sh", job.Name)); err != nil {
					t.Errorf("Cannot find test-infra/jobs/%s.sh", job.Name)
				}
			}

		}
	}
}

func TestCommentBodyMatches(t *testing.T) {
	var testcases = []struct {
		repo         string
		body         string
		expectedJobs []string
	}{
		{
			"org/repo",
			"this is a random comment",
			[]string{},
		},
		{
			"org/repo",
			"ok to test",
			[]string{"gce", "unit"},
		},
		{
			"org/repo",
			"@k8s-bot test this",
			[]string{"gce", "unit", "gke"},
		},
		{
			"org/repo",
			"@k8s-bot unit test this",
			[]string{"unit"},
		},
		{
			"org/repo",
			"@k8s-bot federation test this",
			[]string{"federation"},
		},
		{
			"org/repo2",
			"@k8s-bot test this",
			[]string{"cadveapster"},
		},
		{
			"org/repo3",
			"@k8s-bot test this",
			[]string{},
		},
	}
	ja := &JobAgent{
		jobs: map[string][]JenkinsJob{
			"org/repo": {
				{
					Name:      "gce",
					re:        regexp.MustCompile(`@k8s-bot (gce )?test this`),
					AlwaysRun: true,
				},
				{
					Name:      "unit",
					re:        regexp.MustCompile(`@k8s-bot (unit )?test this`),
					AlwaysRun: true,
				},
				{
					Name:      "gke",
					re:        regexp.MustCompile(`@k8s-bot (gke )?test this`),
					AlwaysRun: false,
				},
				{
					Name:      "federation",
					re:        regexp.MustCompile(`@k8s-bot federation test this`),
					AlwaysRun: false,
				},
			},
			"org/repo2": {
				{
					Name:      "cadveapster",
					re:        regexp.MustCompile(`@k8s-bot test this`),
					AlwaysRun: true,
				},
			},
		},
	}
	for _, tc := range testcases {
		actualJobs := ja.MatchingJobs(tc.repo, tc.body)
		match := true
		if len(actualJobs) != len(tc.expectedJobs) {
			match = false
		} else {
			for _, actualJob := range actualJobs {
				found := false
				for _, expectedJob := range tc.expectedJobs {
					if expectedJob == actualJob.Name {
						found = true
						break
					}
				}
				if !found {
					match = false
					break
				}
			}
		}
		if !match {
			t.Errorf("Wrong jobs for body %s. Got %v, expected %v.", tc.body, actualJobs, tc.expectedJobs)
		}
	}
}

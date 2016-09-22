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

import "testing"

// Make sure that our rerun commands match our triggers.
func TestJobTriggers(t *testing.T) {
	for _, jobs := range defaultJenkinsJobs {
		for i, job := range jobs {
			if job.SkipFailureComment {
				continue
			}
			if len(job.RerunCommand) == 0 {
				t.Fatalf("Job %s needs a rerun command because it comments on failure.", job.Name)
			}
			// Check that the rerun command actually runs the job.
			if !job.Trigger.MatchString(job.RerunCommand) {
				t.Errorf("For job %s: RerunCommand \"%s\" does not match regex \"%v\".", job.Name, job.RerunCommand, job.Trigger)
			}
			// Next check that the rerun command doesn't run any other jobs.
			for j, job2 := range jobs {
				if i == j {
					continue
				}
				if job2.Trigger.MatchString(job.RerunCommand) {
					t.Errorf("RerunCommand \"%s\" from job %s matches \"%v\" from job %s but shouldn't.", job.RerunCommand, job.Name, job2.Trigger, job2.Name)
				}
			}
		}
	}
}

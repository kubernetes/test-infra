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

package sources

import (
	"testing"
	"time"

	"k8s.io/test-infra/robots/issue-creator/creator"

	githubapi "github.com/google/go-github/github"
)

var (
	sampleFlakyJobJSON = []byte(`
	{
	  "ci-kubernetes-e2e-non-cri-gce-etcd3": {
	    "consistency": 0.863,
	    "flakes": 43,
	    "flakiest": {
	      "[k8s.io] Volumes [Volume] [k8s.io] PD should be mountable": 24,
	      "[k8s.io] Services should preserve source pod IP for traffic thru service cluster IP": 7
	    }
	  },
	  "pr:pull-kubernetes-e2e-gce-non-cri": {
	    "consistency": 0.929,
	    "flakes": 62,
	    "flakiest": {
	      "[k8s.io] Volumes [Volume] [k8s.io] PD should be mountable": 28
	    }
	  },
	  "ci-kubernetes-e2e-non-cri-gce": {
	    "consistency": 0.864,
	    "flakes": 42,
	    "flakiest": {
	      "[k8s.io] Volumes [Volume] [k8s.io] PD should be mountable": 24,
	      "[k8s.io] Services should preserve source pod IP for traffic thru service cluster IP": 10
	    }
	  },
	  "ci-kubernetes-e2e-gce-container-vm": {
	    "consistency": 0.863,
	    "flakes": 42,
	    "flakiest": {
	      "[k8s.io] Volumes [Volume] [k8s.io] PD should be mountable": 10,
	      "[k8s.io] Services should preserve source pod IP for traffic thru service cluster IP": 8
	    }
	  },
	  "ci-kubernetes-e2e-non-cri-gce-proto": {
	    "consistency": 0.865,
	    "flakes": 41,
	    "flakiest": {
	      "[k8s.io] Volumes [Volume] [k8s.io] PD should be mountable": 22
	    }
	  },
	  "ci-kubernetes-e2e-gce-gci-ci-master": {
	    "consistency": 0.868,
	    "flakes": 41,
	    "flakiest": {
	      "[k8s.io] Volumes [Volume] [k8s.io] PD should be mountable": 16,
	      "[k8s.io] Services should preserve source pod IP for traffic thru service cluster IP": 8
	    }
	  }
	}`)
)

func TestFJParseFlakyJobs(t *testing.T) {
	reporter := &FlakyJobReporter{creator: &creator.IssueCreator{}}
	jobs, err := reporter.parseFlakyJobs(sampleFlakyJobJSON)
	if err != nil {
		t.Fatalf("Error parsing flaky jobs: %v\n", err)
	}

	if len(jobs) != 6 {
		t.Fatalf("ParseFlakyJobs parsed the wrong number of jobs.  Expected 6, got %d.\n", len(jobs))
	}

	if !checkFlakyJobsSorted(jobs) {
		t.Fatal("The slice of *FlakyJob that was returned by parseFlakyJobs was not sorted.\n")
	}
	if jobs[0].Name != "pr:pull-kubernetes-e2e-gce-non-cri" {
		t.Fatalf("The name of the top flaking job should be 'pr:pull-kubernetes-e2e-gce-non-cri' but is '%s'\n", jobs[0].Name)
	}
	if *jobs[0].Consistency != 0.929 {
		t.Fatalf("The consistency of the top flaking job should be 0.926 but is '%v'\n", *jobs[0].Consistency)
	}
	if *jobs[0].FlakeCount != 62 {
		t.Fatalf("The flake count of the top flaking job should be 63 but is '%d'\n", *jobs[0].FlakeCount)
	}
	if len(jobs[0].FlakyTests) != 1 || jobs[0].FlakyTests["[k8s.io] Volumes [Volume] [k8s.io] PD should be mountable"] != 28 {
		t.Fatal("The dictionary of flaky tests for the top flaking job is invalid.\n")
	}
	for _, job := range jobs {
		if job.reporter == nil {
			t.Errorf("FlakyJob with name: '%s' does not have reporter set.\n", job.Name)
		}
	}
}

// TestFJPrevCloseInWindow checks that FlakyJob issues will abort issue creation by returning an
// empty body if there is a closed issue for the same flaky job that was closed in the past week.
func TestFJPrevCloseInWindow(t *testing.T) {
	reporter := &FlakyJobReporter{creator: &creator.IssueCreator{}}
	fjs, err := reporter.parseFlakyJobs(sampleFlakyJobJSON)
	if err != nil {
		t.Fatalf("Error parsing flaky jobs: %v\n", err)
	}

	lastWeek := time.Now().AddDate(0, 0, -8)
	yesterday := time.Now().AddDate(0, 0, -1)
	num := 1
	// Only need to populate the ClosedAt and Number fields of the Issue.
	prevIssues := []*githubapi.Issue{{ClosedAt: &yesterday, Number: &num}}
	if fjs[0].Body(prevIssues) != "" {
		t.Errorf("FlakyJob returned an issue body when there was a recently closed issue for the job.")
	}

	prevIssues = []*githubapi.Issue{{ClosedAt: &lastWeek, Number: &num}}
	if fjs[0].Body(prevIssues) == "" {
		t.Errorf("FlakyJob returned an empty issue body when it should have returned a valid body.")
	}
}

func checkFlakyJobsSorted(jobs []*FlakyJob) bool {
	for i := 1; i < len(jobs); i++ {
		if *jobs[i-1].FlakeCount < *jobs[i].FlakeCount {
			return false
		}
	}
	return true
}

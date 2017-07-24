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

package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"testing"
)

var podRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const (
	newTestThis   = "/test all"
	newRetestBody = "/test all [submit-queue is verifying that this PR is safe to merge]"
)

type JSONJob struct {
	Scenario string   `json:"scenario"`
	Args     []string `json:"args"`
}

// Consistent but meaningless order.
func flattenJobs(jobs []Presubmit) []Presubmit {
	ret := jobs
	for _, job := range jobs {
		if len(job.RunAfterSuccess) > 0 {
			ret = append(ret, flattenJobs(job.RunAfterSuccess)...)
		}
	}
	return ret
}

func TestPresubmits(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
	if len(c.Presubmits) == 0 {
		t.Fatalf("No jobs found in presubmit.yaml.")
	}
	b, err := ioutil.ReadFile("../../jobs/config.json")
	if err != nil {
		t.Fatalf("Could not load jobs/config.json: %v", err)
	}
	var bootstrapConfig map[string]JSONJob
	json.Unmarshal(b, &bootstrapConfig)
	for _, rootJobs := range c.Presubmits {
		jobs := flattenJobs(rootJobs)
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
			if job.AlwaysRun && !job.re.MatchString(newTestThis) {
				t.Errorf("AlwaysRun job %s: \"%s\" does not match regex \"%v\".", job.Name, newTestThis, job.Trigger)
			}
			if job.AlwaysRun && !job.re.MatchString(newRetestBody) {
				t.Errorf("AlwaysRun job %s: \"%s\" does not match regex \"%v\".", job.Name, newRetestBody, job.Trigger)
			}
			// Check that the merge bot will not run Non-AlwaysRun jobs
			if !job.AlwaysRun && job.re.MatchString(newTestThis) {
				t.Errorf("Non-AlwaysRun job %s: \"%s\" matches regex \"%v\".", job.Name, newTestThis, job.Trigger)
			}
			if !job.AlwaysRun && job.re.MatchString(newRetestBody) {
				t.Errorf("Non-AlwaysRun job %s: \"%s\" matches regex \"%v\".", job.Name, newRetestBody, job.Trigger)
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
				if job.Name == job2.Name && i > j {
					t.Errorf("Two jobs have the same name: %s", job.Name)
				}
				if job.Context == job2.Context && i > j {
					t.Errorf("Jobs %s and %s have the same context: %s", job.Name, job2.Name, job.Context)
				}
				if job2.re.MatchString(job.RerunCommand) {
					t.Errorf("RerunCommand \"%s\" from job %s matches \"%v\" from job %s but shouldn't.", job.RerunCommand, job.Name, job2.Trigger, job2.Name)
				}
			}
			var scenario string
			job.Name = strings.Replace(job.Name, "pull-security-kubernetes", "pull-kubernetes", 1)
			if j, present := bootstrapConfig[job.Name]; present {
				scenario = fmt.Sprintf("scenarios/%s.py", j.Scenario)
			} else {
				scenario = fmt.Sprintf("jobs/%s.sh", job.Name)
			}

			// Ensure that jobs have a shell script of the same name.
			if s, err := os.Stat(fmt.Sprintf("../../%s", scenario)); err != nil {
				t.Errorf("Cannot find test-infra/%s for %s", scenario, job.Name)
			} else {
				if s.Mode()&0111 == 0 {
					t.Errorf("Not executable: test-infra/%s (%o)", scenario, s.Mode()&0777)
				}
				if s.Mode()&0444 == 0 {
					t.Errorf("Not readable: test-infra/%s (%o)", scenario, s.Mode()&0777)
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
	c := &Config{
		Presubmits: map[string][]Presubmit{
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
		actualJobs := c.MatchingPresubmits(tc.repo, tc.body, regexp.MustCompile(`ok to test`))
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

func TestRetestPresubmits(t *testing.T) {
	var testcases = []struct {
		skipContexts     map[string]bool
		runContexts      map[string]bool
		expectedContexts []string
	}{
		{
			map[string]bool{},
			map[string]bool{},
			[]string{"gce", "unit"},
		},
		{
			map[string]bool{"gce": true},
			map[string]bool{},
			[]string{"unit"},
		},
		{
			map[string]bool{},
			map[string]bool{"federation": true, "nonexistent": true},
			[]string{"gce", "unit", "federation"},
		},
		{
			map[string]bool{},
			map[string]bool{"gke": true},
			[]string{"gce", "unit", "gke"},
		},
		{
			map[string]bool{"gce": true},
			map[string]bool{"gce": true}, // should never happen
			[]string{"unit"},
		},
	}
	c := &Config{
		Presubmits: map[string][]Presubmit{
			"org/repo": {
				{
					Context:   "gce",
					AlwaysRun: true,
				},
				{
					Context:   "unit",
					AlwaysRun: true,
				},
				{
					Context:   "gke",
					AlwaysRun: false,
				},
				{
					Context:   "federation",
					AlwaysRun: false,
				},
			},
			"org/repo2": {
				{
					Context:   "shouldneverrun",
					AlwaysRun: true,
				},
			},
		},
	}
	for _, tc := range testcases {
		actualContexts := c.RetestPresubmits("org/repo", tc.skipContexts, tc.runContexts)
		match := true
		if len(actualContexts) != len(tc.expectedContexts) {
			match = false
		} else {
			for _, actualJob := range actualContexts {
				found := false
				for _, expectedContext := range tc.expectedContexts {
					if expectedContext == actualJob.Context {
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
			t.Errorf("Wrong contexts for skip %v run %v. Got %v, expected %v.", tc.runContexts, tc.skipContexts, actualContexts, tc.expectedContexts)
		}
	}

}

func TestConditionalPresubmits(t *testing.T) {
	presubmits := []Presubmit{
		{
			Name:         "cross build",
			RunIfChanged: `(Makefile|\.sh|_(windows|linux|osx|unknown)(_test)?\.go)$`,
		},
	}
	setRegexes(presubmits)
	ps := presubmits[0]
	var testcases = []struct {
		changes  []string
		expected bool
	}{
		{[]string{"some random file"}, false},
		{[]string{"./pkg/util/rlimit/rlimit_linux.go"}, true},
		{[]string{"./pkg/util/rlimit/rlimit_unknown_test.go"}, true},
		{[]string{"build.sh"}, true},
		{[]string{"build.shoo"}, false},
		{[]string{"Makefile"}, true},
	}
	for _, tc := range testcases {
		actual := ps.RunsAgainstChanges(tc.changes)
		if actual != tc.expected {
			t.Errorf("wrong RunsAgainstChanges(%#v) result. Got %v, expected %v", tc.changes, actual, tc.expected)
		}
	}
}

func TestListPresubmit(t *testing.T) {
	c := &Config{
		Presubmits: map[string][]Presubmit{
			"r1": {
				{
					Name: "a",
					RunAfterSuccess: []Presubmit{
						{Name: "aa"},
						{Name: "ab"},
					},
				},
				{Name: "b"},
			},
		},
		Postsubmits: map[string][]Postsubmit{
			"r1": {{Name: "c"}},
		},
		Periodics: []Periodic{
			{Name: "d"},
		},
	}

	expected := []string{"a", "aa", "ab", "b"}
	actual := c.AllPresubmits()
	if len(actual) != len(expected) {
		t.Fatalf("Wrong number of jobs. Got %v, expected %v", actual, expected)
	}
	for _, j1 := range expected {
		found := false
		for _, j2 := range actual {
			if j1 == j2 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find job %s in output", j1)
		}
	}
}

func TestListPostsubmit(t *testing.T) {
	c := &Config{
		Presubmits: map[string][]Presubmit{
			"r1": {{Name: "a"}},
		},
		Postsubmits: map[string][]Postsubmit{
			"r1": {
				{
					Name: "c",
					RunAfterSuccess: []Postsubmit{
						{Name: "ca"},
						{Name: "cb"},
					},
				},
				{Name: "d"},
			},
			"r2": {{Name: "e"}},
		},
		Periodics: []Periodic{
			{Name: "f"},
		},
	}

	expected := []string{"c", "ca", "cb", "d", "e"}
	actual := c.AllPostsubmits()
	if len(actual) != len(expected) {
		t.Fatalf("Wrong number of jobs. Got %v, expected %v", actual, expected)
	}
	for _, j1 := range expected {
		found := false
		for _, j2 := range actual {
			if j1 == j2 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find job %s in output", j1)
		}
	}
}

func TestListPeriodic(t *testing.T) {
	c := &Config{
		Presubmits: map[string][]Presubmit{
			"r1": {{Name: "a"}},
		},
		Postsubmits: map[string][]Postsubmit{
			"r1": {{Name: "b"}},
		},
		Periodics: []Periodic{
			{
				Name: "c",
				RunAfterSuccess: []Periodic{
					{Name: "ca"},
					{Name: "cb"},
				},
			},
			{Name: "d"},
		},
	}

	expected := []string{"c", "ca", "cb", "d"}
	actual := c.AllPeriodics()
	if len(actual) != len(expected) {
		t.Fatalf("Wrong number of jobs. Got %v, expected %v", actual, expected)
	}
	for _, j1 := range expected {
		found := false
		for _, j2 := range actual {
			if j1 == j2 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find job %s in output", j1)
		}
	}
}

func TestRunAgainstBranch(t *testing.T) {
	jobs := []Presubmit{
		{
			Name:     "a",
			Brancher: Brancher{SkipBranches: []string{"s"}},
		},
		{
			Name:     "b",
			Brancher: Brancher{Branches: []string{"r"}},
		},
		{
			Name: "c",
			Brancher: Brancher{
				SkipBranches: []string{"s"},
				Branches:     []string{"r"},
			},
		},
		{
			Name: "d",
			Brancher: Brancher{
				SkipBranches: []string{"s"},
				Branches:     []string{"s", "r"},
			},
		},
		{
			Name: "default",
		},
	}

	for _, job := range jobs {
		if job.Name == "default" {
			if !job.RunsAgainstBranch("s") {
				t.Errorf("Job %s should run branch s", job.Name)
			}
		} else if job.RunsAgainstBranch("s") {
			t.Errorf("Job %s should not run branch s", job.Name)
		}

		if !job.RunsAgainstBranch("r") {
			t.Errorf("Job %s should run branch r", job.Name)
		}
	}
}

func TestValidPodNames(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
	for _, j := range c.AllPresubmits() {
		if !podRe.MatchString(j) {
			t.Errorf("Job \"%s\" must match regex \"%s\".", j, podRe.String())
		}
	}
	for _, j := range c.AllPostsubmits() {
		if !podRe.MatchString(j) {
			t.Errorf("Job \"%s\" must match regex \"%s\".", j, podRe.String())
		}
	}
	for _, j := range c.AllPeriodics() {
		if !podRe.MatchString(j) {
			t.Errorf("Job \"%s\" must match regex \"%s\".", j, podRe.String())
		}
	}
}

func TestNoDuplicateJobs(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}

	allJobs := make(map[string]bool)
	for _, j := range c.AllPresubmits() {
		if allJobs[j] {
			t.Errorf("Found duplicate job in presubmit: %s.", j)
		}
		allJobs[j] = true
	}

	allJobs = make(map[string]bool)
	for _, j := range c.AllPostsubmits() {
		if allJobs[j] {
			t.Errorf("Found duplicate job in postsubmit: %s.", j)
		}
		allJobs[j] = true
	}

	allJobs = make(map[string]bool)
	for _, j := range c.AllPeriodics() {
		if allJobs[j] {
			t.Errorf("Found duplicate job in periodic %s.", j)
		}
		allJobs[j] = true
	}
}

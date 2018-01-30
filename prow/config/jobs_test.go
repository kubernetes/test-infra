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

	"k8s.io/test-infra/prow/kube"
)

var podRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const (
	testThis   = "/test all"
	retestBody = "/test all [submit-queue is verifying that this PR is safe to merge]"
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

// Returns if two brancher has overlapping branches
func checkOverlapBrancher(b1, b2 Brancher) bool {
	if b1.RunsAgainstAllBranch() || b2.RunsAgainstAllBranch() {
		return true
	}

	for _, run1 := range b1.Branches {
		if b2.RunsAgainstBranch(run1) {
			return true
		}
	}

	for _, run2 := range b2.Branches {
		if b1.RunsAgainstBranch(run2) {
			return true
		}
	}

	return false
}

// TODO(spxtr): Some of this is generic prowjob stuff and some of this is k8s-
// specific. Figure out which is which and split this up.
func TestPresubmits(t *testing.T) {
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
			if job.AlwaysRun && !job.re.MatchString(testThis) {
				t.Errorf("AlwaysRun job %s: \"%s\" does not match regex \"%v\".", job.Name, testThis, job.Trigger)
			}
			if job.AlwaysRun && !job.re.MatchString(retestBody) {
				t.Errorf("AlwaysRun job %s: \"%s\" does not match regex \"%v\".", job.Name, retestBody, job.Trigger)
			}
			// Check that the merge bot will not run Non-AlwaysRun jobs
			if !job.AlwaysRun && job.re.MatchString(testThis) {
				t.Errorf("Non-AlwaysRun job %s: \"%s\" matches regex \"%v\".", job.Name, testThis, job.Trigger)
			}
			if !job.AlwaysRun && job.re.MatchString(retestBody) {
				t.Errorf("Non-AlwaysRun job %s: \"%s\" matches regex \"%v\".", job.Name, retestBody, job.Trigger)
			}

			if len(job.Brancher.Branches) > 0 && len(job.Brancher.SkipBranches) > 0 {
				t.Errorf("Job %s : Cannot have both branches and skip_branches set", job.Name)
			}
			// Next check that the rerun command doesn't run any other jobs.
			for j, job2 := range jobs[i+1:] {
				if job.Name == job2.Name {
					// Make sure max_concurrency are the same
					if job.MaxConcurrency != job2.MaxConcurrency {
						t.Errorf("Jobs %s share same name but has different max_concurrency", job.Name)
					}
					// Make sure branches are not overlapping
					if checkOverlapBrancher(job.Brancher, job2.Brancher) {
						t.Errorf("Two jobs have the same name: %s, and has conflicted branches", job.Name)
					}
				} else {
					if job.Context == job2.Context {
						t.Errorf("Jobs %s and %s have the same context: %s", job.Name, job2.Name, job.Context)
					}
					if job2.re.MatchString(job.RerunCommand) {
						t.Errorf("%d, %d, RerunCommand \"%s\" from job %s matches \"%v\" from job %s but shouldn't.", i, j, job.RerunCommand, job.Name, job2.Trigger, job2.Name)
					}
				}
			}
			var scenario string
			job.Name = strings.Replace(job.Name, "pull-security-kubernetes", "pull-kubernetes", 1)
			if j, present := bootstrapConfig[job.Name]; present {
				scenario = fmt.Sprintf("scenarios/%s.py", j.Scenario)
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
			"/ok-to-test",
			[]string{"gce", "unit"},
		},
		{
			"org/repo",
			"/test all",
			[]string{"gce", "unit", "gke"},
		},
		{
			"org/repo",
			"/test unit",
			[]string{"unit"},
		},
		{
			"org/repo",
			"/test federation",
			[]string{"federation"},
		},
		{
			"org/repo2",
			"/test all",
			[]string{"cadveapster", "after-cadveapster", "after-after-cadveapster"},
		},
		{
			"org/repo2",
			"/test really",
			[]string{"after-cadveapster"},
		},
		{
			"org/repo2",
			"/test again really",
			[]string{"after-after-cadveapster"},
		},
		{
			"org/repo3",
			"/test all",
			[]string{},
		},
	}
	c := &Config{
		Presubmits: map[string][]Presubmit{
			"org/repo": {
				{
					Name:      "gce",
					re:        regexp.MustCompile(`/test (gce|all)`),
					AlwaysRun: true,
				},
				{
					Name:      "unit",
					re:        regexp.MustCompile(`/test (unit|all)`),
					AlwaysRun: true,
				},
				{
					Name:      "gke",
					re:        regexp.MustCompile(`/test (gke|all)`),
					AlwaysRun: false,
				},
				{
					Name:      "federation",
					re:        regexp.MustCompile(`/test federation`),
					AlwaysRun: false,
				},
			},
			"org/repo2": {
				{
					Name:      "cadveapster",
					re:        regexp.MustCompile(`/test all`),
					AlwaysRun: true,
					RunAfterSuccess: []Presubmit{
						{
							Name:      "after-cadveapster",
							re:        regexp.MustCompile(`/test (really|all)`),
							AlwaysRun: true,
							RunAfterSuccess: []Presubmit{
								{
									Name:      "after-after-cadveapster",
									re:        regexp.MustCompile(`/test (again really|all)`),
									AlwaysRun: true,
								},
							},
						},
						{
							Name:      "another-after-cadveapster",
							re:        regexp.MustCompile(`@k8s-bot dont test this`),
							AlwaysRun: true,
						},
					},
				},
			},
		},
	}
	for _, tc := range testcases {
		actualJobs := c.MatchingPresubmits(tc.repo, tc.body, regexp.MustCompile(`/ok-to-test`).MatchString(tc.body))
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
	SetRegexes(presubmits)
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
			"r2": {
				{
					Name: "c",
					RunAfterSuccess: []Presubmit{
						{Name: "ca"},
						{Name: "cb"},
					},
				},
				{Name: "d"},
			},
		},
		Postsubmits: map[string][]Postsubmit{
			"r1": {{Name: "e"}},
		},
		Periodics: []Periodic{
			{Name: "f"},
		},
	}

	var testcases = []struct {
		name     string
		expected []string
		repos    []string
	}{
		{
			"all presubmits",
			[]string{"a", "aa", "ab", "b", "c", "ca", "cb", "d"},
			[]string{},
		},
		{
			"r2 presubmits",
			[]string{"c", "ca", "cb", "d"},
			[]string{"r2"},
		},
	}

	for _, tc := range testcases {
		actual := c.AllPresubmits(tc.repos)
		if len(actual) != len(tc.expected) {
			t.Fatalf("test %s - Wrong number of jobs. Got %v, expected %v", tc.name, actual, tc.expected)
		}
		for _, j1 := range tc.expected {
			found := false
			for _, j2 := range actual {
				if j1 == j2.Name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("test %s - Did not find job %s in output", tc.name, j1)
			}
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

	var testcases = []struct {
		name     string
		expected []string
		repos    []string
	}{
		{
			"all postsubmits",
			[]string{"c", "ca", "cb", "d", "e"},
			[]string{},
		},
		{
			"r2 presubmits",
			[]string{"e"},
			[]string{"r2"},
		},
	}

	for _, tc := range testcases {
		actual := c.AllPostsubmits(tc.repos)
		if len(actual) != len(tc.expected) {
			t.Fatalf("%s - Wrong number of jobs. Got %v, expected %v", tc.name, actual, tc.expected)
		}
		for _, j1 := range tc.expected {
			found := false
			for _, j2 := range actual {
				if j1 == j2.Name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Did not find job %s in output", j1)
			}
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
			if j1 == j2.Name {
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
	for _, j := range c.AllPresubmits([]string{}) {
		if !podRe.MatchString(j.Name) {
			t.Errorf("Job \"%s\" must match regex \"%s\".", j.Name, podRe.String())
		}
	}
	for _, j := range c.AllPostsubmits([]string{}) {
		if !podRe.MatchString(j.Name) {
			t.Errorf("Job \"%s\" must match regex \"%s\".", j.Name, podRe.String())
		}
	}
	for _, j := range c.AllPeriodics() {
		if !podRe.MatchString(j.Name) {
			t.Errorf("Job \"%s\" must match regex \"%s\".", j.Name, podRe.String())
		}
	}
}

func TestNoDuplicateJobs(t *testing.T) {
	// Presubmit test is covered under TestPresubmits() above

	allJobs := make(map[string]bool)
	for _, j := range c.AllPostsubmits([]string{}) {
		if allJobs[j.Name] {
			t.Errorf("Found duplicate job in postsubmit: %s.", j.Name)
		}
		allJobs[j.Name] = true
	}

	allJobs = make(map[string]bool)
	for _, j := range c.AllPeriodics() {
		if allJobs[j.Name] {
			t.Errorf("Found duplicate job in periodic %s.", j.Name)
		}
		allJobs[j.Name] = true
	}
}

func TestMergePreset(t *testing.T) {
	tcs := []struct {
		name      string
		jobLabels map[string]string
		pod       *kube.PodSpec
		presets   []Preset

		shouldError  bool
		numEnv       int
		numVol       int
		numVolMounts int
	}{
		{
			name:      "no pod spec",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       nil,
			presets: []Preset{
				{
					Labels: map[string]string{"foo": "bar"},
					Env:    []kube.EnvVar{{Name: "baz"}},
				},
			},
		},
		{
			name:      "one volume",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &kube.PodSpec{},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []kube.Volume{{Name: "baz"}},
				},
			},
			numVol: 1,
		},
		{
			name:      "wrong label",
			jobLabels: map[string]string{"foo": "nope"},
			pod:       &kube.PodSpec{},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []kube.Volume{{Name: "baz"}},
				},
			},
		},
		{
			name:      "conflicting volume name",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &kube.PodSpec{Volumes: []kube.Volume{{Name: "baz"}}},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []kube.Volume{{Name: "baz"}},
				},
			},
			shouldError: true,
		},
		{
			name:      "non conflicting volume name",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &kube.PodSpec{Volumes: []kube.Volume{{Name: "baz"}}},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []kube.Volume{{Name: "qux"}},
				},
			},
			numVol: 2,
		},
		{
			name:      "one env",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &kube.PodSpec{Containers: []kube.Container{{}}},
			presets: []Preset{
				{
					Labels: map[string]string{"foo": "bar"},
					Env:    []kube.EnvVar{{Name: "baz"}},
				},
			},
			numEnv: 1,
		},
		{
			name:      "one vm",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &kube.PodSpec{Containers: []kube.Container{{}}},
			presets: []Preset{
				{
					Labels:       map[string]string{"foo": "bar"},
					VolumeMounts: []kube.VolumeMount{{Name: "baz"}},
				},
			},
			numVolMounts: 1,
		},
		{
			name:      "one of each",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &kube.PodSpec{Containers: []kube.Container{{}}},
			presets: []Preset{
				{
					Labels:       map[string]string{"foo": "bar"},
					Env:          []kube.EnvVar{{Name: "baz"}},
					VolumeMounts: []kube.VolumeMount{{Name: "baz"}},
					Volumes:      []kube.Volume{{Name: "qux"}},
				},
			},
			numEnv:       1,
			numVol:       1,
			numVolMounts: 1,
		},
		{
			name:      "two vm",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &kube.PodSpec{Containers: []kube.Container{{}}},
			presets: []Preset{
				{
					Labels:       map[string]string{"foo": "bar"},
					VolumeMounts: []kube.VolumeMount{{Name: "baz"}, {Name: "foo"}},
				},
			},
			numVolMounts: 2,
		},
	}
	for _, tc := range tcs {
		conf := &Config{
			Presets: tc.presets,
			Periodics: []Periodic{
				{
					Name:     "foo",
					Labels:   tc.jobLabels,
					Agent:    string(kube.JenkinsAgent),
					Interval: "1h",
					Spec:     tc.pod,
				},
			},
		}
		if err := parseConfig(conf); err == nil && tc.shouldError {
			t.Fatalf("For test \"%s\": expected error but got none.", tc.name)
		} else if err != nil && !tc.shouldError {
			t.Fatalf("For test \"%s\": expected no error but got %v.", tc.name, err)
		}
		if tc.shouldError {
			continue
		}
		pod := conf.Periodics[0].Spec
		if pod == nil {
			continue
		}
		if len(pod.Volumes) != tc.numVol {
			t.Errorf("For test \"%s\": wrong number of volumes. Got %d, expected %d.", tc.name, len(pod.Volumes), tc.numVol)
		}
		for _, c := range pod.Containers {
			if len(c.VolumeMounts) != tc.numVolMounts {
				t.Errorf("For test \"%s\": wrong number of volume mounts. Got %d, expected %d.", tc.name, len(c.VolumeMounts), tc.numVolMounts)
			}
			if len(c.Env) != tc.numEnv {
				t.Errorf("For test \"%s\": wrong number of env vars. Got %d, expected %d.", tc.name, len(c.Env), tc.numEnv)
			}
		}
	}
}

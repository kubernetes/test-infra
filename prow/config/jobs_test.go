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
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"testing"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"k8s.io/apimachinery/pkg/util/sets"

	coreapi "k8s.io/api/core/v1"
)

var c *Config
var configPath = flag.String("config", "../config.yaml", "Path to prow config")
var jobConfigPath = flag.String("job-config", "../../config/jobs", "Path to prow job config")
var podRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Returns if two brancher has overlapping branches
func checkOverlapBrancher(b1, b2 Brancher) bool {
	if b1.RunsAgainstAllBranch() || b2.RunsAgainstAllBranch() {
		return true
	}

	for _, run1 := range b1.Branches {
		if b2.ShouldRun(run1) {
			return true
		}
	}

	for _, run2 := range b2.Branches {
		if b1.ShouldRun(run2) {
			return true
		}
	}

	return false
}

func TestMain(m *testing.M) {
	flag.Parse()
	if *configPath == "" {
		fmt.Println("--config must set")
		os.Exit(1)
	}

	conf, err := Load(*configPath, *jobConfigPath)
	if err != nil {
		fmt.Printf("Could not load config: %v", err)
		os.Exit(1)
	}
	c = conf

	os.Exit(m.Run())
}

func TestPresubmits(t *testing.T) {
	if len(c.Presubmits) == 0 {
		t.Fatalf("No jobs found in presubmit.yaml.")
	}

	for _, rootJobs := range c.Presubmits {
		for i, job := range rootJobs {
			if job.Name == "" {
				t.Errorf("Job %v needs a name.", job)
				continue
			}
			if !job.SkipReport && job.Context == "" {
				t.Errorf("Job %s needs a context.", job.Name)
			}
			if job.RerunCommand == "" || job.Trigger == "" {
				t.Errorf("Job %s needs a trigger and a rerun command.", job.Name)
				continue
			}

			if len(job.Brancher.Branches) > 0 && len(job.Brancher.SkipBranches) > 0 {
				t.Errorf("Job %s : Cannot have both branches and skip_branches set", job.Name)
			}
			// Next check that the rerun command doesn't run any other jobs.
			for j, job2 := range rootJobs[i+1:] {
				if job.Name == job2.Name {
					// Make sure max_concurrency are the same
					if job.MaxConcurrency != job2.MaxConcurrency {
						t.Errorf("Jobs %s share same name but has different max_concurrency", job.Name)
					}
					// Make sure branches are not overlapping
					if checkOverlapBrancher(job.Brancher, job2.Brancher) {
						t.Errorf("Two jobs have the same name: %s, and have conflicting branches", job.Name)
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
		}
	}
}

// TODO(krzyzacy): technically this, and TestPresubmits above should belong to config/ instead of prow/
func TestPostsubmits(t *testing.T) {
	if len(c.Postsubmits) == 0 {
		t.Fatalf("No jobs found in presubmit.yaml.")
	}

	for _, rootJobs := range c.Postsubmits {
		for i, job := range rootJobs {
			if job.Name == "" {
				t.Errorf("Job %v needs a name.", job)
				continue
			}
			if job.Report && job.Context == "" {
				t.Errorf("Job %s needs a context.", job.Name)
			}

			if len(job.Brancher.Branches) > 0 && len(job.Brancher.SkipBranches) > 0 {
				t.Errorf("Job %s : Cannot have both branches and skip_branches set", job.Name)
			}
			// Next check that the rerun command doesn't run any other jobs.
			for _, job2 := range rootJobs[i+1:] {
				if job.Name == job2.Name {
					// Make sure max_concurrency are the same
					if job.MaxConcurrency != job2.MaxConcurrency {
						t.Errorf("Jobs %s share same name but has different max_concurrency", job.Name)
					}
					// Make sure branches are not overlapping
					if checkOverlapBrancher(job.Brancher, job2.Brancher) {
						t.Errorf("Two jobs have the same name: %s, and have conflicting branches", job.Name)
					}
				} else {
					if job.Context == job2.Context {
						t.Errorf("Jobs %s and %s have the same context: %s", job.Name, job2.Name, job.Context)
					}
				}
			}
		}
	}
}

func TestRetestPresubmits(t *testing.T) {
	var testcases = []struct {
		skipContexts     sets.String
		runContexts      sets.String
		expectedContexts []string
	}{
		{
			skipContexts:     sets.NewString(),
			runContexts:      sets.NewString(),
			expectedContexts: []string{"gce", "unit"},
		},
		{
			skipContexts:     sets.NewString("gce"),
			runContexts:      sets.NewString(),
			expectedContexts: []string{"unit"},
		},
		{
			skipContexts:     sets.NewString(),
			runContexts:      sets.NewString("federation", "nonexistent"),
			expectedContexts: []string{"gce", "unit", "federation"},
		},
		{
			skipContexts:     sets.NewString(),
			runContexts:      sets.NewString("gke"),
			expectedContexts: []string{"gce", "unit", "gke"},
		},
		{
			skipContexts:     sets.NewString("gce"),
			runContexts:      sets.NewString("gce"), // should never happ)n
			expectedContexts: []string{"unit"},
		},
	}
	c := &Config{
		JobConfig: JobConfig{
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
			JobBase: JobBase{
				Name: "cross build",
			},
			RegexpChangeMatcher: RegexpChangeMatcher{
				RunIfChanged: `(Makefile|\.sh|_(windows|linux|osx|unknown)(_test)?\.go)$`,
			},
		},
	}
	SetPresubmitRegexes(presubmits)
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
		JobConfig: JobConfig{
			Presubmits: map[string][]Presubmit{
				"r1": {
					{
						JobBase: JobBase{
							Name: "a",
						},
					},
					{JobBase: JobBase{Name: "b"}},
				},
				"r2": {
					{
						JobBase: JobBase{
							Name: "c",
						},
					},
					{JobBase: JobBase{Name: "d"}},
				},
			},
			Postsubmits: map[string][]Postsubmit{
				"r1": {{JobBase: JobBase{Name: "e"}}},
			},
			Periodics: []Periodic{
				{JobBase: JobBase{Name: "f"}},
			},
		},
	}

	var testcases = []struct {
		name     string
		expected []string
		repos    []string
	}{
		{
			"all presubmits",
			[]string{"a", "b", "c", "d"},
			[]string{},
		},
		{
			"r2 presubmits",
			[]string{"c", "d"},
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
		JobConfig: JobConfig{
			Presubmits: map[string][]Presubmit{
				"r1": {{JobBase: JobBase{Name: "a"}}},
			},
			Postsubmits: map[string][]Postsubmit{
				"r1": {
					{
						JobBase: JobBase{
							Name: "c",
						},
					},
					{JobBase: JobBase{Name: "d"}},
				},
				"r2": {{JobBase: JobBase{Name: "e"}}},
			},
			Periodics: []Periodic{
				{JobBase: JobBase{Name: "f"}},
			},
		},
	}

	var testcases = []struct {
		name     string
		expected []string
		repos    []string
	}{
		{
			"all postsubmits",
			[]string{"c", "d", "e"},
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
		JobConfig: JobConfig{
			Presubmits: map[string][]Presubmit{
				"r1": {{JobBase: JobBase{Name: "a"}}},
			},
			Postsubmits: map[string][]Postsubmit{
				"r1": {{JobBase: JobBase{Name: "b"}}},
			},
			Periodics: []Periodic{
				{
					JobBase: JobBase{
						Name: "c",
					},
				},
				{JobBase: JobBase{Name: "d"}},
			},
		},
	}

	expected := []string{"c", "d"}
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
			JobBase: JobBase{
				Name: "a",
			},
			Brancher: Brancher{SkipBranches: []string{"s"}},
		},
		{
			JobBase: JobBase{
				Name: "b",
			},
			Brancher: Brancher{Branches: []string{"r"}},
		},
		{
			JobBase: JobBase{
				Name: "c",
			},
			Brancher: Brancher{
				SkipBranches: []string{"s"},
				Branches:     []string{"r"},
			},
		},
		{
			JobBase: JobBase{
				Name: "d",
			},
			Brancher: Brancher{
				SkipBranches: []string{"s"},
				Branches:     []string{"s", "r"},
			},
		},
		{
			JobBase: JobBase{
				Name: "default",
			},
		},
	}

	if err := SetPresubmitRegexes(jobs); err != nil {
		t.Fatalf("could not set regexes: %v", err)
	}

	for _, job := range jobs {
		if job.Name == "default" {
			if !job.Brancher.ShouldRun("s") {
				t.Errorf("Job %s should run branch s", job.Name)
			}
		} else if job.Brancher.ShouldRun("s") {
			t.Errorf("Job %s should not run branch s", job.Name)
		}

		if !job.Brancher.ShouldRun("r") {
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
		pod       *coreapi.PodSpec
		buildSpec *buildv1alpha1.BuildSpec
		presets   []Preset

		shouldError  bool
		numEnv       int
		numVol       int
		numVolMounts int
	}{
		{
			name:      "one volume",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &coreapi.PodSpec{},
			buildSpec: &buildv1alpha1.BuildSpec{},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []coreapi.Volume{{Name: "baz"}},
				},
			},
			numVol: 1,
		},
		{
			name:      "wrong label",
			jobLabels: map[string]string{"foo": "nope"},
			pod:       &coreapi.PodSpec{},
			buildSpec: &buildv1alpha1.BuildSpec{},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []coreapi.Volume{{Name: "baz"}},
				},
			},
		},
		{
			name:      "conflicting volume name for podspec",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &coreapi.PodSpec{Volumes: []coreapi.Volume{{Name: "baz"}}},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []coreapi.Volume{{Name: "baz"}},
				},
			},
			shouldError: true,
		},
		{
			name:      "conflicting volume name for buildspec",
			jobLabels: map[string]string{"foo": "bar"},
			buildSpec: &buildv1alpha1.BuildSpec{Volumes: []coreapi.Volume{{Name: "baz"}}},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []coreapi.Volume{{Name: "baz"}},
				},
			},
			shouldError: true,
		},
		{
			name:      "non conflicting volume name",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &coreapi.PodSpec{Volumes: []coreapi.Volume{{Name: "baz"}}},
			buildSpec: &buildv1alpha1.BuildSpec{Volumes: []coreapi.Volume{{Name: "baz"}}},
			presets: []Preset{
				{
					Labels:  map[string]string{"foo": "bar"},
					Volumes: []coreapi.Volume{{Name: "qux"}},
				},
			},
			numVol: 2,
		},
		{
			name:      "one env",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &coreapi.PodSpec{Containers: []coreapi.Container{{}}},
			buildSpec: &buildv1alpha1.BuildSpec{},
			presets: []Preset{
				{
					Labels: map[string]string{"foo": "bar"},
					Env:    []coreapi.EnvVar{{Name: "baz"}},
				},
			},
			numEnv: 1,
		},
		{
			name:      "one vm",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &coreapi.PodSpec{Containers: []coreapi.Container{{}}},
			buildSpec: &buildv1alpha1.BuildSpec{},
			presets: []Preset{
				{
					Labels:       map[string]string{"foo": "bar"},
					VolumeMounts: []coreapi.VolumeMount{{Name: "baz"}},
				},
			},
			numVolMounts: 1,
		},
		{
			name:      "one of each",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &coreapi.PodSpec{Containers: []coreapi.Container{{}}},
			buildSpec: &buildv1alpha1.BuildSpec{},
			presets: []Preset{
				{
					Labels:       map[string]string{"foo": "bar"},
					Env:          []coreapi.EnvVar{{Name: "baz"}},
					VolumeMounts: []coreapi.VolumeMount{{Name: "baz"}},
					Volumes:      []coreapi.Volume{{Name: "qux"}},
				},
			},
			numEnv:       1,
			numVol:       1,
			numVolMounts: 1,
		},
		{
			name:      "two vm",
			jobLabels: map[string]string{"foo": "bar"},
			pod:       &coreapi.PodSpec{Containers: []coreapi.Container{{}}},
			buildSpec: &buildv1alpha1.BuildSpec{},
			presets: []Preset{
				{
					Labels:       map[string]string{"foo": "bar"},
					VolumeMounts: []coreapi.VolumeMount{{Name: "baz"}, {Name: "foo"}},
				},
			},
			numVolMounts: 2,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if err := resolvePresets("foo", tc.jobLabels, tc.pod, tc.buildSpec, tc.presets); err == nil && tc.shouldError {
				t.Errorf("expected error but got none.")
			} else if err != nil && !tc.shouldError {
				t.Errorf("expected no error but got %v.", err)
			}
			if tc.shouldError {
				return
			}
			if len(tc.pod.Volumes) != tc.numVol {
				t.Errorf("wrong number of volumes for podspec. Got %d, expected %d.", len(tc.pod.Volumes), tc.numVol)
			}
			if len(tc.buildSpec.Volumes) != tc.numVol {
				t.Errorf("wrong number of volumes for buildspec. Got %d, expected %d.", len(tc.pod.Volumes), tc.numVol)
			}
			for _, c := range tc.pod.Containers {
				if len(c.VolumeMounts) != tc.numVolMounts {
					t.Errorf("wrong number of volume mounts for podspec. Got %d, expected %d.", len(c.VolumeMounts), tc.numVolMounts)
				}
				if len(c.Env) != tc.numEnv {
					t.Errorf("wrong number of env vars for podspec. Got %d, expected %d.", len(c.Env), tc.numEnv)
				}
			}
			for _, c := range tc.buildSpec.Steps {
				if len(c.VolumeMounts) != tc.numVolMounts {
					t.Errorf("wrong number of volume mounts for buildspec. Got %d, expected %d.", len(c.VolumeMounts), tc.numVolMounts)
				}
				if len(c.Env) != tc.numEnv {
					t.Errorf("wrong number of env vars  for buildspec. Got %d, expected %d.", len(c.Env), tc.numEnv)
				}
			}
		})
	}
}

func TestPresubmitShouldRun(t *testing.T) {
	var testCases = []struct {
		name        string
		fileChanges []string
		fileError   error
		job         Presubmit
		ref         string
		expectedRun bool
		expectedErr bool
	}{
		{
			name: "job skipped on the branch won't run",
			job: Presubmit{
				Brancher: Brancher{
					SkipBranches: []string{"master"},
				},
			},
			ref:         "master",
			expectedRun: false,
		},
		{
			name: "job enabled on the branch will run",
			job: Presubmit{
				Brancher: Brancher{
					Branches: []string{"something"},
				},
				AlwaysRun: true,
			},
			ref:         "something",
			expectedRun: true,
		},
		{
			name: "job running only on other branches won't run",
			job: Presubmit{
				Brancher: Brancher{
					Branches: []string{"master"},
				},
			},
			ref:         "release-1.11",
			expectedRun: false,
		},
		{
			name: "job on a branch that's not skipped will run",
			job: Presubmit{
				Brancher: Brancher{
					SkipBranches: []string{"master"},
				},
				AlwaysRun: true,
			},
			ref:         "other",
			expectedRun: true,
		},
		{
			name: "job with always_run: true should run",
			job: Presubmit{
				AlwaysRun: true,
			},
			ref:         "master",
			expectedRun: true,
		},
		{
			name: "job with always_run: false and no run_if_changed should not run",
			job: Presubmit{
				AlwaysRun:    false,
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: RegexpChangeMatcher{
					RunIfChanged: "",
				},
			},
			ref:         "master",
			expectedRun: false,
		},
		{
			name: "job with run_if_changed but file get errors should not run",
			job: Presubmit{
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: RegexpChangeMatcher{
					RunIfChanged: "file",
				},
			},
			ref:         "master",
			fileError:   errors.New("oops"),
			expectedRun: false,
			expectedErr: true,
		},
		{
			name: "job with run_if_changed not matching should not run",
			job: Presubmit{
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: RegexpChangeMatcher{
					RunIfChanged: "^file$",
				},
			},
			ref:         "master",
			fileChanges: []string{"something"},
			expectedRun: false,
		},
		{
			name: "job with run_if_changed matching should run",
			job: Presubmit{
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: RegexpChangeMatcher{
					RunIfChanged: "^file$",
				},
			},
			ref:         "master",
			fileChanges: []string{"file"},
			expectedRun: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			jobs := []Presubmit{testCase.job}
			if err := SetPresubmitRegexes(jobs); err != nil {
				t.Fatalf("%s: failed to set presubmit regexes: %v", testCase.name, err)
			}
			jobShouldRun, err := jobs[0].ShouldRun(testCase.ref, func() ([]string, error) {
				return testCase.fileChanges, testCase.fileError
			}, false, false)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error and got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if jobShouldRun != testCase.expectedRun {
				t.Errorf("%s: did not determine if job should run correctly, expected %v but got %v", testCase.name, testCase.expectedRun, jobShouldRun)
			}
		})
	}
}

func TestPostsubmitShouldRun(t *testing.T) {
	var testCases = []struct {
		name        string
		fileChanges []string
		fileError   error
		job         Postsubmit
		ref         string
		expectedRun bool
		expectedErr bool
	}{
		{
			name: "job skipped on the branch won't run",
			job: Postsubmit{
				Brancher: Brancher{
					SkipBranches: []string{"master"},
				},
			},
			ref:         "master",
			expectedRun: false,
		},
		{
			name: "job enabled on the branch will run",
			job: Postsubmit{
				Brancher: Brancher{
					Branches: []string{"something"},
				},
			},
			ref:         "something",
			expectedRun: true,
		},
		{
			name: "job running only on other branches won't run",
			job: Postsubmit{
				Brancher: Brancher{
					Branches: []string{"master"},
				},
			},
			ref:         "release-1.11",
			expectedRun: false,
		},
		{
			name: "job on a branch that's not skipped will run",
			job: Postsubmit{
				Brancher: Brancher{
					SkipBranches: []string{"master"},
				},
			},
			ref:         "other",
			expectedRun: true,
		},
		{
			name:        "job with no run_if_changed should run",
			job:         Postsubmit{},
			ref:         "master",
			expectedRun: true,
		},
		{
			name: "job with run_if_changed but file get errors should not run",
			job: Postsubmit{
				RegexpChangeMatcher: RegexpChangeMatcher{
					RunIfChanged: "file",
				},
			},
			ref:         "master",
			fileError:   errors.New("oops"),
			expectedRun: false,
			expectedErr: true,
		},
		{
			name: "job with run_if_changed not matching should not run",
			job: Postsubmit{
				RegexpChangeMatcher: RegexpChangeMatcher{
					RunIfChanged: "^file$",
				},
			},
			ref:         "master",
			fileChanges: []string{"something"},
			expectedRun: false,
		},
		{
			name: "job with run_if_changed matching should run",
			job: Postsubmit{
				RegexpChangeMatcher: RegexpChangeMatcher{
					RunIfChanged: "^file$",
				},
			},
			ref:         "master",
			fileChanges: []string{"file"},
			expectedRun: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			jobs := []Postsubmit{testCase.job}
			if err := SetPostsubmitRegexes(jobs); err != nil {
				t.Fatalf("%s: failed to set presubmit regexes: %v", testCase.name, err)
			}
			jobShouldRun, err := jobs[0].ShouldRun(testCase.ref, func() ([]string, error) {
				return testCase.fileChanges, testCase.fileError
			})
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error and got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if jobShouldRun != testCase.expectedRun {
				t.Errorf("%s: did not determine if job should run correctly, expected %v but got %v", testCase.name, testCase.expectedRun, jobShouldRun)
			}
		})
	}
}

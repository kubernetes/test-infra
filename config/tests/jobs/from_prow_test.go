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

package tests

import (
	"regexp"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
)

// This file contains tests that previously lived in prow/config/jobs_test.go and prow/config/jobtests/job_config_test.go
// Some (all?) of these tests are not specific to the K8s Prow instance and should
// be ported to checkconfig so that all Prow instances can benefit from them.
// TODO: move such validations to checkconfig if they are not present already.

var podRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Returns if two brancher has overlapping branches
func checkOverlapBrancher(b1, b2 config.Brancher) bool {
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

func TestPresubmits(t *testing.T) {
	if len(c.PresubmitsStatic) == 0 {
		t.Fatalf("No jobs found in presubmit.yaml.")
	}

	for _, rootJobs := range c.PresubmitsStatic {
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
					if job2.TriggerMatches(job.RerunCommand) {
						t.Errorf("%d, %d, RerunCommand \"%s\" from job %s matches \"%v\" from job %s but shouldn't.", i, j, job.RerunCommand, job.Name, job2.Trigger, job2.Name)
					}
				}
			}
		}
	}
}

// TODO(krzyzacy): technically this, and TestPresubmits above should belong to config/ instead of prow/
func TestPostsubmits(t *testing.T) {
	if len(c.PostsubmitsStatic) == 0 {
		t.Fatalf("No jobs found in presubmit.yaml.")
	}

	for _, rootJobs := range c.PostsubmitsStatic {
		for i, job := range rootJobs {
			if job.Name == "" {
				t.Errorf("Job %v needs a name.", job)
				continue
			}
			if !job.SkipReport && job.Context == "" {
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

func TestValidPodNames(t *testing.T) {
	for _, j := range c.AllStaticPresubmits([]string{}) {
		if !podRe.MatchString(j.Name) {
			t.Errorf("Job \"%s\" must match regex \"%s\".", j.Name, podRe.String())
		}
	}
	for _, j := range c.AllStaticPostsubmits([]string{}) {
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
	for _, j := range c.AllStaticPostsubmits([]string{}) {
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

func missingVolumesForContainer(mounts []v1.VolumeMount, volumes []v1.Volume) sets.Set[string] {
	mountNames := sets.New[string]()
	volumeNames := sets.New[string]()
	for _, m := range mounts {
		mountNames.Insert(m.Name)
	}
	for _, v := range volumes {
		volumeNames.Insert(v.Name)
	}
	return mountNames.Difference(volumeNames)
}

func missingVolumesForSpec(spec *v1.PodSpec) map[string]sets.Set[string] {
	malformed := map[string]sets.Set[string]{}
	for _, container := range spec.InitContainers {
		malformed[container.Name] = missingVolumesForContainer(container.VolumeMounts, spec.Volumes)
	}
	for _, container := range spec.Containers {
		malformed[container.Name] = missingVolumesForContainer(container.VolumeMounts, spec.Volumes)
	}
	return malformed
}

func missingMountsForSpec(spec *v1.PodSpec) sets.Set[string] {
	mountNames := sets.New[string]()
	volumeNames := sets.New[string]()
	for _, container := range spec.Containers {
		for _, m := range container.VolumeMounts {
			mountNames.Insert(m.Name)
		}
	}
	for _, container := range spec.InitContainers {
		for _, m := range container.VolumeMounts {
			mountNames.Insert(m.Name)
		}
	}
	for _, v := range spec.Volumes {
		volumeNames.Insert(v.Name)
	}
	return volumeNames.Difference(mountNames)
}

// verify that all volume mounts reference volumes that exist
func TestMountsHaveVolumes(t *testing.T) {
	for _, job := range c.AllStaticPresubmits(nil) {
		if job.Spec != nil {
			validateVolumesAndMounts(job.Name, job.Spec, t)
		}
	}
	for _, job := range c.AllStaticPostsubmits(nil) {
		if job.Spec != nil {
			validateVolumesAndMounts(job.Name, job.Spec, t)
		}
	}
	for _, job := range c.AllPeriodics() {
		if job.Spec != nil {
			validateVolumesAndMounts(job.Name, job.Spec, t)
		}
	}
}

func validateVolumesAndMounts(name string, spec *v1.PodSpec, t *testing.T) {
	for container, missingVolumes := range missingVolumesForSpec(spec) {
		if len(missingVolumes) > 0 {
			t.Errorf("job %s in container %s has mounts that are missing volumes: %v", name, container, sets.List(missingVolumes))
		}
	}
	if missingMounts := missingMountsForSpec(spec); len(missingMounts) > 0 {
		t.Errorf("job %s has volumes that are not mounted: %v", name, sets.List(missingMounts))
	}
}

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

package jobtests

import (
	"flag"
	"fmt"
	"os"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	cfg "k8s.io/test-infra/prow/config"
)

var configPath = flag.String("config", "../../../config/prow/config.yaml", "Path to prow config")
var jobConfigPath = flag.String("job-config", "../../../config/jobs", "Path to prow job config")

// Loaded at TestMain.
var c *cfg.Config

func TestMain(m *testing.M) {
	flag.Parse()
	if *configPath == "" {
		fmt.Println("--config must set")
		os.Exit(1)
	}

	conf, err := cfg.Load(*configPath, *jobConfigPath, nil, "")
	if err != nil {
		fmt.Printf("Could not load config: %v", err)
		os.Exit(1)
	}
	c = conf

	os.Exit(m.Run())
}

func missingVolumesForContainer(mounts []v1.VolumeMount, volumes []v1.Volume) sets.String {
	mountNames := sets.NewString()
	volumeNames := sets.NewString()
	for _, m := range mounts {
		mountNames.Insert(m.Name)
	}
	for _, v := range volumes {
		volumeNames.Insert(v.Name)
	}
	return mountNames.Difference(volumeNames)
}

func missingVolumesForSpec(spec *v1.PodSpec) map[string]sets.String {
	malformed := map[string]sets.String{}
	for _, container := range spec.InitContainers {
		malformed[container.Name] = missingVolumesForContainer(container.VolumeMounts, spec.Volumes)
	}
	for _, container := range spec.Containers {
		malformed[container.Name] = missingVolumesForContainer(container.VolumeMounts, spec.Volumes)
	}
	return malformed
}

func missingMountsForSpec(spec *v1.PodSpec) sets.String {
	mountNames := sets.NewString()
	volumeNames := sets.NewString()
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
			t.Errorf("job %s in container %s has mounts that are missing volumes: %v", name, container, missingVolumes.List())
		}
	}
	if missingMounts := missingMountsForSpec(spec); len(missingMounts) > 0 {
		t.Errorf("job %s has volumes that are not mounted: %v", name, missingMounts.List())
	}
}

func checkContext(t *testing.T, repo string, p cfg.Presubmit) {
	if !p.SkipReport && p.Name != p.Context {
		t.Errorf("Context does not match job name: %s in %s", p.Name, repo)
	}
}

func TestContextMatches(t *testing.T) {
	for repo, presubmits := range c.PresubmitsStatic {
		for _, p := range presubmits {
			checkContext(t, repo, p)
		}
	}
}

func checkRetest(t *testing.T, repo string, presubmits []cfg.Presubmit) {
	for _, p := range presubmits {
		expected := fmt.Sprintf("/test %s", p.Name)
		if p.RerunCommand != expected {
			t.Errorf("%s in %s rerun_command: %s != expected: %s", repo, p.Name, p.RerunCommand, expected)
		}
	}
}

func TestRetestMatchJobsName(t *testing.T) {
	for repo, presubmits := range c.PresubmitsStatic {
		checkRetest(t, repo, presubmits)
	}
}

// TODO(cjwagner): remove this when the submit-queue is removed
type SubmitQueueConfig struct {
	// this is the only field we need for the tests below
	RequiredRetestContexts string `json:"required-retest-contexts"`
}

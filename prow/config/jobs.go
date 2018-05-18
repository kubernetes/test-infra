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
	"fmt"
	"regexp"
	"time"

	"k8s.io/api/core/v1"

	"k8s.io/test-infra/prow/kube"
)

// Preset is intended to match the k8s' PodPreset feature, and may be removed
// if that feature goes beta.
type Preset struct {
	Labels       map[string]string `json:"labels"`
	Env          []v1.EnvVar       `json:"env"`
	Volumes      []v1.Volume       `json:"volumes"`
	VolumeMounts []v1.VolumeMount  `json:"volumeMounts"`
}

func mergePreset(preset Preset, labels map[string]string, pod *v1.PodSpec) error {
	if pod == nil {
		return nil
	}
	for l, v := range preset.Labels {
		if v2, ok := labels[l]; !ok || v2 != v {
			return nil
		}
	}
	for _, e1 := range preset.Env {
		for i := range pod.Containers {
			for _, e2 := range pod.Containers[i].Env {
				if e1.Name == e2.Name {
					return fmt.Errorf("env var duplicated in pod spec: %s", e1.Name)
				}
			}
			pod.Containers[i].Env = append(pod.Containers[i].Env, e1)
		}
	}
	for _, v1 := range preset.Volumes {
		for _, v2 := range pod.Volumes {
			if v1.Name == v2.Name {
				return fmt.Errorf("volume duplicated in pod spec: %s", v1.Name)
			}
		}
		pod.Volumes = append(pod.Volumes, v1)
	}
	for _, vm1 := range preset.VolumeMounts {
		for i := range pod.Containers {
			for _, vm2 := range pod.Containers[i].VolumeMounts {
				if vm1.Name == vm2.Name {
					return fmt.Errorf("volume mount duplicated in pod spec: %s", vm1.Name)
				}
			}
			pod.Containers[i].VolumeMounts = append(pod.Containers[i].VolumeMounts, vm1)
		}
	}
	return nil
}

// Presubmit is the job-specific trigger info.
type Presubmit struct {
	// eg kubernetes-pull-build-test-e2e-gce
	Name string `json:"name"`
	// Labels are added in prowjobs created for this job.
	Labels map[string]string `json:"labels"`
	// Run for every PR, or only when a comment triggers it.
	AlwaysRun bool `json:"always_run"`
	// Run if the PR modifies a file that matches this regex.
	RunIfChanged string `json:"run_if_changed"`
	// Context line for GitHub status.
	Context string `json:"context"`
	// eg @k8s-bot e2e test this
	Trigger string `json:"trigger"`
	// Valid rerun command to give users. Must match Trigger.
	RerunCommand string `json:"rerun_command"`
	// Whether or not to skip commenting and setting status on GitHub.
	SkipReport bool `json:"skip_report"`
	// Maximum number of this job running concurrently, 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency"`
	// Agent that will take care of running this job.
	Agent string `json:"agent"`
	// Cluster is the alias of the cluster to run this job in. (Default: kube.DefaultClusterAlias)
	Cluster string `json:"cluster"`
	// Kubernetes pod spec.
	Spec *v1.PodSpec `json:"spec,omitempty"`
	// Run these jobs after successfully running this one.
	RunAfterSuccess []Presubmit `json:"run_after_success"`
	// Consider job optional for branch protection.
	Optional bool `json:"optional,omitempty"`

	Brancher

	UtilityConfig

	// We'll set these when we load it.
	re        *regexp.Regexp // from Trigger.
	reChanges *regexp.Regexp // from RunIfChanged
}

// Postsubmit runs on push events.
type Postsubmit struct {
	Name string `json:"name"`
	// Labels are added in prowjobs created for this job.
	Labels map[string]string `json:"labels"`
	// Agent that will take care of running this job.
	Agent string `json:"agent"`
	// Cluster is the alias of the cluster to run this job in. (Default: kube.DefaultClusterAlias)
	Cluster string `json:"cluster"`
	// Kubernetes pod spec.
	Spec *v1.PodSpec `json:"spec,omitempty"`
	// Maximum number of this job running concurrently, 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency"`

	Brancher

	UtilityConfig

	// Run these jobs after successfully running this one.
	RunAfterSuccess []Postsubmit `json:"run_after_success"`
}

// Periodic runs on a timer.
type Periodic struct {
	Name string `json:"name"`
	// Labels are added in prowjobs created for this job.
	Labels map[string]string `json:"labels"`
	// Agent that will take care of running this job.
	Agent string `json:"agent"`
	// Cluster is the alias of the cluster to run this job in. (Default: kube.DefaultClusterAlias)
	Cluster string `json:"cluster"`
	// Kubernetes pod spec.
	Spec *v1.PodSpec `json:"spec,omitempty"`
	// (deprecated)Interval to wait between two runs of the job.
	Interval string `json:"interval"`
	// Cron representation of job trigger time
	Cron string `json:"cron"`
	// Tags for config entries
	Tags []string `json:"tags,omitempty"`
	// Run these jobs after successfully running this one.
	RunAfterSuccess []Periodic `json:"run_after_success"`

	UtilityConfig

	interval time.Duration
}

func (p *Periodic) SetInterval(d time.Duration) {
	p.interval = d
}

func (p *Periodic) GetInterval() time.Duration {
	return p.interval
}

// Brancher is for shared code between jobs that only run against certain
// branches. An empty brancher runs against all branches.
type Brancher struct {
	// Do not run against these branches. Default is no branches.
	SkipBranches []string `json:"skip_branches"`
	// Only run against these branches. Default is all branches.
	Branches []string `json:"branches"`
}

func (br Brancher) RunsAgainstAllBranch() bool {
	return len(br.SkipBranches) == 0 && len(br.Branches) == 0
}

func (br Brancher) RunsAgainstBranch(branch string) bool {
	if br.RunsAgainstAllBranch() {
		return true
	}

	// Favor SkipBranches over Branches
	for _, s := range br.SkipBranches {
		if s == branch {
			return false
		}
	}
	if len(br.Branches) == 0 {
		return true
	}
	for _, b := range br.Branches {
		if b == branch {
			return true
		}
	}
	return false
}

func (ps Presubmit) RunsAgainstChanges(changes []string) bool {
	for _, change := range changes {
		if ps.reChanges.MatchString(change) {
			return true
		}
	}
	return false
}

func (ps Presubmit) TriggerMatches(body string) bool {
	return ps.re.MatchString(body)
}

// ContextRequired checks whether a context is required from github points of view (required check).
func (ps Presubmit) ContextRequired() bool {
	if ps.Optional || ps.SkipReport {
		return false
	}
	return true
}

type ChangedFilesProvider func() ([]string, error)

func matching(j Presubmit, body string, testAll bool) []Presubmit {
	// When matching ignore whether the job runs for the branch or whether the job runs for the
	// PR's changes. Even if the job doesn't run, it still matches the PR and may need to be marked
	// as skipped on github.
	var result []Presubmit
	if (testAll && (j.AlwaysRun || j.RunIfChanged != "")) || j.TriggerMatches(body) {
		result = append(result, j)
	}
	for _, child := range j.RunAfterSuccess {
		result = append(result, matching(child, body, testAll)...)
	}
	return result
}

func (c *Config) MatchingPresubmits(fullRepoName, body string, testAll bool) []Presubmit {
	var result []Presubmit
	if jobs, ok := c.Presubmits[fullRepoName]; ok {
		for _, job := range jobs {
			result = append(result, matching(job, body, testAll)...)
		}
	}
	return result
}

type UtilityConfig struct {
	// Decorate determines if we decorate the PodSpec or not
	Decorate bool `json:"decorate,omitempty"`

	// PathAlias is the location under <root-dir>/src
	// where the repository under test is cloned. If this
	// is not set, <root-dir>/src/github.com/org/repo will
	// be used as the default.
	PathAlias string `json:"path_alias,omitempty"`
	// CloneURI is the URI that is used to clone the
	// repository. If unset, will default to
	// `https://github.com/org/repo.git`.
	CloneURI string `json:"clone_uri,omitempty"`

	// ExtraRefs are auxiliary repositories that
	// need to be cloned, determined from config
	ExtraRefs []*kube.Refs `json:"extra_refs,omitempty"`

	// DecorationConfig holds configuration options for
	// decorating PodSpecs that users provide
	*kube.DecorationConfig
}

// RetestPresubmits returns all presubmits that should be run given a /retest command.
// This is the set of all presubmits intersected with ((alwaysRun + runContexts) - skipContexts)
func (c *Config) RetestPresubmits(fullRepoName string, skipContexts, runContexts map[string]bool) []Presubmit {
	var result []Presubmit
	if jobs, ok := c.Presubmits[fullRepoName]; ok {
		for _, job := range jobs {
			if skipContexts[job.Context] {
				continue
			}
			if job.AlwaysRun || job.RunIfChanged != "" || runContexts[job.Context] {
				result = append(result, job)
			}
		}
	}
	return result
}

// GetPresubmit returns the presubmit job for the provided repo and job name.
func (c *Config) GetPresubmit(repo, jobName string) *Presubmit {
	presubmits := c.AllPresubmits([]string{repo})
	for i := range presubmits {
		ps := presubmits[i]
		if ps.Name == jobName {
			return &ps
		}
	}
	return nil
}

func (c *Config) SetPresubmits(jobs map[string][]Presubmit) error {
	nj := map[string][]Presubmit{}
	for k, v := range jobs {
		nj[k] = make([]Presubmit, len(v))
		copy(nj[k], v)
		for i := range v {
			re, err := regexp.Compile(v[i].Trigger)
			if err != nil {
				return err
			}
			nj[k][i].re = re
			if v[i].RunIfChanged == "" {
				continue
			}
			re, err = regexp.Compile(v[i].RunIfChanged)
			if err != nil {
				return err
			}
			nj[k][i].reChanges = re

		}
	}
	c.Presubmits = nj
	return nil
}

// AllPresubmits returns all prow presubmit jobs in repos.
// if repos is empty, return all presubmits.
func (c *Config) AllPresubmits(repos []string) []Presubmit {
	var res []Presubmit
	var listPres func(ps []Presubmit) []Presubmit
	listPres = func(ps []Presubmit) []Presubmit {
		var res []Presubmit
		for _, p := range ps {
			res = append(res, p)
			res = append(res, listPres(p.RunAfterSuccess)...)
		}
		return res
	}

	for repo, v := range c.Presubmits {
		if len(repos) == 0 {
			res = append(res, listPres(v)...)
		} else {
			for _, r := range repos {
				if r == repo {
					res = append(res, listPres(v)...)
					break
				}
			}
		}
	}

	return res
}

// AllPostsubmits returns all prow postsubmit jobs in repos.
// if repos is empty, return all postsubmits.
func (c *Config) AllPostsubmits(repos []string) []Postsubmit {
	var res []Postsubmit
	var listPost func(ps []Postsubmit) []Postsubmit
	listPost = func(ps []Postsubmit) []Postsubmit {
		var res []Postsubmit
		for _, p := range ps {
			res = append(res, p)
			res = append(res, listPost(p.RunAfterSuccess)...)
		}
		return res
	}

	for repo, v := range c.Postsubmits {
		if len(repos) == 0 {
			res = append(res, listPost(v)...)
		} else {
			for _, r := range repos {
				if r == repo {
					res = append(res, listPost(v)...)
					break
				}
			}
		}
	}

	return res
}

// AllPostsubmits returns all prow periodic jobs.
func (c *Config) AllPeriodics() []Periodic {
	var listPeriodic func(ps []Periodic) []Periodic
	listPeriodic = func(ps []Periodic) []Periodic {
		var res []Periodic
		for _, p := range ps {
			res = append(res, p)
			res = append(res, listPeriodic(p.RunAfterSuccess)...)
		}
		return res
	}

	return listPeriodic(c.Periodics)
}

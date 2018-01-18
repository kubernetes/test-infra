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
	"regexp"
	"time"

	"k8s.io/test-infra/prow/kube"
)

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
	Spec *kube.PodSpec `json:"spec,omitempty"`
	// Run these jobs after successfully running this one.
	RunAfterSuccess []Presubmit `json:"run_after_success"`

	Brancher

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
	Spec *kube.PodSpec `json:"spec,omitempty"`
	// Maximum number of this job running concurrently, 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency"`

	Brancher
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
	Spec *kube.PodSpec `json:"spec,omitempty"`
	// (deprecated)Interval to wait between two runs of the job.
	Interval string `json:"interval"`
	// Cron representation of job trigger time
	Cron string `json:"cron"`
	// Tags for config entries
	Tags []string `json:"tags,omitempty"`
	// Run these jobs after successfully running this one.
	RunAfterSuccess []Periodic `json:"run_after_success"`

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

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

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	pipelinev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/github"
)

// Preset is intended to match the k8s' PodPreset feature, and may be removed
// if that feature goes beta.
type Preset struct {
	Labels       map[string]string `json:"labels"`
	Env          []v1.EnvVar       `json:"env"`
	Volumes      []v1.Volume       `json:"volumes"`
	VolumeMounts []v1.VolumeMount  `json:"volumeMounts"`
}

func mergePreset(preset Preset, labels map[string]string, containers []v1.Container, volumes *[]v1.Volume) error {
	for l, v := range preset.Labels {
		if v2, ok := labels[l]; !ok || v2 != v {
			return nil
		}
	}
	for _, e1 := range preset.Env {
		for i := range containers {
			for _, e2 := range containers[i].Env {
				if e1.Name == e2.Name {
					return fmt.Errorf("env var duplicated in pod spec: %s", e1.Name)
				}
			}
			containers[i].Env = append(containers[i].Env, e1)
		}
	}
	for _, v1 := range preset.Volumes {
		for _, v2 := range *volumes {
			if v1.Name == v2.Name {
				return fmt.Errorf("volume duplicated in pod spec: %s", v1.Name)
			}
		}
		*volumes = append(*volumes, v1)
	}
	for _, vm1 := range preset.VolumeMounts {
		for i := range containers {
			for _, vm2 := range containers[i].VolumeMounts {
				if vm1.Name == vm2.Name {
					return fmt.Errorf("volume mount duplicated in pod spec: %s", vm1.Name)
				}
			}
			containers[i].VolumeMounts = append(containers[i].VolumeMounts, vm1)
		}
	}
	return nil
}

// JobBase contains attributes common to all job types
type JobBase struct {
	// The name of the job. Must match regex [A-Za-z0-9-._]+
	// e.g. pull-test-infra-bazel-build
	Name string `json:"name"`
	// Labels are added to prowjobs and pods created for this job.
	Labels map[string]string `json:"labels,omitempty"`
	// MaximumConcurrency of this job, 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency,omitempty"`
	// Agent that will take care of running this job. Defaults to "kubernetes"
	Agent string `json:"agent,omitempty"`
	// Cluster is the alias of the cluster to run this job in.
	// (Default: kube.DefaultClusterAlias)
	Cluster string `json:"cluster,omitempty"`
	// Namespace is the namespace in which pods schedule.
	//   nil: results in config.PodNamespace (aka pod default)
	//   empty: results in config.ProwJobNamespace (aka same as prowjob)
	Namespace *string `json:"namespace,omitempty"`
	// ErrorOnEviction indicates that the ProwJob should be completed and given
	// the ErrorState status if the pod that is executing the job is evicted.
	// If this field is unspecified or false, a new pod will be created to replace
	// the evicted one.
	ErrorOnEviction bool `json:"error_on_eviction,omitempty"`
	// SourcePath contains the path where this job is defined
	SourcePath string `json:"-"`
	// Spec is the Kubernetes pod spec used if Agent is kubernetes.
	Spec *v1.PodSpec `json:"spec,omitempty"`
	// BuildSpec is the Knative build spec used if Agent is knative-build.
	BuildSpec *buildv1alpha1.BuildSpec `json:"build_spec,omitempty"`
	// PipelineRunSpec is the tekton pipeline spec used if Agent is tekton-pipeline.
	PipelineRunSpec *pipelinev1alpha1.PipelineRunSpec `json:"pipeline_run_spec,omitempty"`
	// Annotations are unused by prow itself, but provide a space to configure other automation.
	Annotations map[string]string `json:"annotations,omitempty"`
	// ReporterConfig provides the option to configure reporting on job level
	ReporterConfig *prowapi.ReporterConfig `json:"reporter_config,omitempty"`
	// RerunPermissions specifies who can rerun the job
	RerunPermissions *prowapi.RerunPermissions `json:"rerun_permissions,omitempty"`

	UtilityConfig
}

// Presubmit runs on PRs.
type Presubmit struct {
	JobBase

	// AlwaysRun automatically for every PR, or only when a comment triggers it.
	AlwaysRun bool `json:"always_run"`

	// Optional indicates that the job's status context should not be required for merge.
	Optional bool `json:"optional,omitempty"`

	// Trigger is the regular expression to trigger the job.
	// e.g. `@k8s-bot e2e test this`
	// RerunCommand must also be specified if this field is specified.
	// (Default: `(?m)^/test (?:.*? )?<job name>(?: .*?)?$`)
	Trigger string `json:"trigger,omitempty"`

	// The RerunCommand to give users. Must match Trigger.
	// Trigger must also be specified if this field is specified.
	// (Default: `/test <job name>`)
	RerunCommand string `json:"rerun_command,omitempty"`

	Brancher

	RegexpChangeMatcher

	Reporter

	JenkinsSpec *JenkinsSpec `json:"jenkins_spec,omitempty"`

	// We'll set these when we load it.
	re *regexp.Regexp // from Trigger.
}

// Postsubmit runs on push events.
type Postsubmit struct {
	JobBase

	RegexpChangeMatcher

	Brancher

	// TODO(krzyzacy): Move existing `Report` into `Skip_Report` once this is deployed
	Reporter

	JenkinsSpec *JenkinsSpec `json:"jenkins_spec,omitempty"`
}

// Periodic runs on a timer.
type Periodic struct {
	JobBase

	// (deprecated)Interval to wait between two runs of the job.
	Interval string `json:"interval,omitempty"`
	// Cron representation of job trigger time
	Cron string `json:"cron,omitempty"`
	// Tags for config entries
	Tags []string `json:"tags,omitempty"`

	interval time.Duration
}

// JenkinsSpec holds optional Jenkins job config
type JenkinsSpec struct {
	// Job is managed by the GH branch source plugin
	// and requires a specific path
	GitHubBranchSourceJob bool `json:"github_branch_source_job,omitempty"`
}

// SetInterval updates interval, the frequency duration it runs.
func (p *Periodic) SetInterval(d time.Duration) {
	p.interval = d
}

// GetInterval returns interval, the frequency duration it runs.
func (p *Periodic) GetInterval() time.Duration {
	return p.interval
}

// Brancher is for shared code between jobs that only run against certain
// branches. An empty brancher runs against all branches.
type Brancher struct {
	// Do not run against these branches. Default is no branches.
	SkipBranches []string `json:"skip_branches,omitempty"`
	// Only run against these branches. Default is all branches.
	Branches []string `json:"branches,omitempty"`

	// We'll set these when we load it.
	re     *regexp.Regexp
	reSkip *regexp.Regexp
}

// RegexpChangeMatcher is for code shared between jobs that run only when certain files are changed.
type RegexpChangeMatcher struct {
	// RunIfChanged defines a regex used to select which subset of file changes should trigger this job.
	// If any file in the changeset matches this regex, the job will be triggered
	RunIfChanged string         `json:"run_if_changed,omitempty"`
	reChanges    *regexp.Regexp // from RunIfChanged
}

type Reporter struct {
	// Context is the name of the GitHub status context for the job.
	// Defaults: the same as the name of the job.
	Context string `json:"context,omitempty"`
	// SkipReport skips commenting and setting status on GitHub.
	SkipReport bool `json:"skip_report,omitempty"`
}

// RunsAgainstAllBranch returns true if there are both branches and skip_branches are unset
func (br Brancher) RunsAgainstAllBranch() bool {
	return len(br.SkipBranches) == 0 && len(br.Branches) == 0
}

// ShouldRun returns true if the input branch matches, given the whitelist/blacklist.
func (br Brancher) ShouldRun(branch string) bool {
	if br.RunsAgainstAllBranch() {
		return true
	}

	// Favor SkipBranches over Branches
	if len(br.SkipBranches) != 0 && br.reSkip.MatchString(branch) {
		return false
	}
	if len(br.Branches) == 0 || br.re.MatchString(branch) {
		return true
	}
	return false
}

// Intersects checks if other Brancher would trigger for the same branch.
func (br Brancher) Intersects(other Brancher) bool {
	if br.RunsAgainstAllBranch() || other.RunsAgainstAllBranch() {
		return true
	}
	if len(br.Branches) > 0 {
		baseBranches := sets.NewString(br.Branches...)
		if len(other.Branches) > 0 {
			otherBranches := sets.NewString(other.Branches...)
			if baseBranches.Intersection(otherBranches).Len() > 0 {
				return true
			}
			return false
		}

		// Actually test our branches against the other brancher - if there are regex skip lists, simple comparison
		// is insufficient.
		for _, b := range baseBranches.List() {
			if other.ShouldRun(b) {
				return true
			}
		}
		return false
	}
	if len(other.Branches) == 0 {
		// There can only be one Brancher with skip_branches.
		return true
	}
	return other.Intersects(br)
}

// CouldRun determines if its possible for a set of changes to trigger this condition
func (cm RegexpChangeMatcher) CouldRun() bool {
	return cm.RunIfChanged != ""
}

// ShouldRun determines if we can know for certain that the job should run. We can either
// know for certain that the job should or should not run based on the matcher, or we can
// not be able to determine that fact at all.
func (cm RegexpChangeMatcher) ShouldRun(changes ChangedFilesProvider) (determined bool, shouldRun bool, err error) {
	if cm.CouldRun() {
		changeList, err := changes()
		if err != nil {
			return true, false, err
		}
		return true, cm.RunsAgainstChanges(changeList), nil
	}
	return false, false, nil
}

// RunsAgainstChanges returns true if any of the changed input paths match the run_if_changed regex.
func (cm RegexpChangeMatcher) RunsAgainstChanges(changes []string) bool {
	for _, change := range changes {
		if cm.reChanges.MatchString(change) {
			return true
		}
	}
	return false
}

// CouldRun determines if the postsubmit could run against a specific
// base ref
func (ps Postsubmit) CouldRun(baseRef string) bool {
	return ps.Brancher.ShouldRun(baseRef)
}

// ShouldRun determines if the postsubmit should run in response to a
// set of changes. This is evaluated lazily, if necessary.
func (ps Postsubmit) ShouldRun(baseRef string, changes ChangedFilesProvider) (bool, error) {
	if !ps.CouldRun(baseRef) {
		return false, nil
	}
	if determined, shouldRun, err := ps.RegexpChangeMatcher.ShouldRun(changes); err != nil {
		return false, err
	} else if determined {
		return shouldRun, nil
	}
	// Postsubmits default to always run
	return true, nil
}

// CouldRun determines if the presubmit could run against a specific
// base ref
func (ps Presubmit) CouldRun(baseRef string) bool {
	return ps.Brancher.ShouldRun(baseRef)
}

// ShouldRun determines if the presubmit should run against a specific
// base ref, or in response to a set of changes. The latter mechanism
// is evaluated lazily, if necessary.
func (ps Presubmit) ShouldRun(baseRef string, changes ChangedFilesProvider, forced, defaults bool) (bool, error) {
	if !ps.CouldRun(baseRef) {
		return false, nil
	}
	if ps.AlwaysRun {
		return true, nil
	}
	if forced {
		return true, nil
	}
	if determined, shouldRun, err := ps.RegexpChangeMatcher.ShouldRun(changes); err != nil {
		return false, err
	} else if determined {
		return shouldRun, nil
	}
	return defaults, nil
}

// TriggersConditionally determines if the presubmit triggers conditionally (if it may or may not trigger).
func (ps Presubmit) TriggersConditionally() bool {
	return ps.NeedsExplicitTrigger() || ps.RegexpChangeMatcher.CouldRun()
}

// NeedsExplicitTrigger determines if the presubmit requires a human action to trigger it or not.
func (ps Presubmit) NeedsExplicitTrigger() bool {
	return !ps.AlwaysRun && !ps.RegexpChangeMatcher.CouldRun()
}

// TriggerMatches returns true if the comment body should trigger this presubmit.
//
// This is usually a /test foo string.
func (ps Presubmit) TriggerMatches(body string) bool {
	return ps.Trigger != "" && ps.re.MatchString(body)
}

// ContextRequired checks whether a context is required from github points of view (required check).
func (ps Presubmit) ContextRequired() bool {
	return !ps.Optional && !ps.SkipReport
}

// ChangedFilesProvider returns a slice of modified files.
type ChangedFilesProvider func() ([]string, error)

type githubClient interface {
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

// NewGitHubDeferredChangedFilesProvider uses a closure to lazily retrieve the file changes only if they are needed.
// We only have to fetch the changes if there is at least one RunIfChanged job that is not being force run (due to
// a `/retest` after a failure or because it is explicitly triggered with `/test foo`).
func NewGitHubDeferredChangedFilesProvider(client githubClient, org, repo string, num int) ChangedFilesProvider {
	var changedFiles []string
	return func() ([]string, error) {
		// Fetch the changed files from github at most once.
		if changedFiles == nil {
			changes, err := client.GetPullRequestChanges(org, repo, num)
			if err != nil {
				return nil, fmt.Errorf("error getting pull request changes: %v", err)
			}
			for _, change := range changes {
				changedFiles = append(changedFiles, change.Filename)
			}
		}
		return changedFiles, nil
	}
}

// UtilityConfig holds decoration metadata, such as how to clone and additional containers/etc
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
	// SkipSubmodules determines if submodules should be
	// cloned when the job is run. Defaults to true.
	SkipSubmodules bool `json:"skip_submodules,omitempty"`
	// CloneDepth is the depth of the clone that will be used.
	// A depth of zero will do a full clone.
	CloneDepth int `json:"clone_depth,omitempty"`

	// ExtraRefs are auxiliary repositories that
	// need to be cloned, determined from config
	ExtraRefs []prowapi.Refs `json:"extra_refs,omitempty"`

	// DecorationConfig holds configuration options for
	// decorating PodSpecs that users provide
	DecorationConfig *prowapi.DecorationConfig `json:"decoration_config,omitempty"`
}

// RetestPresubmits returns all presubmits that should be run given a /retest command.
// This is the set of all presubmits intersected with ((alwaysRun + runContexts) - skipContexts)
func (c *JobConfig) RetestPresubmits(fullRepoName string, skipContexts, runContexts sets.String) []Presubmit {
	var result []Presubmit
	if jobs, ok := c.Presubmits[fullRepoName]; ok {
		for _, job := range jobs {
			if skipContexts.Has(job.Context) {
				continue
			}
			if job.AlwaysRun || job.RunIfChanged != "" || runContexts.Has(job.Context) {
				result = append(result, job)
			}
		}
	}
	return result
}

// GetPresubmit returns the presubmit job for the provided repo and job name.
func (c *JobConfig) GetPresubmit(repo, jobName string) *Presubmit {
	presubmits := c.AllPresubmits([]string{repo})
	for i := range presubmits {
		ps := presubmits[i]
		if ps.Name == jobName {
			return &ps
		}
	}
	return nil
}

// SetPresubmits updates c.Presubmits to jobs, after compiling and validating their regexes.
func (c *JobConfig) SetPresubmits(jobs map[string][]Presubmit) error {
	nj := map[string][]Presubmit{}
	for k, v := range jobs {
		nj[k] = make([]Presubmit, len(v))
		copy(nj[k], v)
		if err := SetPresubmitRegexes(nj[k]); err != nil {
			return err
		}
	}
	c.Presubmits = nj
	return nil
}

// SetPostsubmits updates c.Postsubmits to jobs, after compiling and validating their regexes.
func (c *JobConfig) SetPostsubmits(jobs map[string][]Postsubmit) error {
	nj := map[string][]Postsubmit{}
	for k, v := range jobs {
		nj[k] = make([]Postsubmit, len(v))
		copy(nj[k], v)
		if err := SetPostsubmitRegexes(nj[k]); err != nil {
			return err
		}
	}
	c.Postsubmits = nj
	return nil
}

// AllPresubmits returns all prow presubmit jobs in repos.
// if repos is empty, return all presubmits.
func (c *JobConfig) AllPresubmits(repos []string) []Presubmit {
	var res []Presubmit

	for repo, v := range c.Presubmits {
		if len(repos) == 0 {
			res = append(res, v...)
		} else {
			for _, r := range repos {
				if r == repo {
					res = append(res, v...)
					break
				}
			}
		}
	}

	return res
}

// AllPostsubmits returns all prow postsubmit jobs in repos.
// if repos is empty, return all postsubmits.
func (c *JobConfig) AllPostsubmits(repos []string) []Postsubmit {
	var res []Postsubmit

	for repo, v := range c.Postsubmits {
		if len(repos) == 0 {
			res = append(res, v...)
		} else {
			for _, r := range repos {
				if r == repo {
					res = append(res, v...)
					break
				}
			}
		}
	}

	return res
}

// AllPeriodics returns all prow periodic jobs.
func (c *JobConfig) AllPeriodics() []Periodic {
	var listPeriodic func(ps []Periodic) []Periodic
	listPeriodic = func(ps []Periodic) []Periodic {
		var res []Periodic
		for _, p := range ps {
			res = append(res, p)
		}
		return res
	}

	return listPeriodic(c.Periodics)
}

// ClearCompiledRegexes removes compiled regexes from the presubmits,
// useful for testing when deep equality is needed between presubmits
func ClearCompiledRegexes(presubmits []Presubmit) {
	for i := range presubmits {
		presubmits[i].re = nil
		presubmits[i].Brancher.re = nil
		presubmits[i].Brancher.reSkip = nil
		presubmits[i].RegexpChangeMatcher.reChanges = nil
	}
}

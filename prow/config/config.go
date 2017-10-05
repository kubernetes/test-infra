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

// Package config knows how to read and parse config.yaml.
package config

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"text/template"
	"time"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
)

// Config is a read-only snapshot of the config.
type Config struct {
	// Full repo name (such as "kubernetes/kubernetes") -> list of jobs.
	Presubmits  map[string][]Presubmit  `json:"presubmits,omitempty"`
	Postsubmits map[string][]Postsubmit `json:"postsubmits,omitempty"`

	// Periodics are not associated with any repo.
	Periodics []Periodic `json:"periodics,omitempty"`

	Tide   Tide   `json:"tide,omitempty"`
	Plank  Plank  `json:"plank,omitempty"`
	Sinker Sinker `json:"sinker,omitempty"`

	// TODO: Move this out of the main config.
	JenkinsOperator JenkinsOperator `json:"jenkins_operator,omitempty"`

	// ProwJobNamespace is the namespace in the cluster that prow
	// components will use for looking up ProwJobs. The namespace
	// needs to exist and will not be created by prow.
	// Defaults to "default".
	ProwJobNamespace string `json:"prowjob_namespace,omitempty"`
	// PodNamespace is the namespace in the cluster that prow
	// components will use for looking up Pods owned by ProwJobs.
	// The namespace needs to exist and will not be created by prow.
	// Defaults to "default".
	PodNamespace string `json:"pod_namespace,omitempty"`

	// LogLevel enables dynamically updating the log level of the
	// standard logger that is used by all prow components.
	//
	// Valid values:
	//
	// "debug", "info", "warn", "warning", "error", "fatal", "panic"
	//
	// Defaults to "info".
	LogLevel string `json:"log_level,omitempty"`

	// PushGateway is a prometheus push gateway.
	PushGateway PushGateway `json:"push_gateway,omitempty"`
}

type PushGateway struct {
	Endpoint string `json:"endpoint,omitempty"`
}

// Tide is config for the tide pool.
type Tide struct {
	// These must be valid GitHub search queries. They should not overlap,
	// which is to say two queries should never return the same PR.
	Queries []string `json:"queries,omitempty"`
}

// Plank is config for the plank controller.
type Plank struct {
	// JobURLTemplateString compiles into JobURLTemplate at load time.
	JobURLTemplateString string `json:"job_url_template,omitempty"`
	// JobURLTemplate is compiled at load time from JobURLTemplateString. It
	// will be passed a kube.ProwJob and is used to set the URL for the
	// "details" link on GitHub as well as the link from deck.
	JobURLTemplate *template.Template `json:"-"`

	// ReportTemplateString compiles into ReportTemplate at load time.
	ReportTemplateString string `json:"report_template,omitempty"`
	// ReportTemplate is compiled at load time from ReportTemplateString. It
	// will be passed a kube.ProwJob and can provide an optional blurb below
	// the test failures comment.
	ReportTemplate *template.Template `json:"-"`

	// MaxConcurrency is the maximum number of tests running concurrently that
	// will be allowed by plank. 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency,omitempty"`

	// AllowCancellations enables aborting presubmit jobs for commits that
	// have been superseded by newer commits in Github pull requests.
	AllowCancellations bool `json:"allow_cancellations"`
}

// JenkinsOperator is config for the jenkins-operator controller.
type JenkinsOperator struct {
	// JobURLTemplateString compiles into JobURLTemplate at load time.
	JobURLTemplateString string `json:"job_url_template,omitempty"`
	// JobURLTemplate is compiled at load time from JobURLTemplateString. It
	// will be passed a kube.ProwJob and is used to set the URL for the
	// "details" link on GitHub as well as the link from deck.
	JobURLTemplate *template.Template `json:"-"`

	// ReportTemplateString compiles into ReportTemplate at load time.
	ReportTemplateString string `json:"report_template,omitempty"`
	// ReportTemplate is compiled at load time from ReportTemplateString. It
	// will be passed a kube.ProwJob and can provide an optional blurb below
	// the test failures comment.
	ReportTemplate *template.Template `json:"-"`

	// MaxConcurrency is the maximum number of tests running concurrently that
	// will be allowed by jenkins-operator. 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency,omitempty"`

	// AllowCancellations enables aborting presubmit jobs for commits that
	// have been superseded by newer commits in Github pull requests.
	AllowCancellations bool `json:"allow_cancellations"`
}

// Sinker is config for the sinker controller.
type Sinker struct {
	// ResyncPeriodString compiles into ResyncPeriod at load time.
	ResyncPeriodString string `json:"resync_period,omitempty"`
	// ResyncPeriod is how often the controller will perform a garbage
	// collection. Defaults to one hour.
	ResyncPeriod time.Duration `json:"-"`
	// MaxProwJobAgeString compiles into MaxProwJobAge at load time.
	MaxProwJobAgeString string `json:"max_prowjob_age,omitempty"`
	// MaxProwJobAge is how old a ProwJob can be before it is garbage-collected.
	// Defaults to one week.
	MaxProwJobAge time.Duration `json:"-"`
	// MaxPodAgeString compiles into MaxPodAge at load time.
	MaxPodAgeString string `json:"max_pod_age,omitempty"`
	// MaxPodAge is how old a Pod can be before it is garbage-collected.
	// Defaults to one day.
	MaxPodAge time.Duration `json:"-"`
}

// Load loads and parses the config at path.
func Load(path string) (*Config, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", path, err)
	}
	nc := &Config{}
	if err := yaml.Unmarshal(b, nc); err != nil {
		return nil, fmt.Errorf("error unmarshaling %s: %v", path, err)
	}
	if err := parseConfig(nc); err != nil {
		return nil, err
	}
	return nc, nil
}

func parseConfig(c *Config) error {
	// Ensure that presubmit regexes are valid.
	for _, vs := range c.Presubmits {
		if err := SetRegexes(vs); err != nil {
			return fmt.Errorf("could not set regex: %v", err)
		}
	}

	// Validate presubmits.
	for _, v := range c.AllPresubmits(nil) {
		name := v.Name
		agent := v.Agent
		// Ensure that k8s presubmits have a pod spec.
		if agent == string(kube.KubernetesAgent) && v.Spec == nil {
			return fmt.Errorf("job %s has no spec", name)
		}
		// Ensure agent is a known value.
		if agent != string(kube.KubernetesAgent) && agent != string(kube.JenkinsAgent) {
			return fmt.Errorf("job %s has invalid agent (%s), it needs to be one of the following: %s %s",
				name, agent, kube.KubernetesAgent, kube.JenkinsAgent)
		}
		// Ensure max_concurrency is non-negative.
		if v.MaxConcurrency < 0 {
			return fmt.Errorf("job %s jas invalid max_concurrency (%d), it needs to be a non-negative number", name, v.MaxConcurrency)
		}
	}

	// Validate postsubmits.
	for _, j := range c.AllPostsubmits(nil) {
		name := j.Name
		agent := j.Agent
		// Ensure that k8s postsubmits have a pod spec.
		if agent == string(kube.KubernetesAgent) && j.Spec == nil {
			return fmt.Errorf("job %s has no spec", name)
		}
		// Ensure agent is a known value.
		if agent != string(kube.KubernetesAgent) && agent != string(kube.JenkinsAgent) {
			return fmt.Errorf("job %s has invalid agent (%s), it needs to be one of the following: %s %s",
				name, agent, kube.KubernetesAgent, kube.JenkinsAgent)
		}
		// Ensure max_concurrency is non-negative.
		if j.MaxConcurrency < 0 {
			return fmt.Errorf("job %s jas invalid max_concurrency (%d), it needs to be a non-negative number", name, j.MaxConcurrency)
		}
	}

	// Ensure that the periodic durations are valid and specs exist.
	for _, p := range c.AllPeriodics() {
		name := p.Name
		agent := p.Agent
		if agent == string(kube.KubernetesAgent) && p.Spec == nil {
			return fmt.Errorf("job %s has no spec", name)
		}
		if agent != string(kube.KubernetesAgent) && agent != string(kube.JenkinsAgent) {
			return fmt.Errorf("job %s has invalid agent (%s), it needs to be one of the following: %s %s",
				name, agent, kube.KubernetesAgent, kube.JenkinsAgent)
		}
	}
	// Set the interval on the periodic jobs. It doesn't make sense to do this
	// for child jobs.
	for j := range c.Periodics {
		d, err := time.ParseDuration(c.Periodics[j].Interval)
		if err != nil {
			return fmt.Errorf("cannot parse duration for %s: %v", c.Periodics[j].Name, err)
		}
		c.Periodics[j].interval = d
	}

	urlTmpl, err := template.New("JobURL").Parse(c.Plank.JobURLTemplateString)
	if err != nil {
		return fmt.Errorf("parsing template: %v", err)
	}
	c.Plank.JobURLTemplate = urlTmpl

	reportTmpl, err := template.New("Report").Parse(c.Plank.ReportTemplateString)
	if err != nil {
		return fmt.Errorf("parsing template: %v", err)
	}
	c.Plank.ReportTemplate = reportTmpl
	if c.Plank.MaxConcurrency < 0 {
		return fmt.Errorf("plank has invalid max_concurrency (%d), it needs to be a non-negative number", c.Plank.MaxConcurrency)
	}

	jenkinsURLTmpl, err := template.New("JobURL").Parse(c.JenkinsOperator.JobURLTemplateString)
	if err != nil {
		return fmt.Errorf("parsing template: %v", err)
	}
	c.JenkinsOperator.JobURLTemplate = jenkinsURLTmpl

	jenkinsReportTmpl, err := template.New("Report").Parse(c.JenkinsOperator.ReportTemplateString)
	if err != nil {
		return fmt.Errorf("parsing template: %v", err)
	}
	c.JenkinsOperator.ReportTemplate = jenkinsReportTmpl
	if c.JenkinsOperator.MaxConcurrency < 0 {
		return fmt.Errorf("jenkins-operator has invalid max_concurrency (%d), it needs to be a non-negative number", c.JenkinsOperator.MaxConcurrency)
	}

	if c.Sinker.ResyncPeriodString == "" {
		c.Sinker.ResyncPeriod = time.Hour
	} else {
		resyncPeriod, err := time.ParseDuration(c.Sinker.ResyncPeriodString)
		if err != nil {
			return fmt.Errorf("cannot parse duration for resync_period: %v", err)
		}
		c.Sinker.ResyncPeriod = resyncPeriod
	}

	if c.Sinker.MaxProwJobAgeString == "" {
		c.Sinker.MaxProwJobAge = 7 * 24 * time.Hour
	} else {
		maxProwJobAge, err := time.ParseDuration(c.Sinker.MaxProwJobAgeString)
		if err != nil {
			return fmt.Errorf("cannot parse duration for max_prowjob_age: %v", err)
		}
		c.Sinker.MaxProwJobAge = maxProwJobAge
	}

	if c.Sinker.MaxPodAgeString == "" {
		c.Sinker.MaxPodAge = 24 * time.Hour
	} else {
		maxPodAge, err := time.ParseDuration(c.Sinker.MaxPodAgeString)
		if err != nil {
			return fmt.Errorf("cannot parse duration for max_pod_age: %v", err)
		}
		c.Sinker.MaxPodAge = maxPodAge
	}

	if c.ProwJobNamespace == "" {
		c.ProwJobNamespace = "default"
	}
	if c.PodNamespace == "" {
		c.PodNamespace = "default"
	}

	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	lvl, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(lvl)

	return nil
}

// SetRegexes compiles and validates all the regural expressions for
// the provided presubmits.
func SetRegexes(js []Presubmit) error {
	for i, j := range js {
		if re, err := regexp.Compile(j.Trigger); err == nil {
			js[i].re = re
		} else {
			return fmt.Errorf("could not compile trigger regex for %s: %v", j.Name, err)
		}
		if !js[i].re.MatchString(j.RerunCommand) {
			return fmt.Errorf("for job %s, rerun command \"%s\" does not match trigger \"%s\"", j.Name, j.RerunCommand, j.Trigger)
		}
		if err := SetRegexes(j.RunAfterSuccess); err != nil {
			return err
		}
		if j.RunIfChanged != "" {
			re, err := regexp.Compile(j.RunIfChanged)
			if err != nil {
				return fmt.Errorf("could not compile changes regex for %s: %v", j.Name, err)
			}
			js[i].reChanges = re
		}
	}
	return nil
}

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
	cron "gopkg.in/robfig/cron.v2"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/test-infra/prow/kube"
)

// Config is a read-only snapshot of the config.
type Config struct {
	// Full repo name (such as "kubernetes/kubernetes") -> list of jobs.
	Presubmits  map[string][]Presubmit  `json:"presubmits,omitempty"`
	Postsubmits map[string][]Postsubmit `json:"postsubmits,omitempty"`

	// Periodics are not associated with any repo.
	Periodics []Periodic `json:"periodics,omitempty"`

	Tide             Tide             `json:"tide,omitempty"`
	Plank            Plank            `json:"plank,omitempty"`
	Sinker           Sinker           `json:"sinker,omitempty"`
	Deck             Deck             `json:"deck,omitempty"`
	BranchProtection BranchProtection `json:"branch-protection,omitempty"`

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

// PushGateway is a prometheus push gateway.
type PushGateway struct {
	// Endpoint is the location of the prometheus pushgateway
	// where prow will push metrics to.
	Endpoint string `json:"endpoint,omitempty"`
	// IntervalString compiles into Interval at load time.
	IntervalString string `json:"interval,omitempty"`
	// Interval specifies how often prow will push metrics
	// to the pushgateway. Defaults to 1m.
	Interval time.Duration `json:"-"`
}

// Controller holds configuration applicable to all agent-specific
// prow controllers.
type Controller struct {
	// JobURLTemplateString compiles into JobURLTemplate at load time.
	JobURLTemplateString string `json:"job_url_template,omitempty"`
	// JobURLTemplate is compiled at load time from JobURLTemplateString. It
	// will be passed a kube.ProwJob and is used to set the URL for the
	// "Details" link on GitHub as well as the link from deck.
	JobURLTemplate *template.Template `json:"-"`

	// ReportTemplateString compiles into ReportTemplate at load time.
	ReportTemplateString string `json:"report_template,omitempty"`
	// ReportTemplate is compiled at load time from ReportTemplateString. It
	// will be passed a kube.ProwJob and can provide an optional blurb below
	// the test failures comment.
	ReportTemplate *template.Template `json:"-"`

	// MaxConcurrency is the maximum number of tests running concurrently that
	// will be allowed by the controller. 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency,omitempty"`

	// MaxGoroutines is the maximum number of goroutines spawned inside the
	// controller to handle tests. Defaults to 20. Needs to be a positive
	// number.
	MaxGoroutines int `json:"max_goroutines,omitempty"`

	// AllowCancellations enables aborting presubmit jobs for commits that
	// have been superseded by newer commits in Github pull requests.
	AllowCancellations bool `json:"allow_cancellations"`
}

// Plank is config for the plank controller.
type Plank struct {
	Controller `json:",inline"`
}

// JenkinsOperator is config for the jenkins-operator controller.
type JenkinsOperator struct {
	Controller `json:",inline"`
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

// Deck holds config for deck.
type Deck struct {
	// TideUpdatePeriodString compiles into TideUpdatePeriod at load time.
	TideUpdatePeriodString string `json:"tide_update_period,omitempty"`
	// TideUpdatePeriod specifies how often Deck will fetch status from Tide. Defaults to 10s.
	TideUpdatePeriod time.Duration `json:"-"`
	// HiddenRepos is a list of orgs and/or repos that should not be displayed by Deck.
	HiddenRepos []string `json:"hidden_repos,omitempty"`
	// ExternalAgentLogs ensures external agents can expose
	// their logs in prow.
	ExternalAgentLogs []ExternalAgentLog `json:"external_agent_logs,omitempty"`
	// Branding of the frontend
	Branding Branding `json:"branding,omitempty"`
}

// ExternalAgentLog ensures an external agent like Jenkins can expose
// its logs in prow.
type ExternalAgentLog struct {
	// Agent is an external prow agent that supports exposing
	// logs via deck.
	Agent string `json:"agent,omitempty"`
	// SelectorString compiles into Selector at load time.
	SelectorString string `json:"selector,omitempty"`
	// Selector can be used in prow deployments where the workload has
	// been sharded between controllers of the same agent. For more info
	// see https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector labels.Selector `json:"-"`
	// URLTemplateString compiles into URLTemplate at load time.
	URLTemplateString string `json:"url_template,omitempty"`
	// URLTemplate is compiled at load time from URLTemplateString. It
	// will be passed a kube.ProwJob and the generated URL should provide
	// logs for the ProwJob.
	URLTemplate *template.Template `json:"-"`
}

// Branding holds branding configuration for deck.
type Branding struct {
	// Logo is the location of the logo that will be loaded in deck.
	Logo string `json:"logo,omitempty"`
	// Favicon is the location of the favicon that will be loaded in deck.
	Favicon string `json:"favicon,omitempty"`
	// BackgroundColor is the color of the background.
	BackgroundColor string `json:"background_color,omitempty"`
	// HeaderColor is the color of the header.
	HeaderColor string `json:"header_color,omitempty"`
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
		if agent == string(kube.KubernetesAgent) {
			if v.Spec == nil {
				return fmt.Errorf("job %s has no spec", name)
			} else {
				if len(v.Spec.Containers) > 1 {
					return fmt.Errorf("job %s has more than one container; see https://github.com/kubernetes/test-infra/pull/5403 for discussion on this restriction", name)
				}
			}
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
		if agent == string(kube.KubernetesAgent) {
			if j.Spec == nil {
				return fmt.Errorf("job %s has no spec", name)
			} else {
				if len(j.Spec.Containers) > 1 {
					return fmt.Errorf("job %s has more than one container; see https://github.com/kubernetes/test-infra/pull/5403 for discussion on this restriction", name)
				}
			}
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
		if agent == string(kube.KubernetesAgent) {
			if p.Spec == nil {
				return fmt.Errorf("job %s has no spec", name)
			} else {
				if len(p.Spec.Containers) > 1 {
					return fmt.Errorf("job %s has more than one container; see https://github.com/kubernetes/test-infra/pull/5403 for discussion on this restriction", name)
				}
			}
		}
		if agent != string(kube.KubernetesAgent) && agent != string(kube.JenkinsAgent) {
			return fmt.Errorf("job %s has invalid agent (%s), it needs to be one of the following: %s %s",
				name, agent, kube.KubernetesAgent, kube.JenkinsAgent)
		}
	}
	// Set the interval on the periodic jobs. It doesn't make sense to do this
	// for child jobs.
	for j, p := range c.Periodics {
		if p.Cron != "" && p.Interval != "" {
			return fmt.Errorf("cron and interval cannot be both set in periodic %s", p.Name)
		} else if p.Cron == "" && p.Interval == "" {
			return fmt.Errorf("cron and interval cannot be both empty in periodic %s", p.Name)
		} else if p.Cron != "" {
			if _, err := cron.Parse(p.Cron); err != nil {
				return fmt.Errorf("invalid cron string %s in periodic %s: %v", p.Cron, p.Name, err)
			}
		} else {
			d, err := time.ParseDuration(c.Periodics[j].Interval)
			if err != nil {
				return fmt.Errorf("cannot parse duration for %s: %v", c.Periodics[j].Name, err)
			}
			c.Periodics[j].interval = d
		}
	}

	if err := ValidateController(&c.Plank.Controller); err != nil {
		return fmt.Errorf("validating plank config: %v", err)
	}

	if err := ValidateController(&c.JenkinsOperator.Controller); err != nil {
		return fmt.Errorf("validating jenkins-operator config: %v", err)
	}

	for i, agentToTmpl := range c.Deck.ExternalAgentLogs {
		urlTemplate, err := template.New(agentToTmpl.Agent).Parse(agentToTmpl.URLTemplateString)
		if err != nil {
			return fmt.Errorf("parsing template for agent %q: %v", agentToTmpl.Agent, err)
		}
		c.Deck.ExternalAgentLogs[i].URLTemplate = urlTemplate
		// we need to validate selectors used by deck since these are not
		// sent to the api server.
		s, err := labels.Parse(c.Deck.ExternalAgentLogs[i].SelectorString)
		if err != nil {
			return fmt.Errorf("error parsing selector %q: %v", c.Deck.ExternalAgentLogs[i].SelectorString, err)
		}
		c.Deck.ExternalAgentLogs[i].Selector = s
	}

	if c.Deck.TideUpdatePeriodString == "" {
		c.Deck.TideUpdatePeriod = time.Second * 10
	} else {
		period, err := time.ParseDuration(c.Deck.TideUpdatePeriodString)
		if err != nil {
			return fmt.Errorf("cannot parse duration for deck.tide_update_period: %v", err)
		}
		c.Deck.TideUpdatePeriod = period
	}

	if c.PushGateway.IntervalString == "" {
		c.PushGateway.Interval = time.Minute
	} else {
		interval, err := time.ParseDuration(c.PushGateway.IntervalString)
		if err != nil {
			return fmt.Errorf("cannot parse duration for push_gateway.interval: %v", err)
		}
		c.PushGateway.Interval = interval
	}

	if c.Sinker.ResyncPeriodString == "" {
		c.Sinker.ResyncPeriod = time.Hour
	} else {
		resyncPeriod, err := time.ParseDuration(c.Sinker.ResyncPeriodString)
		if err != nil {
			return fmt.Errorf("cannot parse duration for sinker.resync_period: %v", err)
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

	if c.Tide.SyncPeriodString == "" {
		c.Tide.SyncPeriod = time.Minute
	} else {
		period, err := time.ParseDuration(c.Tide.SyncPeriodString)
		if err != nil {
			return fmt.Errorf("cannot parse duration for tide.sync_period: %v", err)
		}
		c.Tide.SyncPeriod = period
	}

	if c.Tide.MaxGoroutines == 0 {
		c.Tide.MaxGoroutines = 20
	}
	if c.Tide.MaxGoroutines <= 0 {
		return fmt.Errorf("tide has invalid max_goroutines (%d), it needs to be a positive number", c.Tide.MaxGoroutines)
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

// ValidateController validates the provided controller config.
func ValidateController(c *Controller) error {
	urlTmpl, err := template.New("JobURL").Parse(c.JobURLTemplateString)
	if err != nil {
		return fmt.Errorf("parsing template: %v", err)
	}
	c.JobURLTemplate = urlTmpl

	reportTmpl, err := template.New("Report").Parse(c.ReportTemplateString)
	if err != nil {
		return fmt.Errorf("parsing template: %v", err)
	}
	c.ReportTemplate = reportTmpl
	if c.MaxConcurrency < 0 {
		return fmt.Errorf("controller has invalid max_concurrency (%d), it needs to be a non-negative number", c.MaxConcurrency)
	}
	if c.MaxGoroutines == 0 {
		c.MaxGoroutines = 20
	}
	if c.MaxGoroutines <= 0 {
		return fmt.Errorf("controller has invalid max_goroutines (%d), it needs to be a positive number", c.MaxGoroutines)
	}
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

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
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	cron "gopkg.in/robfig/cron.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	buildapi "github.com/knative/build/pkg/apis/build/v1alpha1"
	pipelinev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

const (
	// DefaultJobTimeout represents the default deadline for a prow job.
	DefaultJobTimeout = 24 * time.Hour

	ProwImplicitGitResource = "PROW_IMPLICIT_GIT_REF"
)

// Config is a read-only snapshot of the config.
type Config struct {
	JobConfig
	ProwConfig
}

// JobConfig is config for all prow jobs
type JobConfig struct {
	// Presets apply to all job types.
	Presets []Preset `json:"presets,omitempty"`
	// Full repo name (such as "kubernetes/kubernetes") -> list of jobs.
	Presubmits  map[string][]Presubmit  `json:"presubmits,omitempty"`
	Postsubmits map[string][]Postsubmit `json:"postsubmits,omitempty"`

	// Periodics are not associated with any repo.
	Periodics []Periodic `json:"periodics,omitempty"`
}

// ProwConfig is config for all prow controllers
type ProwConfig struct {
	Tide             Tide             `json:"tide,omitempty"`
	Plank            Plank            `json:"plank,omitempty"`
	Sinker           Sinker           `json:"sinker,omitempty"`
	Deck             Deck             `json:"deck,omitempty"`
	BranchProtection BranchProtection `json:"branch-protection,omitempty"`
	Gerrit           Gerrit           `json:"gerrit,omitempty"`
	GitHubReporter   GitHubReporter   `json:"github_reporter,omitempty"`
	SlackReporter    *SlackReporter   `json:"slack_reporter,omitempty"`

	// TODO: Move this out of the main config.
	JenkinsOperators []JenkinsOperator `json:"jenkins_operators,omitempty"`

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

	// OwnersDirBlacklist is used to configure regular expressions matching directories
	// to ignore when searching for OWNERS{,_ALIAS} files in a repo.
	OwnersDirBlacklist OwnersDirBlacklist `json:"owners_dir_blacklist,omitempty"`

	// Pub/Sub Subscriptions that we want to listen to
	PubSubSubscriptions PubsubSubscriptions `json:"pubsub_subscriptions,omitempty"`

	// GitHubOptions allows users to control how prow applications display GitHub website links.
	GitHubOptions GitHubOptions `json:"github,omitempty"`

	// StatusErrorLink is the url that will be used for jenkins prowJobs that can't be
	// found, or have another generic issue. The default that will be used if this is not set
	// is: https://github.com/kubernetes/test-infra/issues
	StatusErrorLink string `json:"status_error_link,omitempty"`

	// DefaultJobTimeout this is default deadline for prow jobs. This value is used when
	// no timeout is configured at the job level. This value is set to 24 hours.
	DefaultJobTimeout *metav1.Duration `json:"default_job_timeout,omitempty"`
}

// PresubmitsStatic returns the presubmits in Prows main config.
// **Warning:** This does not return dynamic Presubmits configured
// inside the code repo, hence giving an incomplete view. Use
// `GetPresubmits` instead if possible.
func (c *Config) PresubmitsStatic() map[string][]Presubmit {
	return c.Presubmits
}

// GetPresubmits will return all presumits for the given identifier.
// Once https://github.com/kubernetes/test-infra/issues/13370 is resolved, it will
// also return Presubmits that are versioned inside the tested repo, if that feature
// is enabled.
func (c *Config) GetPresubmits(gc *git.Client, identifier, baseSHA string, headRefs ...string) ([]Presubmit, error) {
	if gc == nil {
		return nil, errors.New("gitClient is nil")
	}
	if identifier == "" {
		return nil, errors.New("no identifier for repo given")
	}
	if baseSHA == "" {
		return nil, errors.New("baseSHA is empty")
	}
	return c.Presubmits[identifier], nil
}

// OwnersDirBlacklist is used to configure regular expressions matching directories
// to ignore when searching for OWNERS{,_ALIAS} files in a repo.
type OwnersDirBlacklist struct {
	// Repos configures a directory blacklist per repo (or org)
	Repos map[string][]string `json:"repos"`
	// Default configures a default blacklist for all repos (or orgs).
	// Some directories like ".git", "_output" and "vendor/.*/OWNERS"
	// are already preconfigured to be blacklisted, and need not be included here.
	Default []string `json:"default"`
	// By default, some directories like ".git", "_output" and "vendor/.*/OWNERS"
	// are preconfigured to be blacklisted.
	// If set, IgnorePreconfiguredDefaults will not add these preconfigured directories
	// to the blacklist.
	IgnorePreconfiguredDefaults bool `json:"ignore_preconfigured_defaults,omitempty"`
}

// DirBlacklist returns regular expressions matching directories to ignore when
// searching for OWNERS{,_ALIAS} files in a repo.
func (ownersDirBlacklist OwnersDirBlacklist) DirBlacklist(org, repo string) (blacklist []string) {
	blacklist = append(blacklist, ownersDirBlacklist.Default...)
	if bl, ok := ownersDirBlacklist.Repos[org]; ok {
		blacklist = append(blacklist, bl...)
	}
	if bl, ok := ownersDirBlacklist.Repos[org+"/"+repo]; ok {
		blacklist = append(blacklist, bl...)
	}

	preconfiguredDefaults := []string{"\\.git$", "_output$", "vendor/.*/.*"}
	if !ownersDirBlacklist.IgnorePreconfiguredDefaults {
		blacklist = append(blacklist, preconfiguredDefaults...)
	}
	return
}

// PushGateway is a prometheus push gateway.
type PushGateway struct {
	// Endpoint is the location of the prometheus pushgateway
	// where prow will push metrics to.
	Endpoint string `json:"endpoint,omitempty"`
	// Interval specifies how often prow will push metrics
	// to the pushgateway. Defaults to 1m.
	Interval *metav1.Duration `json:"interval,omitempty"`
	// ServeMetrics tells if or not the components serve metrics
	ServeMetrics bool `json:"serve_metrics"`
}

// Controller holds configuration applicable to all agent-specific
// prow controllers.
type Controller struct {
	// JobURLTemplateString compiles into JobURLTemplate at load time.
	JobURLTemplateString string `json:"job_url_template,omitempty"`
	// JobURLTemplate is compiled at load time from JobURLTemplateString. It
	// will be passed a prowapi.ProwJob and is used to set the URL for the
	// "Details" link on GitHub as well as the link from deck.
	JobURLTemplate *template.Template `json:"-"`

	// ReportTemplateString compiles into ReportTemplate at load time.
	ReportTemplateString string `json:"report_template,omitempty"`
	// ReportTemplate is compiled at load time from ReportTemplateString. It
	// will be passed a prowapi.ProwJob and can provide an optional blurb below
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
	// have been superseded by newer commits in GitHub pull requests.
	AllowCancellations bool `json:"allow_cancellations,omitempty"`
}

// Plank is config for the plank controller.
type Plank struct {
	Controller `json:",inline"`
	// PodPendingTimeout is after how long the controller will perform a garbage
	// collection on pending pods. Defaults to one day.
	PodPendingTimeout *metav1.Duration `json:"pod_pending_timeout,omitempty"`
	// PodRunningTimeout is after how long the controller will abort a prowjob pod
	// stuck in running state. Defaults to two days.
	PodRunningTimeout *metav1.Duration `json:"pod_running_timeout,omitempty"`
	// DefaultDecorationConfig are defaults for shared fields for ProwJobs
	// that request to have their PodSpecs decorated
	DefaultDecorationConfig *prowapi.DecorationConfig `json:"default_decoration_config,omitempty"`
	// Deprecated, use JobURLPrefixConfig instead
	// JobURLPrefix is the host and path prefix under
	// which job details will be viewable
	// TODO @alvaroaleman: Remove in September 2019
	JobURLPrefix string `json:"job_url_prefix,omitempty"`
	// JobURLPrefixConfig is the host and path prefix under which job details
	// will be viewable. Use `org/repo`, `org` or `*`as key and an url as value
	JobURLPrefixConfig map[string]string `json:"job_url_prefix_config,omitempty"`
}

func (p Plank) GetJobURLPrefix(refs *prowapi.Refs) string {
	if refs == nil {
		return p.JobURLPrefixConfig["*"]
	}
	if p.JobURLPrefixConfig[fmt.Sprintf("%s/%s", refs.Org, refs.Repo)] != "" {
		return p.JobURLPrefixConfig[fmt.Sprintf("%s/%s", refs.Org, refs.Repo)]
	}
	if p.JobURLPrefixConfig[refs.Org] != "" {
		return p.JobURLPrefixConfig[refs.Org]
	}
	return p.JobURLPrefixConfig["*"]
}

// Gerrit is config for the gerrit controller.
type Gerrit struct {
	// TickInterval is how often we do a sync with binded gerrit instance
	TickInterval *metav1.Duration `json:"tick_interval,omitempty"`
	// RateLimit defines how many changes to query per gerrit API call
	// default is 5
	RateLimit int `json:"ratelimit,omitempty"`
}

// JenkinsOperator is config for the jenkins-operator controller.
type JenkinsOperator struct {
	Controller `json:",inline"`
	// LabelSelectorString compiles into LabelSelector at load time.
	// If set, this option needs to match --label-selector used by
	// the desired jenkins-operator. This option is considered
	// invalid when provided with a single jenkins-operator config.
	//
	// For label selector syntax, see below:
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	LabelSelectorString string `json:"label_selector,omitempty"`
	// LabelSelector is used so different jenkins-operator replicas
	// can use their own configuration.
	LabelSelector labels.Selector `json:"-"`
}

// GitHubReporter holds the config for report behavior in github
type GitHubReporter struct {
	// JobTypesToReport is used to determine which type of prowjob
	// should be reported to github
	//
	// defaults to both presubmit and postsubmit jobs.
	JobTypesToReport []prowapi.ProwJobType `json:"job_types_to_report,omitempty"`
}

// Sinker is config for the sinker controller.
type Sinker struct {
	// ResyncPeriod is how often the controller will perform a garbage
	// collection. Defaults to one hour.
	ResyncPeriod *metav1.Duration `json:"resync_period,omitempty"`
	// MaxProwJobAge is how old a ProwJob can be before it is garbage-collected.
	// Defaults to one week.
	MaxProwJobAge *metav1.Duration `json:"max_prowjob_age,omitempty"`
	// MaxPodAge is how old a Pod can be before it is garbage-collected.
	// Defaults to one day.
	MaxPodAge *metav1.Duration `json:"max_pod_age,omitempty"`
}

// LensConfig names a specific lens, and optionally provides some configuration for it.
type LensConfig struct {
	// Name is the name of the lens.
	Name string `json:"name"`
	// Config is some lens-specific configuration. Interpreting it is the responsibility of the
	// lens in question.
	Config json.RawMessage `json:"config"`
}

// LensFileConfig is a single entry under Lenses, describing how to configure a lens
// to read a given set of files.
type LensFileConfig struct {
	// RequiredFiles is a list of regexes of file paths that must all be present for a lens to appear.
	// The list entries are ANDed together, i.e. all of them are required. You can achieve an OR
	// by using a pipe in a regex.
	RequiredFiles []string `json:"required_files"`
	// OptionalFiles is a list of regexes of file paths that will be provided to the lens if they are
	// present, but will not preclude the lens being rendered by their absence.
	// The list entries are ORed together, so if only one of them is present it will be provided to
	// the lens even if the others are not.
	OptionalFiles []string `json:"optional_files,omitempty"`
	// Lens is the lens to use, alongside any lens-specific configuration.
	Lens LensConfig `json:"lens"`
}

// Spyglass holds config for Spyglass
type Spyglass struct {
	// Lenses is a list of lens configurations.
	Lenses []LensFileConfig `json:"lenses,omitempty"`
	// Viewers is deprecated, prefer Lenses instead.
	// Viewers was a map of Regexp strings to viewer names that defines which sets
	// of artifacts need to be consumed by which viewers. It is copied in to Lenses at load time.
	Viewers map[string][]string `json:"viewers,omitempty"`
	// RegexCache is a map of lens regexp strings to their compiled equivalents.
	RegexCache map[string]*regexp.Regexp `json:"-"`
	// SizeLimit is the max size artifact in bytes that Spyglass will attempt to
	// read in entirety. This will only affect viewers attempting to use
	// artifact.ReadAll(). To exclude outlier artifacts, set this limit to
	// expected file size + variance. To include all artifacts with high
	// probability, use 2*maximum observed artifact size.
	SizeLimit int64 `json:"size_limit,omitempty"`
	// GCSBrowserPrefix is used to generate a link to a human-usable GCS browser.
	// If left empty, the link will be not be shown. Otherwise, a GCS path (with no
	// prefix or scheme) will be appended to GCSBrowserPrefix and shown to the user.
	GCSBrowserPrefix string `json:"gcs_browser_prefix,omitempty"`
	// If set, Announcement is used as a Go HTML template string to be displayed at the top of
	// each spyglass page. Using HTML in the template is acceptable.
	// Currently the only variable available is .ArtifactPath, which contains the GCS path for the job artifacts.
	Announcement string `json:"announcement,omitempty"`
	// TestGridConfig is the path to the TestGrid config proto. If the path begins with
	// "gs://" it is assumed to be a GCS reference, otherwise it is read from the local filesystem.
	// If left blank, TestGrid links will not appear.
	TestGridConfig string `json:"testgrid_config,omitempty"`
	// TestGridRoot is the root URL to the TestGrid frontend, e.g. "https://testgrid.k8s.io/".
	// If left blank, TestGrid links will not appear.
	TestGridRoot string `json:"testgrid_root,omitempty"`
}

// RerunAuthConfig holds information about who can trigger job reruns when we allow this feature.
type RerunAuthConfig struct {
	// AllowAnyone, if true, allows anyone to rerun any job. If false, only users listed in
	// AuthorizedUsers can rerun any job.
	AllowAnyone bool `json:"allow_anyone,omitempty"`
	// AuthorizedUsers is a list of GitHub users who can rerun any job. If AllowAnyone is true,
	// AuthorizedUsers should be empty.
	AuthorizedUsers []string `json:"authorized_users,omitempty"`
}

// Deck holds config for deck.
type Deck struct {
	// Spyglass specifies which viewers will be used for which artifacts when viewing a job in Deck
	Spyglass Spyglass `json:"spyglass,omitempty"`
	// TideUpdatePeriod specifies how often Deck will fetch status from Tide. Defaults to 10s.
	TideUpdatePeriod *metav1.Duration `json:"tide_update_period,omitempty"`
	// HiddenRepos is a list of orgs and/or repos that should not be displayed by Deck.
	HiddenRepos []string `json:"hidden_repos,omitempty"`
	// ExternalAgentLogs ensures external agents can expose
	// their logs in prow.
	ExternalAgentLogs []ExternalAgentLog `json:"external_agent_logs,omitempty"`
	// Branding of the frontend
	Branding *Branding `json:"branding,omitempty"`
	// GoogleAnalytics, if specified, include a Google Analytics tracking code on each page.
	GoogleAnalytics string `json:"google_analytics,omitempty"`
	// RerunAuthConfig specifies who will be able to trigger job reruns when we allow this feature.
	// Currently, this does nothing.
	RerunAuthConfig RerunAuthConfig `json:"rerun_auth_config,omitempty"`
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
	// will be passed a prowapi.ProwJob and the generated URL should provide
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

// PubSubSubscriptions maps GCP projects to a list of Topics.
type PubsubSubscriptions map[string][]string

// GitHubOptions allows users to control how prow applications display GitHub website links.
type GitHubOptions struct {
	// LinkURLFromConfig is the string representation of the link_url config parameter.
	// This config parameter allows users to override the default GitHub link url for all plugins.
	// If this option is not set, we assume "https://github.com".
	LinkURLFromConfig string `json:"link_url,omitempty"`

	// LinkURL is the url representation of LinkURLFromConfig. This variable should be used
	// in all places internally.
	LinkURL *url.URL
}

// SlackReporter represents the config for the Slack reporter. The channel can be overridden
// on the job via the .reporter_config.slack.channel property
type SlackReporter struct {
	JobTypesToReport  []prowapi.ProwJobType  `json:"job_types_to_report"`
	JobStatesToReport []prowapi.ProwJobState `json:"job_states_to_report"`
	Channel           string                 `json:"channel"`
	ReportTemplate    string                 `json:"report_template"`
}

func (cfg *SlackReporter) DefaultAndValidate() error {
	// Default ReportTemplate
	if cfg.ReportTemplate == "" {
		cfg.ReportTemplate = `Job {{.Spec.Job}} of type {{.Spec.Type}} ended with state {{.Status.State}}. <{{.Status.URL}}|View logs>`
	}

	if cfg.Channel == "" {
		return errors.New("channel must be set")
	}

	// Validate ReportTemplate
	tmpl, err := template.New("").Parse(cfg.ReportTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}
	if err := tmpl.Execute(&bytes.Buffer{}, &prowapi.ProwJob{}); err != nil {
		return fmt.Errorf("failed to execute report_template: %v", err)
	}

	return nil
}

// Load loads and parses the config at path.
func Load(prowConfig, jobConfig string) (c *Config, err error) {
	// we never want config loading to take down the prow components
	defer func() {
		if r := recover(); r != nil {
			c, err = nil, fmt.Errorf("panic loading config: %v", r)
		}
	}()
	c, err = loadConfig(prowConfig, jobConfig)
	if err != nil {
		return nil, err
	}
	if err := c.finalizeJobConfig(); err != nil {
		return nil, err
	}
	if err := c.validateComponentConfig(); err != nil {
		return nil, err
	}
	if err := c.validateJobConfig(); err != nil {
		return nil, err
	}
	return c, nil
}

// ReadJobConfig reads the JobConfig yaml, but does not expand or validate it.
func ReadJobConfig(jobConfig string) (JobConfig, error) {
	stat, err := os.Stat(jobConfig)
	if err != nil {
		return JobConfig{}, err
	}

	if !stat.IsDir() {
		// still support a single file
		var jc JobConfig
		if err := yamlToConfig(jobConfig, &jc); err != nil {
			return JobConfig{}, err
		}
		return jc, nil
	}

	// we need to ensure all config files have unique basenames,
	// since updateconfig plugin will use basename as a key in the configmap
	uniqueBasenames := sets.String{}

	jc := JobConfig{}
	err = filepath.Walk(jobConfig, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logrus.WithError(err).Errorf("walking path %q.", path)
			// bad file should not stop us from parsing the directory
			return nil
		}

		if strings.HasPrefix(info.Name(), "..") {
			// kubernetes volumes also include files we
			// should not look be looking into for keys
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		base := filepath.Base(path)
		if uniqueBasenames.Has(base) {
			return fmt.Errorf("duplicated basename is not allowed: %s", base)
		}
		uniqueBasenames.Insert(base)

		var subConfig JobConfig
		if err := yamlToConfig(path, &subConfig); err != nil {
			return err
		}
		jc, err = mergeJobConfigs(jc, subConfig)
		return err
	})

	if err != nil {
		return JobConfig{}, err
	}

	return jc, nil
}

// loadConfig loads one or multiple config files and returns a config object.
func loadConfig(prowConfig, jobConfig string) (*Config, error) {
	stat, err := os.Stat(prowConfig)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("prowConfig cannot be a dir - %s", prowConfig)
	}

	var nc Config
	if err := yamlToConfig(prowConfig, &nc); err != nil {
		return nil, err
	}
	if err := parseProwConfig(&nc); err != nil {
		return nil, err
	}

	// TODO(krzyzacy): temporary allow empty jobconfig
	//                 also temporary allow job config in prow config
	if jobConfig == "" {
		return &nc, nil
	}

	jc, err := ReadJobConfig(jobConfig)
	if err != nil {
		return nil, err
	}
	if err := nc.mergeJobConfig(jc); err != nil {
		return nil, err
	}

	return &nc, nil
}

// yamlToConfig converts a yaml file into a Config object
func yamlToConfig(path string, nc interface{}) error {
	b, err := ReadFileMaybeGZIP(path)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", path, err)
	}
	if err := yaml.Unmarshal(b, nc); err != nil {
		return fmt.Errorf("error unmarshaling %s: %v", path, err)
	}
	var jc *JobConfig
	switch v := nc.(type) {
	case *JobConfig:
		jc = v
	case *Config:
		jc = &v.JobConfig
	}
	for rep := range jc.Presubmits {
		var fix func(*Presubmit)
		fix = func(job *Presubmit) {
			job.SourcePath = path
		}
		for i := range jc.Presubmits[rep] {
			fix(&jc.Presubmits[rep][i])
		}
	}
	for rep := range jc.Postsubmits {
		var fix func(*Postsubmit)
		fix = func(job *Postsubmit) {
			job.SourcePath = path
		}
		for i := range jc.Postsubmits[rep] {
			fix(&jc.Postsubmits[rep][i])
		}
	}

	var fix func(*Periodic)
	fix = func(job *Periodic) {
		job.SourcePath = path
	}
	for i := range jc.Periodics {
		fix(&jc.Periodics[i])
	}
	return nil
}

// ReadFileMaybeGZIP wraps ioutil.ReadFile, returning the decompressed contents
// if the file is gzipped, or otherwise the raw contents
func ReadFileMaybeGZIP(path string) ([]byte, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// check if file contains gzip header: http://www.zlib.org/rfc-gzip.html
	if !bytes.HasPrefix(b, []byte("\x1F\x8B")) {
		// go ahead and return the contents if not gzipped
		return b, nil
	}
	// otherwise decode
	gzipReader, err := gzip.NewReader(bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(gzipReader)
}

func (c *Config) mergeJobConfig(jc JobConfig) error {
	m, err := mergeJobConfigs(JobConfig{
		Presets:     c.Presets,
		Presubmits:  c.Presubmits,
		Periodics:   c.Periodics,
		Postsubmits: c.Postsubmits,
	}, jc)
	if err != nil {
		return err
	}
	c.Presets = m.Presets
	c.Presubmits = m.Presubmits
	c.Periodics = m.Periodics
	c.Postsubmits = m.Postsubmits
	return nil
}

// mergeJobConfigs merges two JobConfig together
// It will try to merge:
//	- Presubmits
//	- Postsubmits
// 	- Periodics
//	- PodPresets
func mergeJobConfigs(a, b JobConfig) (JobConfig, error) {
	// Merge everything
	// *** Presets ***
	c := JobConfig{}
	c.Presets = append(a.Presets, b.Presets...)

	// validate no duplicated preset key-value pairs
	validLabels := map[string]bool{}
	for _, preset := range c.Presets {
		for label, val := range preset.Labels {
			pair := label + ":" + val
			if _, ok := validLabels[pair]; ok {
				return JobConfig{}, fmt.Errorf("duplicated preset 'label:value' pair : %s", pair)
			}
			validLabels[pair] = true
		}
	}

	// *** Periodics ***
	c.Periodics = append(a.Periodics, b.Periodics...)

	// *** Presubmits ***
	c.Presubmits = make(map[string][]Presubmit)
	for repo, jobs := range a.Presubmits {
		c.Presubmits[repo] = jobs
	}
	for repo, jobs := range b.Presubmits {
		c.Presubmits[repo] = append(c.Presubmits[repo], jobs...)
	}

	// *** Postsubmits ***
	c.Postsubmits = make(map[string][]Postsubmit)
	for repo, jobs := range a.Postsubmits {
		c.Postsubmits[repo] = jobs
	}
	for repo, jobs := range b.Postsubmits {
		c.Postsubmits[repo] = append(c.Postsubmits[repo], jobs...)
	}
	return c, nil
}

func setPresubmitDecorationDefaults(c *Config, ps *Presubmit) {
	if ps.Decorate {
		ps.DecorationConfig = ps.DecorationConfig.ApplyDefault(c.Plank.DefaultDecorationConfig)
	}
}

func setPostsubmitDecorationDefaults(c *Config, ps *Postsubmit) {
	if ps.Decorate {
		ps.DecorationConfig = ps.DecorationConfig.ApplyDefault(c.Plank.DefaultDecorationConfig)
	}
}

func setPeriodicDecorationDefaults(c *Config, ps *Periodic) {
	if ps.Decorate {
		ps.DecorationConfig = ps.DecorationConfig.ApplyDefault(c.Plank.DefaultDecorationConfig)
	}
}

// finalizeJobConfig mutates and fixes entries for jobspecs
func (c *Config) finalizeJobConfig() error {
	if c.decorationRequested() {
		if c.Plank.DefaultDecorationConfig == nil {
			return errors.New("no default decoration config provided for plank")
		}
		if c.Plank.DefaultDecorationConfig.UtilityImages == nil {
			return errors.New("no default decoration image pull specs provided for plank")
		}
		if c.Plank.DefaultDecorationConfig.GCSConfiguration == nil {
			return errors.New("no default GCS decoration config provided for plank")
		}
		if c.Plank.DefaultDecorationConfig.GCSCredentialsSecret == "" {
			return errors.New("no default GCS credentials secret provided for plank")
		}

		for _, vs := range c.Presubmits {
			for i := range vs {
				setPresubmitDecorationDefaults(c, &vs[i])
			}
		}

		for _, js := range c.Postsubmits {
			for i := range js {
				setPostsubmitDecorationDefaults(c, &js[i])
			}
		}

		for i := range c.Periodics {
			setPeriodicDecorationDefaults(c, &c.Periodics[i])
		}
	}

	// Ensure that regexes are valid and set defaults.
	for _, vs := range c.Presubmits {
		c.defaultPresubmitFields(vs)
		if err := SetPresubmitRegexes(vs); err != nil {
			return fmt.Errorf("could not set regex: %v", err)
		}
	}
	for _, js := range c.Postsubmits {
		c.defaultPostsubmitFields(js)
		if err := SetPostsubmitRegexes(js); err != nil {
			return fmt.Errorf("could not set regex: %v", err)
		}
	}

	c.defaultPeriodicFields(c.Periodics)

	for _, v := range c.AllPresubmits(nil) {
		if err := resolvePresets(v.Name, v.Labels, v.Spec, v.BuildSpec, c.Presets); err != nil {
			return err
		}
	}

	for _, v := range c.AllPostsubmits(nil) {
		if err := resolvePresets(v.Name, v.Labels, v.Spec, v.BuildSpec, c.Presets); err != nil {
			return err
		}
	}

	for _, v := range c.AllPeriodics() {
		if err := resolvePresets(v.Name, v.Labels, v.Spec, v.BuildSpec, c.Presets); err != nil {
			return err
		}
	}

	return nil
}

// validateComponentConfig validates the infrastructure component configuration
func (c *Config) validateComponentConfig() error {
	if c.Plank.JobURLPrefix != "" && c.Plank.JobURLPrefixConfig["*"] != "" {
		return errors.New(`Planks job_url_prefix must be unset when job_url_prefix_config["*"] is set. The former is deprecated, use the latter`)
	}
	for k, v := range c.Plank.JobURLPrefixConfig {
		if _, err := url.Parse(v); err != nil {
			return fmt.Errorf(`Invalid value for Planks job_url_prefix_config["%s"]: %v`, k, err)
		}
	}

	if c.SlackReporter != nil {
		if err := c.SlackReporter.DefaultAndValidate(); err != nil {
			return fmt.Errorf("failed to validate slackreporter config: %v", err)
		}
	}
	return nil
}

var jobNameRegex = regexp.MustCompile(`^[A-Za-z0-9-._]+$`)

func validateJobBase(v JobBase, jobType prowapi.ProwJobType, podNamespace string) error {
	if !jobNameRegex.MatchString(v.Name) {
		return fmt.Errorf("name: must match regex %q", jobNameRegex.String())
	}
	// Ensure max_concurrency is non-negative.
	if v.MaxConcurrency < 0 {
		return fmt.Errorf("max_concurrency: %d must be a non-negative number", v.MaxConcurrency)
	}
	if err := validateAgent(v, podNamespace); err != nil {
		return err
	}
	if err := validatePodSpec(jobType, v.Spec); err != nil {
		return err
	}
	if err := ValidatePipelineRunSpec(jobType, v.ExtraRefs, v.PipelineRunSpec); err != nil {
		return err
	}
	if err := validateLabels(v.Labels); err != nil {
		return err
	}
	if v.Spec == nil || len(v.Spec.Containers) == 0 {
		return nil // knative-build and jenkins jobs have no spec
	}
	if v.RerunPermissions != nil && v.RerunPermissions.AllowAnyone && (len(v.RerunPermissions.GitHubUsers) > 0 || len(v.RerunPermissions.GitHubTeams) > 0) {
		return errors.New("allow anyone is set to true and permitted users or groups are specified")
	}
	return validateDecoration(v.Spec.Containers[0], v.DecorationConfig)
}

// validateJobConfig validates if all the jobspecs/presets are valid
// if you are mutating the jobs, please add it to finalizeJobConfig above
func (c *Config) validateJobConfig() error {
	type orgRepoJobName struct {
		orgRepo, jobName string
	}

	// Validate presubmits.
	// Checking that no duplicate job in prow config exists on the same org / repo / branch.
	validPresubmits := map[orgRepoJobName][]Presubmit{}
	for repo, jobs := range c.Presubmits {
		for _, job := range jobs {
			repoJobName := orgRepoJobName{repo, job.Name}
			for _, existingJob := range validPresubmits[repoJobName] {
				if existingJob.Brancher.Intersects(job.Brancher) {
					return fmt.Errorf("duplicated presubmit job: %s", job.Name)
				}
			}
			validPresubmits[repoJobName] = append(validPresubmits[repoJobName], job)
		}
	}

	for _, v := range c.AllPresubmits(nil) {
		if err := validateJobBase(v.JobBase, prowapi.PresubmitJob, c.PodNamespace); err != nil {
			return fmt.Errorf("invalid presubmit job %s: %v", v.Name, err)
		}
		if err := validateTriggering(v); err != nil {
			return err
		}
	}

	// Validate postsubmits.
	// Checking that no duplicate job in prow config exists on the same org / repo / branch.
	validPostsubmits := map[orgRepoJobName][]Postsubmit{}
	for repo, jobs := range c.Postsubmits {
		for _, job := range jobs {
			repoJobName := orgRepoJobName{repo, job.Name}
			for _, existingJob := range validPostsubmits[repoJobName] {
				if existingJob.Brancher.Intersects(job.Brancher) {
					return fmt.Errorf("duplicated postsubmit job: %s", job.Name)
				}
			}
			validPostsubmits[repoJobName] = append(validPostsubmits[repoJobName], job)
		}
	}

	for _, j := range c.AllPostsubmits(nil) {
		if err := validateJobBase(j.JobBase, prowapi.PostsubmitJob, c.PodNamespace); err != nil {
			return fmt.Errorf("invalid postsubmit job %s: %v", j.Name, err)
		}
	}

	// validate no duplicated periodics
	validPeriodics := sets.NewString()
	// Ensure that the periodic durations are valid and specs exist.
	for _, p := range c.AllPeriodics() {
		if validPeriodics.Has(p.Name) {
			return fmt.Errorf("duplicated periodic job : %s", p.Name)
		}
		validPeriodics.Insert(p.Name)
		if err := validateJobBase(p.JobBase, prowapi.PeriodicJob, c.PodNamespace); err != nil {
			return fmt.Errorf("invalid periodic job %s: %v", p.Name, err)
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

	return nil
}

// DefaultConfigPath will be used if a --config-path is unset
const DefaultConfigPath = "/etc/config/config.yaml"

// ConfigPath returns the value for the component's configPath if provided
// explicitly or default otherwise.
func ConfigPath(value string) string {

	if value != "" {
		return value
	}
	logrus.Warningf("defaulting to %s until 15 July 2019, please migrate", DefaultConfigPath)
	return DefaultConfigPath
}

func parseProwConfig(c *Config) error {
	if err := ValidateController(&c.Plank.Controller); err != nil {
		return fmt.Errorf("validating plank config: %v", err)
	}

	if c.Plank.PodPendingTimeout == nil {
		c.Plank.PodPendingTimeout = &metav1.Duration{Duration: 24 * time.Hour}
	}

	if c.Plank.PodRunningTimeout == nil {
		c.Plank.PodRunningTimeout = &metav1.Duration{Duration: 48 * time.Hour}
	}

	if c.Gerrit.TickInterval == nil {
		c.Gerrit.TickInterval = &metav1.Duration{Duration: time.Minute}
	}

	if c.Gerrit.RateLimit == 0 {
		c.Gerrit.RateLimit = 5
	}

	if len(c.GitHubReporter.JobTypesToReport) == 0 {
		c.GitHubReporter.JobTypesToReport = append(c.GitHubReporter.JobTypesToReport, prowapi.PresubmitJob, prowapi.PostsubmitJob)
	}

	// validate entries are valid job types
	// Currently only presubmit and postsubmit can be reported to github
	for _, t := range c.GitHubReporter.JobTypesToReport {
		if t != prowapi.PresubmitJob && t != prowapi.PostsubmitJob {
			return fmt.Errorf("invalid job_types_to_report: %v", t)
		}
	}

	for i := range c.JenkinsOperators {
		if err := ValidateController(&c.JenkinsOperators[i].Controller); err != nil {
			return fmt.Errorf("validating jenkins_operators config: %v", err)
		}
		sel, err := labels.Parse(c.JenkinsOperators[i].LabelSelectorString)
		if err != nil {
			return fmt.Errorf("invalid jenkins_operators.label_selector option: %v", err)
		}
		c.JenkinsOperators[i].LabelSelector = sel
		// TODO: Invalidate overlapping selectors more
		if len(c.JenkinsOperators) > 1 && c.JenkinsOperators[i].LabelSelectorString == "" {
			return errors.New("selector overlap: cannot use an empty label_selector with multiple selectors")
		}
		if len(c.JenkinsOperators) == 1 && c.JenkinsOperators[0].LabelSelectorString != "" {
			return errors.New("label_selector is invalid when used for a single jenkins-operator")
		}
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

	if c.Deck.TideUpdatePeriod == nil {
		c.Deck.TideUpdatePeriod = &metav1.Duration{Duration: time.Second * 10}
	}

	if c.Deck.Spyglass.SizeLimit == 0 {
		c.Deck.Spyglass.SizeLimit = 100e6
	} else if c.Deck.Spyglass.SizeLimit <= 0 {
		return fmt.Errorf("invalid value for deck.spyglass.size_limit, must be >=0")
	}

	// If a whitelist is specified, the user probably does not intend for anyone to be able
	// to rerun any job.
	if c.Deck.RerunAuthConfig.AllowAnyone && c.Deck.RerunAuthConfig.AuthorizedUsers != nil {
		return fmt.Errorf("allow_anyone is set to true and whitelist is specified.")
	}

	// Migrate the old `viewers` format to the new `lenses` format.
	var oldLenses []LensFileConfig
	for regex, viewers := range c.Deck.Spyglass.Viewers {
		for _, viewer := range viewers {
			lfc := LensFileConfig{
				RequiredFiles: []string{regex},
				Lens: LensConfig{
					Name: viewer,
				},
			}
			oldLenses = append(oldLenses, lfc)
		}
	}
	// Ensure the ordering is stable, because these are referenced by index elsewhere.
	sort.Slice(oldLenses, func(i, j int) bool { return oldLenses[i].Lens.Name < oldLenses[j].Lens.Name })
	c.Deck.Spyglass.Lenses = append(c.Deck.Spyglass.Lenses, oldLenses...)

	// Parse and cache all our regexes upfront
	c.Deck.Spyglass.RegexCache = make(map[string]*regexp.Regexp)
	for _, lens := range c.Deck.Spyglass.Lenses {
		toCompile := append(lens.OptionalFiles, lens.RequiredFiles...)
		for _, v := range toCompile {
			if _, ok := c.Deck.Spyglass.RegexCache[v]; ok {
				continue
			}
			r, err := regexp.Compile(v)
			if err != nil {
				return fmt.Errorf("cannot compile regexp %q, err: %v", v, err)
			}
			c.Deck.Spyglass.RegexCache[v] = r
		}
	}

	// Map old viewer names to the new ones for backwards compatibility.
	// TODO(Katharine, #10274): remove this, eventually.
	oldViewers := map[string]string{
		"build-log-viewer": "buildlog",
		"metadata-viewer":  "metadata",
		"junit-viewer":     "junit",
	}

	for re, viewers := range c.Deck.Spyglass.Viewers {
		for i, v := range viewers {
			if rename, ok := oldViewers[v]; ok {
				c.Deck.Spyglass.Viewers[re][i] = rename
			}
		}
	}

	if c.PushGateway.Interval == nil {
		c.PushGateway.Interval = &metav1.Duration{Duration: time.Minute}
	}

	if c.Sinker.ResyncPeriod == nil {
		c.Sinker.ResyncPeriod = &metav1.Duration{Duration: time.Hour}
	}

	if c.Sinker.MaxProwJobAge == nil {
		c.Sinker.MaxProwJobAge = &metav1.Duration{Duration: 7 * 24 * time.Hour}
	}

	if c.Sinker.MaxPodAge == nil {
		c.Sinker.MaxPodAge = &metav1.Duration{Duration: 24 * time.Hour}
	}

	if c.Tide.SyncPeriod == nil {
		c.Tide.SyncPeriod = &metav1.Duration{Duration: time.Minute}
	}

	if c.Tide.StatusUpdatePeriod == nil {
		c.Tide.StatusUpdatePeriod = c.Tide.SyncPeriod
	}

	if c.Tide.MaxGoroutines == 0 {
		c.Tide.MaxGoroutines = 20
	}
	if c.Tide.MaxGoroutines <= 0 {
		return fmt.Errorf("tide has invalid max_goroutines (%d), it needs to be a positive number", c.Tide.MaxGoroutines)
	}

	for name, method := range c.Tide.MergeType {
		if method != github.MergeMerge &&
			method != github.MergeRebase &&
			method != github.MergeSquash {
			return fmt.Errorf("merge type %q for %s is not a valid type", method, name)
		}
	}

	for name, templates := range c.Tide.MergeTemplate {
		if templates.TitleTemplate != "" {
			titleTemplate, err := template.New("CommitTitle").Parse(templates.TitleTemplate)

			if err != nil {
				return fmt.Errorf("parsing template for commit title: %v", err)
			}

			templates.Title = titleTemplate
		}

		if templates.BodyTemplate != "" {
			bodyTemplate, err := template.New("CommitBody").Parse(templates.BodyTemplate)

			if err != nil {
				return fmt.Errorf("parsing template for commit body: %v", err)
			}

			templates.Body = bodyTemplate
		}

		c.Tide.MergeTemplate[name] = templates
	}

	for i, tq := range c.Tide.Queries {
		if err := tq.Validate(); err != nil {
			return fmt.Errorf("tide query (index %d) is invalid: %v", i, err)
		}
	}

	if c.ProwJobNamespace == "" {
		c.ProwJobNamespace = "default"
	}
	if c.PodNamespace == "" {
		c.PodNamespace = "default"
	}

	if c.Plank.JobURLPrefixConfig == nil {
		c.Plank.JobURLPrefixConfig = map[string]string{}
	}
	if c.Plank.JobURLPrefix != "" && c.Plank.JobURLPrefixConfig["*"] == "" {
		c.Plank.JobURLPrefixConfig["*"] = c.Plank.JobURLPrefix
		// Set JobURLPrefix to an empty string to indicate we've moved
		// it to JobURLPrefixConfig["*"] without overwriting the latter
		// so validation succeeds
		c.Plank.JobURLPrefix = ""
	}

	if c.GitHubOptions.LinkURLFromConfig == "" {
		c.GitHubOptions.LinkURLFromConfig = "https://github.com"
	}
	linkURL, err := url.Parse(c.GitHubOptions.LinkURLFromConfig)
	if err != nil {
		return fmt.Errorf("unable to parse github.link_url, might not be a valid url: %v", err)
	}
	c.GitHubOptions.LinkURL = linkURL

	if c.StatusErrorLink == "" {
		c.StatusErrorLink = "https://github.com/kubernetes/test-infra/issues"
	}

	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	lvl, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(lvl)

	// Avoid using a job timeout of infinity by setting the default value to 24 hours
	if c.DefaultJobTimeout == nil {
		c.DefaultJobTimeout = &metav1.Duration{Duration: DefaultJobTimeout}
	}

	return nil
}

func (c *JobConfig) decorationRequested() bool {
	for _, vs := range c.Presubmits {
		for i := range vs {
			if vs[i].Decorate {
				return true
			}
		}
	}

	for _, js := range c.Postsubmits {
		for i := range js {
			if js[i].Decorate {
				return true
			}
		}
	}

	for i := range c.Periodics {
		if c.Periodics[i].Decorate {
			return true
		}
	}

	return false
}

func validateLabels(labels map[string]string) error {
	for label, value := range labels {
		for _, prowLabel := range decorate.Labels() {
			if label == prowLabel {
				return fmt.Errorf("label %s is reserved for decoration", label)
			}
		}
		if errs := validation.IsQualifiedName(label); len(errs) != 0 {
			return fmt.Errorf("invalid label %s: %v", label, errs)
		}
		if errs := validation.IsValidLabelValue(labels[label]); len(errs) != 0 {
			return fmt.Errorf("label %s has invalid value %s: %v", label, value, errs)
		}
	}
	return nil
}

func validateAgent(v JobBase, podNamespace string) error {
	k := string(prowapi.KubernetesAgent)
	b := string(prowapi.KnativeBuildAgent)
	j := string(prowapi.JenkinsAgent)
	p := string(prowapi.TektonAgent)
	agents := sets.NewString(k, b, j, p)
	agent := v.Agent
	switch {
	case !agents.Has(agent):
		logrus.Warningf("agent %s is unknown and cannot be validated: use at your own risk", agent)
		return nil
	case v.Spec != nil && agent != k:
		return fmt.Errorf("job specs require agent: %s (found %q)", k, agent)
	case agent == k && v.Spec == nil:
		return errors.New("kubernetes jobs require a spec")
	case v.BuildSpec != nil && agent != b:
		return fmt.Errorf("job build_specs require agent: %s (found %q)", b, agent)
	case agent == b && v.BuildSpec == nil:
		return errors.New("knative-build jobs require a build_spec")
	case v.PipelineRunSpec != nil && agent != p:
		return fmt.Errorf("job pipeline_run_spec require agent: %s (found %q)", p, agent)
	case agent == p && v.PipelineRunSpec == nil:
		return fmt.Errorf("agent: %s jobs require a pipeline_run_spec", p)
	case v.DecorationConfig != nil && agent != k && agent != b:
		// TODO(fejta): only source decoration supported...
		return fmt.Errorf("decoration requires agent: %s or %s (found %q)", k, b, agent)
	case v.ErrorOnEviction && agent != k:
		return fmt.Errorf("error_on_eviction only applies to agent: %s (found %q)", k, agent)
	case v.Namespace == nil || *v.Namespace == "":
		return fmt.Errorf("failed to default namespace")
	case *v.Namespace != podNamespace && agent != b && agent != p:
		// TODO(fejta): update plank to allow this (depends on client change)
		return fmt.Errorf("namespace customization requires agent: %s or %s (found %q)", b, p, agent)
	}
	return nil
}

func validateDecoration(container v1.Container, config *prowapi.DecorationConfig) error {
	if config == nil {
		return nil
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid decoration config: %v", err)
	}
	var args []string
	args = append(append(args, container.Command...), container.Args...)
	if len(args) == 0 || args[0] == "" {
		return errors.New("decorated job containers must specify command and/or args")
	}
	return nil
}

func resolvePresets(name string, labels map[string]string, spec *v1.PodSpec, buildSpec *buildapi.BuildSpec, presets []Preset) error {
	for _, preset := range presets {
		if spec != nil {
			if err := mergePreset(preset, labels, spec.Containers, &spec.Volumes); err != nil {
				return fmt.Errorf("job %s failed to merge presets for podspec: %v", name, err)
			}
		}

		if buildSpec != nil {
			if err := mergePreset(preset, labels, buildSpec.Steps, &buildSpec.Volumes); err != nil {
				return fmt.Errorf("job %s failed to merge presets for buildspec: %v", name, err)
			}
		}
	}

	return nil
}

var ReProwExtraRef = regexp.MustCompile(`PROW_EXTRA_GIT_REF_(\d+)`)

func ValidatePipelineRunSpec(jobType prowapi.ProwJobType, extraRefs []prowapi.Refs, spec *pipelinev1alpha1.PipelineRunSpec) error {
	if spec == nil {
		return nil
	}
	// Validate that that the refs match what is requested by the job.
	// The implicit git ref is optional to use, but any extra refs specified must
	// be used or removed. (Specifying an unused extra ref must always be
	// unintentional so we want to warn the user.)
	extraIndexes := sets.NewInt()
	for _, resource := range spec.Resources {
		// Validate that periodic jobs don't request an implicit git ref
		if jobType == prowapi.PeriodicJob && resource.ResourceRef.Name == ProwImplicitGitResource {
			return fmt.Errorf("periodic jobs do not have an implicit git ref to replace %s", ProwImplicitGitResource)
		}

		match := ReProwExtraRef.FindStringSubmatch(resource.ResourceRef.Name)
		if len(match) != 2 {
			continue
		}
		if len(match[1]) > 1 && match[1][0] == '0' {
			return fmt.Errorf("resource %q: leading zeros are not allowed in PROW_EXTRA_GIT_REF_* indexes", resource.Name)
		}
		i, _ := strconv.Atoi(match[1]) // This can't error based on the regexp.
		extraIndexes.Insert(i)
	}
	for i := range extraRefs {
		if !extraIndexes.Has(i) {
			return fmt.Errorf("extra_refs[%d] is not used; some resource must reference PROW_EXTRA_GIT_REF_%d", i, i)
		}
	}
	if len(extraRefs) != extraIndexes.Len() {
		strs := make([]string, 0, extraIndexes.Len())
		for i := range extraIndexes {
			strs = append(strs, strconv.Itoa(i))
		}
		return fmt.Errorf(
			"%d extra_refs are specified, but the following PROW_EXTRA_GIT_REF_* indexes are used: %s.",
			len(extraRefs),
			strings.Join(strs, ", "),
		)
	}
	return nil
}

func validatePodSpec(jobType prowapi.ProwJobType, spec *v1.PodSpec) error {
	if spec == nil {
		return nil
	}

	if len(spec.InitContainers) != 0 {
		return errors.New("pod spec may not use init containers")
	}

	if n := len(spec.Containers); n != 1 {
		return fmt.Errorf("pod spec must specify exactly 1 container, found: %d", n)
	}

	for _, env := range spec.Containers[0].Env {
		for _, prowEnv := range downwardapi.EnvForType(jobType) {
			if env.Name == prowEnv {
				// TODO(fejta): consider allowing this
				return fmt.Errorf("env %s is reserved", env.Name)
			}
		}
	}

	for _, mount := range spec.Containers[0].VolumeMounts {
		for _, prowMount := range decorate.VolumeMounts() {
			if mount.Name == prowMount {
				return fmt.Errorf("volumeMount name %s is reserved for decoration", prowMount)
			}
		}
		for _, prowMountPath := range decorate.VolumeMountPaths() {
			if strings.HasPrefix(mount.MountPath, prowMountPath) || strings.HasPrefix(prowMountPath, mount.MountPath) {
				return fmt.Errorf("mount %s at %s conflicts with decoration mount at %s", mount.Name, mount.MountPath, prowMountPath)
			}
		}
	}

	for _, volume := range spec.Volumes {
		for _, prowVolume := range decorate.VolumeMounts() {
			if volume.Name == prowVolume {
				return fmt.Errorf("volume %s is a reserved for decoration", volume.Name)
			}
		}
	}

	return nil
}

func validateTriggering(job Presubmit) error {
	if job.AlwaysRun && job.RunIfChanged != "" {
		return fmt.Errorf("job %s is set to always run but also declares run_if_changed targets, which are mutually exclusive", job.Name)
	}

	if !job.SkipReport && job.Context == "" {
		return fmt.Errorf("job %s is set to report but has no context configured", job.Name)
	}

	if (job.Trigger != "" && job.RerunCommand == "") || (job.Trigger == "" && job.RerunCommand != "") {
		return fmt.Errorf("Either both of job.Trigger and job.RerunCommand must be set, wasnt the case for job %q", job.Name)
	}

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

// DefaultTriggerFor returns the default regexp string used to match comments
// that should trigger the job with this name.
func DefaultTriggerFor(name string) string {
	return fmt.Sprintf(`(?m)^/test( | .* )%s,?($|\s.*)`, name)
}

// DefaultRerunCommandFor returns the default rerun command for the job with
// this name.
func DefaultRerunCommandFor(name string) string {
	return fmt.Sprintf("/test %s", name)
}

// defaultJobBase configures common parameters, currently Agent and Namespace.
func (c *ProwConfig) defaultJobBase(base *JobBase) {
	if base.Agent == "" { // Use kubernetes by default
		base.Agent = string(prowapi.KubernetesAgent)
	}
	if base.Namespace == nil || *base.Namespace == "" {
		s := c.PodNamespace
		base.Namespace = &s
	}
	if base.Cluster == "" {
		base.Cluster = kube.DefaultClusterAlias
	}
}

func (c *ProwConfig) defaultPresubmitFields(js []Presubmit) {
	for i := range js {
		c.defaultJobBase(&js[i].JobBase)
		if js[i].Context == "" {
			js[i].Context = js[i].Name
		}
		// Default the values of Trigger and RerunCommand if both fields are
		// specified. Otherwise let validation fail as both or neither should have
		// been specified.
		if js[i].Trigger == "" && js[i].RerunCommand == "" {
			js[i].Trigger = DefaultTriggerFor(js[i].Name)
			js[i].RerunCommand = DefaultRerunCommandFor(js[i].Name)
		}
	}
}

func (c *ProwConfig) defaultPostsubmitFields(js []Postsubmit) {
	for i := range js {
		c.defaultJobBase(&js[i].JobBase)
		if js[i].Context == "" {
			js[i].Context = js[i].Name
		}
	}
}

func (c *ProwConfig) defaultPeriodicFields(js []Periodic) {
	for i := range js {
		c.defaultJobBase(&js[i].JobBase)
	}
}

// SetPresubmitRegexes compiles and validates all the regular expressions for
// the provided presubmits.
func SetPresubmitRegexes(js []Presubmit) error {
	for i, j := range js {
		if re, err := regexp.Compile(j.Trigger); err == nil {
			js[i].re = re
		} else {
			return fmt.Errorf("could not compile trigger regex for %s: %v", j.Name, err)
		}
		if !js[i].re.MatchString(j.RerunCommand) {
			return fmt.Errorf("for job %s, rerun command \"%s\" does not match trigger \"%s\"", j.Name, j.RerunCommand, j.Trigger)
		}
		b, err := setBrancherRegexes(j.Brancher)
		if err != nil {
			return fmt.Errorf("could not set branch regexes for %s: %v", j.Name, err)
		}
		js[i].Brancher = b

		c, err := setChangeRegexes(j.RegexpChangeMatcher)
		if err != nil {
			return fmt.Errorf("could not set change regexes for %s: %v", j.Name, err)
		}
		js[i].RegexpChangeMatcher = c
	}
	return nil
}

// setBrancherRegexes compiles and validates all the regular expressions for
// the provided branch specifiers.
func setBrancherRegexes(br Brancher) (Brancher, error) {
	if len(br.Branches) > 0 {
		if re, err := regexp.Compile(strings.Join(br.Branches, `|`)); err == nil {
			br.re = re
		} else {
			return br, fmt.Errorf("could not compile positive branch regex: %v", err)
		}
	}
	if len(br.SkipBranches) > 0 {
		if re, err := regexp.Compile(strings.Join(br.SkipBranches, `|`)); err == nil {
			br.reSkip = re
		} else {
			return br, fmt.Errorf("could not compile negative branch regex: %v", err)
		}
	}
	return br, nil
}

func setChangeRegexes(cm RegexpChangeMatcher) (RegexpChangeMatcher, error) {
	if cm.RunIfChanged != "" {
		re, err := regexp.Compile(cm.RunIfChanged)
		if err != nil {
			return cm, fmt.Errorf("could not compile run_if_changed regex: %v", err)
		}
		cm.reChanges = re
	}
	return cm, nil
}

// SetPostsubmitRegexes compiles and validates all the regular expressions for
// the provided postsubmits.
func SetPostsubmitRegexes(ps []Postsubmit) error {
	for i, j := range ps {
		b, err := setBrancherRegexes(j.Brancher)
		if err != nil {
			return fmt.Errorf("could not set branch regexes for %s: %v", j.Name, err)
		}
		ps[i].Brancher = b
		c, err := setChangeRegexes(j.RegexpChangeMatcher)
		if err != nil {
			return fmt.Errorf("could not set change regexes for %s: %v", j.Name, err)
		}
		ps[i].RegexpChangeMatcher = c
	}
	return nil
}

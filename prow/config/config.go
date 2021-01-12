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
	"sync"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	pipelinev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"gopkg.in/robfig/cron.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	gerrit "k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
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
	// .PresubmitsStatic contains the presubmits in Prows main config.
	// **Warning:** This does not return dynamic Presubmits configured
	// inside the code repo, hence giving an incomplete view. Use
	// `GetPresubmits` instead if possible.
	PresubmitsStatic map[string][]Presubmit `json:"presubmits,omitempty"`
	// .PostsubmitsStatic contains the Postsubmits in Prows main config.
	// **Warning:** This does not return dynamic postsubmits configured
	// inside the code repo, hence giving an incomplete view. Use
	// `GetPostsubmits` instead if possible.
	PostsubmitsStatic map[string][]Postsubmit `json:"postsubmits,omitempty"`

	// Periodics are not associated with any repo.
	Periodics []Periodic `json:"periodics,omitempty"`

	// AllRepos contains all Repos that have one or more jobs configured or
	// for which a tide query is configured.
	AllRepos sets.String `json:"-"`

	// ProwYAMLGetter is the function to get a ProwYAML. Tests should
	// provide their own implementation.
	ProwYAMLGetter ProwYAMLGetter `json:"-"`

	// DecorateAllJobs determines whether all jobs are decorated by default
	DecorateAllJobs bool `json:"decorate_all_jobs,omitempty"`
}

// ProwConfig is config for all prow controllers
type ProwConfig struct {
	Tide             Tide             `json:"tide,omitempty"`
	Plank            Plank            `json:"plank,omitempty"`
	Sinker           Sinker           `json:"sinker,omitempty"`
	Deck             Deck             `json:"deck,omitempty"`
	BranchProtection BranchProtection `json:"branch-protection"`
	Gerrit           Gerrit           `json:"gerrit"`
	GitHubReporter   GitHubReporter   `json:"github_reporter"`
	// Deprecated: this option will be removed in May 2020.
	SlackReporter        *SlackReporter       `json:"slack_reporter,omitempty"`
	SlackReporterConfigs SlackReporterConfigs `json:"slack_reporter_configs,omitempty"`
	InRepoConfig         InRepoConfig         `json:"in_repo_config"`

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

	// ManagedWebhooks contains information about all github repositories and organizations which are using
	// non-global Hmac token.
	ManagedWebhooks ManagedWebhooks `json:"managed_webhooks,omitempty"`
}

type InRepoConfig struct {
	// Enabled describes whether InRepoConfig is enabled for a given repository. This can
	// be set globally, per org or per repo using '*', 'org' or 'org/repo' as key. The
	// narrowest match always takes precedence.
	Enabled map[string]*bool `json:"enabled,omitempty"`
	// AllowedClusters is a list of allowed clusternames that can be used for jobs on
	// a given repo. All clusters that are allowed for the specific repo, its org or
	// globally can be used.
	AllowedClusters map[string][]string `json:"allowed_clusters,omitempty"`
}

// InRepoConfigEnabled returns whether InRepoConfig is enabled for a given repository.
func (c *Config) InRepoConfigEnabled(identifier string) bool {
	if c.InRepoConfig.Enabled[identifier] != nil {
		return *c.InRepoConfig.Enabled[identifier]
	}
	identifierSlashSplit := strings.Split(identifier, "/")
	if len(identifierSlashSplit) == 2 && c.InRepoConfig.Enabled[identifierSlashSplit[0]] != nil {
		return *c.InRepoConfig.Enabled[identifierSlashSplit[0]]
	}
	if c.InRepoConfig.Enabled["*"] != nil {
		return *c.InRepoConfig.Enabled["*"]
	}
	return false
}

// InRepoConfigAllowsCluster determines if a given cluster may be used for a given repository
func (c *Config) InRepoConfigAllowsCluster(clusterName, repoIdentifier string) bool {
	for _, allowedCluster := range c.InRepoConfig.AllowedClusters[repoIdentifier] {
		if allowedCluster == clusterName {
			return true
		}
	}

	identifierSlashSplit := strings.Split(repoIdentifier, "/")
	if len(identifierSlashSplit) == 2 {
		for _, allowedCluster := range c.InRepoConfig.AllowedClusters[identifierSlashSplit[0]] {
			if allowedCluster == clusterName {
				return true
			}
		}
	}

	for _, allowedCluster := range c.InRepoConfig.AllowedClusters["*"] {
		if allowedCluster == clusterName {
			return true
		}
	}
	return false
}

// RefGetter is used to retrieve a Git Reference. Its purpose is
// to be able to defer calling out to GitHub in the context of
// inrepoconfig to make sure its only done when we actually need
// to have that info.
type RefGetter = func() (string, error)

type refGetterForGitHubPullRequestClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
}

// NewRefGetterForGitHubPullRequest returns a brand new RefGetterForGitHubPullRequest
func NewRefGetterForGitHubPullRequest(ghc refGetterForGitHubPullRequestClient, org, repo string, number int) *RefGetterForGitHubPullRequest {
	return &RefGetterForGitHubPullRequest{
		ghc:    ghc,
		org:    org,
		repo:   repo,
		number: number,
		lock:   &sync.Mutex{},
	}
}

// RefGetterForGitHubPullRequest is used to get the Presubmits for a GitHub PullRequest
// when that PullRequest wasn't fetched yet. It will only fetch it if someone calls
// its .PullRequest() func. It is threadsafe.
type RefGetterForGitHubPullRequest struct {
	ghc     refGetterForGitHubPullRequestClient
	org     string
	repo    string
	number  int
	lock    *sync.Mutex
	pr      *github.PullRequest
	baseSHA string
}

func (rg *RefGetterForGitHubPullRequest) PullRequest() (*github.PullRequest, error) {
	rg.lock.Lock()
	defer rg.lock.Unlock()
	if rg.pr != nil {
		return rg.pr, nil
	}

	pr, err := rg.ghc.GetPullRequest(rg.org, rg.repo, rg.number)
	if err != nil {
		return nil, err
	}

	rg.pr = pr
	return rg.pr, nil
}

// HeadSHA is a RefGetter that returns the headSHA for the PullRequst
func (rg *RefGetterForGitHubPullRequest) HeadSHA() (string, error) {
	if rg.pr == nil {
		if _, err := rg.PullRequest(); err != nil {
			return "", err
		}
	}
	return rg.pr.Head.SHA, nil
}

// BaseSHA is a RefGetter that returns the baseRef for the PullRequest
func (rg *RefGetterForGitHubPullRequest) BaseSHA() (string, error) {
	if rg.pr == nil {
		if _, err := rg.PullRequest(); err != nil {
			return "", err
		}
	}

	// rg.PullRequest also wants the lock, so we must not acquire it before
	// caling that
	rg.lock.Lock()
	defer rg.lock.Unlock()

	if rg.baseSHA != "" {
		return rg.baseSHA, nil
	}

	baseSHA, err := rg.ghc.GetRef(rg.org, rg.repo, "heads/"+rg.pr.Base.Ref)
	if err != nil {
		return "", err
	}
	rg.baseSHA = baseSHA

	return rg.baseSHA, nil
}

// getProwYAML will load presubmits and postsubmits for the given identifier that are
// versioned inside the tested repo, if the inrepoconfig feature is enabled.
// Consumers that pass in a RefGetter implementation that does a call to GitHub and who
// also need the result of that GitHub call just keep a pointer to its result, but must
// nilcheck that pointer before accessing it.
func (c *Config) getProwYAML(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {
	if identifier == "" {
		return nil, errors.New("no identifier for repo given")
	}
	if !c.InRepoConfigEnabled(identifier) {
		return &ProwYAML{}, nil
	}

	baseSHA, err := baseSHAGetter()
	if err != nil {
		return nil, fmt.Errorf("failed to get baseSHA: %v", err)
	}
	var headSHAs []string
	for _, headSHAGetter := range headSHAGetters {
		headSHA, err := headSHAGetter()
		if err != nil {
			return nil, fmt.Errorf("failed to get headRef: %v", err)
		}
		headSHAs = append(headSHAs, headSHA)
	}
	prowYAML, err := c.ProwYAMLGetter(c, gc, identifier, baseSHA, headSHAs...)
	if err != nil {
		return nil, err
	}

	return prowYAML, nil
}

// GetPresubmits will return all presubmits for the given identifier. This includes
// Presubmits that are versioned inside the tested repo, if the inrepoconfig feature
// is enabled.
// Consumers that pass in a RefGetter implementation that does a call to GitHub and who
// also need the result of that GitHub call just keep a pointer to its result, but must
// nilcheck that pointer before accessing it.
func (c *Config) GetPresubmits(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {
	prowYAML, err := c.getProwYAML(gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	return append(c.PresubmitsStatic[identifier], prowYAML.Presubmits...), nil
}

// GetPostsubmits will return all postsubmits for the given identifier. This includes
// Postsubmits that are versioned inside the tested repo, if the inrepoconfig feature
// is enabled.
// Consumers that pass in a RefGetter implementation that does a call to GitHub and who
// also need the result of that GitHub call just keep a pointer to its result, but must
// nilcheck that pointer before accessing it.
func (c *Config) GetPostsubmits(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error) {
	prowYAML, err := c.getProwYAML(gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	return append(c.PostsubmitsStatic[identifier], prowYAML.Postsubmits...), nil
}

// OwnersDirBlacklist is used to configure regular expressions matching directories
// to ignore when searching for OWNERS{,_ALIAS} files in a repo.
type OwnersDirBlacklist struct {
	// Repos configures a directory blacklist per repo (or org)
	Repos map[string][]string `json:"repos,omitempty"`
	// Default configures a default blacklist for all repos (or orgs).
	// Some directories like ".git", "_output" and "vendor/.*/OWNERS"
	// are already preconfigured to be blacklisted, and need not be included here.
	Default []string `json:"default,omitempty"`
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

	// ReportTemplateStrings is a mapping of template comments.
	// Use `org/repo`, `org` or `*` as a key.
	ReportTemplateStrings map[string]string `json:"report_templates,omitempty"`

	// ReportTemplates is a mapping of templates that is compliled at load
	// time from ReportTemplateStrings.
	ReportTemplates map[string]*template.Template `json:"-"`

	// MaxConcurrency is the maximum number of tests running concurrently that
	// will be allowed by the controller. 0 implies no limit.
	MaxConcurrency int `json:"max_concurrency,omitempty"`

	// MaxGoroutines is the maximum number of goroutines spawned inside the
	// controller to handle tests. Defaults to 20. Needs to be a positive
	// number.
	MaxGoroutines int `json:"max_goroutines,omitempty"`
}

// ReportTemplateForRepo returns the template that belong to a specific repository.
// If the repository doesn't exist in the report_templates configuration it will
// inherit the values from its organization, otherwise the default values will be used.
func (c *Controller) ReportTemplateForRepo(refs *prowapi.Refs) *template.Template {
	def := c.ReportTemplates["*"]

	if refs == nil {
		return def
	}

	orgRepo := fmt.Sprintf("%s/%s", refs.Org, refs.Repo)
	if tmplByRepo, ok := c.ReportTemplates[orgRepo]; ok {
		return tmplByRepo
	}
	if tmplByOrg, ok := c.ReportTemplates[refs.Org]; ok {
		return tmplByOrg
	}
	return def
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
	// PodUnscheduledTimeout is after how long the controller will abort a prowjob
	// stuck in an unscheduled state. Defaults to one day.
	PodUnscheduledTimeout *metav1.Duration `json:"pod_unscheduled_timeout,omitempty"`
	// DefaultDecorationConfigs holds the default decoration config for specific values.
	// This config will be used on each Presubmit and Postsubmit's corresponding org/repo, and on Periodics
	// if extraRefs[0] exists.
	// Use `org/repo`, `org` or `*` as a key.
	DefaultDecorationConfigs map[string]*prowapi.DecorationConfig `json:"default_decoration_configs,omitempty"`

	// JobURLPrefixConfig is the host and path prefix under which job details
	// will be viewable. Use `org/repo`, `org` or `*`as key and an url as value
	JobURLPrefixConfig map[string]string `json:"job_url_prefix_config,omitempty"`

	// JobURLPrefixDisableAppendStorageProvider disables that the storageProvider is
	// automatically appended to the JobURLPrefix
	JobURLPrefixDisableAppendStorageProvider bool `json:"jobURLPrefixDisableAppendStorageProvider,omitempty"`
}

func (p Plank) GetDefaultDecorationConfigs(repo string) *prowapi.DecorationConfig {
	def := p.DefaultDecorationConfigs["*"]
	if dcByRepo, ok := p.DefaultDecorationConfigs[repo]; ok {
		return dcByRepo.ApplyDefault(def)
	}
	org := strings.Split(repo, "/")[0]
	if dcByOrg, ok := p.DefaultDecorationConfigs[org]; ok {
		return dcByOrg.ApplyDefault(def)
	}
	return def
}

// GetJobURLPrefix gets the job url prefix from the config
// for the given refs. As we're deprecating the "gcs/" suffix
// (to allow using multiple storageProviders within a repo)
// we always trim the suffix here. Thus, every caller can assume
// the job url prefix does not have a storageProvider suffix.
func (p Plank) GetJobURLPrefix(pj *prowapi.ProwJob) string {
	jobURLPrefix := p.getJobURLPrefix(pj)
	if strings.HasSuffix(jobURLPrefix, "gcs/") {
		return strings.TrimSuffix(jobURLPrefix, "gcs/")
	}
	return strings.TrimSuffix(jobURLPrefix, "gcs")
}

func (p Plank) getJobURLPrefix(pj *prowapi.ProwJob) string {
	if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.GCSConfiguration != nil && pj.Spec.DecorationConfig.GCSConfiguration.JobURLPrefix != "" {
		return pj.Spec.DecorationConfig.GCSConfiguration.JobURLPrefix
	}

	var org, repo string
	if pj.Spec.Refs != nil {
		org = pj.Spec.Refs.Org
		repo = pj.Spec.Refs.Repo
	} else if len(pj.Spec.ExtraRefs) > 0 {
		org = pj.Spec.ExtraRefs[0].Org
		repo = pj.Spec.ExtraRefs[0].Repo
	}

	if org == "" {
		return p.JobURLPrefixConfig["*"]
	}
	if p.JobURLPrefixConfig[fmt.Sprintf("%s/%s", org, repo)] != "" {
		return p.JobURLPrefixConfig[fmt.Sprintf("%s/%s", org, repo)]
	}
	if p.JobURLPrefixConfig[org] != "" {
		return p.JobURLPrefixConfig[org]
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
	// TerminatedPodTTL is how long a Pod can live after termination before it is
	// garbage collected.
	// Defaults to matching MaxPodAge.
	TerminatedPodTTL *metav1.Duration `json:"terminated_pod_ttl,omitempty"`
	// ExcludeClusters are build clusters that don't want to be managed by sinker
	ExcludeClusters []string `json:"exclude_clusters,omitempty"`
}

// LensConfig names a specific lens, and optionally provides some configuration for it.
type LensConfig struct {
	// Name is the name of the lens.
	Name string `json:"name"`
	// Config is some lens-specific configuration. Interpreting it is the responsibility of the
	// lens in question.
	Config json.RawMessage `json:"config,omitempty"`
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
	// RemoteConfig specifies how to access remote lenses
	RemoteConfig *LensRemoteConfig `json:"remote_config,omitempty"`
}

// LensRemoteConfig is the configuration for a remote lens.
type LensRemoteConfig struct {
	// The endpoint for the lense
	Endpoint string `json:"endpoint"`
	// The parsed endpoint
	ParsedEndpoint *url.URL `json:"-"`
	// The endpoint for static resources
	StaticRoot string `json:"static_root"`
	// The human-readable title for the lens
	Title string `json:"title"`
	// Priority for lens ordering, lowest priority first
	Priority *uint `json:"priority"`
	// HideTitle defines if we will keep showing the title after lens loads
	HideTitle *bool `json:"hide_title"`
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
	// GCSBrowserPrefixes are used to generate a link to a human-usable GCS browser.
	// They are mapped by org, org/repo or '*' which is the default value.
	GCSBrowserPrefixes GCSBrowserPrefixes `json:"gcs_browser_prefixes,omitempty"`
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

type GCSBrowserPrefixes map[string]string

func (p GCSBrowserPrefixes) GetGCSBrowserPrefix(org, repo string) string {
	if prefix, exists := p[fmt.Sprintf("%s/%s", org, repo)]; exists {
		return prefix
	}

	if prefix, exists := p[org]; exists {
		return prefix
	}

	return p["*"]
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
	// Deprecated: RerunAuthConfig specifies who is able to trigger job reruns if that feature is enabled.
	// The permissions here apply to all jobs.
	// This option will be removed in favor of RerunAuthConfigs in July 2020.
	RerunAuthConfig *prowapi.RerunAuthConfig `json:"rerun_auth_config,omitempty"`
	// RerunAuthConfigs is a map of configs that specify who is able to trigger job reruns. The field
	// accepts a key of: `org/repo`, `org` or `*` (wildcard) to define what GitHub org (or repo) a particular
	// config applies to and a value of: `RerunAuthConfig` struct to define the users/groups authorized to rerun jobs.
	RerunAuthConfigs RerunAuthConfigs `json:"rerun_auth_configs,omitempty"`
	// SkipStoragePathValidation skips validation that restricts artifact requests to specific buckets.
	// By default, buckets listed in the GCSConfiguration are automatically allowed.
	// Additional locations can be allowed via `AdditionalAllowedBuckets` fields.
	// When unspecified (nil), it defaults to true (until ~Jan 2021).
	SkipStoragePathValidation *bool `json:"skip_storage_path_validation,omitempty"`
	// AdditionalAllowedBuckets is a list of storage buckets to allow in artifact requests
	// (in addition to those listed in the GCSConfiguration).
	// Setting this field requires "SkipStoragePathValidation" also be set to `false`.
	AdditionalAllowedBuckets []string `json:"additional_allowed_buckets,omitempty"`
	// AllKnownStorageBuckets contains all storage buckets configured in all of the
	// job configs.
	AllKnownStorageBuckets sets.String `json:"-"`
}

// Validate performs validation and sanitization on the Deck object.
func (d *Deck) Validate() error {
	if len(d.AdditionalAllowedBuckets) > 0 && !d.ShouldValidateStorageBuckets() {
		return fmt.Errorf("deck.skip_storage_path_validation is enabled despite deck.additional_allowed_buckets being configured: %v", d.AdditionalAllowedBuckets)
	}

	// TODO(@clarketm): Remove "rerun_auth_config" validation in July 2020
	if d.RerunAuthConfig != nil {
		logrus.Warning("rerun_auth_config will be deprecated in July 2020, and it will be replaced with rerun_auth_configs['*'].")

		if d.RerunAuthConfigs != nil {
			return errors.New("rerun_auth_config and rerun_auth_configs['*'] are mutually exclusive")
		}

		d.RerunAuthConfigs = RerunAuthConfigs{"*": *d.RerunAuthConfig}
	}

	// Note: The RerunAuthConfigs logic isn't deprecated, only the above RerunAuthConfig stuff is
	if d.RerunAuthConfigs != nil {
		for k, config := range d.RerunAuthConfigs {
			if err := config.Validate(); err != nil {
				return fmt.Errorf("rerun_auth_configs[%s]: %v", k, err)
			}
		}
	}

	return nil
}

var warnInRepoStorageBucketValidation time.Time

// ValidateStorageBucket validates a storage bucket (unless the `Deck.SkipStoragePathValidation` field is true).
// The bucket name must be included in any of the following:
//    1) Any job's `.DecorationConfig.GCSConfiguration.Bucket` (except jobs defined externally via InRepoConfig)
//    2) `Plank.DefaultDecorationConfigs.GCSConfiguration.Bucket`
//    3) `Deck.AdditionalAllowedBuckets`
func (c *Config) ValidateStorageBucket(bucketName string) error {
	if len(c.InRepoConfig.Enabled) > 0 && len(c.Deck.AdditionalAllowedBuckets) == 0 {
		logrusutil.ThrottledWarnf(&warnInRepoStorageBucketValidation, 1*time.Hour,
			"skipping storage-path validation because `in_repo_config` is enabled, but `deck.additional_allowed_buckets` empty. "+
				"(Note: Validation will be enabled by default in January 2021. "+
				"To disable this message, populate `deck.additional_allowed_buckets` with at least one storage bucket. "+
				"When `deck.additional_allowed_buckets` is populated, this message will be disabled.)")
		return nil
	}

	if !c.Deck.ShouldValidateStorageBuckets() {
		return nil
	}

	if !c.Deck.AllKnownStorageBuckets.Has(bucketName) {
		return fmt.Errorf("bucket %q not in allowed list (%v); you may allow it by including it in `deck.additional_allowed_buckets`", bucketName, c.Deck.AllKnownStorageBuckets.List())
	}
	return nil
}

// ShouldValidateStorageBuckets returns whether or not the Deck's storage path should be validated.
// Validation could be either disabled by default or explicitly turned off.
func (d *Deck) ShouldValidateStorageBuckets() bool {
	if d.SkipStoragePathValidation == nil {
		// TODO(e-blackwelder): validate storage paths by default (~Jan 2021)
		return false
	}
	return !*d.SkipStoragePathValidation
}

func calculateStorageBuckets(c *Config) sets.String {
	knownBuckets := sets.NewString(c.Deck.AdditionalAllowedBuckets...)
	for _, dc := range c.Plank.DefaultDecorationConfigs {
		knownBuckets.Insert(dc.GCSConfiguration.Bucket)
	}
	for _, j := range c.Periodics {
		if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
			knownBuckets.Insert(j.DecorationConfig.GCSConfiguration.Bucket)
		}
	}
	for _, jobs := range c.PresubmitsStatic {
		for _, j := range jobs {
			if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
				knownBuckets.Insert(j.DecorationConfig.GCSConfiguration.Bucket)
			}
		}
	}
	for _, jobs := range c.PostsubmitsStatic {
		for _, j := range jobs {
			if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
				knownBuckets.Insert(j.DecorationConfig.GCSConfiguration.Bucket)
			}
		}
	}
	return knownBuckets
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

// RerunAuthConfigs represents the configs for rerun authorization in Deck.
// Use `org/repo`, `org` or `*` as key and a `RerunAuthConfig` struct as value.
type RerunAuthConfigs map[string]prowapi.RerunAuthConfig

// GetRerunAuthConfig returns the appropriate RerunAuthConfig based on the provided Refs.
func (rac RerunAuthConfigs) GetRerunAuthConfig(refs *prowapi.Refs) prowapi.RerunAuthConfig {
	if refs == nil || refs.Org == "" {
		return rac["*"]
	}

	if rerun, exists := rac[fmt.Sprintf("%s/%s", refs.Org, refs.Repo)]; exists {
		return rerun
	}

	if rerun, exists := rac[refs.Org]; exists {
		return rerun
	}

	return rac["*"]
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
	LinkURL *url.URL `json:"-"`
}

// ManagedWebhookInfo contains metadata about the repo/org which is onboarded.
type ManagedWebhookInfo struct {
	TokenCreatedAfter time.Time `json:"token_created_after"`
}

// ManagedWebhooks contains information about all the repos/orgs which are onboarded with auto-generated tokens.
type ManagedWebhooks struct {
	RespectLegacyGlobalToken bool                          `json:"respect_legacy_global_token"`
	OrgRepoConfig            map[string]ManagedWebhookInfo `json:"org_repo_config"`
}

// SlackReporter represents the config for the Slack reporter. The channel can be overridden
// on the job via the .reporter_config.slack.channel property
type SlackReporter struct {
	JobTypesToReport  []prowapi.ProwJobType  `json:"job_types_to_report"`
	JobStatesToReport []prowapi.ProwJobState `json:"job_states_to_report"`
	Channel           string                 `json:"channel"`
	ReportTemplate    string                 `json:"report_template"`
}

// SlackReporterConfigs represents the config for the Slack reporter(s).
// Use `org/repo`, `org` or `*` as key and an `SlackReporter` struct as value.
type SlackReporterConfigs map[string]SlackReporter

func (cfg SlackReporterConfigs) GetSlackReporter(refs *prowapi.Refs) SlackReporter {
	if refs == nil {
		return cfg["*"]
	}

	if slack, exists := cfg[fmt.Sprintf("%s/%s", refs.Org, refs.Repo)]; exists {
		return slack
	}

	if slack, exists := cfg[refs.Org]; exists {
		return slack
	}

	return cfg["*"]
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
func Load(prowConfig, jobConfig string, additionals ...func(*Config) error) (c *Config, err error) {
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
	if err := c.ValidateJobConfig(); err != nil {
		return nil, err
	}

	for _, additional := range additionals {
		if err := additional(c); err != nil {
			return nil, err
		}
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

	nc.AllRepos = sets.String{}
	for _, query := range nc.Tide.Queries {
		for _, repo := range query.Repos {
			nc.AllRepos.Insert(repo)
		}
	}

	nc.ProwYAMLGetter = defaultProwYAMLGetter

	if nc.InRepoConfig.AllowedClusters == nil {
		nc.InRepoConfig.AllowedClusters = map[string][]string{}
	}

	if len(nc.InRepoConfig.AllowedClusters["*"]) == 0 {
		nc.InRepoConfig.AllowedClusters["*"] = []string{kube.DefaultClusterAlias}
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
	for rep := range jc.PresubmitsStatic {
		fix := func(job *Presubmit) {
			job.SourcePath = path
		}
		for i := range jc.PresubmitsStatic[rep] {
			fix(&jc.PresubmitsStatic[rep][i])
		}
	}
	for rep := range jc.PostsubmitsStatic {
		fix := func(job *Postsubmit) {
			job.SourcePath = path
		}
		for i := range jc.PostsubmitsStatic[rep] {
			fix(&jc.PostsubmitsStatic[rep][i])
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
		Presets:           c.Presets,
		PresubmitsStatic:  c.PresubmitsStatic,
		Periodics:         c.Periodics,
		PostsubmitsStatic: c.PostsubmitsStatic,
	}, jc)
	if err != nil {
		return err
	}
	c.Presets = m.Presets
	c.PresubmitsStatic = m.PresubmitsStatic
	c.Periodics = m.Periodics
	c.PostsubmitsStatic = m.PostsubmitsStatic
	return nil
}

// mergeJobConfigs merges two JobConfig together
// It will try to merge:
//	- Presubmits
//	- Postsubmits
// 	- Periodics
//	- Presets
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
	c.PresubmitsStatic = make(map[string][]Presubmit)
	for repo, jobs := range a.PresubmitsStatic {
		c.PresubmitsStatic[repo] = jobs
	}
	for repo, jobs := range b.PresubmitsStatic {
		c.PresubmitsStatic[repo] = append(c.PresubmitsStatic[repo], jobs...)
	}

	// *** Postsubmits ***
	c.PostsubmitsStatic = make(map[string][]Postsubmit)
	for repo, jobs := range a.PostsubmitsStatic {
		c.PostsubmitsStatic[repo] = jobs
	}
	for repo, jobs := range b.PostsubmitsStatic {
		c.PostsubmitsStatic[repo] = append(c.PostsubmitsStatic[repo], jobs...)
	}
	return c, nil
}

func ShouldDecorate(c *JobConfig, util UtilityConfig) bool {
	if util.Decorate != nil {
		return *util.Decorate
	}
	return c.DecorateAllJobs
}

func setPresubmitDecorationDefaults(c *Config, ps *Presubmit, repo string) {
	if ShouldDecorate(&c.JobConfig, ps.JobBase.UtilityConfig) {
		def := c.Plank.GetDefaultDecorationConfigs(repo)
		ps.DecorationConfig = ps.DecorationConfig.ApplyDefault(def)
	}
}

func setPostsubmitDecorationDefaults(c *Config, ps *Postsubmit, repo string) {
	if ShouldDecorate(&c.JobConfig, ps.JobBase.UtilityConfig) {
		def := c.Plank.GetDefaultDecorationConfigs(repo)
		ps.DecorationConfig = ps.DecorationConfig.ApplyDefault(def)
	}
}

func setPeriodicDecorationDefaults(c *Config, ps *Periodic) {
	if ShouldDecorate(&c.JobConfig, ps.JobBase.UtilityConfig) {
		var orgRepo string
		if len(ps.UtilityConfig.ExtraRefs) > 0 {
			orgRepo = fmt.Sprintf("%s/%s", ps.UtilityConfig.ExtraRefs[0].Org, ps.UtilityConfig.ExtraRefs[0].Repo)
		}

		def := c.Plank.GetDefaultDecorationConfigs(orgRepo)
		ps.DecorationConfig = ps.DecorationConfig.ApplyDefault(def)
	}
}

// defaultPresubmits defaults the presubmits for one repo
func defaultPresubmits(presubmits []Presubmit, c *Config, repo string) error {
	var errs []error
	for idx, ps := range presubmits {
		setPresubmitDecorationDefaults(c, &presubmits[idx], repo)
		if err := resolvePresets(ps.Name, ps.Labels, ps.Spec, c.Presets); err != nil {
			errs = append(errs, err)
		}
	}
	c.defaultPresubmitFields(presubmits)
	if err := SetPresubmitRegexes(presubmits); err != nil {
		errs = append(errs, fmt.Errorf("could not set regex: %v", err))
	}

	return utilerrors.NewAggregate(errs)
}

// defaultPostsubmits defaults the postsubmits for one repo
func defaultPostsubmits(postsubmits []Postsubmit, c *Config, repo string) error {
	var errs []error
	for idx, ps := range postsubmits {
		setPostsubmitDecorationDefaults(c, &postsubmits[idx], repo)
		if err := resolvePresets(ps.Name, ps.Labels, ps.Spec, c.Presets); err != nil {
			errs = append(errs, err)
		}
	}
	c.defaultPostsubmitFields(postsubmits)
	if err := SetPostsubmitRegexes(postsubmits); err != nil {
		errs = append(errs, fmt.Errorf("could not set regex: %v", err))
	}
	return utilerrors.NewAggregate(errs)
}

// defaultPeriodics defaults periodics
func defaultPeriodics(periodics []Periodic, c *Config) error {
	var errs []error
	c.defaultPeriodicFields(periodics)
	for _, periodic := range periodics {
		if err := resolvePresets(periodic.Name, periodic.Labels, periodic.Spec, c.Presets); err != nil {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// finalizeJobConfig mutates and fixes entries for jobspecs
func (c *Config) finalizeJobConfig() error {
	if c.decorationRequested() {

		def, ok := c.Plank.DefaultDecorationConfigs["*"]
		if !ok {
			return errors.New("default_decoration_configs['*'] is missing")
		}

		for key, valCfg := range c.Plank.DefaultDecorationConfigs {
			if err := valCfg.ApplyDefault(def).Validate(); err != nil {
				return fmt.Errorf("default_decoration_configs[%q]: validation error: %v", key, err)
			}
		}

		for i := range c.Periodics {
			setPeriodicDecorationDefaults(c, &c.Periodics[i])
		}
	}

	for repo, jobs := range c.PresubmitsStatic {
		if err := defaultPresubmits(jobs, c, repo); err != nil {
			return err
		}
		c.AllRepos.Insert(repo)
	}

	for repo, jobs := range c.PostsubmitsStatic {
		if err := defaultPostsubmits(jobs, c, repo); err != nil {
			return err
		}
		c.AllRepos.Insert(repo)
	}

	if err := defaultPeriodics(c.Periodics, c); err != nil {
		return err
	}

	return nil
}

// validateComponentConfig validates the various infrastructure components' configurations.
func (c *Config) validateComponentConfig() error {
	for k, v := range c.Plank.JobURLPrefixConfig {
		if _, err := url.Parse(v); err != nil {
			return fmt.Errorf(`Invalid value for Planks job_url_prefix_config["%s"]: %v`, k, err)
		}
		// TODO(@sbueringer): Remove in September 2020
		if strings.HasSuffix(v, "gcs/") {
			logrus.Warning(strings.Join([]string{
				"configuring the 'gcs/' storage provider suffix in the job url prefix is now deprecated, ",
				"please configure the job url prefix without the suffix as it's now appended automatically. Handling of the old ",
				"configuration will be removed in September 2020",
			}, ""))
		}
	}

	var validationErrs []error

	if c.ManagedWebhooks.OrgRepoConfig != nil {
		for repoName, repoValue := range c.ManagedWebhooks.OrgRepoConfig {
			if repoValue.TokenCreatedAfter.After(time.Now()) {
				validationErrs = append(validationErrs, fmt.Errorf("token_created_after %s can be no later than current time for repo/org %s", repoValue.TokenCreatedAfter, repoName))
			}
		}
		if len(validationErrs) > 0 {
			return utilerrors.NewAggregate(validationErrs)
		}
	}

	// TODO(@clarketm): Remove in May 2020
	if c.SlackReporter != nil {
		logrus.Warning("slack_reporter will be deprecated on May 2020, and it will be replaced with slack_reporter_configs['*'].")

		if c.SlackReporterConfigs != nil {
			return errors.New("slack_reporter and slack_reporter_configs['*'] are mutually exclusive")
		}

		c.SlackReporterConfigs = SlackReporterConfigs{"*": *c.SlackReporter}
	}

	if c.SlackReporterConfigs != nil {
		for k, config := range c.SlackReporterConfigs {
			if err := config.DefaultAndValidate(); err != nil {
				return fmt.Errorf("failed to validate slackreporter config: %v", err)
			}
			c.SlackReporterConfigs[k] = config
		}
	}

	if err := c.Deck.Validate(); err != nil {
		return err
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
	if err := validatePodSpec(jobType, v.Spec, v.DecorationConfig != nil); err != nil {
		return err
	}
	if err := ValidatePipelineRunSpec(jobType, v.ExtraRefs, v.PipelineRunSpec); err != nil {
		return err
	}
	if err := validateLabels(v.Labels); err != nil {
		return err
	}
	if err := validateAnnotation(v.Annotations); err != nil {
		return err
	}
	if v.Spec == nil || len(v.Spec.Containers) == 0 {
		return nil // jenkins jobs have no spec
	}
	if err := v.RerunAuthConfig.Validate(); err != nil {
		return err
	}
	if err := v.UtilityConfig.Validate(); err != nil {
		return err
	}
	for i := range v.Spec.Containers {
		if err := validateDecoration(v.Spec.Containers[i], v.DecorationConfig); err != nil {
			return err
		}
	}
	return nil
}

// validatePresubmits validates the presubmits for one repo
func validatePresubmits(presubmits []Presubmit, podNamespace string) error {
	validPresubmits := map[string][]Presubmit{}
	var errs []error
	for _, ps := range presubmits {
		// Checking that no duplicate job in prow config exists on the same branch.
		for _, existingJob := range validPresubmits[ps.Name] {
			if existingJob.Brancher.Intersects(ps.Brancher) {
				errs = append(errs, fmt.Errorf("duplicated presubmit job: %s", ps.Name))
			}
		}
		for _, otherPS := range presubmits {
			if otherPS.Name == ps.Name || !otherPS.Brancher.Intersects(ps.Brancher) {
				continue
			}
			if otherPS.Context == ps.Context {
				errs = append(errs, fmt.Errorf("jobs %s and %s report to the same GitHub context %q", otherPS.Name, ps.Name, otherPS.Context))
			}
		}
		if err := validateJobBase(ps.JobBase, prowapi.PresubmitJob, podNamespace); err != nil {
			errs = append(errs, fmt.Errorf("invalid presubmit job %s: %v", ps.Name, err))
		}
		if err := validateTriggering(ps); err != nil {
			errs = append(errs, err)
		}
		if err := validateReporting(ps.JobBase, ps.Reporter); err != nil {
			errs = append(errs, fmt.Errorf("invalid presubmit job %s: %v", ps.Name, err))
		}
		validPresubmits[ps.Name] = append(validPresubmits[ps.Name], ps)
	}

	return utilerrors.NewAggregate(errs)
}

// ValidateRefs validates the extra refs on a presubmit for one repo
func ValidateRefs(repo string, jobBase JobBase) error {
	gitRefs := map[string]int{
		repo: 1,
	}
	for _, ref := range jobBase.UtilityConfig.ExtraRefs {
		gitRefs[fmt.Sprintf("%s/%s", ref.Org, ref.Repo)]++
	}

	dupes := sets.NewString()
	for gitRef, count := range gitRefs {
		if count > 1 {
			dupes.Insert(gitRef)
		}
	}

	if dupes.Len() > 0 {
		return fmt.Errorf("Invalid job %s on repo %s: the following refs specified more than once: %s",
			jobBase.Name, repo, strings.Join(dupes.List(), ","))
	}
	return nil
}

// validatePostsubmits validates the postsubmits for one repo
func validatePostsubmits(postsubmits []Postsubmit, podNamespace string) error {
	validPostsubmits := map[string][]Postsubmit{}

	var errs []error
	for _, ps := range postsubmits {
		// Checking that no duplicate job in prow config exists on the same repo / branch.
		for _, existingJob := range validPostsubmits[ps.Name] {
			if existingJob.Brancher.Intersects(ps.Brancher) {
				errs = append(errs, fmt.Errorf("duplicated postsubmit job: %s", ps.Name))
			}
		}
		for _, otherPS := range postsubmits {
			if otherPS.Name == ps.Name || !otherPS.Brancher.Intersects(ps.Brancher) {
				continue
			}
			if otherPS.Context == ps.Context {
				errs = append(errs, fmt.Errorf("jobs %s and %s report to the same GitHub context %q", otherPS.Name, ps.Name, otherPS.Context))
			}
		}

		if err := validateJobBase(ps.JobBase, prowapi.PostsubmitJob, podNamespace); err != nil {
			errs = append(errs, fmt.Errorf("invalid postsubmit job %s: %v", ps.Name, err))
		}
		if err := validateReporting(ps.JobBase, ps.Reporter); err != nil {
			errs = append(errs, fmt.Errorf("invalid postsubmit job %s: %v", ps.Name, err))
		}
		validPostsubmits[ps.Name] = append(validPostsubmits[ps.Name], ps)
	}

	return utilerrors.NewAggregate(errs)
}

// validatePeriodics validates a set of periodics
func validatePeriodics(periodics []Periodic, podNamespace string) error {

	// validate no duplicated periodics
	validPeriodics := sets.NewString()
	// Ensure that the periodic durations are valid and specs exist.
	for _, p := range periodics {
		if validPeriodics.Has(p.Name) {
			return fmt.Errorf("duplicated periodic job : %s", p.Name)
		}
		validPeriodics.Insert(p.Name)
		if err := validateJobBase(p.JobBase, prowapi.PeriodicJob, podNamespace); err != nil {
			return fmt.Errorf("invalid periodic job %s: %v", p.Name, err)
		}
	}

	return nil
}

// ValidateJobConfig validates if all the jobspecs/presets are valid
// if you are mutating the jobs, please add it to finalizeJobConfig above
func (c *Config) ValidateJobConfig() error {

	var errs []error

	// Validate presubmits.
	for _, jobs := range c.PresubmitsStatic {
		if err := validatePresubmits(jobs, c.PodNamespace); err != nil {
			errs = append(errs, err)
		}
	}

	// Validate postsubmits.
	for _, jobs := range c.PostsubmitsStatic {
		if err := validatePostsubmits(jobs, c.PodNamespace); err != nil {
			errs = append(errs, err)
		}
	}

	if err := validatePeriodics(c.Periodics, c.PodNamespace); err != nil {
		errs = append(errs, err)
	}

	// Set the interval on the periodic jobs. It doesn't make sense to do this
	// for child jobs.
	for j, p := range c.Periodics {
		if p.Cron != "" && p.Interval != "" {
			errs = append(errs, fmt.Errorf("cron and interval cannot be both set in periodic %s", p.Name))
		} else if p.Cron == "" && p.Interval == "" {
			errs = append(errs, fmt.Errorf("cron and interval cannot be both empty in periodic %s", p.Name))
		} else if p.Cron != "" {
			if _, err := cron.Parse(p.Cron); err != nil {
				errs = append(errs, fmt.Errorf("invalid cron string %s in periodic %s: %v", p.Cron, p.Name, err))
			}
		} else {
			d, err := time.ParseDuration(c.Periodics[j].Interval)
			if err != nil {
				errs = append(errs, fmt.Errorf("cannot parse duration for %s: %v", c.Periodics[j].Name, err))
			}
			c.Periodics[j].interval = d
		}
	}

	c.Deck.AllKnownStorageBuckets = calculateStorageBuckets(c)

	return utilerrors.NewAggregate(errs)
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

	if c.Plank.PodUnscheduledTimeout == nil {
		c.Plank.PodUnscheduledTimeout = &metav1.Duration{Duration: 24 * time.Hour}
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

	if c.Deck.Spyglass.GCSBrowserPrefixes == nil {
		c.Deck.Spyglass.GCSBrowserPrefixes = make(map[string]string)
	}

	_, exists := c.Deck.Spyglass.GCSBrowserPrefixes["*"]
	if exists && c.Deck.Spyglass.GCSBrowserPrefix != "" {
		return fmt.Errorf("both gcs_browser_prefixes and gcs_browser_prefix['*'] are specified.")
	}

	if !exists {
		c.Deck.Spyglass.GCSBrowserPrefixes["*"] = c.Deck.Spyglass.GCSBrowserPrefix
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

	if c.Sinker.TerminatedPodTTL == nil {
		c.Sinker.TerminatedPodTTL = &metav1.Duration{Duration: c.Sinker.MaxPodAge.Duration}
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

	if c.Tide.PRStatusBaseURLs == nil {
		c.Tide.PRStatusBaseURLs = map[string]string{}
	}

	if len(c.Tide.PRStatusBaseURL) > 0 {
		if len(c.Tide.PRStatusBaseURLs) > 0 {
			return fmt.Errorf("both pr_status_base_url and pr_status_base_urls are defined")
		} else {
			logrus.Warning("The `pr_status_base_url` setting is deprecated and it has been replaced by `pr_status_base_urls`. It will be removed in June 2020")
			c.Tide.PRStatusBaseURLs["*"] = c.Tide.PRStatusBaseURL
		}
	}

	if len(c.Tide.PRStatusBaseURLs) > 0 {
		if _, ok := c.Tide.PRStatusBaseURLs["*"]; !ok {
			return fmt.Errorf("pr_status_base_urls is defined but the default value ('*') is missing")
		}
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
	for _, vs := range c.PresubmitsStatic {
		for i := range vs {
			if ShouldDecorate(c, vs[i].JobBase.UtilityConfig) {
				return true
			}
		}
	}

	for _, js := range c.PostsubmitsStatic {
		for i := range js {
			if ShouldDecorate(c, js[i].JobBase.UtilityConfig) {
				return true
			}
		}
	}

	for i := range c.Periodics {
		if ShouldDecorate(c, c.Periodics[i].JobBase.UtilityConfig) {
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

func validateAnnotation(a map[string]string) error {
	for key := range a {
		if errs := validation.IsQualifiedName(key); len(errs) > 0 {
			return fmt.Errorf("invalid annotation key %q: %v", key, errs)
		}
	}
	return nil
}

func validateAgent(v JobBase, podNamespace string) error {
	k := string(prowapi.KubernetesAgent)
	j := string(prowapi.JenkinsAgent)
	p := string(prowapi.TektonAgent)
	agents := sets.NewString(k, j, p)
	agent := v.Agent
	switch {
	case !agents.Has(agent):
		logrus.Warningf("agent %s is unknown and cannot be validated: use at your own risk", agent)
		return nil
	case v.Spec != nil && agent != k:
		return fmt.Errorf("job specs require agent: %s (found %q)", k, agent)
	case agent == k && v.Spec == nil:
		return errors.New("kubernetes jobs require a spec")
	case v.PipelineRunSpec != nil && agent != p:
		return fmt.Errorf("job pipeline_run_spec require agent: %s (found %q)", p, agent)
	case agent == p && v.PipelineRunSpec == nil:
		return fmt.Errorf("agent: %s jobs require a pipeline_run_spec", p)
	case v.DecorationConfig != nil && agent != k:
		// TODO(fejta): only source decoration supported...
		return fmt.Errorf("decoration requires agent: %s (found %q)", k, agent)
	case v.ErrorOnEviction && agent != k:
		return fmt.Errorf("error_on_eviction only applies to agent: %s (found %q)", k, agent)
	case v.Namespace == nil || *v.Namespace == "":
		return fmt.Errorf("failed to default namespace")
	case *v.Namespace != podNamespace && agent != p:
		// TODO(fejta): update plank to allow this (depends on client change)
		return fmt.Errorf("namespace customization requires agent: %s (found %q)", p, agent)
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

func resolvePresets(name string, labels map[string]string, spec *v1.PodSpec, presets []Preset) error {
	for _, preset := range presets {
		if spec != nil {
			if err := mergePreset(preset, labels, spec.Containers, &spec.Volumes); err != nil {
				return fmt.Errorf("job %s failed to merge presets for podspec: %v", name, err)
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

func validatePodSpec(jobType prowapi.ProwJobType, spec *v1.PodSpec, decorationEnabled bool) error {
	if spec == nil {
		return nil
	}

	var errs []error

	if len(spec.InitContainers) != 0 {
		errs = append(errs, errors.New("pod spec may not use init containers"))
	}

	if n := len(spec.Containers); n < 1 {
		// We must return here to not cause an out of bounds panic in the remaining validation
		return utilerrors.NewAggregate(append(errs, fmt.Errorf("pod spec must specify at least 1 container, found: %d", n)))
	}

	if n := len(spec.Containers); n > 1 && !decorationEnabled {
		return utilerrors.NewAggregate(append(errs, fmt.Errorf("pod utility decoration must be enabled to use multiple containers: %d", n)))
	}

	if len(spec.Containers) > 1 {
		containerNames := sets.String{}
		for _, container := range spec.Containers {
			if container.Name == "" {
				errs = append(errs, fmt.Errorf("container does not have name. all containers must have names when defining multiple containers"))
			}

			if containerNames.Has(container.Name) {
				errs = append(errs, fmt.Errorf("container named %q is defined more than once", container.Name))
			}
			containerNames.Insert(container.Name)

			if decorate.PodUtilsContainerNames().Has(container.Name) {
				errs = append(errs, fmt.Errorf("container name %s is a reserved for decoration. please specify a different container name that does not conflict with pod utility container names", container.Name))
			}
		}
	}

	for i := range spec.Containers {
		envNames := sets.String{}
		for _, env := range spec.Containers[i].Env {
			if envNames.Has(env.Name) {
				errs = append(errs, fmt.Errorf("env var named %q is defined more than once", env.Name))
			}
			envNames.Insert(env.Name)

			for _, prowEnv := range downwardapi.EnvForType(jobType) {
				if env.Name == prowEnv {
					// TODO(fejta): consider allowing this
					errs = append(errs, fmt.Errorf("env %s is reserved", env.Name))
				}
			}
		}
	}

	volumeNames := sets.String{}
	for _, volume := range spec.Volumes {
		if volumeNames.Has(volume.Name) {
			errs = append(errs, fmt.Errorf("volume named %q is defined more than once", volume.Name))
		}
		volumeNames.Insert(volume.Name)

		if decorate.VolumeMounts().Has(volume.Name) {
			errs = append(errs, fmt.Errorf("volume %s is a reserved for decoration", volume.Name))
		}
	}

	for i := range spec.Containers {
		for _, mount := range spec.Containers[i].VolumeMounts {
			if !volumeNames.Has(mount.Name) && !decorate.VolumeMounts().Has(mount.Name) {
				errs = append(errs, fmt.Errorf("volumeMount named %q is undefined", mount.Name))
			}
			if decorate.VolumeMountsOnTestContainer().Has(mount.Name) {
				errs = append(errs, fmt.Errorf("volumeMount name %s is reserved for decoration", mount.Name))
			}
			if decorate.VolumeMountPathsOnTestContainer().Has(mount.MountPath) {
				errs = append(errs, fmt.Errorf("mount %s at %s conflicts with decoration mount", mount.Name, mount.MountPath))
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

func validateTriggering(job Presubmit) error {
	if job.AlwaysRun && job.RunIfChanged != "" {
		return fmt.Errorf("job %s is set to always run but also declares run_if_changed targets, which are mutually exclusive", job.Name)
	}

	if (job.Trigger != "" && job.RerunCommand == "") || (job.Trigger == "" && job.RerunCommand != "") {
		return fmt.Errorf("Either both of job.Trigger and job.RerunCommand must be set, wasnt the case for job %q", job.Name)
	}

	return nil
}

func validateReporting(j JobBase, r Reporter) error {
	if !r.SkipReport && r.Context == "" {
		return errors.New("job is set to report but has no context configured")
	}
	if !r.SkipReport {
		return nil
	}
	for label, value := range j.Labels {
		if label == gerrit.GerritReportLabel && value != "" {
			return fmt.Errorf("Gerrit report label %s set to non-empty string but job is configured to skip reporting.", label)
		}
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

	if err := defaultAndValidateReportTemplate(c); err != nil {
		return err
	}
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

func defaultAndValidateReportTemplate(c *Controller) error {
	if c.ReportTemplateString == "" && c.ReportTemplateStrings == nil {
		return nil
	}

	if c.ReportTemplateString != "" {
		if len(c.ReportTemplateStrings) > 0 {
			return errors.New("both report_template and report_templates are specified")
		}

		logrus.Warning("report_template is deprecated and it will be removed on September 2020. It will be replaced with report_templates['*']")
		c.ReportTemplateStrings = make(map[string]string)
		c.ReportTemplateStrings["*"] = c.ReportTemplateString
	}

	c.ReportTemplates = make(map[string]*template.Template)
	for orgRepo, value := range c.ReportTemplateStrings {
		reportTmpl, err := template.New("Report").Parse(value)
		if err != nil {
			return fmt.Errorf("error while parsing template for %s: %v", orgRepo, err)
		}
		c.ReportTemplates[orgRepo] = reportTmpl
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

// OrgRepo supercedes org/repo string handling
type OrgRepo struct {
	Org  string
	Repo string
}

func (repo OrgRepo) String() string {
	return fmt.Sprintf("%s/%s", repo.Org, repo.Repo)
}

// NewOrgRepo creates a OrgRepo from org/repo string
func NewOrgRepo(orgRepo string) *OrgRepo {
	parts := strings.Split(orgRepo, "/")
	switch len(parts) {
	case 1:
		return &OrgRepo{Org: parts[0]}
	case 2:
		return &OrgRepo{Org: parts[0], Repo: parts[1]}
	default:
		return nil
	}
}

// OrgReposToStrings converts a list of OrgRepo to its String() equivalent
func OrgReposToStrings(vs []OrgRepo) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = v.String()
	}
	return vsm
}

// StringsToOrgRepos converts a list of org/repo strings to its OrgRepo equivalent
func StringsToOrgRepos(vs []string) []OrgRepo {
	vsm := make([]OrgRepo, len(vs))
	for i, v := range vs {
		vsm[i] = *NewOrgRepo(v)
	}
	return vsm
}

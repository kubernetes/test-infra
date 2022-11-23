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
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	gitignore "github.com/denormal/go-gitignore"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	pipelinev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"gopkg.in/robfig/cron.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	gerritsource "k8s.io/test-infra/prow/gerrit/source"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

const (
	// DefaultJobTimeout represents the default deadline for a prow job.
	DefaultJobTimeout = 24 * time.Hour

	ProwImplicitGitResource = "PROW_IMPLICIT_GIT_REF"

	// ConfigVersionFileName is the name of a file that will be added to
	// all configmaps by the configupdater and contain the git sha that
	// triggered said configupdate. The configloading in turn will pick
	// it up if present. This allows components to include the config version
	// in their logs, which can be useful for debugging.
	ConfigVersionFileName = "VERSION"

	DefaultTenantID = "GlobalDefaultID"

	ProwIgnoreFileName = ".prowignore"
)

// Config is a read-only snapshot of the config.
type Config struct {
	JobConfig
	ProwConfig
}

// JobConfig is config for all prow jobs.
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

	// ProwYAMLGetterWithDefaults is the function to get a ProwYAML with
	// defaults based on the rest of the Config. Tests should provide their own
	// implementation.
	ProwYAMLGetterWithDefaults ProwYAMLGetter `json:"-"`

	// ProwYAMLGetter is like ProwYAMLGetterWithDefaults, but does not default
	// the retrieved ProwYAML with defaulted values. It is mocked by
	// TestGetPresubmitsAndPostubmitsCached (and in production, prowYAMLGetter()
	// is used).
	ProwYAMLGetter ProwYAMLGetter `json:"-"`

	// DecorateAllJobs determines whether all jobs are decorated by default.
	DecorateAllJobs bool `json:"decorate_all_jobs,omitempty"`

	// ProwIgnored is a well known, unparsed field where non-Prow fields can
	// be defined without conflicting with unknown field validation.
	ProwIgnored *json.RawMessage `json:"prow_ignored,omitempty"`
}

// ProwConfig is config for all prow controllers.
type ProwConfig struct {
	// The git sha from which this config was generated.
	ConfigVersionSHA     string               `json:"config_version_sha,omitempty"`
	Tide                 Tide                 `json:"tide,omitempty"`
	Plank                Plank                `json:"plank,omitempty"`
	Sinker               Sinker               `json:"sinker,omitempty"`
	Deck                 Deck                 `json:"deck,omitempty"`
	BranchProtection     BranchProtection     `json:"branch-protection"`
	Gerrit               Gerrit               `json:"gerrit"`
	GitHubReporter       GitHubReporter       `json:"github_reporter"`
	Horologium           Horologium           `json:"horologium"`
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

	// OwnersDirDenylist is used to configure regular expressions matching directories
	// to ignore when searching for OWNERS{,_ALIAS} files in a repo.
	OwnersDirDenylist *OwnersDirDenylist `json:"owners_dir_denylist,omitempty"`

	// Pub/Sub Subscriptions that we want to listen to.
	PubSubSubscriptions PubsubSubscriptions `json:"pubsub_subscriptions,omitempty"`

	// PubSubTriggers defines Pub/Sub Subscriptions that we want to listen to,
	// can be used to restrict build cluster on a topic.
	PubSubTriggers PubSubTriggers `json:"pubsub_triggers,omitempty"`

	// GitHubOptions allows users to control how prow applications display GitHub website links.
	GitHubOptions GitHubOptions `json:"github,omitempty"`

	// StatusErrorLink is the url that will be used for jenkins prowJobs that can't be
	// found, or have another generic issue. The default that will be used if this is not set
	// is: https://github.com/kubernetes/test-infra/issues.
	StatusErrorLink string `json:"status_error_link,omitempty"`

	// DefaultJobTimeout this is default deadline for prow jobs. This value is used when
	// no timeout is configured at the job level. This value is set to 24 hours.
	DefaultJobTimeout *metav1.Duration `json:"default_job_timeout,omitempty"`

	// ManagedWebhooks contains information about all github repositories and organizations which are using
	// non-global Hmac token.
	ManagedWebhooks ManagedWebhooks `json:"managed_webhooks,omitempty"`

	// ProwJobDefaultEntries holds a list of defaults for specific values
	// Each entry in the slice specifies Repo and CLuster regexp filter fields to
	// match against the jobs and a corresponding ProwJobDefault . All entries that
	// match a job are used. Later matching entries override the fields of earlier
	// matching entires.
	ProwJobDefaultEntries []*ProwJobDefaultEntry `json:"prowjob_default_entries,omitempty"`
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

func SplitRepoName(fullRepoName string) (string, string, error) {
	// Gerrit org/repo contains https://, should be handled differently.
	if gerritsource.IsGerritOrg(fullRepoName) {
		return gerritsource.OrgRepoFromCloneURI(fullRepoName)
	}

	s := strings.SplitN(fullRepoName, "/", 2)
	if len(s) != 2 {
		return "", "", fmt.Errorf("repo %s cannot be split into org/repo", fullRepoName)
	}
	return s[0], s[1], nil
}

// InRepoConfigEnabled returns whether InRepoConfig is enabled for a given repository.
// There is no assumption that config will include http:// or https:// or not.
func (c *Config) InRepoConfigEnabled(identifier string) bool {
	for _, key := range keysForIdentifier(identifier) {
		if c.InRepoConfig.Enabled[key] != nil {
			return *c.InRepoConfig.Enabled[key]
		}
	}
	return false
}

// InRepoConfigAllowsCluster determines if a given cluster may be used for a given repository
// Assumes that config will not include http:// or https://
func (c *Config) InRepoConfigAllowsCluster(clusterName, identifier string) bool {
	for _, key := range keysForIdentifier(identifier) {
		for _, allowedCluster := range c.InRepoConfig.AllowedClusters[key] {
			if allowedCluster == clusterName {
				return true
			}
		}
	}
	return false
}

// keysForIdentifier returns all possible identifiers for given keys. In
// consideration of Gerrit identifiers that contain `https://` prefix, it
// returns keys contain both `https://foo/bar` and `foo/bar` for identifier
// `https://foo/bar`. The returned keys also include `https://foo`, `foo`, and
// `*`.
func keysForIdentifier(identifier string) []string {
	var candidates []string

	normalizedIdentifier := identifier
	if gerritsource.IsGerritOrg(identifier) {
		normalizedIdentifier = gerritsource.NormalizeCloneURI(identifier)
	}

	candidates = append(candidates, normalizedIdentifier)
	// gerritsource.TrimHTTPSPrefix(identifier) trims https:// prefix, it
	// doesn't hurt for identifier without https://
	candidates = append(candidates, gerritsource.TrimHTTPSPrefix(identifier))

	org, _, _ := SplitRepoName(normalizedIdentifier)
	// Errors if failed to split. We are ignoring this and just checking if org != "" instead.
	if org != "" {
		candidates = append(candidates, org)
		// gerritsource.TrimHTTPSPrefix(identifier) trims https:// prefix, it
		// doesn't hurt for identifier without https://
		candidates = append(candidates, gerritsource.TrimHTTPSPrefix(org))
	}

	candidates = append(candidates, "*")

	var res []string
	visited := sets.NewString()
	for _, cand := range candidates {
		if visited.Has(cand) {
			continue
		}
		res = append(res, cand)
		visited.Insert(cand)
	}

	return res
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

// NewRefGetterForGitHubPullRequest returns a brand new RefGetterForGitHubPullRequest.
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

// HeadSHA is a RefGetter that returns the headSHA for the PullRequest.
func (rg *RefGetterForGitHubPullRequest) HeadSHA() (string, error) {
	if rg.pr == nil {
		if _, err := rg.PullRequest(); err != nil {
			return "", err
		}
	}
	return rg.pr.Head.SHA, nil
}

// BaseSHA is a RefGetter that returns the baseRef for the PullRequest.
func (rg *RefGetterForGitHubPullRequest) BaseSHA() (string, error) {
	if rg.pr == nil {
		if _, err := rg.PullRequest(); err != nil {
			return "", err
		}
	}

	// rg.PullRequest also wants the lock, so we must not acquire it before
	// calling that.
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

// GetAndCheckRefs resolves all uniquely-identifying information related to the
// retrieval of a *ProwYAML.
func GetAndCheckRefs(
	baseSHAGetter RefGetter,
	headSHAGetters ...RefGetter) (string, []string, error) {

	// Parse "baseSHAGetter".
	baseSHA, err := baseSHAGetter()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get baseSHA: %v", err)
	}

	// Parse "headSHAGetters".
	var headSHAs []string
	for _, headSHAGetter := range headSHAGetters {
		headSHA, err := headSHAGetter()
		if err != nil {
			return "", nil, fmt.Errorf("failed to get headRef: %v", err)
		}
		if headSHA != "" {
			headSHAs = append(headSHAs, headSHA)
		}
	}

	return baseSHA, headSHAs, nil
}

// getProwYAMLWithDefaults will load presubmits and postsubmits for the given
// identifier that are versioned inside the tested repo, if the inrepoconfig
// feature is enabled. Consumers that pass in a RefGetter implementation that
// does a call to GitHub and who also need the result of that GitHub call just
// keep a pointer to its result, but must nilcheck that pointer before accessing
// it.
func (c *Config) getProwYAMLWithDefaults(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {
	if identifier == "" {
		return nil, errors.New("no identifier for repo given")
	}
	if !c.InRepoConfigEnabled(identifier) {
		return &ProwYAML{}, nil
	}

	baseSHA, headSHAs, err := GetAndCheckRefs(baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	prowYAML, err := c.ProwYAMLGetterWithDefaults(c, gc, identifier, baseSHA, headSHAs...)
	if err != nil {
		return nil, err
	}

	return prowYAML, nil
}

// getProwYAML is like getProwYAMLWithDefaults, minus the defaulting logic.
func (c *Config) getProwYAML(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {
	if identifier == "" {
		return nil, errors.New("no identifier for repo given")
	}
	if !c.InRepoConfigEnabled(identifier) {
		return &ProwYAML{}, nil
	}

	baseSHA, headSHAs, err := GetAndCheckRefs(baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
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
	prowYAML, err := c.getProwYAMLWithDefaults(gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	return append(c.GetPresubmitsStatic(identifier), prowYAML.Presubmits...), nil
}

// GetPresubmitsStatic will return presubmits for the given identifier that are versioned inside the tested repo.
func (c *Config) GetPresubmitsStatic(identifier string) []Presubmit {
	keys := []string{identifier}
	if gerritsource.IsGerritOrg(identifier) {
		// For Gerrit, allow users to define jobs without https:// prefix, which
		// is what's supported right now.
		keys = append(keys, gerritsource.TrimHTTPSPrefix(identifier))
	}
	var res []Presubmit
	for _, key := range keys {
		res = append(res, c.PresubmitsStatic[key]...)
	}
	return res
}

// GetPostsubmits will return all postsubmits for the given identifier. This includes
// Postsubmits that are versioned inside the tested repo, if the inrepoconfig feature
// is enabled.
// Consumers that pass in a RefGetter implementation that does a call to GitHub and who
// also need the result of that GitHub call just keep a pointer to its result, but must
// nilcheck that pointer before accessing it.
func (c *Config) GetPostsubmits(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error) {
	prowYAML, err := c.getProwYAMLWithDefaults(gc, identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		return nil, err
	}

	return append(c.GetPostsubmitsStatic(identifier), prowYAML.Postsubmits...), nil
}

// GetPostsubmitsStatic will return postsubmits for the given identifier that are versioned inside the tested repo.
func (c *Config) GetPostsubmitsStatic(identifier string) []Postsubmit {
	keys := []string{identifier}
	if gerritsource.IsGerritOrg(identifier) {
		// For Gerrit, allow users to define jobs without https:// prefix, which
		// is what's supported right now.
		keys = append(keys, gerritsource.TrimHTTPSPrefix(identifier))
	}
	var res []Postsubmit
	for _, key := range keys {
		res = append(res, c.PostsubmitsStatic[key]...)
	}
	return res
}

// OwnersDirDenylist is used to configure regular expressions matching directories
// to ignore when searching for OWNERS{,_ALIAS} files in a repo.
type OwnersDirDenylist struct {
	// Repos configures a directory denylist per repo (or org).
	Repos map[string][]string `json:"repos,omitempty"`
	// Default configures a default denylist for all repos (or orgs).
	// Some directories like ".git", "_output" and "vendor/.*/OWNERS"
	// are already preconfigured to be denylisted, and need not be included here.
	Default []string `json:"default,omitempty"`
	// By default, some directories like ".git", "_output" and "vendor/.*/OWNERS"
	// are preconfigured to be denylisted.
	// If set, IgnorePreconfiguredDefaults will not add these preconfigured directories
	// to the denylist.
	IgnorePreconfiguredDefaults bool `json:"ignore_preconfigured_defaults,omitempty"`
}

// ListIgnoredDirs returns regular expressions matching directories to ignore when
// searching for OWNERS{,_ALIAS} files in a repo.
func (o OwnersDirDenylist) ListIgnoredDirs(org, repo string) (ignorelist []string) {
	ignorelist = append(ignorelist, o.Default...)
	if bl, ok := o.Repos[org]; ok {
		ignorelist = append(ignorelist, bl...)
	}
	if bl, ok := o.Repos[org+"/"+repo]; ok {
		ignorelist = append(ignorelist, bl...)
	}

	preconfiguredDefaults := []string{"\\.git$", "_output$", "vendor/.*/.*"}
	if !o.IgnorePreconfiguredDefaults {
		ignorelist = append(ignorelist, preconfiguredDefaults...)
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
	// ServeMetrics tells if or not the components serve metrics.
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
	// collection on pending pods. Defaults to 10 minutes.
	PodPendingTimeout *metav1.Duration `json:"pod_pending_timeout,omitempty"`
	// PodRunningTimeout is after how long the controller will abort a prowjob pod
	// stuck in running state. Defaults to two days.
	PodRunningTimeout *metav1.Duration `json:"pod_running_timeout,omitempty"`
	// PodUnscheduledTimeout is after how long the controller will abort a prowjob
	// stuck in an unscheduled state. Defaults to 5 minutes.
	PodUnscheduledTimeout *metav1.Duration `json:"pod_unscheduled_timeout,omitempty"`

	// DefaultDecorationConfigs holds the default decoration config for specific values.
	//
	// Each entry in the slice specifies Repo and Cluster regexp filter fields to
	// match against jobs and a corresponding DecorationConfig. All entries that
	// match a job are used. Later matching entries override the fields of earlier
	// matching entries.
	//
	// In FinalizeDefaultDecorationConfigs(), this field is populated either directly from
	// DefaultDecorationConfigEntries, or from DefaultDecorationConfigsMap after
	// it is converted to a slice. These fields are mutually exclusive, and
	// defining both is an error.
	DefaultDecorationConfigs []*DefaultDecorationConfigEntry `json:"-"`
	// DefaultDecorationConfigsMap is a mapping from 'org', 'org/repo', or the
	// literal string '*', to the default decoration config to use for that key.
	// The '*' key matches all jobs. (Periodics use extra_refs[0] for matching
	// if present.)
	//
	// This field is mutually exclusive with the DefaultDecorationConfigEntries field.
	DefaultDecorationConfigsMap map[string]*prowapi.DecorationConfig `json:"default_decoration_configs,omitempty"`
	// DefaultDecorationConfigEntries is used to populate DefaultDecorationConfigs.
	//
	// Each entry in the slice specifies Repo and Cluster regexp filter fields to
	// match against jobs and a corresponding DecorationConfig. All entries that
	// match a job are used. Later matching entries override the fields of earlier
	// matching entries.
	//
	// This field is smarter than the DefaultDecorationConfigsMap, because each
	// entry includes additional Cluster regexp information that the old format
	// does not consider.
	//
	// This field is mutually exclusive with the DefaultDecorationConfigsMap field.
	DefaultDecorationConfigEntries []*DefaultDecorationConfigEntry `json:"default_decoration_config_entries,omitempty"`

	// JobURLPrefixConfig is the host and path prefix under which job details
	// will be viewable. Use `org/repo`, `org` or `*`as key and an url as value.
	JobURLPrefixConfig map[string]string `json:"job_url_prefix_config,omitempty"`

	// JobURLPrefixDisableAppendStorageProvider disables that the storageProvider is
	// automatically appended to the JobURLPrefix.
	JobURLPrefixDisableAppendStorageProvider bool `json:"jobURLPrefixDisableAppendStorageProvider,omitempty"`

	// BuildClusterStatusFile is an optional field used to specify the blob storage location
	// to publish cluster status information.
	// e.g. gs://my-bucket/cluster-status.json
	BuildClusterStatusFile string `json:"build_cluster_status_file,omitempty"`

	// JobQueueConcurrencies is an optional field used to define job queue max concurrency.
	// Each job can be assigned to a specific queue which has its own max concurrency,
	// independent from the job's name. Setting the concurrency to 0 will block any job
	// from being triggered. Setting the concurrency to a negative value will remove the
	// limit. An example use case would be easier scheduling of jobs using boskos resources.
	// This mechanism is separate from ProwJob's MaxConcurrency setting.
	JobQueueConcurrencies map[string]int `json:"job_queue_capacities,omitempty"`
}

type ProwJobDefaultEntry struct {
	// Matching/filtering fields. All filters must match for an entry to match.

	// OrgRepo matches against the "org" or "org/repo" that the presubmit or postsubmit
	// is associated with. If the job is a periodic, extra_refs[0] is used. If the
	// job is a periodic without extra_refs, the empty string will be used.
	// If this field is omitted all jobs will match.
	OrgRepo string `json:"repo,omitempty"`
	// Cluster matches against the cluster alias of the build cluster that the
	// ProwJob is configured to run on. Recall that ProwJobs default to running on
	// the "default" build cluster if they omit the "cluster" field in config.
	Cluster string `json:"cluster,omitempty"`

	// Config is the ProwJobDefault to apply if the filter fields all match the
	// ProwJob. Note that when multiple entries match a ProwJob they are all used
	// by sequentially merging with later entries overriding fields from earlier
	// entries.
	Config *prowapi.ProwJobDefault `json:"config,omitempty"`
}

// DefaultDecorationConfigEntry contains a DecorationConfig and a set of
// regexps. If the regexps here match a ProwJob, then that ProwJob uses defaults
// by looking the DecorationConfig defined here in this entry.
//
// If multiple entries match a single ProwJob, the multiple entries'
// DecorationConfigs are merged, with later entries overriding values from
// earlier entries. Then finally that merged DecorationConfig is used by the
// matching ProwJob.
type DefaultDecorationConfigEntry struct {
	// Matching/filtering fields. All filters must match for an entry to match.

	// OrgRepo matches against the "org" or "org/repo" that the presubmit or postsubmit
	// is associated with. If the job is a periodic, extra_refs[0] is used. If the
	// job is a periodic without extra_refs, the empty string will be used.
	// If this field is omitted all jobs will match.
	OrgRepo string `json:"repo,omitempty"`
	// Cluster matches against the cluster alias of the build cluster that the
	// ProwJob is configured to run on. Recall that ProwJobs default to running on
	// the "default" build cluster if they omit the "cluster" field in config.
	Cluster string `json:"cluster,omitempty"`

	// Config is the DecorationConfig to apply if the filter fields all match the
	// ProwJob. Note that when multiple entries match a ProwJob they are all used
	// by sequentially merging with later entries overriding fields from earlier
	// entries.
	Config *prowapi.DecorationConfig `json:"config,omitempty"`
}

// TODO(mpherman): Make a Matcher struct embedded in both ProwJobDefaultEntry and
// DefaultDecorationConfigEntry and DefaultRerunAuthConfigEntry.
func matches(givenOrgRepo, givenCluster, orgRepo, cluster string) bool {
	orgRepoMatch := givenOrgRepo == "" || givenOrgRepo == "*" || givenOrgRepo == strings.Split(orgRepo, "/")[0] || givenOrgRepo == orgRepo
	clusterMatch := givenCluster == "" || givenCluster == "*" || givenCluster == cluster
	return orgRepoMatch && clusterMatch
}

// matches returns true iff all the filters for the entry match a job.
func (d *ProwJobDefaultEntry) matches(repo, cluster string) bool {
	return matches(d.OrgRepo, d.Cluster, repo, cluster)
}

// matches returns true iff all the filters for the entry match a job.
func (d *DefaultDecorationConfigEntry) matches(repo, cluster string) bool {
	return matches(d.OrgRepo, d.Cluster, repo, cluster)
}

// matches returns true iff all the filters for the entry match a job.
func (d *DefaultRerunAuthConfigEntry) matches(repo, cluster string) bool {
	return matches(d.OrgRepo, d.Cluster, repo, cluster)
}

// mergeProwJobDefault finds all matching ProwJobDefaultEntry
// for a job and merges them sequentially before merging into the job's own
// PrwoJobDefault. Configs merged later override values from earlier configs.
func (pc *ProwConfig) mergeProwJobDefault(repo, cluster string, jobDefault *prowapi.ProwJobDefault) *prowapi.ProwJobDefault {
	var merged *prowapi.ProwJobDefault
	for _, entry := range pc.ProwJobDefaultEntries {
		if entry.matches(repo, cluster) {
			merged = entry.Config.ApplyDefault(merged)
		}
	}
	merged = jobDefault.ApplyDefault(merged)
	if merged == nil {
		merged = &prowapi.ProwJobDefault{}
	}
	if merged.TenantID == "" {
		merged.TenantID = DefaultTenantID
	}
	return merged
}

// mergeDefaultDecorationConfig finds all matching DefaultDecorationConfigEntry
// for a job and merges them sequentially before merging into the job's own
// DecorationConfig. Configs merged later override values from earlier configs.
func (p *Plank) mergeDefaultDecorationConfig(repo, cluster string, jobDC *prowapi.DecorationConfig) *prowapi.DecorationConfig {
	var merged *prowapi.DecorationConfig
	for _, entry := range p.DefaultDecorationConfigs {
		if entry.matches(repo, cluster) {
			merged = entry.Config.ApplyDefault(merged)
		}
	}
	merged = jobDC.ApplyDefault(merged)
	if merged == nil {
		merged = &prowapi.DecorationConfig{}
	}
	return merged
}

// GetProwJobDefault finds the resolved prowJobDefault config for a given repo and
// cluster.
func (c *Config) GetProwJobDefault(repo, cluster string) *prowapi.ProwJobDefault {
	return c.mergeProwJobDefault(repo, cluster, nil)
}

// GuessDefaultDecorationConfig attempts to find the resolved default decoration
// config for a given repo and cluster. It is primarily used for best effort
// guesses about GCS configuration for undecorated jobs.
func (p *Plank) GuessDefaultDecorationConfig(repo, cluster string) *prowapi.DecorationConfig {
	return p.mergeDefaultDecorationConfig(repo, cluster, nil)
}

// GuessDefaultDecorationConfig attempts to find the resolved default decoration
// config for a given repo, cluster and job DecorationConfig. It is primarily used for best effort
// guesses about GCS configuration for undecorated jobs.
func (p *Plank) GuessDefaultDecorationConfigWithJobDC(repo, cluster string, jobDC *prowapi.DecorationConfig) *prowapi.DecorationConfig {
	return p.mergeDefaultDecorationConfig(repo, cluster, jobDC)
}

// defaultDecorationMapToSlice converts the old format
// (map[string]*prowapi.DecorationConfig) to the new format
// ([]*DefaultDecorationConfigEntry).
func defaultDecorationMapToSlice(m map[string]*prowapi.DecorationConfig) []*DefaultDecorationConfigEntry {
	var entries []*DefaultDecorationConfigEntry
	add := func(repo string, dc *prowapi.DecorationConfig) {
		entries = append(entries, &DefaultDecorationConfigEntry{
			OrgRepo: repo,
			Cluster: "",
			Config:  dc,
		})
	}
	// Ensure "*" comes first...
	if dc, ok := m["*"]; ok {
		add("*", dc)
	}
	// then orgs...
	for key, dc := range m {
		if key == "*" || strings.Contains(key, "/") {
			continue
		}
		add(key, dc)
	}
	// then repos.
	for key, dc := range m {
		if key == "*" || !strings.Contains(key, "/") {
			continue
		}
		add(key, dc)
	}
	return entries
}

// DefaultDecorationMapToSliceTesting is a convenience function that is exposed
// to allow unit tests to convert the old map format to the new slice format.
// It should only be used in testing.
func DefaultDecorationMapToSliceTesting(m map[string]*prowapi.DecorationConfig) []*DefaultDecorationConfigEntry {
	return defaultDecorationMapToSlice(m)
}

// FinalizeDefaultDecorationConfigs prepares the entries of
// Plank.DefaultDecorationConfigs for use in finalizing the job config.
// It sets p.DefaultDecorationConfigs into either the old map
// format or the new slice format:
// Old format: map[string]*prowapi.DecorationConfig where the key is org,
//             org/repo, or "*".
// New format: []*DefaultDecorationConfigEntry
// If the old format is parsed it is converted to the new format, then all
// filter regexp are compiled.
func (p *Plank) FinalizeDefaultDecorationConfigs() error {
	mapped, sliced := len(p.DefaultDecorationConfigsMap) > 0, len(p.DefaultDecorationConfigEntries) > 0
	if mapped && sliced {
		return fmt.Errorf("plank.default_decoration_configs and plank.default_decoration_config_entries are mutually exclusive, please use one or the other")
	}
	if mapped {
		p.DefaultDecorationConfigs = defaultDecorationMapToSlice(p.DefaultDecorationConfigsMap)
	} else {
		p.DefaultDecorationConfigs = p.DefaultDecorationConfigEntries
	}
	return nil
}

// GetJobURLPrefix gets the job url prefix from the config
// for the given refs.
func (p Plank) GetJobURLPrefix(pj *prowapi.ProwJob) string {
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
	// TickInterval is how often we do a sync with binded gerrit instance.
	TickInterval *metav1.Duration `json:"tick_interval,omitempty"`
	// RateLimit defines how many changes to query per gerrit API call
	// default is 5.
	RateLimit int `json:"ratelimit,omitempty"`
	// DeckURL is the root URL of Deck. This is used to construct links to
	// job runs for a given CL.
	DeckURL        string                `json:"deck_url,omitempty"`
	OrgReposConfig *GerritOrgRepoConfigs `json:"org_repos_config,omitempty"`
}

// GerritOrgRepoConfigs is config for repos.
type GerritOrgRepoConfigs []GerritOrgRepoConfig

// GerritOrgRepoConfig is config for repos.
type GerritOrgRepoConfig struct {
	// Org is the name of the Gerrit instance/host. It's required to keep the
	// https:// or http:// prefix.
	Org string `json:"org,omitempty"`
	// Repos are a slice of repos under the `Org`.
	Repos []string `json:"repos,omitempty"`
	// OptOutHelp is the flag for determining whether the repos defined under
	// here opting out of help or not. If this is true, Prow will not command
	// the help message with comments like `/test ?`, `/retest ?`, `/test
	// job-not-exist`, `/test job-only-available-from-another-prow`.
	OptOutHelp bool `json:"opt_out_help,omitempty"`
	// Filters are used for limiting the scope of querying the Gerrit server.
	// Currently supports branches and excluded branches.
	Filters *GerritQueryFilter `json:"filters,omitempty"`
}

type GerritQueryFilter struct {
	Branches         []string `json:"branches,omitempty"`
	ExcludedBranches []string `json:"excluded_branches,omitempty"`
	// OptInByDefault indicates that all of the PRs are considered by Tide from
	// these repos, unless `Prow-Auto-Submit` label is voted -1.
	OptInByDefault bool `json:"opt_in_by_default,omitempty"`
}

func (goc *GerritOrgRepoConfigs) AllRepos() map[string]map[string]*GerritQueryFilter {
	var res map[string]map[string]*GerritQueryFilter
	for _, orgConfig := range *goc {
		if res == nil {
			res = make(map[string]map[string]*GerritQueryFilter)
		}
		for _, repo := range orgConfig.Repos {
			if _, ok := res[orgConfig.Org]; !ok {
				res[orgConfig.Org] = make(map[string]*GerritQueryFilter)
			}
			res[orgConfig.Org][repo] = orgConfig.Filters
		}
	}
	return res
}

func (goc *GerritOrgRepoConfigs) OptOutHelpRepos() map[string]sets.String {
	var res map[string]sets.String
	for _, orgConfig := range *goc {
		if !orgConfig.OptOutHelp {
			continue
		}
		if res == nil {
			res = make(map[string]sets.String)
		}
		res[orgConfig.Org] = res[orgConfig.Org].Union(sets.NewString(orgConfig.Repos...))
	}
	return res
}

// Horologium is config for the Horologium.
type Horologium struct {
	// TickInterval is the interval in which we check if new jobs need to be
	// created. Defaults to one minute.
	TickInterval *metav1.Duration `json:"tick_interval,omitempty"`
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

// GitHubReporter holds the config for report behavior in github.
type GitHubReporter struct {
	// JobTypesToReport is used to determine which type of prowjob
	// should be reported to github.
	//
	// defaults to both presubmit and postsubmit jobs.
	JobTypesToReport []prowapi.ProwJobType `json:"job_types_to_report,omitempty"`
	// NoCommentRepos is a list of orgs and org/repos for which failure report
	// comments should not be maintained. Status contexts will still be written.
	NoCommentRepos []string `json:"no_comment_repos,omitempty"`
	// SummaryCommentRepos is a list of orgs and org/repos for which failure report
	// comments is only sent when all jobs from current SHA are finished. Status
	// contexts will still be written.
	SummaryCommentRepos []string `json:"summary_comment_repos,omitempty"`
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
	// ExcludeClusters are build clusters that don't want to be managed by sinker.
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
	// RemoteConfig specifies how to access remote lenses.
	RemoteConfig *LensRemoteConfig `json:"remote_config,omitempty"`
}

// LensRemoteConfig is the configuration for a remote lens.
type LensRemoteConfig struct {
	// The endpoint for the lense.
	Endpoint string `json:"endpoint"`
	// The parsed endpoint.
	ParsedEndpoint *url.URL `json:"-"`
	// The endpoint for static resources.
	StaticRoot string `json:"static_root"`
	// The human-readable title for the lens.
	Title string `json:"title"`
	// Priority for lens ordering, lowest priority first.
	Priority *uint `json:"priority"`
	// HideTitle defines if we will keep showing the title after lens loads.
	HideTitle *bool `json:"hide_title"`
}

// Spyglass holds config for Spyglass.
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
	// GCSBrowserPrefixesByRepo are used to generate a link to a human-usable GCS browser.
	// They are mapped by org, org/repo or '*' which is the default value.
	// These are the most specific and will override GCSBrowserPrefixesByBucket if both are resolved.
	GCSBrowserPrefixesByRepo GCSBrowserPrefixes `json:"gcs_browser_prefixes,omitempty"`
	// GCSBrowserPrefixesByBucket are used to generate a link to a human-usable GCS browser.
	// They are mapped by bucket name or '*' which is the default value.
	// They will only be utilized if there is not a GCSBrowserPrefixesByRepo for the org/repo.
	GCSBrowserPrefixesByBucket GCSBrowserPrefixes `json:"gcs_browser_prefixes_by_bucket,omitempty"`
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
	// HidePRHistLink allows prow hiding PR History link from deck, this is handy especially for
	// prow instances that only serves gerrit.
	// This might become obsolete once https://github.com/kubernetes/test-infra/issues/24130 is fixed.
	HidePRHistLink bool `json:"hide_pr_history_link,omitempty"`
	// PRHistLinkTemplate is the template for constructing href of `PR History` button,
	// by default it's "/pr-history?org={{.Org}}&repo={{.Repo}}&pr={{.Number}}"
	PRHistLinkTemplate string `json:"pr_history_link_template,omitempty"`
}

type GCSBrowserPrefixes map[string]string

// GetGCSBrowserPrefix determines the GCS Browser prefix by checking for a config in order of:
//   1. If org (and optionally repo) is provided resolve the GCSBrowserPrefixesByRepo config.
//   2. If bucket is provided resolve the GCSBrowserPrefixesByBucket config.
//   3. If not found in either use the default from GCSBrowserPrefixesByRepo or GCSBrowserPrefixesByBucket if not found.
func (s Spyglass) GetGCSBrowserPrefix(org, repo, bucket string) string {
	if org != "" {
		if prefix, ok := s.GCSBrowserPrefixesByRepo[fmt.Sprintf("%s/%s", org, repo)]; ok {
			return prefix
		}
		if prefix, ok := s.GCSBrowserPrefixesByRepo[org]; ok {
			return prefix
		}
	}
	if bucket != "" {
		if prefix, ok := s.GCSBrowserPrefixesByBucket[bucket]; ok {
			return prefix
		}
	}

	// If we don't find anything specific use the default by repo, if that isn't present use the default by bucket.
	if prefix, ok := s.GCSBrowserPrefixesByRepo["*"]; ok {
		return prefix
	}

	return s.GCSBrowserPrefixesByBucket["*"]
}

// Deck holds config for deck.
type Deck struct {
	// Spyglass specifies which viewers will be used for which artifacts when viewing a job in Deck.
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
	// RerunAuthConfigs is not deprecated but DefaultRerunAuthConfigs should be used in favor.
	// It remains a part of Deck for the purposes of backwards compatibility.
	// RerunAuthConfigs is a map of configs that specify who is able to trigger job reruns. The field
	// accepts a key of: `org/repo`, `org` or `*` (wildcard) to define what GitHub org (or repo) a particular
	// config applies to and a value of: `RerunAuthConfig` struct to define the users/groups authorized to rerun jobs.
	RerunAuthConfigs RerunAuthConfigs `json:"rerun_auth_configs,omitempty"`
	// DefaultRerunAuthConfigs is a list of DefaultRerunAuthConfigEntry structures that specify who can
	// trigger job reruns. Reruns are based on whether the entry's org/repo or cluster matches with the
	// expected fields in the given configuration.
	//
	// Each entry in the slice specifies Repo and Cluster regexp filter fields to
	// match against jobs and a corresponding RerunAuthConfig. The entry matching the job with the
	// most specification is for authentication purposes.
	//
	// This field is smarter than the RerunAuthConfigs, because each
	// entry includes additional Cluster regexp information that the old format
	// does not consider.
	//
	// This field is mutually exclusive with the RerunAuthConfigs field.
	DefaultRerunAuthConfigs []*DefaultRerunAuthConfigEntry `json:"default_rerun_auth_configs,omitempty"`
	// SkipStoragePathValidation skips validation that restricts artifact requests to specific buckets.
	// By default, buckets listed in the GCSConfiguration are automatically allowed.
	// Additional locations can be allowed via `AdditionalAllowedBuckets` fields.
	// When unspecified (nil), it defaults to false
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
	if len(d.AdditionalAllowedBuckets) > 0 && !d.shouldValidateStorageBuckets() {
		return fmt.Errorf("deck.skip_storage_path_validation is enabled despite deck.additional_allowed_buckets being configured: %v", d.AdditionalAllowedBuckets)
	}

	for k, config := range d.DefaultRerunAuthConfigs {
		if err := config.Config.Validate(); err != nil {
			return fmt.Errorf("default_rerun_auth_configs[%d]: %w", k, err)
		}
	}

	return nil
}

type notAllowedBucketError struct {
	err error
}

func (ne notAllowedBucketError) Error() string {
	return fmt.Sprintf("bucket not in allowed list; you may allow it by including it in `deck.additional_allowed_buckets`: %s", ne.err.Error())
}

func (notAllowedBucketError) Is(err error) bool {
	_, ok := err.(notAllowedBucketError)
	return ok
}

// NotAllowedBucketError wraps an error and return a notAllowedBucketError error.
func NotAllowedBucketError(err error) error {
	return &notAllowedBucketError{err: err}
}

func IsNotAllowedBucketError(err error) bool {
	return errors.Is(err, notAllowedBucketError{})
}

// ValidateStorageBucket validates a storage bucket (unless the `Deck.SkipStoragePathValidation` field is true).
// The bucket name must be included in any of the following:
//    1) Any job's `.DecorationConfig.GCSConfiguration.Bucket` (except jobs defined externally via InRepoConfig).
//    2) `Plank.DefaultDecorationConfigs.GCSConfiguration.Bucket`.
//    3) `Deck.AdditionalAllowedBuckets`.
func (c *Config) ValidateStorageBucket(bucketName string) error {
	if !c.Deck.shouldValidateStorageBuckets() {
		return nil
	}

	if !c.Deck.AllKnownStorageBuckets.Has(bucketName) {
		return NotAllowedBucketError(fmt.Errorf("bucket %q not in allowed list (%v)", bucketName, c.Deck.AllKnownStorageBuckets.List()))
	}
	return nil
}

// shouldValidateStorageBuckets returns whether or not the Deck's storage path should be validated.
// Validation could be either enabled by default or explicitly turned off.
func (d *Deck) shouldValidateStorageBuckets() bool {
	if d.SkipStoragePathValidation == nil {
		return false
	}
	return !*d.SkipStoragePathValidation
}

func calculateStorageBuckets(c *Config) sets.String {
	knownBuckets := sets.NewString(c.Deck.AdditionalAllowedBuckets...)
	for _, dc := range c.Plank.DefaultDecorationConfigs {
		if dc.Config != nil && dc.Config.GCSConfiguration != nil && dc.Config.GCSConfiguration.Bucket != "" {
			knownBuckets.Insert(stripProviderPrefixFromBucket(dc.Config.GCSConfiguration.Bucket))
		}
	}
	for _, j := range c.Periodics {
		if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
			knownBuckets.Insert(stripProviderPrefixFromBucket(j.DecorationConfig.GCSConfiguration.Bucket))
		}
	}
	for _, jobs := range c.PresubmitsStatic {
		for _, j := range jobs {
			if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
				knownBuckets.Insert(stripProviderPrefixFromBucket(j.DecorationConfig.GCSConfiguration.Bucket))
			}
		}
	}
	for _, jobs := range c.PostsubmitsStatic {
		for _, j := range jobs {
			if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
				knownBuckets.Insert(stripProviderPrefixFromBucket(j.DecorationConfig.GCSConfiguration.Bucket))
			}
		}
	}
	return knownBuckets
}

func stripProviderPrefixFromBucket(bucket string) string {
	if split := strings.Split(bucket, "://"); len(split) == 2 {
		return split[1]
	}
	return bucket
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
	// see https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors.
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

type DefaultRerunAuthConfigEntry struct {
	// Matching/filtering fields. All filters must match for an entry to match.

	// OrgRepo matches against the "org" or "org/repo" that the presubmit or postsubmit
	// is associated with. If the job is a periodic, extra_refs[0] is used. If the
	// job is a periodic without extra_refs, the empty string will be used.
	// If this field is omitted all jobs will match.
	OrgRepo string `json:"repo,omitempty"`
	// Cluster matches against the cluster alias of the build cluster that the
	// ProwJob is configured to run on. Recall that ProwJobs default to running on
	// the "default" build cluster if they omit the "cluster" field in config.
	Cluster string `json:"cluster,omitempty"`

	// Config is the RerunAuthConfig to apply if the filter fields all match the
	// ProwJob. Note that when multiple entries match a ProwJob the entry with the
	// highest specification is used.
	Config *prowapi.RerunAuthConfig `json:"rerun_auth_configs,omitempty"`
}

func (d *Deck) GetRerunAuthConfig(jobSpec *prowapi.ProwJobSpec) *prowapi.RerunAuthConfig {
	var config *prowapi.RerunAuthConfig

	var orgRepo string
	if jobSpec.Refs != nil {
		orgRepo = jobSpec.Refs.OrgRepoString()
	} else if len(jobSpec.ExtraRefs) > 0 {
		orgRepo = jobSpec.ExtraRefs[0].OrgRepoString()
	}

	for _, drac := range d.DefaultRerunAuthConfigs {
		if drac.matches(orgRepo, jobSpec.Cluster) {
			config = drac.Config
		}
	}

	return config
}

// defaultRerunAuthMapToSlice converts the old format
// (map[string]*prowapi.RerunAuthConfig) to the new format
// ([]*DefaultRerunAuthConfigEntry) or DefaultRerunAuthConfigs.
func defaultRerunAuthMapToSlice(m map[string]prowapi.RerunAuthConfig) ([]*DefaultRerunAuthConfigEntry, error) {
	mLength := len(m)
	var entries []*DefaultRerunAuthConfigEntry
	add := func(repo string, rac prowapi.RerunAuthConfig) {
		entries = append(entries, &DefaultRerunAuthConfigEntry{
			OrgRepo: repo,
			Cluster: "",
			Config:  &rac,
		})
	}

	// Ensure "" comes first...
	if rac, ok := m[""]; ok {
		add("", rac)
		delete(m, "")
	}
	// Ensure "*" comes first...
	if rac, ok := m["*"]; ok {
		add("*", rac)
		delete(m, "*")
	}
	// then orgs...
	for key, rac := range m {
		if strings.Contains(key, "/") {
			continue
		}
		add(key, rac)
		delete(m, key)
	}
	// then repos.
	for key, rac := range m {
		add(key, rac)
	}

	if mLength != len(entries) {
		return nil, fmt.Errorf("deck.rerun_auth_configs and deck.default_rerun_auth_configs are mutually exclusive, please use one or the other")
	}

	return entries, nil
}

// FinalizeDefaultRerunAuthConfigs prepares the entries of
// Deck.DefaultRerunAuthConfigs for use in finalizing the job config.
// It parses either d.RerunAuthConfigs or d.DefaultRerunAuthConfigEntries, not both.
// Old format: map[string]*prowapi.RerunAuthConfig where the key is org,
//             org/repo, or "*".
// New format: []*DefaultRerunAuthConfigEntry
// If the old format is parsed it is converted to the new format, then all
// filter regexp are compiled.
func (d *Deck) FinalizeDefaultRerunAuthConfigs() error {
	mapped, sliced := len(d.RerunAuthConfigs) > 0, len(d.DefaultRerunAuthConfigs) > 0

	// This case should be guarded against by prow config test. Checking here is
	// for cases where prow config test didn't run.
	if mapped && sliced {
		return fmt.Errorf("deck.rerun_auth_configs and deck.default_rerun_auth_configs are mutually exclusive, please use one or the other")
	}

	// Set up DefaultRerunAuthConfigEntries.
	if mapped {
		var err error
		d.DefaultRerunAuthConfigs, err = defaultRerunAuthMapToSlice(d.RerunAuthConfigs)
		if err != nil {
			return err
		}
	}

	return nil
}

const (
	defaultMaxOutstandingMessages = 10
)

// PubsubSubscriptions maps GCP project IDs to a list of subscription IDs.
type PubsubSubscriptions map[string][]string

// PubSubTriggers contains pubsub configurations.
type PubSubTriggers []PubSubTrigger

// PubSubTrigger contain pubsub configuration for a single project.
type PubSubTrigger struct {
	Project         string   `json:"project"`
	Topics          []string `json:"topics"`
	AllowedClusters []string `json:"allowed_clusters"`
	// MaxOutstandingMessages is the max number of messaged being processed, default is 10.
	MaxOutstandingMessages int `json:"max_outstanding_messages"`
}

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
	RespectLegacyGlobalToken bool `json:"respect_legacy_global_token"`
	// Controls whether org/repo invitation for prow bot should be automatically
	// accepted or not. Only admin level invitations related to orgs and repos
	// in the managed_webhooks config will be accepted and all other invitations
	// will be left pending.
	AutoAcceptInvitation bool                          `json:"auto_accept_invitation"`
	OrgRepoConfig        map[string]ManagedWebhookInfo `json:"org_repo_config,omitempty"`
}

// SlackReporter represents the config for the Slack reporter. The channel can be overridden
// on the job via the .reporter_config.slack.channel property.
type SlackReporter struct {
	JobTypesToReport            []prowapi.ProwJobType `json:"job_types_to_report,omitempty"`
	prowapi.SlackReporterConfig `json:",inline"`
}

// SlackReporterConfigs represents the config for the Slack reporter(s).
// Use `org/repo`, `org` or `*` as key and an `SlackReporter` struct as value.
type SlackReporterConfigs map[string]SlackReporter

func (cfg SlackReporterConfigs) mergeFrom(additional *SlackReporterConfigs) error {
	if additional == nil {
		return nil
	}

	var errs []error
	for orgOrRepo, slackReporter := range *additional {
		if _, alreadyConfigured := cfg[orgOrRepo]; alreadyConfigured {
			errs = append(errs, fmt.Errorf("config for org or repo %s passed more than once", orgOrRepo))
			continue
		}
		cfg[orgOrRepo] = slackReporter
	}

	return utilerrors.NewAggregate(errs)
}

func (cfg SlackReporterConfigs) GetSlackReporter(refs *prowapi.Refs) SlackReporter {
	if refs == nil {
		return cfg["*"]
	}

	if slack, ok := cfg[fmt.Sprintf("%s/%s", refs.Org, refs.Repo)]; ok {
		return slack
	}

	if slack, ok := cfg[refs.Org]; ok {
		return slack
	}

	return cfg["*"]
}

func (cfg SlackReporterConfigs) HasGlobalConfig() bool {
	_, exists := cfg["*"]
	return exists
}

func (cfg *SlackReporter) DefaultAndValidate() error {
	// Default ReportTemplate.
	if cfg.ReportTemplate == "" {
		cfg.ReportTemplate = `Job {{.Spec.Job}} of type {{.Spec.Type}} ended with state {{.Status.State}}. <{{.Status.URL}}|View logs>`
	}

	if cfg.Channel == "" {
		return errors.New("channel must be set")
	}

	// Validate ReportTemplate.
	tmpl, err := template.New("").Parse(cfg.ReportTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}
	if err := tmpl.Execute(&bytes.Buffer{}, &prowapi.ProwJob{}); err != nil {
		return fmt.Errorf("failed to execute report_template: %w", err)
	}

	return nil
}

// Load loads and parses the config at path.
func Load(prowConfig, jobConfig string, supplementalProwConfigDirs []string, supplementalProwConfigsFileNameSuffix string, additionals ...func(*Config) error) (c *Config, err error) {
	return loadWithYamlOpts(nil, prowConfig, jobConfig, supplementalProwConfigDirs, supplementalProwConfigsFileNameSuffix, additionals...)
}

// LoadStrict loads and parses the config at path.
// Unlike Load it unmarshalls yaml with strict parsing.
func LoadStrict(prowConfig, jobConfig string, supplementalProwConfigDirs []string, supplementalProwConfigsFileNameSuffix string, additionals ...func(*Config) error) (c *Config, err error) {
	return loadWithYamlOpts([]yaml.JSONOpt{yaml.DisallowUnknownFields}, prowConfig, jobConfig, supplementalProwConfigDirs, supplementalProwConfigsFileNameSuffix, additionals...)
}

func loadWithYamlOpts(yamlOpts []yaml.JSONOpt, prowConfig, jobConfig string, supplementalProwConfigDirs []string, supplementalProwConfigsFileNameSuffix string, additionals ...func(*Config) error) (c *Config, err error) {
	// we never want config loading to take down the prow components.
	defer func() {
		if r := recover(); r != nil {
			c, err = nil, fmt.Errorf("panic loading config: %v\n%s", r, string(debug.Stack()))
		}
	}()
	c, err = loadConfig(prowConfig, jobConfig, supplementalProwConfigDirs, supplementalProwConfigsFileNameSuffix, yamlOpts...)
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
func ReadJobConfig(jobConfig string, yamlOpts ...yaml.JSONOpt) (JobConfig, error) {
	stat, err := os.Stat(jobConfig)
	if err != nil {
		return JobConfig{}, err
	}

	if !stat.IsDir() {
		// still support a single file.
		var jc JobConfig
		if err := yamlToConfig(jobConfig, &jc, yamlOpts...); err != nil {
			return JobConfig{}, err
		}
		return jc, nil
	}

	prowIgnore, err := gitignore.NewRepositoryWithFile(jobConfig, ProwIgnoreFileName)
	if err != nil {
		return JobConfig{}, fmt.Errorf("failed to create `%s` parser: %w", ProwIgnoreFileName, err)
	}
	// we need to ensure all config files have unique basenames,
	// since updateconfig plugin will use basename as a key in the configmap.
	uniqueBasenames := sets.String{}

	jobConfigCount := 0
	allStart := time.Now()
	jc := JobConfig{}
	var errs []error
	err = filepath.Walk(jobConfig, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logrus.WithError(err).Errorf("walking path %q.", path)
			// bad file should not stop us from parsing the directory.
			return nil
		}

		if strings.HasPrefix(info.Name(), "..") {
			// kubernetes volumes also include files we
			// should not look be looking into for keys.
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		// Use 'Match' directly because 'Ignore' and 'Include' don't work properly for repositories.
		match := prowIgnore.Match(path)
		if match != nil && match.Ignore() {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		base := filepath.Base(path)
		if uniqueBasenames.Has(base) {
			errs = append(errs, fmt.Errorf("duplicated basename is not allowed: %s", base))
			return nil
		}
		uniqueBasenames.Insert(base)

		fileStart := time.Now()
		var subConfig JobConfig
		if err := yamlToConfig(path, &subConfig, yamlOpts...); err != nil {
			errs = append(errs, err)
			return nil
		}
		jc, err = mergeJobConfigs(jc, subConfig)
		if err == nil {
			logrus.WithField("jobConfig", path).WithField("duration", time.Since(fileStart)).Traceln("config loaded")
			jobConfigCount++
		} else {
			errs = append(errs, err)
		}
		return nil
	})
	err = utilerrors.NewAggregate(append(errs, err))
	if err != nil {
		return JobConfig{}, err
	}
	logrus.WithField("count", jobConfigCount).WithField("duration", time.Since(allStart)).Traceln("jobConfigs loaded successfully")

	return jc, nil
}

// loadConfig loads one or multiple config files and returns a config object.
func loadConfig(prowConfig, jobConfig string, additionalProwConfigDirs []string, supplementalProwConfigsFileNameSuffix string, yamlOpts ...yaml.JSONOpt) (*Config, error) {
	stat, err := os.Stat(prowConfig)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("prowConfig cannot be a dir - %s", prowConfig)
	}

	var nc Config
	if err := yamlToConfig(prowConfig, &nc, yamlOpts...); err != nil {
		return nil, err
	}

	prowConfigCount := 0
	allStart := time.Now()
	for _, additionalProwConfigDir := range additionalProwConfigDirs {
		var errs []error
		errs = append(errs, filepath.Walk(additionalProwConfigDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Finish walking and handle all errors in bulk at the end, otherwise this is annoying as a user.
				errs = append(errs, err)
				return nil
			}
			// Kubernetes configmap mounts create symlinks for the configmap keys that point to files prefixed with '..'.
			// This allows it to do  atomic changes by changing the symlink to a new target when the configmap content changes.
			// This means that we should ignore the '..'-prefixed files, otherwise we might end up reading a half-written file and will
			// get duplicate data.
			if strings.HasPrefix(info.Name(), "..") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if info.IsDir() || !strings.HasSuffix(path, supplementalProwConfigsFileNameSuffix) {
				return nil
			}

			fileStart := time.Now()
			var cfg ProwConfig
			if err := yamlToConfig(path, &cfg); err != nil {
				errs = append(errs, err)
				return nil
			}

			if err := nc.ProwConfig.mergeFrom(&cfg); err != nil {
				errs = append(errs, fmt.Errorf("failed to merge in config from %s: %w", path, err))
			} else {
				logrus.WithField("prowConfig", path).WithField("duration", time.Since(fileStart)).Traceln("config loaded")
				prowConfigCount++
			}

			return nil
		}))

		if err := utilerrors.NewAggregate(errs); err != nil {
			return nil, err
		}
	}
	logrus.WithField("count", prowConfigCount).WithField("duration", time.Since(allStart)).Traceln("prowConfigs loaded successfully")

	if err := parseProwConfig(&nc); err != nil {
		return nil, err
	}

	versionFilePath := filepath.Join(path.Dir(prowConfig), ConfigVersionFileName)
	if _, errAccess := os.Stat(versionFilePath); errAccess == nil {
		content, err := os.ReadFile(versionFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read versionfile %s: %w", versionFilePath, err)
		}
		nc.ConfigVersionSHA = string(content)
	}

	nc.AllRepos = sets.String{}
	for _, query := range nc.Tide.Queries {
		for _, repo := range query.Repos {
			nc.AllRepos.Insert(repo)
		}
	}

	// For production, use these functions for getting ProwYAML values. In
	// tests, we can override these fields with mocked versions.
	nc.ProwYAMLGetter = prowYAMLGetter
	nc.ProwYAMLGetterWithDefaults = prowYAMLGetterWithDefaults

	if deduplicatedTideQueries, err := deduplicateTideQueries(nc.Tide.Queries); err != nil {
		logrus.WithError(err).Error("failed to deduplicate tide queriees")
	} else {
		nc.Tide.Queries = deduplicatedTideQueries
	}

	if nc.InRepoConfig.AllowedClusters == nil {
		nc.InRepoConfig.AllowedClusters = map[string][]string{}
	}

	// Respect `"*": []`, which disabled default global cluster.
	if nc.InRepoConfig.AllowedClusters["*"] == nil {
		nc.InRepoConfig.AllowedClusters["*"] = []string{kube.DefaultClusterAlias}
	}

	// merge pubsub configs.
	if nc.PubSubSubscriptions != nil {
		if nc.PubSubTriggers != nil {
			return nil, errors.New("pubsub_subscriptions and pubsub_triggers are mutually exclusive")
		}
		for proj, topics := range nc.PubSubSubscriptions {
			nc.PubSubTriggers = append(nc.PubSubTriggers, PubSubTrigger{
				Project:         proj,
				Topics:          topics,
				AllowedClusters: []string{"*"},
			})
		}
	}
	for i, trigger := range nc.PubSubTriggers {
		if trigger.MaxOutstandingMessages == 0 {
			nc.PubSubTriggers[i].MaxOutstandingMessages = defaultMaxOutstandingMessages
		}
	}

	// TODO(krzyzacy): temporary allow empty jobconfig
	//                 also temporary allow job config in prow config.
	if jobConfig == "" {
		return &nc, nil
	}

	jc, err := ReadJobConfig(jobConfig, yamlOpts...)
	if err != nil {
		return nil, err
	}
	if err := nc.mergeJobConfig(jc); err != nil {
		return nil, err
	}

	return &nc, nil
}

// yamlToConfig converts a yaml file into a Config object.
func yamlToConfig(path string, nc interface{}, opts ...yaml.JSONOpt) error {
	b, err := ReadFileMaybeGZIP(path)
	if err != nil {
		return fmt.Errorf("error reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(b, nc, opts...); err != nil {
		return fmt.Errorf("error unmarshaling %s: %w", path, err)
	}
	var jc *JobConfig
	switch v := nc.(type) {
	case *JobConfig:
		jc = v
	case *Config:
		jc = &v.JobConfig
	default:
		// No job config, skip inserting filepaths into the jobs.
		return nil
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

	fix := func(job *Periodic) {
		job.SourcePath = path
	}
	for i := range jc.Periodics {
		fix(&jc.Periodics[i])
	}
	return nil
}

// ReadFileMaybeGZIP wraps os.ReadFile, returning the decompressed contents
// if the file is gzipped, or otherwise the raw contents.
func ReadFileMaybeGZIP(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// check if file contains gzip header: http://www.zlib.org/rfc-gzip.html.
	if !bytes.HasPrefix(b, []byte("\x1F\x8B")) {
		// go ahead and return the contents if not gzipped.
		return b, nil
	}
	// otherwise decode.
	gzipReader, err := gzip.NewReader(bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(gzipReader)
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

// mergeJobConfigs merges two JobConfig together.
// It will try to merge:
//	- Presubmits
//	- Postsubmits
// 	- Periodics
//	- Presets
func mergeJobConfigs(a, b JobConfig) (JobConfig, error) {
	// Merge everything.
	// *** Presets ***
	c := JobConfig{}
	c.Presets = append(a.Presets, b.Presets...)

	// validate no duplicated preset key-value pairs.
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

func shouldDecorate(c *JobConfig, util *UtilityConfig) bool {
	if util.Decorate != nil {
		return *util.Decorate
	} else {
		b := c.DecorateAllJobs
		util.Decorate = &b
	}
	return c.DecorateAllJobs
}

func setPresubmitProwJobDefaults(c *Config, ps *Presubmit, repo string) {
	ps.ProwJobDefault = c.mergeProwJobDefault(repo, ps.Cluster, ps.ProwJobDefault)
}

func setPostsubmitProwJobDefaults(c *Config, ps *Postsubmit, repo string) {
	ps.ProwJobDefault = c.mergeProwJobDefault(repo, ps.Cluster, ps.ProwJobDefault)
}

func setPeriodicProwJobDefaults(c *Config, ps *Periodic) {
	var repo string
	if len(ps.UtilityConfig.ExtraRefs) > 0 {
		repo = fmt.Sprintf("%s/%s", ps.UtilityConfig.ExtraRefs[0].Org, ps.UtilityConfig.ExtraRefs[0].Repo)
	}

	ps.ProwJobDefault = c.mergeProwJobDefault(repo, ps.Cluster, ps.ProwJobDefault)
}
func setPresubmitDecorationDefaults(c *Config, ps *Presubmit, repo string) {
	if shouldDecorate(&c.JobConfig, &ps.JobBase.UtilityConfig) {
		ps.DecorationConfig = c.Plank.mergeDefaultDecorationConfig(repo, ps.Cluster, ps.DecorationConfig)
	}
}

func setPostsubmitDecorationDefaults(c *Config, ps *Postsubmit, repo string) {
	if shouldDecorate(&c.JobConfig, &ps.JobBase.UtilityConfig) {
		ps.DecorationConfig = c.Plank.mergeDefaultDecorationConfig(repo, ps.Cluster, ps.DecorationConfig)
	}
}

func setPeriodicDecorationDefaults(c *Config, ps *Periodic) {
	if shouldDecorate(&c.JobConfig, &ps.JobBase.UtilityConfig) {
		var repo string
		if len(ps.UtilityConfig.ExtraRefs) > 0 {
			repo = fmt.Sprintf("%s/%s", ps.UtilityConfig.ExtraRefs[0].Org, ps.UtilityConfig.ExtraRefs[0].Repo)
		}

		ps.DecorationConfig = c.Plank.mergeDefaultDecorationConfig(repo, ps.Cluster, ps.DecorationConfig)
	}
}

// defaultPresubmits defaults the presubmits for one repo.
func defaultPresubmits(presubmits []Presubmit, additionalPresets []Preset, c *Config, repo string) error {
	c.defaultPresubmitFields(presubmits)
	var errs []error
	for idx, ps := range presubmits {
		setPresubmitDecorationDefaults(c, &presubmits[idx], repo)
		setPresubmitProwJobDefaults(c, &presubmits[idx], repo)
		if err := resolvePresets(ps.Name, ps.Labels, ps.Spec, append(c.Presets, additionalPresets...)); err != nil {
			errs = append(errs, err)
		}
	}
	if err := SetPresubmitRegexes(presubmits); err != nil {
		errs = append(errs, fmt.Errorf("could not set regex: %w", err))
	}

	return utilerrors.NewAggregate(errs)
}

// defaultPostsubmits defaults the postsubmits for one repo.
func defaultPostsubmits(postsubmits []Postsubmit, additionalPresets []Preset, c *Config, repo string) error {
	c.defaultPostsubmitFields(postsubmits)
	var errs []error
	for idx, ps := range postsubmits {
		setPostsubmitDecorationDefaults(c, &postsubmits[idx], repo)
		setPostsubmitProwJobDefaults(c, &postsubmits[idx], repo)
		if err := resolvePresets(ps.Name, ps.Labels, ps.Spec, append(c.Presets, additionalPresets...)); err != nil {
			errs = append(errs, err)
		}
	}
	if err := SetPostsubmitRegexes(postsubmits); err != nil {
		errs = append(errs, fmt.Errorf("could not set regex: %w", err))
	}
	return utilerrors.NewAggregate(errs)
}

// DefaultPeriodic defaults (mutates) a single Periodic.
func (c *Config) DefaultPeriodic(periodic *Periodic) error {
	c.defaultPeriodicFields(periodic)
	setPeriodicDecorationDefaults(c, periodic)
	setPeriodicProwJobDefaults(c, periodic)
	return resolvePresets(periodic.Name, periodic.Labels, periodic.Spec, c.Presets)
}

// defaultPeriodics defaults c.Periodics.
func defaultPeriodics(c *Config) error {
	var errs []error
	for i := range c.Periodics {
		errs = append(errs, c.DefaultPeriodic(&c.Periodics[i]))
	}
	return utilerrors.NewAggregate(errs)
}

// finalizeJobConfig mutates and fixes entries for jobspecs.
func (c *Config) finalizeJobConfig() error {
	if err := c.Plank.FinalizeDefaultDecorationConfigs(); err != nil {
		return err
	}

	for repo, jobs := range c.PresubmitsStatic {
		if err := defaultPresubmits(jobs, nil, c, repo); err != nil {
			return err
		}
		c.AllRepos.Insert(repo)
	}

	for repo, jobs := range c.PostsubmitsStatic {
		if err := defaultPostsubmits(jobs, nil, c, repo); err != nil {
			return err
		}
		c.AllRepos.Insert(repo)
	}

	if err := defaultPeriodics(c); err != nil {
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
	}
	if c.Gerrit.DeckURL != "" {
		if _, err := url.Parse(c.Gerrit.DeckURL); err != nil {
			return fmt.Errorf(`Invalid value for gerrit.deck_url: %v`, err)
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

	if c.SlackReporterConfigs != nil {
		for k, config := range c.SlackReporterConfigs {
			if err := config.DefaultAndValidate(); err != nil {
				return fmt.Errorf("failed to validate slackreporter config: %w", err)
			}
			c.SlackReporterConfigs[k] = config
		}
	}

	if err := c.Deck.FinalizeDefaultRerunAuthConfigs(); err != nil {
		return err
	}

	if err := c.Deck.Validate(); err != nil {
		return err
	}

	return nil
}

var (
	jobNameRegex        = regexp.MustCompile(`^[A-Za-z0-9-._]+$`)
	jobNameRegexJenkins = regexp.MustCompile(`^[A-Za-z0-9-._]([A-Za-z0-9-._/]*[A-Za-z0-9-_])?$`)
)

func validateJobName(v JobBase) error {
	nameRegex := jobNameRegex
	if v.Agent == string(prowapi.JenkinsAgent) {
		nameRegex = jobNameRegexJenkins
	}

	if !nameRegex.MatchString(v.Name) {
		return fmt.Errorf("name: must match regex %q", nameRegex.String())
	}

	return nil
}

func (c Config) validateJobBase(v JobBase, jobType prowapi.ProwJobType) error {
	if err := validateJobName(v); err != nil {
		return err
	}

	// Ensure max_concurrency is non-negative.
	if v.MaxConcurrency < 0 {
		return fmt.Errorf("max_concurrency: %d must be a non-negative number", v.MaxConcurrency)
	}
	if err := validateAgent(v, c.PodNamespace); err != nil {
		return err
	}
	if err := validatePodSpec(jobType, v.Spec, v.DecorationConfig); err != nil {
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
	validJobQueueNames := sets.StringKeySet(c.Plank.JobQueueConcurrencies)
	if err := validateJobQueueName(v.JobQueueName, validJobQueueNames); err != nil {
		return err
	}
	if v.Spec == nil || len(v.Spec.Containers) == 0 {
		return nil // jenkins jobs have no spec.
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

// validatePresubmits validates the presubmits for one repo.
func (c Config) validatePresubmits(presubmits []Presubmit) error {
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

		if err := c.validateJobBase(ps.JobBase, prowapi.PresubmitJob); err != nil {
			errs = append(errs, fmt.Errorf("invalid presubmit job %s: %w", ps.Name, err))
		}
		if err := validateTriggering(ps); err != nil {
			errs = append(errs, err)
		}
		if err := validateReporting(ps.JobBase, ps.Reporter); err != nil {
			errs = append(errs, fmt.Errorf("invalid presubmit job %s: %w", ps.Name, err))
		}
		validPresubmits[ps.Name] = append(validPresubmits[ps.Name], ps)
	}

	return utilerrors.NewAggregate(errs)
}

// ValidateRefs validates the extra refs on a presubmit for one repo.
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

// validatePostsubmits validates the postsubmits for one repo.
func (c Config) validatePostsubmits(postsubmits []Postsubmit) error {
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

		if err := c.validateJobBase(ps.JobBase, prowapi.PostsubmitJob); err != nil {
			errs = append(errs, fmt.Errorf("invalid postsubmit job %s: %w", ps.Name, err))
		}
		if err := validateAlwaysRun(ps); err != nil {
			errs = append(errs, err)
		}
		if err := validateReporting(ps.JobBase, ps.Reporter); err != nil {
			errs = append(errs, fmt.Errorf("invalid postsubmit job %s: %w", ps.Name, err))
		}
		validPostsubmits[ps.Name] = append(validPostsubmits[ps.Name], ps)
	}

	return utilerrors.NewAggregate(errs)
}

// validatePeriodics validates a set of periodics.
func (c Config) validatePeriodics(periodics []Periodic) error {
	var errs []error

	// validate no duplicated periodics.
	validPeriodics := sets.NewString()
	// Ensure that the periodic durations are valid and specs exist.
	for j, p := range periodics {
		if validPeriodics.Has(p.Name) {
			errs = append(errs, fmt.Errorf("duplicated periodic job: %s", p.Name))
		}
		validPeriodics.Insert(p.Name)
		if err := c.validateJobBase(p.JobBase, prowapi.PeriodicJob); err != nil {
			errs = append(errs, fmt.Errorf("invalid periodic job %s: %w", p.Name, err))
		}

		// Validate mutually exclusive properties
		seen := 0
		if p.Cron != "" {
			seen += 1
		}
		if p.Interval != "" {
			seen += 1
		}
		if p.MinimumInterval != "" {
			seen += 1
		}
		if seen > 1 {
			errs = append(errs, fmt.Errorf("cron, interval, and minimum_interval are mutually exclusive in periodic %s", p.Name))
			continue
		}
		if seen == 0 {
			errs = append(errs, fmt.Errorf("at least one of cron, interval, or minimum_interval must be set in periodic %s", p.Name))
			continue
		}

		if p.Cron != "" {
			if _, err := cron.Parse(p.Cron); err != nil {
				errs = append(errs, fmt.Errorf("invalid cron string %s in periodic %s: %w", p.Cron, p.Name, err))
			}
		}

		// Set the interval on the periodic jobs. It doesn't make sense to do this
		// for child jobs.
		if p.Interval != "" {
			d, err := time.ParseDuration(periodics[j].Interval)
			if err != nil {
				errs = append(errs, fmt.Errorf("cannot parse duration for %s: %w", periodics[j].Name, err))
			}
			periodics[j].interval = d
		}

		if p.MinimumInterval != "" {
			d, err := time.ParseDuration(periodics[j].MinimumInterval)
			if err != nil {
				errs = append(errs, fmt.Errorf("cannot parse duration for %s: %w", periodics[j].Name, err))
			}
			periodics[j].minimum_interval = d
		}

	}

	return utilerrors.NewAggregate(errs)
}

// ValidateJobConfig validates if all the jobspecs/presets are valid
// if you are mutating the jobs, please add it to finalizeJobConfig above.
func (c *Config) ValidateJobConfig() error {

	var errs []error

	// Validate presubmits.
	for _, jobs := range c.PresubmitsStatic {
		if err := c.validatePresubmits(jobs); err != nil {
			errs = append(errs, err)
		}
	}

	// Validate postsubmits.
	for _, jobs := range c.PostsubmitsStatic {
		if err := c.validatePostsubmits(jobs); err != nil {
			errs = append(errs, err)
		}
	}

	// Validate periodics.
	if err := c.validatePeriodics(c.Periodics); err != nil {
		errs = append(errs, err)
	}

	c.Deck.AllKnownStorageBuckets = calculateStorageBuckets(c)

	return utilerrors.NewAggregate(errs)
}

func parseProwConfig(c *Config) error {
	if err := ValidateController(&c.Plank.Controller); err != nil {
		return fmt.Errorf("validating plank config: %w", err)
	}

	if c.Plank.PodPendingTimeout == nil {
		c.Plank.PodPendingTimeout = &metav1.Duration{Duration: 10 * time.Minute}
	}

	if c.Plank.PodRunningTimeout == nil {
		c.Plank.PodRunningTimeout = &metav1.Duration{Duration: 48 * time.Hour}
	}

	if c.Plank.PodUnscheduledTimeout == nil {
		c.Plank.PodUnscheduledTimeout = &metav1.Duration{Duration: 5 * time.Minute}
	}

	if c.Gerrit.TickInterval == nil {
		c.Gerrit.TickInterval = &metav1.Duration{Duration: time.Minute}
	}

	if c.Gerrit.RateLimit == 0 {
		c.Gerrit.RateLimit = 5
	}

	if c.Tide.Gerrit != nil {
		if c.Tide.Gerrit.RateLimit == 0 {
			c.Tide.Gerrit.RateLimit = 5
		}
	}

	if len(c.GitHubReporter.JobTypesToReport) == 0 {
		c.GitHubReporter.JobTypesToReport = append(c.GitHubReporter.JobTypesToReport, prowapi.PresubmitJob, prowapi.PostsubmitJob)
	}

	// validate entries are valid job types.
	// Currently only presubmit and postsubmit can be reported to github.
	for _, t := range c.GitHubReporter.JobTypesToReport {
		if t != prowapi.PresubmitJob && t != prowapi.PostsubmitJob {
			return fmt.Errorf("invalid job_types_to_report: %v", t)
		}
	}

	for i := range c.JenkinsOperators {
		if err := ValidateController(&c.JenkinsOperators[i].Controller); err != nil {
			return fmt.Errorf("validating jenkins_operators config: %w", err)
		}
		sel, err := labels.Parse(c.JenkinsOperators[i].LabelSelectorString)
		if err != nil {
			return fmt.Errorf("invalid jenkins_operators.label_selector option: %w", err)
		}
		c.JenkinsOperators[i].LabelSelector = sel
		// TODO: Invalidate overlapping selectors more.
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
			return fmt.Errorf("parsing template for agent %q: %w", agentToTmpl.Agent, err)
		}
		c.Deck.ExternalAgentLogs[i].URLTemplate = urlTemplate
		// we need to validate selectors used by deck since these are not
		// sent to the api server.
		s, err := labels.Parse(c.Deck.ExternalAgentLogs[i].SelectorString)
		if err != nil {
			return fmt.Errorf("error parsing selector %q: %w", c.Deck.ExternalAgentLogs[i].SelectorString, err)
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

	// Parse and cache all our regexes upfront.
	c.Deck.Spyglass.RegexCache = make(map[string]*regexp.Regexp)
	for _, lens := range c.Deck.Spyglass.Lenses {
		toCompile := append(lens.OptionalFiles, lens.RequiredFiles...)
		for _, v := range toCompile {
			if _, ok := c.Deck.Spyglass.RegexCache[v]; ok {
				continue
			}
			r, err := regexp.Compile(v)
			if err != nil {
				return fmt.Errorf("cannot compile regexp %q, err: %w", v, err)
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

	if c.Deck.Spyglass.GCSBrowserPrefixesByRepo == nil {
		c.Deck.Spyglass.GCSBrowserPrefixesByRepo = make(map[string]string)
	}

	_, defaultByRepoExists := c.Deck.Spyglass.GCSBrowserPrefixesByRepo["*"]
	if defaultByRepoExists && c.Deck.Spyglass.GCSBrowserPrefix != "" {
		return fmt.Errorf("both gcs_browser_prefix and gcs_browser_prefixes['*'] are specified.")
	}
	if !defaultByRepoExists {
		c.Deck.Spyglass.GCSBrowserPrefixesByRepo["*"] = c.Deck.Spyglass.GCSBrowserPrefix
	}

	if c.Deck.Spyglass.GCSBrowserPrefixesByBucket == nil {
		c.Deck.Spyglass.GCSBrowserPrefixesByBucket = make(map[string]string)
	}

	_, defaultByBucketExists := c.Deck.Spyglass.GCSBrowserPrefixesByBucket["*"]
	if defaultByBucketExists && c.Deck.Spyglass.GCSBrowserPrefix != "" {
		return fmt.Errorf("both gcs_browser_prefix and gcs_browser_prefixes_by_bucket['*'] are specified.")
	}
	if !defaultByBucketExists {
		c.Deck.Spyglass.GCSBrowserPrefixesByBucket["*"] = c.Deck.Spyglass.GCSBrowserPrefix
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

	if len(c.Tide.TargetURLs) > 0 && c.Tide.TargetURL != "" {
		return fmt.Errorf("tide.target_url and tide.target_urls are mutually exclusive")
	}

	if c.Tide.TargetURLs == nil {
		c.Tide.TargetURLs = map[string]string{}
	}
	if c.Tide.TargetURL != "" {
		c.Tide.TargetURLs["*"] = c.Tide.TargetURL
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
		if method != types.MergeMerge &&
			method != types.MergeRebase &&
			method != types.MergeSquash {
			return fmt.Errorf("merge type %q for %s is not a valid type", method, name)
		}
	}

	for name, templates := range c.Tide.MergeTemplate {
		if templates.TitleTemplate != "" {
			titleTemplate, err := template.New("CommitTitle").Parse(templates.TitleTemplate)

			if err != nil {
				return fmt.Errorf("parsing template for commit title: %w", err)
			}

			templates.Title = titleTemplate
		}

		if templates.BodyTemplate != "" {
			bodyTemplate, err := template.New("CommitBody").Parse(templates.BodyTemplate)

			if err != nil {
				return fmt.Errorf("parsing template for commit body: %w", err)
			}

			templates.Body = bodyTemplate
		}

		c.Tide.MergeTemplate[name] = templates
	}

	for i, tq := range c.Tide.Queries {
		if err := tq.Validate(); err != nil {
			return fmt.Errorf("tide query (index %d) is invalid: %w", i, err)
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
		return fmt.Errorf("unable to parse github.link_url, might not be a valid url: %w", err)
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

	// Avoid using a job timeout of infinity by setting the default value to 24 hours.
	if c.DefaultJobTimeout == nil {
		c.DefaultJobTimeout = &metav1.Duration{Duration: DefaultJobTimeout}
	}

	// Ensure Policy.Include and Policy.Exclude are mutually exclusive.
	if len(c.BranchProtection.Include) > 0 && len(c.BranchProtection.Exclude) > 0 {
		return fmt.Errorf("Forbidden to set both Policy.Include and Policy.Exclude, Please use either Include or Exclude!")
	}

	return nil
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

func validateJobQueueName(name string, validNames sets.String) error {
	if name != "" && !validNames.Has(name) {
		return fmt.Errorf("invalid job queue name %s", name)
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
		// TODO(fejta): update plank to allow this (depends on client change).
		return fmt.Errorf("namespace customization requires agent: %s (found %q)", p, agent)
	}
	return nil
}

func validateDecoration(container v1.Container, config *prowapi.DecorationConfig) error {
	if config == nil {
		return nil
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid decoration config: %w", err)
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
				return fmt.Errorf("job %s failed to merge presets for podspec: %w", name, err)
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
		// Validate that periodic jobs don't request an implicit git ref.
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

func validatePodSpec(jobType prowapi.ProwJobType, spec *v1.PodSpec, decorationConfig *prowapi.DecorationConfig) error {
	if spec == nil {
		return nil
	}

	var errs []error

	if len(spec.InitContainers) != 0 {
		errs = append(errs, errors.New("pod spec may not use init containers"))
	}

	if n := len(spec.Containers); n < 1 {
		// We must return here to not cause an out of bounds panic in the remaining validation.
		return utilerrors.NewAggregate(append(errs, fmt.Errorf("pod spec must specify at least 1 container, found: %d", n)))
	}

	if n := len(spec.Containers); n > 1 && decorationConfig == nil {
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
					// TODO(fejta): consider allowing this.
					errs = append(errs, fmt.Errorf("env %s is reserved", env.Name))
				}
			}
		}
	}

	volumeNames := sets.String{}
	decoratedVolumeNames := decorate.VolumeMounts(decorationConfig)
	for _, volume := range spec.Volumes {
		if volumeNames.Has(volume.Name) {
			errs = append(errs, fmt.Errorf("volume named %q is defined more than once", volume.Name))
		}
		volumeNames.Insert(volume.Name)

		if decoratedVolumeNames.Has(volume.Name) {
			errs = append(errs, fmt.Errorf("volume %s is a reserved for decoration", volume.Name))
		}
	}

	for i := range spec.Containers {
		for _, mount := range spec.Containers[i].VolumeMounts {
			if !volumeNames.Has(mount.Name) && !decoratedVolumeNames.Has(mount.Name) {
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

func validateAlwaysRun(job Postsubmit) error {
	if job.AlwaysRun != nil && *job.AlwaysRun {
		if job.RunIfChanged != "" {
			return fmt.Errorf("job %s is set to always run but also declares run_if_changed targets, which are mutually exclusive", job.Name)
		}
		if job.SkipIfOnlyChanged != "" {
			return fmt.Errorf("job %s is set to always run but also declares skip_if_only_changed targets, which are mutually exclusive", job.Name)
		}
	}
	if job.RunIfChanged != "" && job.SkipIfOnlyChanged != "" {
		return fmt.Errorf("job %s declares run_if_changed and skip_if_only_changed, which are mutually exclusive", job.Name)
	}
	return nil
}

func validateTriggering(job Presubmit) error {
	if job.AlwaysRun {
		if job.RunIfChanged != "" {
			return fmt.Errorf("job %s is set to always run but also declares run_if_changed targets, which are mutually exclusive", job.Name)
		}
		if job.SkipIfOnlyChanged != "" {
			return fmt.Errorf("job %s is set to always run but also declares skip_if_only_changed targets, which are mutually exclusive", job.Name)
		}
	}
	if job.RunIfChanged != "" && job.SkipIfOnlyChanged != "" {
		return fmt.Errorf("job %s declares run_if_changed and skip_if_only_changed, which are mutually exclusive", job.Name)
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
		if label == kube.GerritReportLabel && value != "" {
			return fmt.Errorf("Gerrit report label %s set to non-empty string but job is configured to skip reporting.", label)
		}
	}
	return nil
}

// ValidateController validates the provided controller config.
func ValidateController(c *Controller) error {
	urlTmpl, err := template.New("JobURL").Parse(c.JobURLTemplateString)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
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
			return fmt.Errorf("error while parsing template for %s: %w", orgRepo, err)
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
	if base.Agent == "" { // Use kubernetes by default.
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

func (c *ProwConfig) defaultPeriodicFields(js *Periodic) {
	c.defaultJobBase(&js.JobBase)
}

// SetPresubmitRegexes compiles and validates all the regular expressions for
// the provided presubmits.
func SetPresubmitRegexes(js []Presubmit) error {
	for i, j := range js {
		if re, err := regexp.Compile(j.Trigger); err == nil {
			js[i].re = &CopyableRegexp{re}
		} else {
			return fmt.Errorf("could not compile trigger regex for %s: %w", j.Name, err)
		}
		if !js[i].re.MatchString(j.RerunCommand) {
			return fmt.Errorf("for job %s, rerun command \"%s\" does not match trigger \"%s\"", j.Name, j.RerunCommand, j.Trigger)
		}
		b, err := setBrancherRegexes(j.Brancher)
		if err != nil {
			return fmt.Errorf("could not set branch regexes for %s: %w", j.Name, err)
		}
		js[i].Brancher = b

		c, err := setChangeRegexes(j.RegexpChangeMatcher)
		if err != nil {
			return fmt.Errorf("could not set change regexes for %s: %w", j.Name, err)
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
			br.re = &CopyableRegexp{re}
		} else {
			return br, fmt.Errorf("could not compile positive branch regex: %w", err)
		}
	}
	if len(br.SkipBranches) > 0 {
		if re, err := regexp.Compile(strings.Join(br.SkipBranches, `|`)); err == nil {
			br.reSkip = &CopyableRegexp{re}
		} else {
			return br, fmt.Errorf("could not compile negative branch regex: %w", err)
		}
	}
	return br, nil
}

func setChangeRegexes(cm RegexpChangeMatcher) (RegexpChangeMatcher, error) {
	var reString, propName string
	if reString = cm.RunIfChanged; reString != "" {
		propName = "run_if_changed"
	} else if reString = cm.SkipIfOnlyChanged; reString != "" {
		propName = "skip_if_only_changed"
	}
	if reString != "" {
		re, err := regexp.Compile(reString)
		if err != nil {
			return cm, fmt.Errorf("could not compile %s regex: %w", propName, err)
		}
		cm.reChanges = &CopyableRegexp{re}
	}
	return cm, nil
}

// SetPostsubmitRegexes compiles and validates all the regular expressions for
// the provided postsubmits.
func SetPostsubmitRegexes(ps []Postsubmit) error {
	for i, j := range ps {
		b, err := setBrancherRegexes(j.Brancher)
		if err != nil {
			return fmt.Errorf("could not set branch regexes for %s: %w", j.Name, err)
		}
		ps[i].Brancher = b
		c, err := setChangeRegexes(j.RegexpChangeMatcher)
		if err != nil {
			return fmt.Errorf("could not set change regexes for %s: %w", j.Name, err)
		}
		ps[i].RegexpChangeMatcher = c
	}
	return nil
}

// OrgRepo supercedes org/repo string handling.
type OrgRepo struct {
	Org  string
	Repo string
}

func (repo OrgRepo) String() string {
	return fmt.Sprintf("%s/%s", repo.Org, repo.Repo)
}

// NewOrgRepo creates a OrgRepo from org/repo string.
func NewOrgRepo(orgRepo string) *OrgRepo {
	org, repo, err := SplitRepoName(orgRepo)
	// SplitRepoName errors when Unable to split to Org/Repo
	// If we error, that means there is no slash, so org == OrgRepo.
	if err != nil {
		return &OrgRepo{Org: orgRepo}
	}
	return &OrgRepo{Org: org, Repo: repo}
}

// OrgReposToStrings converts a list of OrgRepo to its String() equivalent.
func OrgReposToStrings(vs []OrgRepo) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = v.String()
	}
	return vsm
}

// StringsToOrgRepos converts a list of org/repo strings to its OrgRepo equivalent.
func StringsToOrgRepos(vs []string) []OrgRepo {
	vsm := make([]OrgRepo, len(vs))
	for i, v := range vs {
		vsm[i] = *NewOrgRepo(v)
	}
	return vsm
}

// mergeFrom merges two prow configs. It must be called _before_ doing any
// defaulting.
// If you extend this, please also extend HasConfigFor accordingly.
func (pc *ProwConfig) mergeFrom(additional *ProwConfig) error {
	emptyReference := &ProwConfig{
		BranchProtection:     additional.BranchProtection,
		Tide:                 Tide{TideGitHubConfig: TideGitHubConfig{MergeType: additional.Tide.MergeType, Queries: additional.Tide.Queries}},
		SlackReporterConfigs: additional.SlackReporterConfigs,
	}

	var errs []error
	if diff := cmp.Diff(additional, emptyReference); diff != "" {
		errs = append(errs, fmt.Errorf("only 'branch-protection', 'slack_reporter_configs', 'tide.merge_method' and 'tide.queries' may be set via additional config, all other fields have no merging logic yet. Diff: %s", diff))
	}
	if err := pc.BranchProtection.merge(&additional.BranchProtection); err != nil {
		errs = append(errs, fmt.Errorf("failed to merge branch protection config: %w", err))
	}
	if err := pc.Tide.mergeFrom(&additional.Tide); err != nil {
		errs = append(errs, fmt.Errorf("failed to merge tide config: %w", err))
	}

	if pc.SlackReporterConfigs == nil {
		pc.SlackReporterConfigs = additional.SlackReporterConfigs
	} else if err := pc.SlackReporterConfigs.mergeFrom(&additional.SlackReporterConfigs); err != nil {
		errs = append(errs, fmt.Errorf("failed to merge slack-reporter config: %w", err))
	}

	return utilerrors.NewAggregate(errs)
}

// ContextDescriptionWithBaseSha is used by the GitHub reporting to store the baseSHA of a context
// in the status context description. Tide will read this if present using the BaseSHAFromContextDescription
// func. Storing the baseSHA in the status context allows us to store job results pretty much forever,
// instead of having to rerun everything after sinker cleaned up the ProwJobs.
func ContextDescriptionWithBaseSha(humanReadable, baseSHA string) string {
	var suffix string
	if baseSHA != "" {
		suffix = contextDescriptionBaseSHADelimiter + baseSHA
		// Leftpad the baseSHA suffix so its shown at a stable position on the right side in the GitHub UI.
		// The GitHub UI will also trim it on the right side and replace some part of it with '...'. The
		// API always returns the full string.
		if len(humanReadable+suffix) < contextDescriptionMaxLen {
			for i := 0; i < contextDescriptionMaxLen-len(humanReadable+suffix); i++ {
				// This looks like a standard space but is U+2001, because GitHub seems to deduplicate normal
				// spaces in their frontend.
				suffix = " " + suffix
			}
		}
	}
	return truncate(humanReadable, contextDescriptionMaxLen-len(suffix)) + suffix
}

// BaseSHAFromContextDescription is used by Tide to decode a baseSHA from a github status context
// description created via ContextDescriptionWithBaseSha. It will return an empty string if no
// valid sha was found.
func BaseSHAFromContextDescription(description string) string {
	split := strings.Split(description, contextDescriptionBaseSHADelimiter)
	// SHA1s are always 40 digits long.
	if len(split) != 2 || len(split[1]) != 40 {
		// Fallback to deprecated one if available.
		if split = strings.Split(description, contextDescriptionBaseSHADelimiterDeprecated); len(split) == 2 && len(split[1]) == 40 {
			return split[1]
		}
		return ""
	}
	return split[1]
}

const (
	contextDescriptionBaseSHADelimiter           = " BaseSHA:"
	contextDescriptionBaseSHADelimiterDeprecated = " Basesha:"
	contextDescriptionMaxLen                     = 140 // https://developer.github.com/v3/repos/deployments/#parameters-2
	elide                                        = " ... "
)

// truncate converts "really long messages" into "really ... messages".
func truncate(in string, maxLen int) string {
	half := (maxLen - len(elide)) / 2
	if len(in) <= maxLen {
		return in
	}
	return in[:half] + elide + in[len(in)-half:]
}

func (pc *ProwConfig) HasConfigFor() (global bool, orgs sets.String, repos sets.String) {
	global = pc.hasGlobalConfig()
	orgs = sets.String{}
	repos = sets.String{}

	for org, orgConfig := range pc.BranchProtection.Orgs {
		if isPolicySet(orgConfig.Policy) {
			orgs.Insert(org)
		}
		for repo := range orgConfig.Repos {
			repos.Insert(org + "/" + repo)
		}
	}

	for orgOrRepo := range pc.Tide.MergeType {
		if strings.Contains(orgOrRepo, "/") {
			repos.Insert(orgOrRepo)
		} else {
			orgs.Insert(orgOrRepo)
		}
	}

	for _, query := range pc.Tide.Queries {
		orgs.Insert(query.Orgs...)
		repos.Insert(query.Repos...)
	}

	for orgOrRepo := range pc.SlackReporterConfigs {
		if orgOrRepo == "*" {
			// configuration for "*" is globally available
			continue
		}

		if strings.Contains(orgOrRepo, "/") {
			repos.Insert(orgOrRepo)
		} else {
			orgs.Insert(orgOrRepo)
		}
	}

	return global, orgs, repos
}

func (pc *ProwConfig) hasGlobalConfig() bool {
	if pc.BranchProtection.ProtectTested != nil || pc.BranchProtection.AllowDisabledPolicies != nil || pc.BranchProtection.AllowDisabledJobPolicies != nil || pc.BranchProtection.ProtectReposWithOptionalJobs != nil || isPolicySet(pc.BranchProtection.Policy) || pc.SlackReporterConfigs.HasGlobalConfig() {
		return true
	}
	emptyReference := &ProwConfig{
		BranchProtection:     pc.BranchProtection,
		Tide:                 Tide{TideGitHubConfig: TideGitHubConfig{MergeType: pc.Tide.MergeType, Queries: pc.Tide.Queries}},
		SlackReporterConfigs: pc.SlackReporterConfigs,
	}
	return cmp.Diff(pc, emptyReference) != ""
}

// tideQueryMap is a map[tideQueryConfig]*tideQueryTarget. Because slices are not comparable, they
// or structs containing them are not allowed as map keys. We sidestep this by using a json serialization
// of the object as key instead. This is pretty inefficient but also something  we only do once during
// load.
type tideQueryMap map[string]*tideQueryTarget

func (tm tideQueryMap) queries() (TideQueries, error) {
	var result TideQueries
	for k, v := range tm {
		var queryConfig tideQueryConfig
		if err := json.Unmarshal([]byte(k), &queryConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %q: %w", k, err)
		}
		result = append(result, TideQuery{
			Orgs:                   v.Orgs,
			Repos:                  v.Repos,
			ExcludedRepos:          v.ExcludedRepos,
			Author:                 queryConfig.Author,
			ExcludedBranches:       queryConfig.ExcludedBranches,
			IncludedBranches:       queryConfig.IncludedBranches,
			Labels:                 queryConfig.Labels,
			MissingLabels:          queryConfig.MissingLabels,
			Milestone:              queryConfig.Milestone,
			ReviewApprovedRequired: queryConfig.ReviewApprovedRequired,
		})

	}

	// Sort the queries here to make sure that the de-duplication results
	// in a deterministic order.
	var errs []error
	sort.SliceStable(result, func(i, j int) bool {
		iSerialized, err := json.Marshal(result[i])
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to marshal %+v: %w", result[i], err))
		}
		jSerialized, err := json.Marshal(result[j])
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to marshal %+v: %w", result[j], err))
		}
		return string(iSerialized) < string(jSerialized)
	})

	return result, utilerrors.NewAggregate(errs)
}

// sortStringSlice is a tiny wrapper that returns
// the slice after sorting.
func sortStringSlice(s []string) []string {
	sort.Strings(s)
	return s
}

func deduplicateTideQueries(queries TideQueries) (TideQueries, error) {
	m := tideQueryMap{}
	for _, query := range queries {
		key := tideQueryConfig{
			Author:                 query.Author,
			ExcludedBranches:       sortStringSlice(query.ExcludedBranches),
			IncludedBranches:       sortStringSlice(query.IncludedBranches),
			Labels:                 sortStringSlice(query.Labels),
			MissingLabels:          sortStringSlice(query.MissingLabels),
			Milestone:              query.Milestone,
			ReviewApprovedRequired: query.ReviewApprovedRequired,
		}
		keyRaw, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %+v: %w", key, err)
		}
		val, ok := m[string(keyRaw)]
		if !ok {
			val = &tideQueryTarget{}
			m[string(keyRaw)] = val
		}
		val.Orgs = append(val.Orgs, query.Orgs...)
		val.Repos = append(val.Repos, query.Repos...)
		val.ExcludedRepos = append(val.ExcludedRepos, query.ExcludedRepos...)
	}

	return m.queries()
}

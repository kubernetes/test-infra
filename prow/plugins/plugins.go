/*
Copyright 2016 The Kubernetes Authors.

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

package plugins

import (
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/slack"
)

var (
	allPlugins                 = map[string]struct{}{}
	genericCommentHandlers     = map[string]GenericCommentHandler{}
	issueHandlers              = map[string]IssueHandler{}
	issueCommentHandlers       = map[string]IssueCommentHandler{}
	pullRequestHandlers        = map[string]PullRequestHandler{}
	pushEventHandlers          = map[string]PushEventHandler{}
	reviewEventHandlers        = map[string]ReviewEventHandler{}
	reviewCommentEventHandlers = map[string]ReviewCommentEventHandler{}
	statusEventHandlers        = map[string]StatusEventHandler{}
)

type IssueHandler func(PluginClient, github.IssueEvent) error

func RegisterIssueHandler(name string, fn IssueHandler) {
	allPlugins[name] = struct{}{}
	issueHandlers[name] = fn
}

type IssueCommentHandler func(PluginClient, github.IssueCommentEvent) error

func RegisterIssueCommentHandler(name string, fn IssueCommentHandler) {
	allPlugins[name] = struct{}{}
	issueCommentHandlers[name] = fn
}

type PullRequestHandler func(PluginClient, github.PullRequestEvent) error

func RegisterPullRequestHandler(name string, fn PullRequestHandler) {
	allPlugins[name] = struct{}{}
	pullRequestHandlers[name] = fn
}

type StatusEventHandler func(PluginClient, github.StatusEvent) error

func RegisterStatusEventHandler(name string, fn StatusEventHandler) {
	allPlugins[name] = struct{}{}
	statusEventHandlers[name] = fn
}

type PushEventHandler func(PluginClient, github.PushEvent) error

func RegisterPushEventHandler(name string, fn PushEventHandler) {
	allPlugins[name] = struct{}{}
	pushEventHandlers[name] = fn
}

type ReviewEventHandler func(PluginClient, github.ReviewEvent) error

func RegisterReviewEventHandler(name string, fn ReviewEventHandler) {
	allPlugins[name] = struct{}{}
	reviewEventHandlers[name] = fn
}

type ReviewCommentEventHandler func(PluginClient, github.ReviewCommentEvent) error

func RegisterReviewCommentEventHandler(name string, fn ReviewCommentEventHandler) {
	allPlugins[name] = struct{}{}
	reviewCommentEventHandlers[name] = fn
}

type GenericCommentHandler func(PluginClient, github.GenericCommentEvent) error

func RegisterGenericCommentHandler(name string, fn GenericCommentHandler) {
	allPlugins[name] = struct{}{}
	genericCommentHandlers[name] = fn
}

// PluginClient may be used concurrently, so each entry must be thread-safe.
type PluginClient struct {
	GitHubClient *github.Client
	KubeClient   *kube.Client
	GitClient    *git.Client
	SlackClient  *slack.Client

	// Config provides information about the jobs
	// that we know how to run for repos.
	Config *config.Config
	// PluginConfig provides plugin-specific options
	PluginConfig *Configuration

	Logger *logrus.Entry
}

type PluginAgent struct {
	PluginClient

	mut           sync.Mutex
	configuration *Configuration
}

// Configuration is the top-level serialization
// target for plugin Configuration
type Configuration struct {
	// Repo (eg "k/k") -> list of handler names.
	Plugins  map[string][]string `json:"plugins,omitempty"`
	Triggers []Trigger           `json:"triggers,omitempty"`
	Heart    Heart               `json:"heart,omitempty"`
	Label    Label               `json:"label,omitempty"`
	Slack    Slack               `json:"slack,omitempty"`
	// ConfigUpdater holds config for the config-updater plugin.
	ConfigUpdater ConfigUpdater `json:"config_updater,omitempty"`
}

type Trigger struct {
	// Repos is either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// TrustedOrg is the org whose members' PRs will be automatically built
	// for PRs to the above repos. The default is the PR's org.
	TrustedOrg string `json:"trusted_org,omitempty"`
}

type Heart struct {
	// Adorees is a list of GitHub logins for members
	// for whom we will add emojis to comments
	Adorees []string `json:"adorees,omitempty"`
}

type Label struct {
	// SigOrg is the organization that owns the
	// special interest groups tagged in this repo
	SigOrg string `json:"sig_org,omitempty"`
	// ID of the github team for the milestone maintainers (used for setting status labels)
	// You can curl the following endpoint in order to determine the github ID of your team
	// responsible for maintaining the milestones:
	// curl -H "Authorization: token <token>" https://api.github.com/orgs/<org-name>/teams
	MilestoneMaintainersID int `json:"milestone_maintainers_id,omitempty"`
}

type Slack struct {
	MentionChannels []string       `json:"mentionchannels,omitempty"`
	MergeWarnings   []MergeWarning `json:"mergewarnings,omitempty"`
}

type ConfigUpdater struct {
	// The location of the prow configuration file inside the repository
	// where the config-updater plugin is enabled. This needs to be relative
	// to the root of the repository, eg. "prow/config.yaml" will match
	// github.com/kubernetes/test-infra/prow/config.yaml assuming the config-updater
	// plugin is enabled for kubernetes/test-infra. Defaults to "prow/config.yaml".
	ConfigFile string `json:"config_file,omitempty"`
	// The location of the prow plugin configuration file inside the repository
	// where the config-updater plugin is enabled. This needs to be relative
	// to the root of the repository, eg. "prow/plugins.yaml" will match
	// github.com/kubernetes/test-infra/prow/plugins.yaml assuming the config-updater
	// plugin is enabled for kubernetes/test-infra. Defaults to "prow/plugins.yaml".
	PluginFile string `json:"plugin_file,omitempty"`
}

// MergeWarning is a config for the slackevents plugin's manual merge warings.
// If a PR is pushed to any of the repos listed in the config
// then send messages to the all the  slack channels listed if pusher is NOT in the whitelist.
type MergeWarning struct {
	// Repos is either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// List of channels on which a event is published.
	Channels []string `json:"channels,omitempty"`
	// A slack event is published if the user is not part of the WhiteList.
	WhiteList []string `json:"whitelist,omitempty"`
}

// TriggerFor finds the Trigger for a repo, if one exists
// a trigger can be listed for the repo itself or for the
// owning organization
func (c *Configuration) TriggerFor(org, repo string) *Trigger {
	for _, tr := range c.Triggers {
		for _, r := range tr.Repos {
			if r == org || r == fmt.Sprintf("%s/%s", org, repo) {
				return &tr
			}
		}
	}
	return nil
}

func (c *Configuration) setDefaults() {
	if c.ConfigUpdater.ConfigFile == "" {
		c.ConfigUpdater.ConfigFile = "prow/config.yaml"
	}
	if c.ConfigUpdater.PluginFile == "" {
		c.ConfigUpdater.PluginFile = "prow/plugins.yaml"
	}
}

// Load attempts to load config from the path. It returns an error if either
// the file can't be read or it contains an unknown plugin.
func (pa *PluginAgent) Load(path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	np := &Configuration{}
	if err := yaml.Unmarshal(b, np); err != nil {
		return err
	}

	if len(np.Plugins) == 0 {
		logrus.Warn("no plugins specified-- check syntax?")
	}

	if err := validatePlugins(np.Plugins); err != nil {
		return err
	}
	np.setDefaults()
	pa.Set(np)
	return nil
}

func (pa *PluginAgent) Config() *Configuration {
	pa.mut.Lock()
	defer pa.mut.Unlock()
	return pa.configuration
}

// validatePlugins will return error if
// there are unknown or duplicated plugins.
func validatePlugins(plugins map[string][]string) error {
	errors := []string{}
	for _, configuration := range plugins {
		for _, plugin := range configuration {
			if _, ok := allPlugins[plugin]; !ok {
				errors = append(errors, fmt.Sprintf("unknown plugin: %s", plugin))
			}
		}
	}
	for repo, repoConfig := range plugins {
		if strings.Contains(repo, "/") {
			org := strings.Split(repo, "/")[0]
			if dupes := findDuplicatedPluginConfig(repoConfig, plugins[org]); len(dupes) > 0 {
				errors = append(errors, fmt.Sprintf("plugins %v are duplicated for %s and %s", dupes, repo, org))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("invalid plugin configuration:\n\t%v", strings.Join(errors, "\n\t"))
	}
	return nil
}

func findDuplicatedPluginConfig(repoConfig, orgConfig []string) []string {
	dupes := []string{}
	for _, repoPlugin := range repoConfig {
		for _, orgPlugin := range orgConfig {
			if repoPlugin == orgPlugin {
				dupes = append(dupes, repoPlugin)
			}
		}
	}

	return dupes
}

// Set attempts to set the plugins that are enabled on repos. Plugins are listed
// as a map from repositories to the list of plugins that are enabled on them.
// Specifying simply an org name will also work, and will enable the plugin on
// all repos in the org.
func (pa *PluginAgent) Set(pc *Configuration) {
	pa.mut.Lock()
	defer pa.mut.Unlock()
	pa.configuration = pc
}

// Start starts polling path for plugin config. If the first attempt fails,
// then start returns the error. Future errors will halt updates but not stop.
func (pa *PluginAgent) Start(path string) error {
	if err := pa.Load(path); err != nil {
		return err
	}
	ticker := time.Tick(1 * time.Minute)
	go func() {
		for range ticker {
			if err := pa.Load(path); err != nil {
				logrus.WithField("path", path).WithError(err).Error("Error loading plugin config.")
			}
		}
	}()
	return nil
}

// GenericCommentHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) GenericCommentHandlers(owner, repo string) map[string]GenericCommentHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]GenericCommentHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := genericCommentHandlers[p]; ok {
			hs[p] = h
		}
	}
	return hs
}

// IssueHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) IssueHandlers(owner, repo string) map[string]IssueHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]IssueHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := issueHandlers[p]; ok {
			hs[p] = h
		}
	}
	return hs
}

// IssueCommentHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) IssueCommentHandlers(owner, repo string) map[string]IssueCommentHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]IssueCommentHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := issueCommentHandlers[p]; ok {
			hs[p] = h
		}
	}

	return hs
}

// PullRequestHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) PullRequestHandlers(owner, repo string) map[string]PullRequestHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]PullRequestHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := pullRequestHandlers[p]; ok {
			hs[p] = h
		}
	}

	return hs
}

// ReviewEventHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) ReviewEventHandlers(owner, repo string) map[string]ReviewEventHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]ReviewEventHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := reviewEventHandlers[p]; ok {
			hs[p] = h
		}
	}

	return hs
}

// ReviewCommentEventHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) ReviewCommentEventHandlers(owner, repo string) map[string]ReviewCommentEventHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]ReviewCommentEventHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := reviewCommentEventHandlers[p]; ok {
			hs[p] = h
		}
	}

	return hs
}

// StatusEventHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) StatusEventHandlers(owner, repo string) map[string]StatusEventHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]StatusEventHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := statusEventHandlers[p]; ok {
			hs[p] = h
		}
	}

	return hs
}

// PushEventHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) PushEventHandlers(owner, repo string) map[string]PushEventHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]PushEventHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := pushEventHandlers[p]; ok {
			hs[p] = h
		}
	}

	return hs
}

// getPlugins returns a list of plugins that are enabled on a given (org, repository).
func (pa *PluginAgent) getPlugins(owner, repo string) []string {
	var plugins []string

	fullName := fmt.Sprintf("%s/%s", owner, repo)
	plugins = append(plugins, pa.configuration.Plugins[owner]...)
	plugins = append(plugins, pa.configuration.Plugins[fullName]...)

	return plugins
}

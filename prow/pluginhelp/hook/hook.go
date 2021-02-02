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

// Package hook provides the plugin help components to be compiled into the hook binary.
// This includes the code to fetch help from normal and external plugins and the code to build and
// serve a pluginhelp.Help struct.
package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	prowconfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
	"k8s.io/test-infra/prow/plugins"
)

// TODO: unit test to ensure that external plugins with the same name have the same endpoint and events.

const (
	// newRepoDetectionLimit is the maximum allowable time before a repo will appear in the help
	// information if it is a new repo that is only referenced via its parent org in the config.
	// (i.e. max time before help is available for brand new "kubernetes/foo" repo if only
	// "kubernetes" is listed in the config)
	newRepoDetectionLimit = time.Hour
)

type pluginAgent interface {
	Config() *plugins.Configuration
}

type githubClient interface {
	GetRepos(org string, isUser bool) ([]github.Repo, error)
}

// HelpAgent is a handler that generates and serve plugin help information.
type HelpAgent struct {
	log *logrus.Entry
	pa  pluginAgent
	oa  *orgAgent
}

// NewHelpAgent constructs a new HelpAgent.
func NewHelpAgent(pa pluginAgent, ghc githubClient) *HelpAgent {
	l := logrus.WithField("client", "plugin-help")
	return &HelpAgent{
		log: l,
		pa:  pa,
		oa:  newOrgAgent(l, ghc, newRepoDetectionLimit),
	}
}

func (ha *HelpAgent) generateNormalPluginHelp(config *plugins.Configuration, revMap map[string][]prowconfig.OrgRepo) (allPlugins []string, pluginHelp map[string]pluginhelp.PluginHelp) {
	pluginHelp = map[string]pluginhelp.PluginHelp{}
	for name, provider := range plugins.HelpProviders() {
		allPlugins = append(allPlugins, name)
		if provider == nil {
			ha.log.Warnf("No help is provided for plugin %q.", name)
			continue
		}
		help, err := provider(config, revMap[name])
		if err != nil {
			ha.log.WithError(err).Errorf("Generating help from normal plugin %q.", name)
			continue
		}
		help.Events = plugins.EventsForPlugin(name)
		pluginHelp[name] = *help
	}
	return
}

func (ha *HelpAgent) generateExternalPluginHelp(config *plugins.Configuration, revMap map[string][]prowconfig.OrgRepo) (allPlugins []string, pluginHelp map[string]pluginhelp.PluginHelp) {
	externals := map[string]plugins.ExternalPlugin{}
	for _, exts := range config.ExternalPlugins {
		for _, ext := range exts {
			externals[ext.Name] = ext
		}
	}

	type externalResult struct {
		name string
		help *pluginhelp.PluginHelp
	}
	externalResultChan := make(chan externalResult, len(externals))
	for _, ext := range externals {
		allPlugins = append(allPlugins, ext.Name)
		go func(ext plugins.ExternalPlugin) {
			help, err := externalHelpProvider(ha.log, ext.Endpoint)(revMap[ext.Name])
			if err != nil {
				ha.log.WithError(err).Errorf("Getting help from external plugin %q.", ext.Name)
				help = nil
			} else {
				help.Events = ext.Events
			}
			externalResultChan <- externalResult{name: ext.Name, help: help}
		}(ext)
	}

	pluginHelp = map[string]pluginhelp.PluginHelp{}
	timeout := time.After(time.Second)
Done:
	for {
		select {
		case <-timeout:
			break Done
		case result, ok := <-externalResultChan:
			if !ok {
				break Done
			}
			if result.help == nil {
				continue
			}
			pluginHelp[result.name] = *result.help
		}
	}
	return
}

// GeneratePluginHelp compiles and returns the help information for all plugins.
func (ha *HelpAgent) GeneratePluginHelp() *pluginhelp.Help {
	config := ha.pa.Config()
	orgToRepos := ha.oa.orgToReposMap(config)

	normalRevMap, externalRevMap := reversePluginMaps(config, orgToRepos)

	allPlugins, pluginHelp := ha.generateNormalPluginHelp(config, normalRevMap)

	allExternalPlugins, externalPluginHelp := ha.generateExternalPluginHelp(config, externalRevMap)

	// Load repo->plugins maps from config
	repoPlugins := map[string][]string{
		"": allPlugins,
	}
	for repo, plugins := range config.Plugins {
		repoPlugins[repo] = plugins.Plugins
	}
	repoExternalPlugins := map[string][]string{
		"": allExternalPlugins,
	}
	for repo, exts := range config.ExternalPlugins {
		for _, ext := range exts {
			repoExternalPlugins[repo] = append(repoExternalPlugins[repo], ext.Name)
		}
	}

	return &pluginhelp.Help{
		AllRepos:            allRepos(config, orgToRepos),
		RepoPlugins:         repoPlugins,
		RepoExternalPlugins: repoExternalPlugins,
		PluginHelp:          pluginHelp,
		ExternalPluginHelp:  externalPluginHelp,
	}
}

func allRepos(config *plugins.Configuration, orgToRepos map[string]sets.String) []string {
	all := sets.NewString()
	for repo := range config.Plugins {
		all.Insert(repo)
	}
	for repo := range config.ExternalPlugins {
		all.Insert(repo)
	}

	flattened := sets.NewString()
	for repo := range all {
		if strings.Contains(repo, "/") {
			flattened.Insert(repo)
			continue
		}
		flattened = flattened.Union(orgToRepos[repo])
	}
	return flattened.List()
}

func externalHelpProvider(log *logrus.Entry, endpoint string) externalplugins.ExternalPluginHelpProvider {
	return func(enabledRepos []prowconfig.OrgRepo) (*pluginhelp.PluginHelp, error) {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("error parsing url: %s err: %v", endpoint, err)
		}
		u.Path = path.Join(u.Path, "/help")
		b, err := json.Marshal(enabledRepos)
		if err != nil {
			return nil, fmt.Errorf("error marshalling enabled repos: %q, err: %v", enabledRepos, err)
		}

		// Don't retry because user is waiting for response to their browser.
		// If there is an error the user can refresh to get the plugin info that failed to load.
		urlString := u.String()
		resp, err := http.Post(urlString, "application/json", bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("error posting to %s err: %v", urlString, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("post to %s failed with status %d: %s", urlString, resp.StatusCode, resp.Status)
		}
		var help pluginhelp.PluginHelp
		if err := json.NewDecoder(resp.Body).Decode(&help); err != nil {
			return nil, fmt.Errorf("failed to decode json response from %s err: %v", urlString, err)
		}
		return &help, nil
	}
}

// reversePluginMaps inverts the Configuration.Plugins and Configuration.ExternalPlugins maps and
// expands any org strings to org/repo strings.
// The returned values map plugin names to the set of org/repo strings they are enabled on.
func reversePluginMaps(config *plugins.Configuration, orgToRepos map[string]sets.String) (normal, external map[string][]prowconfig.OrgRepo) {
	normal = map[string][]prowconfig.OrgRepo{}
	for repo, enabledPlugins := range config.Plugins {
		var repos []prowconfig.OrgRepo
		if !strings.Contains(repo, "/") {
			if flattened, ok := orgToRepos[repo]; ok {
				repos = prowconfig.StringsToOrgRepos(flattened.List())
			}
		} else {
			repos = []prowconfig.OrgRepo{*prowconfig.NewOrgRepo(repo)}
		}
		for _, plugin := range enabledPlugins.Plugins {
			normal[plugin] = append(normal[plugin], repos...)
		}
	}
	external = map[string][]prowconfig.OrgRepo{}
	for repo, extPlugins := range config.ExternalPlugins {
		var repos []prowconfig.OrgRepo
		if flattened, ok := orgToRepos[repo]; ok {
			repos = prowconfig.StringsToOrgRepos(flattened.List())
		} else {
			repos = []prowconfig.OrgRepo{*prowconfig.NewOrgRepo(repo)}
		}
		for _, plugin := range extPlugins {
			external[plugin.Name] = append(external[plugin.Name], repos...)
		}
	}
	return
}

// orgAgent provides a cached mapping of orgs to the repos that are in that org.
// Caching is necessary to prevent excessive github API token usage.
type orgAgent struct {
	log *logrus.Entry
	ghc githubClient

	syncPeriod time.Duration

	lock       sync.Mutex
	nextSync   time.Time
	orgs       sets.String
	orgToRepos map[string]sets.String
}

func newOrgAgent(log *logrus.Entry, ghc githubClient, syncPeriod time.Duration) *orgAgent {
	return &orgAgent{
		log:        log,
		ghc:        ghc,
		syncPeriod: syncPeriod,
	}
}

func (oa *orgAgent) orgToReposMap(config *plugins.Configuration) map[string]sets.String {
	oa.lock.Lock()
	defer oa.lock.Unlock()
	// Only need to sync if either:
	// - the sync period has passed (we sync periodically to pick up new repos in known orgs)
	//  or
	// - new org(s) have been added in the config
	var syncReason string
	if time.Now().After(oa.nextSync) {
		syncReason = "the sync period elapsed"
	} else if diff := orgsInConfig(config).Difference(oa.orgs); diff.Len() > 0 {
		syncReason = fmt.Sprintf("the following orgs were added to the config: %q", diff.List())
	}
	if syncReason != "" {
		oa.log.Infof("Syncing org to repos mapping because %s.", syncReason)
		oa.sync(config)
	}
	return oa.orgToRepos
}

func (oa *orgAgent) sync(config *plugins.Configuration) {

	// QUESTION: If we fail to list repos for a single org should we reuse the old orgToRepos or just
	// log the error and omit the org from orgToRepos as is done now?
	// I could remove the failed org from 'orgs' to force a resync the next time it is called, but
	// that could waste tokens if the call continues to fail for some reason.

	orgs := orgsInConfig(config)
	orgToRepos := map[string]sets.String{}
	for _, org := range orgs.List() {
		repos, err := oa.ghc.GetRepos(org, false /*isUser*/)
		if err != nil {
			oa.log.WithError(err).Errorf("Getting repos in org: %s.", org)
			// Remove 'org' from 'orgs' here to force future resync?
			continue
		}
		repoSet := sets.NewString()
		for _, repo := range repos {
			repoSet.Insert(repo.FullName)
		}
		orgToRepos[org] = repoSet
	}

	oa.orgs = orgs
	oa.orgToRepos = orgToRepos
	oa.nextSync = time.Now().Add(oa.syncPeriod)
}

// orgsInConfig gets all the org strings (not org/repo) in config.Plugins and config.ExternalPlugins.
func orgsInConfig(config *plugins.Configuration) sets.String {
	orgs := sets.NewString()
	for repo := range config.Plugins {
		if !strings.Contains(repo, "/") {
			orgs.Insert(repo)
		}
	}
	for repo := range config.ExternalPlugins {
		if !strings.Contains(repo, "/") {
			orgs.Insert(repo)
		}
	}
	return orgs
}

func (ha *HelpAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")

	serverError := func(action string, err error) {
		ha.log.WithError(err).Errorf("Error %s.", action)
		msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
		http.Error(w, msg, http.StatusInternalServerError)
	}

	if r.Method != http.MethodGet {
		ha.log.Errorf("Invalid request method: %v.", r.Method)
		http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	help := ha.GeneratePluginHelp()
	b, err := json.Marshal(help)
	if err != nil {
		serverError("marshaling plugin help", err)
		return
	}

	fmt.Fprint(w, string(b))
}

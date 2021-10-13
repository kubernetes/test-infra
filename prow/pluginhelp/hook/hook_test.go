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

package hook

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	prowconfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
	"k8s.io/test-infra/prow/plugins"
)

type fakeGitHubClient map[string][]string

func (fghc fakeGitHubClient) GetRepos(org string, _ bool) ([]github.Repo, error) {
	var repos []github.Repo
	for _, repo := range fghc[org] {
		repos = append(repos, github.Repo{FullName: fmt.Sprintf("%s/%s", org, repo)})
	}
	return repos, nil
}

type fakePluginAgent plugins.Configuration

func (fpa fakePluginAgent) Config() *plugins.Configuration {
	config := plugins.Configuration(fpa)
	return &config
}

func TestGeneratePluginHelp(t *testing.T) {
	orgToRepos := map[string][]string{"org1": {"repo1", "repo2", "repo3"}, "org2": {"repo1"}}
	fghc := fakeGitHubClient(orgToRepos)

	normalHelp := map[string]pluginhelp.PluginHelp{
		"org-plugin": {Description: "org-plugin", Config: map[string]string{"": "overall config"}},
		"repo-plugin1": {
			Description: "repo-plugin1",
			Config: map[string]string{
				"org1/repo1": "repo1 config",
				"org1/repo2": "repo2 config",
			},
		},
		"repo-plugin2": {Description: "repo-plugin2", Config: map[string]string{}},
		"repo-plugin3": {Description: "repo-plugin3", Config: map[string]string{}},
	}
	helpfulExternalHelp := pluginhelp.PluginHelp{Description: "helpful-external"}
	noHelpSever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "404 Not Found", http.StatusNotFound)
	}))
	defer noHelpSever.Close()
	mux := http.NewServeMux()
	externalplugins.ServeExternalPluginHelp(
		mux,
		logrus.WithField("plugin", "helpful-external"),
		func(enabledRepos []prowconfig.OrgRepo) (*pluginhelp.PluginHelp, error) {
			if got, expected := enabledRepos, []prowconfig.OrgRepo{{Org: "org1", Repo: "repo1"}}; !reflect.DeepEqual(got, expected) {
				t.Errorf("Plugin 'helpful-external' expected to be enabled on repos %q, but got %q.", expected, got)
			}
			return &helpfulExternalHelp, nil
		},
	)
	helpfulServer := httptest.NewServer(mux)
	defer helpfulServer.Close()

	config := &plugins.Configuration{
		Plugins: plugins.Plugins{
			"org1":       {Plugins: []string{"org-plugin"}},
			"org1/repo1": {Plugins: []string{"repo-plugin1", "no-help-plugin"}},
			"org1/repo2": {Plugins: []string{"repo-plugin1", "repo-plugin2"}},
			"org2/repo1": {Plugins: []string{"repo-plugin3"}},
		},
		ExternalPlugins: map[string][]plugins.ExternalPlugin{
			"org1/repo1": {
				{Name: "no-endpoint-external", Endpoint: "http://no-endpoint-external", Events: []string{"issue_comment"}},
				{Name: "no-help-external", Endpoint: noHelpSever.URL, Events: []string{"issue_comment"}},
				{Name: "helpful-external", Endpoint: helpfulServer.URL, Events: []string{"pull_request", "issue"}},
			},
		},
	}
	fpa := fakePluginAgent(*config)

	expectedAllRepos := []string{"org1/repo1", "org1/repo2", "org1/repo3", "org2/repo1"}

	normalExpectedReposForPlugin := map[string][]string{
		"org-plugin":     {"org1/repo1", "org1/repo2", "org1/repo3"},
		"repo-plugin1":   {"org1/repo1", "org1/repo2"},
		"repo-plugin2":   {"org1/repo2"},
		"repo-plugin3":   {"org2/repo1"},
		"no-help-plugin": {"org1/repo1"},
	}
	normalExpectedPluginsForRepo := map[string][]string{
		"":           {"org-plugin", "repo-plugin1", "repo-plugin2", "repo-plugin3", "no-help-plugin"},
		"org1":       {"org-plugin"},
		"org1/repo1": {"repo-plugin1", "no-help-plugin"},
		"org1/repo2": {"repo-plugin1", "repo-plugin2"},
		"org2/repo1": {"repo-plugin3"},
	}
	normalExpectedEvents := map[string][]string{
		"org-plugin":     {"issue_comment"},
		"repo-plugin1":   {"issue"},
		"repo-plugin2":   {"pull_request"},
		"repo-plugin3":   {"pull_request_review", "pull_request_review_comment"},
		"no-help-plugin": {"issue_comment"},
	}

	externalExpectedPluginsForRepo := map[string][]string{
		"":           {"no-endpoint-external", "no-help-external", "helpful-external"},
		"org1/repo1": {"no-endpoint-external", "no-help-external", "helpful-external"},
	}
	externalExpectedEvents := map[string][]string{
		"no-endpoint-external": {"issue_comment"},
		"no-help-external":     {"issue_comment"},
		"helpful-external":     {"pull_request", "issue"},
	}

	registerNormalPlugins(t, normalExpectedEvents, normalHelp, normalExpectedReposForPlugin)

	help := NewHelpAgent(fpa, fghc).GeneratePluginHelp()
	if help == nil {
		t.Fatal("NewHelpAgent returned nil HelpAgent struct pointer.")
	}
	if got, expected := sets.NewString(help.AllRepos...), sets.NewString(expectedAllRepos...); !got.Equal(expected) {
		t.Errorf("Expected 'AllRepos' to be %q, but got %q.", expected.List(), got.List())
	}
	checkPluginsForRepo := func(expected, got map[string][]string) {
		for _, plugins := range expected {
			sort.Strings(plugins)
		}
		for _, plugins := range got {
			sort.Strings(plugins)
		}
		if !reflect.DeepEqual(expected, got) {
			t.Errorf("Expected repo->plugin map %v, but got %v.", expected, got)
		}
	}
	checkPluginsForRepo(normalExpectedPluginsForRepo, help.RepoPlugins)
	checkPluginsForRepo(externalExpectedPluginsForRepo, help.RepoExternalPlugins)

	checkPluginHelp := func(plugin string, expected, got pluginhelp.PluginHelp, eventsForPlugin []string) {
		sort.Strings(eventsForPlugin)
		sort.Strings(got.Events)
		if expected, got := eventsForPlugin, got.Events; !reflect.DeepEqual(expected, got) {
			t.Errorf("Expected plugin '%s' to subscribe to events %q, but got %q.", plugin, expected, got)
		}
		// Events field is correct, everything else should match the input exactly.
		got.Events = nil
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("Expected plugin '%s' to have help: %v, but got %v.", plugin, expected, got)
		}
	}

	for plugin, expected := range normalHelp {
		checkPluginHelp(plugin, expected, help.PluginHelp[plugin], normalExpectedEvents[plugin])
	}
	checkPluginHelp("helpful-external", helpfulExternalHelp, help.ExternalPluginHelp["helpful-external"], externalExpectedEvents["helpful-external"])
}

func registerNormalPlugins(t *testing.T, pluginsToEvents map[string][]string, pluginHelp map[string]pluginhelp.PluginHelp, expectedRepos map[string][]string) {
	for plugin, events := range pluginsToEvents {
		plugin := plugin
		helpProvider := func(_ *plugins.Configuration, enabledRepos []prowconfig.OrgRepo) (*pluginhelp.PluginHelp, error) {
			if got, expected := sets.NewString(prowconfig.OrgReposToStrings(enabledRepos)...), sets.NewString(expectedRepos[plugin]...); !got.Equal(expected) {
				t.Errorf("Plugin '%s' expected to be enabled on repos %q, but got %q.", plugin, expected.List(), got.List())
			}
			help := pluginHelp[plugin]
			return &help, nil
		}
		for _, event := range events {
			switch event {
			case "issue_comment":
				plugins.RegisterIssueCommentHandler(plugin, nil, helpProvider)
			case "issue":
				plugins.RegisterIssueHandler(plugin, nil, helpProvider)
			case "pull_request":
				plugins.RegisterPullRequestHandler(plugin, nil, helpProvider)
			case "pull_request_review":
				plugins.RegisterReviewEventHandler(plugin, nil, helpProvider)
			case "pull_request_review_comment":
				plugins.RegisterReviewCommentEventHandler(plugin, nil, helpProvider)
			default:
				t.Fatalf("Invalid test! Unknown event type '%s' for plugin '%s'.", event, plugin)
			}
		}
	}
}

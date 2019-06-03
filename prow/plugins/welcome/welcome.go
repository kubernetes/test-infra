/*
Copyright 2018 The Kubernetes Authors.

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

// Package welcome implements a prow plugin to welcome new contributors
package welcome

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName            = "welcome"
	defaultWelcomeMessage = "Welcome @{{.AuthorLogin}}! It looks like this is your first PR to {{.Org}}/{{.Repo}} 🎉"
)

// PRInfo contains info used provided to the welcome message template
type PRInfo struct {
	Org         string
	Repo        string
	AuthorLogin string
	AuthorName  string
}

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	welcomeConfig := map[string]string{}
	for _, repo := range enabledRepos {
		parts := strings.Split(repo, "/")
		var messageTemplate string
		switch len(parts) {
		case 1:
			messageTemplate = welcomeMessageForRepo(config, repo, "")
		case 2:
			messageTemplate = welcomeMessageForRepo(config, parts[0], parts[1])
		default:
			return nil, fmt.Errorf("invalid repo in enabledRepos: %q", repo)
		}
		welcomeConfig[repo] = fmt.Sprintf("The welcome plugin is configured to post using following welcome template: %s.", messageTemplate)
	}

	// The {WhoCanUse, Usage, Examples} fields are omitted because this plugin is not triggered with commands.
	return &pluginhelp.PluginHelp{
			Description: "The welcome plugin posts a welcoming message when it detects a user's first contribution to a repo.",
			Config:      welcomeConfig,
		},
		nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	FindIssues(query, sort string, asc bool) ([]github.Issue, error)
}

type client struct {
	GitHubClient githubClient
	Logger       *logrus.Entry
}

func getClient(pc plugins.Agent) client {
	return client{
		GitHubClient: pc.GitHubClient,
		Logger:       pc.Logger,
	}
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	return handlePR(getClient(pc), pre, welcomeMessageForRepo(pc.PluginConfig, pre.Repo.Owner.Login, pre.Repo.Name))
}

func handlePR(c client, pre github.PullRequestEvent, welcomeTemplate string) error {
	// Only consider newly opened PRs
	if pre.Action != github.PullRequestActionOpened {
		return nil
	}

	// search for PRs from the author in this repo
	org := pre.PullRequest.Base.Repo.Owner.Login
	repo := pre.PullRequest.Base.Repo.Name
	user := pre.PullRequest.User.Login
	query := fmt.Sprintf("is:pr repo:%s/%s author:%s", org, repo, user)
	issues, err := c.GitHubClient.FindIssues(query, "", false)
	if err != nil {
		return err
	}

	// if there are no results, this is the first! post the welcome comment
	if len(issues) == 0 || len(issues) == 1 && issues[0].Number == pre.Number {
		// load the template, and run it over the PR info
		parsedTemplate, err := template.New("welcome").Parse(welcomeTemplate)
		if err != nil {
			return err
		}
		var msgBuffer bytes.Buffer
		err = parsedTemplate.Execute(&msgBuffer, PRInfo{
			Org:         org,
			Repo:        repo,
			AuthorLogin: user,
			AuthorName:  pre.PullRequest.User.Name,
		})
		if err != nil {
			return err
		}

		// actually post the comment
		return c.GitHubClient.CreateComment(org, repo, pre.PullRequest.Number, msgBuffer.String())
	}

	return nil
}

func welcomeMessageForRepo(config *plugins.Configuration, org, repo string) string {
	opts := optionsForRepo(config, org, repo)
	if opts.MessageTemplate != "" {
		return opts.MessageTemplate
	}
	return defaultWelcomeMessage
}

// optionsForRepo gets the plugins.Welcome struct that is applicable to the indicated repo.
func optionsForRepo(config *plugins.Configuration, org, repo string) *plugins.Welcome {
	fullName := fmt.Sprintf("%s/%s", org, repo)

	// First search for repo config
	for _, c := range config.Welcome {
		if !sets.NewString(c.Repos...).Has(fullName) {
			continue
		}
		return &c
	}

	// If you don't find anything, loop again looking for an org config
	for _, c := range config.Welcome {
		if !sets.NewString(c.Repos...).Has(org) {
			continue
		}
		return &c
	}

	// Return an empty config, and default to defaultWelcomeMessage
	return &plugins.Welcome{}
}

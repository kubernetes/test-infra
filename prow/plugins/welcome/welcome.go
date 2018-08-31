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

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "welcome"
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
	// The {WhoCanUse, Usage, Examples} fields are omitted because this plugin is not triggered with commands.
	return &pluginhelp.PluginHelp{
			Description: "The welcome plugin posts a welcoming message when it detects a user's first contribution to a repo.",
			Config: map[string]string{
				"": fmt.Sprintf(
					"The welcome plugin is configured to post using following welcome template: %s.",
					config.Welcome.MessageTemplate,
				),
			},
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

func getClient(pc plugins.PluginClient) client {
	return client{
		GitHubClient: pc.GitHubClient,
		Logger:       pc.Logger,
	}
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	return handlePR(getClient(pc), pre, pc.PluginConfig.Welcome.MessageTemplate)
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

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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/trigger"
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

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	welcomeConfig := map[string]string{}
	for _, repo := range enabledRepos {
		messageTemplate := welcomeMessageForRepo(config, repo.Org, repo.Repo)
		welcomeConfig[repo.String()] = fmt.Sprintf("The welcome plugin is configured to post using following welcome template: %s.", messageTemplate)
	}

	// The {WhoCanUse, Usage, Examples} fields are omitted because this plugin is not triggered with commands.
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Welcome: []plugins.Welcome{
			{
				Repos: []string{
					"org/repo1",
					"org/repo2",
				},
				MessageTemplate: "Welcome @{{.AuthorLogin}}!",
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	return &pluginhelp.PluginHelp{
			Description: "The welcome plugin posts a welcoming message when it detects a user's first contribution to a repo.",
			Config:      welcomeConfig,
			Snippet:     yamlSnippet,
		},
		nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	FindIssues(query, sort string, asc bool) ([]github.Issue, error)
	IsCollaborator(org, repo, user string) (bool, error)
	IsMember(org, user string) (bool, error)
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
	t := pc.PluginConfig.TriggerFor(pre.PullRequest.Base.Repo.Owner.Login, pre.PullRequest.Base.Repo.Name)
	return handlePR(getClient(pc), t, pre, welcomeMessageForRepo(pc.PluginConfig, pre.Repo.Owner.Login, pre.Repo.Name))
}

func handlePR(c client, t plugins.Trigger, pre github.PullRequestEvent, welcomeTemplate string) error {
	// Only consider newly opened PRs
	if pre.Action != github.PullRequestActionOpened {
		return nil
	}

	// ignore bots, we can't query their PRs
	if pre.PullRequest.User.Type != github.UserTypeUser {
		return nil
	}

	org := pre.PullRequest.Base.Repo.Owner.Login
	repo := pre.PullRequest.Base.Repo.Name
	user := pre.PullRequest.User.Login

	trustedResponse, err := trigger.TrustedUser(c.GitHubClient, t.OnlyOrgMembers, t.TrustedOrg, user, org, repo)
	if err != nil {
		return fmt.Errorf("check if user %s is trusted: %v", user, err)
	}
	if trustedResponse.IsTrusted {
		return nil
	}

	// search for PRs from the author in this repo
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

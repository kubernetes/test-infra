package jira_linker

import (
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName  = "jira-linker"
	jiraPrefix  = "jira/"
	noJiraLabel = jiraPrefix + "no-ticket"
)

var (
	jiraRegex     = regexp.MustCompile("([A-Z]+)-\\d+")
	enabledEvents = []github.PullRequestEventAction{
		github.PullRequestActionOpened,
		github.PullRequestActionEdited,
		github.PullRequestActionReopened,
	}
)

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	CreateComment(owner, repo string, number int, comment string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func helpProvider(_ *plugins.Configuration, _ []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The jira-linker plugin tries to detect JIRA references in PR titles and automatically adds a link to the ticket and a label",
	}
	return pluginHelp, nil
}

func init() {
	plugins.RegisterPullRequestHandler(pluginName, pullRequestHandler, helpProvider)
}

func pullRequestHandler(pc plugins.PluginClient, event github.PullRequestEvent) error {
	return handle(pc.GitHubClient, pc.Logger, pc.PluginConfig.JiraLinker, &event)
}

func handle(gc githubClient, log *logrus.Entry, config plugins.JiraLinker, event *github.PullRequestEvent) error {
	// We only care about certain events, so ignore others - this significantly limits the number of race conditions
	//  that can cause a double comment
	relevantEvent := false
	for _, candidate := range enabledEvents {
		if event.Action == candidate {
			relevantEvent = true
		}
	}

	if !relevantEvent {
		return nil
	}

	org := event.Repo.Owner.Login
	repo := event.Repo.Name

	labels, err := gc.GetIssueLabels(org, repo, event.Number)
	if err != nil {
		log.WithError(err).Errorf("Failed to get the labels on %s/%s#%d.", org, repo, event.Number)
	}

	matches := jiraRegex.FindStringSubmatch(event.PullRequest.Title)
	if len(matches) > 1 {
		ticketName := matches[0]
		jiraTeamName := matches[1]

		hasLabel := false
		for _, candidate := range labels {
			if candidate.Name == noJiraLabel {
				gc.RemoveLabel(org, repo, event.Number, noJiraLabel)
			}

			if candidate.Name == jiraLabel(jiraTeamName) {
				hasLabel = true
			}
		}

		if !hasLabel {
			gc.CreateComment(org, repo, event.Number, commentForTicket(jiraLink(config.JiraBaseUrl, ticketName)))
			gc.AddLabel(org, repo, event.Number, jiraLabel(jiraTeamName))
		}
	} else {
		hasNoJiraLabel := false
		for _, candidate := range labels {
			if candidate.Name != noJiraLabel && strings.HasPrefix(candidate.Name, jiraPrefix) {
				gc.RemoveLabel(org, repo, event.Number, candidate.Name)
			}

			if candidate.Name == noJiraLabel {
				hasNoJiraLabel = true
			}
		}

		if !hasNoJiraLabel {
			gc.AddLabel(org, repo, event.Number, noJiraLabel)
		}
	}

	return nil
}

func jiraLabel(team string) string {
	return jiraPrefix + team
}

func commentForTicket(jiraLink string) string {
	return "Corresponding JIRA ticket: " + jiraLink
}

func jiraLink(jiraBaseUrl, ticketName string) string {
	return jiraBaseUrl + "/browse/" + ticketName
}

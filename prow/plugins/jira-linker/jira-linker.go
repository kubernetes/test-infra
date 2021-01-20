package jira_linker

import (
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
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
	jiraTitleRegex  = regexp.MustCompile("([A-Z]+)-\\d+")
	jiraBranchRegex = regexp.MustCompile("^\\w+/(([A-Za-z]+)-\\d+)(-|$|_)")
	enabledEvents   = []github.PullRequestEventAction{
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

func helpProvider(_ *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The jira-linker plugin tries to detect JIRA references in PR titles and automatically adds a link to the ticket and a label",
	}
	return pluginHelp, nil
}

func init() {
	plugins.RegisterPullRequestHandler(pluginName, pullRequestHandler, helpProvider)
}

func pullRequestHandler(pc plugins.Agent, event github.PullRequestEvent) error {
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

	found, jiraTeamName, ticketName := extractJiraTicketDetails(event.PullRequest.Title, event.PullRequest.Head.Ref)
	if found {
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
			jiraServerURL := config.JiraBaseUrl
			out:
			for _, v := range config.JiraOverrides{
				for _, x := range v.Repos {
					if x == repo {
						jiraServerURL = v.JiraUrl
						break out
					}
				}
			}
			gc.CreateComment(org, repo, event.Number, commentForTicket(jiraLink(jiraServerURL, ticketName)))
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

// Returns if found, and if so respectively the ticket type (e.g. ENG) and the ticket ref (e.g. ENG-23)
func extractJiraTicketDetails(title string, ref string) (bool, string, string) {
	matches := jiraTitleRegex.FindStringSubmatch(title)
	if len(matches) > 0 {
		return true, matches[1], matches[0]
	}
	matches = jiraBranchRegex.FindStringSubmatch(ref)
	if len(matches) > 0 {
		return true, strings.ToUpper(matches[2]), strings.ToUpper(matches[1])
	}

	return false, "", ""
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

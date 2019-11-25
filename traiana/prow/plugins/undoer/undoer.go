package undoer

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "okro/undoer"
)

var (
	reservedLabels = []string{
		"do-not-merge",
		labels.Approved,
		labels.BlockedPaths,
		labels.CpApproved,
		labels.CpUnapproved,
		labels.Hold,
		labels.InvalidOwners,
		labels.LGTM,
		labels.NeedsOkToTest,
		labels.NeedsRebase,
		labels.OkToTest,
		labels.WorkInProgress,
	}
)

func init() {
	plugins.RegisterPriorityPullRequestHandler(pluginName, handlePullRequestEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The undoer plugin removes reserved labels that are added to PRs directly and not through commands",
	}
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	RemoveLabel(org, repo string, number int, label string) error
	BotName() (string, error)
}

func handlePullRequestEvent(pc plugins.Agent, pre github.PullRequestEvent) (plugins.HandlerResult, error) {
	return handlePullRequest(pc.Logger, pc.GitHubClient, &pre)
}

func handlePullRequest(log *logrus.Entry, ghc githubClient, pe *github.PullRequestEvent) (plugins.HandlerResult, error) {
	if pe.Action != github.PullRequestActionLabeled || !isReserved(pe.Label.Name) {
		return plugins.ContinueResult, nil
	}

	botname, err := ghc.BotName()
	if err != nil {
		return plugins.ContinueResult, err
	}
	if pe.Sender.Login == botname {
		return plugins.ContinueResult, nil
	}

	org := pe.PullRequest.Base.Repo.Owner.Login
	repo := pe.PullRequest.Base.Repo.Name
	number := pe.PullRequest.Number

	msg := "@%s: You tried to add the `%s` label manually. This is forbidden, as this label is " +
		"reserved and should only be added using one of my commands. I'm going to remove the label now."
	if err := ghc.CreateComment(org, repo, number, fmt.Sprintf(msg, pe.Sender.Login, pe.Label.Name)); err != nil {
		log.WithError(err).Errorf("Failed to create comment in %s/%s#%d.", org, repo, number)
	}

	if err := ghc.RemoveLabel(pe.Repo.Owner.Login, pe.Repo.Name, pe.Number, pe.Label.Name); err != nil {
		log.WithError(err).Errorf("Failed to remove %q label from %s/%s#%d.", pe.Label.Name, org, repo, number)
	}
	return plugins.BreakResult, nil
}

func isReserved(label string) bool {
	for _, l := range reservedLabels {
		if l == label {
			return true
		}
	}
	return false
}

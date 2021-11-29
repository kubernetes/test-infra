package branch_updater

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	gitHubContextNameTide        = "tide"
	gitHubMergeableStateBehind   = "behind"
	gitHubMergeStateRefreshDelay = 10
	pluginName                   = "branch-updater"
	tideContextStatusSuccess     = "success"
)

var (
	// This plugin should ignore any event type not on this list.
	enabledEvents = []github.PullRequestEventAction{
		github.PullRequestActionOpened,
		github.PullRequestActionEdited,
		github.PullRequestActionReadyForReview,
		github.PullRequestActionReopened,
	}
)

type githubClient interface {
	UpdatePullRequestBranch(owner, repo string, number int, expectedHeadSha *string) error
	GetPullRequest(owner, repo string, number int) (*github.PullRequest, error)
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	configInfo := map[string]string{}
	for _, repo := range enabledRepos {
		repoUpdateTmpl := "PR branches in repo %s will not be automatically updated."
		if config.BranchUpdater.ShouldUpdateBranchesForRepo(repo.String()) {
			repoUpdateTmpl = "PR branches in repo %s will be automatically updated."
		}
		configInfo[repo.String()] = fmt.Sprintf(repoUpdateTmpl, repo)
	}

	yamlSnippet, _ := plugins.CommentMap.GenYaml(&plugins.Configuration{
		BranchUpdater: plugins.BranchUpdater{
			DefaultEnabled: true,
			IgnoredRepos:   []string{"db-improbable/do-not-update-me"},
			IncludeRepos:   []string{"db-improbable/please-update-me"},
		},
	})

	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The branch-updater plugin updates the branch when an otherwise-mergeable PR is blocked from merging because it's out of date from the HEAD branch",
		Config:      configInfo,
		Snippet:     yamlSnippet,
	}
	return pluginHelp, nil
}

func init() {
	plugins.RegisterPullRequestHandler(pluginName, pullRequestHandler, helpProvider)
}

func pullRequestHandler(pc plugins.Agent, event github.PullRequestEvent) error {
	return handlePR(pc.GitHubClient, pc.Logger, pc.PluginConfig.BranchUpdater, &event)
}

// If Tide thinks this PR should be mergeable, but GitHub says that the PR's branch is "behind",
// ask GitHub to update the branch for us.
func handlePR(gc githubClient, log *logrus.Entry, config plugins.BranchUpdater, event *github.PullRequestEvent) error {
	// Ignore any event subtype that we don't care about
	if !isEventRelevant(event.Action, enabledEvents) {
		return nil
	}

	org := event.Repo.Owner.Login
	repo := event.Repo.Name
	pr := event.PullRequest

	logger := log.WithFields(logrus.Fields{
		"org":            org,
		"repo":           repo,
		"pr":             pr,
		"mergeable":      *pr.Mergable,
		"mergeableState": pr.MergeableState,
	})

	// Find the Tide status.
	tideStatus, err := findTideStatus(gc, org, repo, pr)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get context statuses for PR.")
		return err
	}
	// Do nothing if Tide isn't in use here.
	if tideStatus == "" {
		logger.Debugf("Skipping PR: repo not monitored by Tide.")
		return nil
	}

	// And only take action if that status is success.
	if tideStatus != tideContextStatusSuccess {
		logger.Debugf("Skipping PR: not yet mergeable by Tide.")
		return nil
	}

	// If we have a "Tide mergeable" PR, check if GitHub agrees, and refresh our local PR state if we don't have an answer
	// https://docs.github.com/en/rest/guides/getting-started-with-the-git-database-api#checking-mergeability-of-pull-requests
	if pr.Mergable == nil {
		refreshLogger := logger.WithField("statusRefreshPeriod", gitHubMergeStateRefreshDelay)
		refreshLogger.Warnf("GitHub has not yet reported mergeable status for PR. Sleeping and retrying.")

		// Crude, but this should always be enough time for GitHub to reach a decision.
		// Worst case, we'll re-check the next time the PR is updated.
		time.Sleep(gitHubMergeStateRefreshDelay * time.Second)
		refreshedPr, err := gc.GetPullRequest(org, repo, pr.Number)
		if err != nil {
			refreshLogger.WithError(err).Errorf("Failed to refresh PR's mergable state")
			return err
		}
		if refreshedPr.Mergable == nil {
			refreshLogger.Errorf("Skipping PR: mergable state refreshed, still unset.")
			return err
		}
		pr = *refreshedPr
	}

	// If the PR is already mergable, or mergeable_state is anything other than behind, there's nothing for us to do.
	if *pr.Mergable || pr.MergeableState != gitHubMergeableStateBehind {
		logger.Debugf("Skipping PR: no action required.")
		return nil
	}

	// And finally, if we get to this point, tell GitHub to update the branch
	updateErr := gc.UpdatePullRequestBranch(org, repo, pr.Number, &pr.Head.SHA)
	if updateErr != nil {
		logger.WithError(updateErr).Errorf("Failed to update PR branch.")
		return updateErr
	}

	logger.Infof("Triggered UpdatePullRequestBranch")
	return nil
}

// We only care about certain events, so ignore others to limit duplicate actions
func isEventRelevant(eventAction github.PullRequestEventAction, relevantEvents []github.PullRequestEventAction) bool {
	for _, candidate := range relevantEvents {
		if eventAction == candidate {
			return true
		}
	}

	return false
}

// Returns the state of the tide context, or an empty string if there is no Tide context.
// Error means we failed to fetch statuses entirely.
func findTideStatus(gc githubClient, org string, repo string, pr github.PullRequest) (string, error) {
	// Find out if Tide thinks this PR should be mergeable.
	statuses, err := gc.GetCombinedStatus(org, repo, pr.Head.Ref)
	if err != nil {
		return "", err
	}

	// Find the Tide context and bail out if it's not success
	for _, status := range statuses.Statuses {
		if status.Context == gitHubContextNameTide {
			return status.State, nil
		}
	}

	return "", nil
}

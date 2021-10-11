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
	// We only care about certain events, so ignore others - this significantly limits the number of race conditions
	// that can cause multiple actions
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
	pr := event.PullRequest

	// Find out if Tide thinks this PR should be mergeable.
	statuses, err := gc.GetCombinedStatus(org, repo, pr.Head.Ref)
	if err != nil {
		log.WithError(err).Errorf("Failed to get the context statuses on %s/%s#%d.", org, repo, pr.Number)
		return err
	}

	// Find the Tide context and bail out if it's not success
	foundTideContext := false
	for _, status := range statuses.Statuses {
		if status.Context == gitHubContextNameTide {
			if status.State == tideContextStatusSuccess {
				foundTideContext = true
				break
			}
			return nil
		}
	}

	if !foundTideContext {
		log.Debugf("Skipping PR %d in repo %s/%s not monitored by Tide.", pr.Number, org, repo)
		return nil
	}

	// If we have a "Tide mergeable" PR, check if GitHub agrees, and refresh our local PR state if we don't have an answer
	// https://docs.github.com/en/rest/guides/getting-started-with-the-git-database-api#checking-mergeability-of-pull-requests
	if pr.Mergable == nil {
		// Crude, but this should always be enough time for GitHub to reach a decision.
		// Worst case, we'll re-check the next time the PR is updated.
		time.Sleep(gitHubMergeStateRefreshDelay * time.Second)
		refreshedPr, err := gc.GetPullRequest(org, repo, pr.Number)
		if err != nil {
			log.WithError(err).Errorf("Failed to refresh PR's mergable state on %s/%s#%d: %v, %s.", org, repo, pr.Number, *refreshedPr.Mergable, err.Error())
			return err
		}
		if refreshedPr.Mergable == nil {
			log.Errorf("Skipping PR: no reported mergable state after %ds on %s/%s#%d.", gitHubMergeStateRefreshDelay, org, repo, pr.Number)
			return err
		}
		pr = *refreshedPr
	}

	// If the PR is already mergable, or mergeable_state is anything other than behind, there's nothing for us to do.
	if *pr.Mergable || pr.MergeableState != gitHubMergeableStateBehind {
		return nil
	}

	// And finally, if we get to this point, tell GitHub to update the branch
	updateErr := gc.UpdatePullRequestBranch(org, repo, pr.Number, &pr.Head.SHA)
	if err != nil {
		log.WithError(updateErr).Errorf("Failed to update PR branch on %s/%s#%d: %s.", org, repo, pr.Number, updateErr.Error())
		return updateErr
	}

	log.Infof("Triggered update of branch %s/%s#%d", org, repo, pr.Number)
	return nil
}

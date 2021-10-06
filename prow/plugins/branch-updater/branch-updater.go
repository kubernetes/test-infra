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
	pluginName  = "branch-updater"
	tideContext = "tide"
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

	// - See if this PR has the Tide context in a passing state (eg Tide thinks this PR should be mergeable)
	//   - If not, return - nothing to do.
	//   - If so, see if GitHub thinks the PR is mergeable
	//     - If null, refresh to get an answer.
	//     - If true, return - nothing to do.
	//     - If false, check mergeable_state
	//       - If 'behind', call the rebase API.
	//       - Otherwise, return - nothing to do

	// Find out if Tide thinks this PR should be mergeable and bail out if not.
	statuses, err := gc.GetCombinedStatus(org, repo, pr.Head.Ref)
	if err != nil {
		log.WithError(err).Errorf("Failed to get the context statuses on %s/%s#%d.", org, repo, pr.Number)
		return err
	}

	for _, status := range statuses.Statuses {
		if status.Context == tideContext && status.State == "success" {
			return nil
		}
	}

	// If we have a "Tide mergeable" PR, check if GitHub agrees, and refresh our local PR state if we don't have an answer
	// https://docs.github.com/en/rest/guides/getting-started-with-the-git-database-api#checking-mergeability-of-pull-requests
	if pr.Mergable == nil {
		// Crude, but this should always be enough time for GitHub to reach a decision.
		time.Sleep(10 * time.Second)
		refreshedPr, err := gc.GetPullRequest(org, repo, pr.Number)
		if err != nil || refreshedPr.Mergable == nil {
			log.WithError(err).Errorf("Failed to refresh PR's mergable state on %s/%s#%d: %s, %s.", org, repo, pr.Number, refreshedPr.Mergable, err.Error())
			return err
		}
		pr = *refreshedPr
	}

	// If the PR is already mergable, we don't need to do anything.
	if *pr.Mergable {
		return nil
	}

	// If the mergeable state is anything other than behind, do nothing
	if pr.MergeableState != "behind" {
		return nil
	}

	// And finally if we get to this point, tell GitHub to update the branch
	updateErr := gc.UpdatePullRequestBranch(org, repo, pr.Number, &pr.Head.SHA)
	if err != nil {
		log.WithError(updateErr).Errorf("Failed to update PR branch on %s/%s#%d: %s.", org, repo, pr.Number, updateErr.Error())
		return updateErr
	}

	log.Infof("Triggered update of branch %s/%s#%d", org, repo, pr.Number)
	return nil
}

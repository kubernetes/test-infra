package updatebot

import (
	"fmt"
	"regexp"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	utypes "k8s.io/test-infra/prow/plugins/updatebot/internal"
)

const (
	PluginName string = "updatebot"
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequestEvent, nil)
	plugins.RegisterReviewEventHandler(PluginName, handleReviewEvent, nil)
	plugins.RegisterStatusEventHandler(PluginName, handleStatusEvent, nil)
	plugins.RegisterWorkflowRunEventHandler(PluginName, handleWorkflowRunEvent, nil)
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericCommentEvent, nil)
}

func handlePullRequestEvent(agent plugins.Agent, pullRequestEvent github.PullRequestEvent) error {
	switch pullRequestEvent.Action {
	case github.PullRequestActionOpened, github.PullRequestActionReopened, github.PullRequestActionSynchronize:
		TriggerUpdate(agent, &pullRequestEvent.PullRequest)
		for _, session := range Sessions {
			if pullRequestEvent.Repo.Owner.Login == session.OwnerLogin {
				for _, submodule := range session.Submodules {
					if pullRequestEvent.Repo.Name == submodule.BaseInfo.Name {
						submodule.PRInfo = &pullRequestEvent.PullRequest
						break
					}
				}
			}
		}
	case github.PullRequestActionClosed:
		// Handle submodule merge event, user may manually merge submodule's pull request when it's ready
		for _, session := range Sessions {
			if pullRequestEvent.Repo.Owner.Login == session.OwnerLogin {
				for _, submodule := range session.Submodules {
					if pullRequestEvent.Repo.Name == submodule.BaseInfo.Name {
						if submodule.PRInfo.Number == pullRequestEvent.Number &&
							!submodule.PRInfo.Merged && pullRequestEvent.PullRequest.Merged {
							submodule.MergedSHA = *pullRequestEvent.PullRequest.MergeSHA
							if session.IsStage(utypes.SUBMERGING) && session.SubmoduleMerged() {
								session.stage.Release()
								session.Next();
							}
						}
						submodule.PRInfo = &pullRequestEvent.PullRequest
						break
					}
				}
			}
		}
	}
	return nil
}

func handleReviewEvent(agent plugins.Agent, reviewEvent github.ReviewEvent) error {
	switch reviewEvent.Action {
	case github.ReviewActionSubmitted:
		owner := reviewEvent.Repo.Owner.Login
		repo := reviewEvent.Repo.Name
		number := reviewEvent.PullRequest.Number
		SHA := reviewEvent.PullRequest.Head.SHA
		session, exist := Sessions[GenerateSessionID(owner, repo, number, SHA)]
		if exist {
			return session.CheckAndUpdate()
		}
	default:
		break
	}
	return nil
}

func handleStatusEvent(agent plugins.Agent, statusEvent github.StatusEvent) error {
	// First look for the session
	for _, session := range Sessions {
		if session.OwnerLogin == statusEvent.Repo.Owner.Login {
			for _, submodule := range session.Submodules {
				if submodule.BaseInfo.Name == statusEvent.Repo.Name && submodule.PRInfo != nil && submodule.PRInfo.Head.SHA == statusEvent.SHA {
					return session.ConcludeSubmoduleStatus(submodule)
				}
			}
		}
	}
	return nil
}

func handleWorkflowRunEvent(agent plugins.Agent, workflowRunEvent github.WorkflowRunEvent) error {
	switch workflowRunEvent.Action {
	case "completed":
		for _, pullRequest := range workflowRunEvent.WorkflowRun.PullRequests {
			for _, session := range Sessions {
				if session.OwnerLogin == workflowRunEvent.Repo.Owner.Login {
					if session.MainRepo == workflowRunEvent.Repo.Name && pullRequest.Number == session.UpdatePRNumber {
						if workflowRunEvent.WorkflowRun.Conclusion == "success" || workflowRunEvent.WorkflowRun.Conclusion == "skipped" {
							return session.CheckAndUpdate()
						}
					} else {
						for _, submodule := range session.Submodules {
							if submodule.BaseInfo.Name == workflowRunEvent.Repo.Name {
								return session.ConcludeSubmoduleStatus(submodule)
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func handleGenericCommentEvent(agent plugins.Agent, genericCommentEvent github.GenericCommentEvent) error {
	switch genericCommentEvent.Action {
	case github.GenericCommentActionCreated:
		if genericCommentEvent.IsPR {
			// Check if there is a session whose stage is not IDLE
			owner := genericCommentEvent.Repo.Owner.Login
			repo := genericCommentEvent.Repo.Name
			number := genericCommentEvent.Number
			pullRequest, err := agent.GitHubClient.GetPullRequest(owner, repo, number)
			if err != nil {
				return fmt.Errorf("cannot get pull request for status. %w", err)
			}
			switch genericCommentEvent.Body {
			case "/retrigger":
				return TriggerUpdate(agent, pullRequest)
			}
		}
	}
	return nil
}

func TriggerUpdate(agent plugins.Agent, pullRequest *github.PullRequest) (err error) {
	gc := agent.GitHubClient
	owner := pullRequest.Base.Repo.Owner.Login
	repo := pullRequest.Base.Repo.Name
	number := pullRequest.Number
	SHA := pullRequest.Head.SHA
	shouldTrigger := func(org, repo string) bool {
		fullName := fmt.Sprintf("%s/%s", org, repo)
		for _, mainRepo := range agent.PluginConfig.UpdateBot.Repos {
			if fullName == mainRepo {
				return true
			}
		}
		return false
	}
	if !shouldTrigger(owner, repo) {
		return nil
	}
	repoOwners, err := agent.OwnersClient.LoadRepoOwners(owner, repo, pullRequest.Base.Ref)
	if err != nil {
		return fmt.Errorf("cannot get repo owners, %w", err)
	}
	if !repoOwners.AllOwners().Has(pullRequest.User.Login) {
		return &NotAuthorizedError{user: pullRequest.User.Login}
	}
	id := GenerateSessionID(owner, repo, number, SHA)
	session := FindSession(id)
	if session != nil && !session.IsStage(utypes.IDLE) {
		session.Continue()
	} else {
		status := github.Status{
			Context:     "auto-update / check-pr",
			Description: "Checking pull request...",
		}
		session := &Session{
			ID:               id,
			OwnerLogin:       owner,
			MainRepo:         repo,
			UpdatePRNumber:   number,
			UpdatePR:         pullRequest,
			Client:           agent.GitHubClient,
			Git:              agent.GitClient,
			UpdateHeadBranch: "topic-update-nosync",
			UpdateSHA:        SHA,
			Logger:           agent.Logger,
			UpdateBaseBranch: pullRequest.Base.Ref,
			Submodules:       map[string]*SubmoduleInfo{},
			stage:            utypes.CreateStage(utypes.IDLE),
		}
		session.mut.Lock()
		bot, err := agent.GitHubClient.BotUser()
		if err != nil {
			return session.Fail(status, "Cannot get bot user.", err)
		}
		session.BotUser = *bot
		session.mut.Unlock()
		commits, err := gc.ListPRCommits(owner, repo, number)
		if err != nil {
			return session.Fail(status, "Cannot get pull request commits.", err)
		}
		if len(commits) != 1 {
			return session.Fail(status, "There should be one and just one commit.", nil)
		}
		commit := commits[0]
		update := false
		commitDetail, err := gc.GetSingleCommit(owner, repo, commit.SHA)
		if err != nil {
			return session.Fail(status, "Cannot get commit for pull request.", err)
		}
		var changelogCommitFile github.CommitFile
		for _, file := range commitDetail.Files {
			if file.Filename == "debian/changelog" {
				update = true
				changelogCommitFile = file
			}
		}
		if update {
			versionPattern := regexp.MustCompile(`\+dtk\s\((?P<version>\d+(\.\d+)*)\)`)
			result := versionPattern.FindStringSubmatch(changelogCommitFile.Patch)
			if len(result) < 3 {
				// At least we have one whole match and two groups
				return session.Fail(status, "Cannot find version in changelog.", nil)
			}
			session.mut.Lock()
			session.UpdateToVersion = result[1]
			session.mut.Unlock()
			Sessions[id] = session
			session.RequestStage(utypes.PROCESSING)
		}
	}
	return nil
}

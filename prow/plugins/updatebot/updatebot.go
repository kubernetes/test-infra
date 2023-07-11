package updatebot

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	PluginName string = "updatebot"
)

const (
	IDLE = iota
	PROCESSING
	DELIVERING
	WAITING
	UPDATING
)

type SubmoduleContext struct {
	BaseInfo  *SubmoduleEntry
	PRInfo    *github.PullRequest
	Status    *github.Status
	MergedSHA string
	StartedAt time.Time
}

var UpdateMainRepo = []string{
	"dtk",
}

var Contexts = map[string]*UpdateContext{}

type UpdateContext struct {
	MainRepo        string
	OwnerLogin      string
	UpdatePRNumber  int
	UpdatePR        *github.PullRequest
	UpdateSHA       string
	SubmoduleInfo   map[string]*SubmoduleContext
	UpdateToVersion string
	UpdateHead      string // Temporary branch used to update
	UpdateBase      string
	BotUser         *github.UserData
	Stage           int // Stage of the update process

	mut                       sync.Mutex
	sem                       sync.WaitGroup
	UpdateSubmodulesStartedAt time.Time
}

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequestEvent, nil)
	plugins.RegisterReviewEventHandler(PluginName, handleReviewEvent, nil)
	plugins.RegisterStatusEventHandler(PluginName, handleStatusEvent, nil)
	plugins.RegisterWorkflowRunEventHandler(PluginName, handleWorkflowRunEvent, nil)
}

func handlePullRequestEvent(agent plugins.Agent, pullRequestEvent github.PullRequestEvent) error {
	switch pullRequestEvent.Action {
	case github.PullRequestActionOpened, github.PullRequestActionReopened, github.PullRequestActionSynchronize:
		for _, repo := range UpdateMainRepo {
			if repo == pullRequestEvent.Repo.Name {
				return triggerUpdate(&agent, &pullRequestEvent)
			}
		}
		for _, context := range Contexts {
			if pullRequestEvent.Repo.Owner.Login == context.OwnerLogin {
				for _, submodule := range context.SubmoduleInfo {
					if pullRequestEvent.Repo.Name == submodule.BaseInfo.Name {
						submodule.PRInfo = &pullRequestEvent.PullRequest
						break
					}
				}
			}
		}
	case github.PullRequestActionClosed:
		// Handle submodule merge event, user may manually merge submodule's pull request when it's ready
		for _, context := range Contexts {
			if pullRequestEvent.Repo.Owner.Login == context.OwnerLogin {
				for _, submodule := range context.SubmoduleInfo {
					if pullRequestEvent.Repo.Name == submodule.BaseInfo.Name {
						if submodule.PRInfo.Number == pullRequestEvent.Number &&
							!submodule.PRInfo.Merged && pullRequestEvent.PullRequest.Merged {
							submodule.MergedSHA = *pullRequestEvent.PullRequest.MergeSHA
							context.sem.Done() // One submodule pull request is just merged
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
		context, exist := Contexts[reviewEvent.Repo.Name]
		if exist && reviewEvent.PullRequest.Number == context.UpdatePRNumber {
			return checkAndUpdate(&agent, context)
		}
	default:
		break
	}
	return nil
}

func handleStatusEvent(agent plugins.Agent, statusEvent github.StatusEvent) error {
	// First look for the context
	for _, context := range Contexts {
		if context.OwnerLogin == statusEvent.Repo.Owner.Login {
			for _, submodule := range context.SubmoduleInfo {
				if submodule.BaseInfo.Name == statusEvent.Repo.Name {
					return concludeSubmoduleStatus(&agent, submodule, context)
				}
			}
		}
	}
	return nil
}

func handleWorkflowRunEvent(agent plugins.Agent, workflowRunEvent github.WorkflowRunEvent) error {
	switch workflowRunEvent.Action {
	case "completed":
		associatedPRCount := len(workflowRunEvent.WorkflowRun.PullRequests)
		if associatedPRCount == 0 {
			return nil
		}
		for _, pullRequest := range workflowRunEvent.WorkflowRun.PullRequests {
			for _, context := range Contexts {
				if context.OwnerLogin == workflowRunEvent.Repo.Owner.Login {
					if context.MainRepo == workflowRunEvent.Repo.Name && pullRequest.Number == context.UpdatePRNumber {
						if workflowRunEvent.WorkflowRun.Conclusion == "success" || workflowRunEvent.WorkflowRun.Conclusion == "skipped" {
							return checkAndUpdate(&agent, context)
						}
					} else {
						for _, submodule := range context.SubmoduleInfo {
							if submodule.BaseInfo.Name == workflowRunEvent.Repo.Name {
								return concludeSubmoduleStatus(&agent, submodule, context)
							}
						}
					}
				}
			}
		}
	default:
		break
	}
	return nil
}

func triggerUpdate(agent *plugins.Agent, pullRequestEvent *github.PullRequestEvent) error {
	githubClient := agent.GitHubClient
	org := pullRequestEvent.Repo.Owner.Login
	repo := pullRequestEvent.Repo.Name
	PRNumber := pullRequestEvent.PullRequest.Number
	commits, err := githubClient.ListPRCommits(org, repo, PRNumber)
	update := false
	var changelogCommitFile *github.CommitFile
	for _, commit := range commits {
		commitDetail, err := githubClient.GetSingleCommit(org, repo, commit.SHA)
		if err != nil {
			return err
		}
		for _, file := range commitDetail.Files {
			if file.Filename == "debian/changelog" {
				update = true
				changelogCommitFile = &file
			}
		}
	}
	if update {
		versionPattern := regexp.MustCompile(`\+dtk\s\((?P<version>\d+(\.\d+)*)\)`)
		result := versionPattern.FindStringSubmatch(changelogCommitFile.Patch)
		if len(result) < 3 {
			// At least we have one whole match and two groups
			return fmt.Errorf("cannot find version in changelog")
		}
		// Create a new update context
		context, exist := Contexts[pullRequestEvent.Repo.Name]
		if exist {
			// Only reconstruct the context if not in UPDATING stage
			if context.Stage == UPDATING {
				return fmt.Errorf("there is an update process ongoing")
			}
		} else {
			context = &UpdateContext{}
			Contexts[pullRequestEvent.Repo.Name] = context
		}
		context.mut.Lock()
		context.Stage = PROCESSING
		context.MainRepo = pullRequestEvent.Repo.Name
		context.OwnerLogin = pullRequestEvent.Repo.Owner.Login
		context.UpdatePR = &pullRequestEvent.PullRequest
		context.UpdatePRNumber = pullRequestEvent.PullRequest.Number
		context.UpdateSHA = pullRequestEvent.PullRequest.Head.SHA
		context.BotUser, err = agent.GitHubClient.BotUser()
		context.mut.Unlock()
		if err != nil {
			return fmt.Errorf("cannot get bot user, error: %w", err)
		}

		deliverTimeChan := make(chan time.Time, 1)

		go func() {
			// Create status for delivering PR
			githubClient.CreateStatus(context.OwnerLogin, context.MainRepo, context.UpdateSHA, github.Status{
				State:       github.StatusPending,
				Context:     "auto-update / deliver-pr",
				Description: "Delivering pull requests to submodules...",
			})
			// Create submoduleStatus for updating submodules
			githubClient.CreateStatus(context.OwnerLogin, context.MainRepo, context.UpdateSHA, github.Status{
				State:       github.StatusPending,
				Context:     "auto-update / update-submodules",
				Description: "Waiting for submodules to be updated...",
			})
			context.UpdateSubmodulesStartedAt = time.Now()
			deliverTimeChan <- time.Now()
		}()

		context.UpdateToVersion = result[1]
		context.UpdateBase = pullRequestEvent.PullRequest.Base.Ref
		context.UpdatePR = &pullRequestEvent.PullRequest
		// Just build submodule info at this time to ensure submoduleInfo track is correct
		context.SubmoduleInfo = map[string]*SubmoduleContext{}
		gitmodules, err := githubClient.GetFile(context.OwnerLogin, context.MainRepo, ".gitmodules", pullRequestEvent.PullRequest.Head.SHA)
		if err != nil {
			// Cannot find a gitmodules file, just return
			return err
		}
		moduleEntries, err := ParseDotGitmodulesContent(gitmodules)
		agent.Logger.Info(moduleEntries)
		if err != nil {
			return err
		}
		var sem sync.WaitGroup
		sem.Add(len(moduleEntries))
		context.mut.Lock()
		context.Stage = DELIVERING
		context.mut.Unlock()
		for _, moduleEntry := range moduleEntries {
			module := moduleEntry
			submoduleContext := &SubmoduleContext{
				BaseInfo: &module,
			}
			context.SubmoduleInfo[moduleEntry.Name] = submoduleContext
			go func() {
				err := deliverUpdatePR(agent, context, submoduleContext)
				if err != nil {
					agent.Logger.WithError(err).Warnf("Cannot deliver update pull request for %s/%s", context.OwnerLogin, submoduleContext.BaseInfo.Name)
				}
				sem.Done()
			}()
		}
		go func() {
			defer close(deliverTimeChan)
			sem.Wait()
			context.mut.Lock()
			context.Stage = WAITING
			context.mut.Unlock()
			deliverStartedAt := <-deliverTimeChan
			agent.GitHubClient.CreateStatus(context.OwnerLogin, context.MainRepo, context.UpdateSHA, github.Status{
				State:       github.StatusSuccess,
				Context:     "auto-update / deliver-pr",
				Description: fmt.Sprintf("Successful in %s.", time.Since(deliverStartedAt).Round(time.Second)),
			})
		}()
	}
	return err
}

// func checksCompleted(agent *plugins.Agent, owner, repo, SHA string) bool {
// 	checkRunList, err := agent.GitHubClient.ListCheckRuns(owner, repo, SHA)
// 	if err != nil {
// 		agent.Logger.WithError(err).Warnf("Cannot list check runs for %s/%s, SHA: %s", owner, repo, SHA)
// 		return false
// 	}
// 	hasPending := false
// 	for _, checkRun := range checkRunList.CheckRuns {
// 		if checkRun.Status == "completed" {
// 			if checkRun.Conclusion == "cancelled" || checkRun.Conclusion == "failure" {
// 				// If there is one cancelled or failed check, consider this commit as checks completed
// 				return true
// 			}
// 		} else {
// 			hasPending = true
// 		}
// 	}
// 	return !hasPending
// }

// Make sure param is correct
func moreImportantStatus(result, single string) string {
	priority := map[string]int{
		github.StatusSuccess: 1,
		github.StatusPending: 2,
		github.StatusFailure: 3,
		github.StatusError:   4,
	}
	resultP, exist := priority[result]
	if !exist {
		return single
	}
	singleP, exist := priority[single]
	if !exist {
		return result
	}
	if resultP > singleP {
		return result
	} else {
		return single
	}
}

func combinedStatus(agent *plugins.Agent, owner, repo, SHA string) string {
	statuses, err := agent.GitHubClient.GetCombinedStatus(owner, repo, SHA)
	if err != nil {
		agent.Logger.WithError(err).Warnf("Cannot list statuses for %s/%s, SHA: %s", owner, repo, SHA)
		return github.StatusPending
	}
	result := github.StatusSuccess
	for _, status := range statuses.Statuses {
		if status.Context == "tide" {
			continue
		}
		result = moreImportantStatus(result, status.State)
	}
	return result
}

// func allCompleted(agent *plugins.Agent, owner, repo, SHA string) bool {
// 	checksChan := make(chan bool, 1)
// 	defer close(checksChan)
// 	go func() {
// 		checksChan <- checksCompleted(agent, owner, repo, SHA)
// 	}()
// 	statusCompleted := (combinedStatus(agent, owner, repo, SHA) != github.StatusPending)
// 	checks := <-checksChan
// 	return checks && statusCompleted
// }

// If all checks are passed (success or skipped), return true
func checksPassed(agent *plugins.Agent, owner, repo, SHA string) bool {
	checks, err := agent.GitHubClient.ListCheckRuns(owner, repo, SHA)
	if err != nil {
		agent.Logger.WithError(err).Warnf("Cannot list check runs for %s/%s, SHA: %s", owner, repo, SHA)
		return false
	}
	for _, check := range checks.CheckRuns {
		if !(check.Conclusion == "success" || check.Conclusion == "skipped") {
			return false
		}
	}
	return true
}


func moreImportantConclusion(prev, next string) string {
	priority := map[string]int{
		"skipped": 1,
		"neutral": 2,
		"success": 3,
		"stale": 4,
		"pending": 5,
		"action_required": 6,
		"failure": 7,
		"timed_out": 8,
		"cancelled": 9,
	}
	prevP, nextP := priority[prev], priority[next]
	if nextP > prevP {
		return next
	} else {
		return prev
	}
}

func commitStatus(agent *plugins.Agent, owner, repo, SHA string) string {
	client := agent.GitHubClient
	entry := agent.Logger
	checks, err := client.ListCheckRuns(owner, repo, SHA)
	if err != nil {
		entry.WithError(err).Warnf("Cannot list check runs for %s/%s, SHA: %s", owner, repo, SHA)
		return github.StatusPending
	}
	conclusion := "skipped"
	for _, check := range checks.CheckRuns {
		switch check.Status {
		case "completed":
			conclusion = moreImportantConclusion(conclusion, check.Conclusion)
		default:
			conclusion = moreImportantConclusion(conclusion, "pending")
		}
	}
	var status string
	switch conclusion {
	case "failure", "timed_out":
		return github.StatusFailure
	case "cancelled":
		return github.StatusError
	case "pending", "action_required":
		status = github.StatusPending
	default:
		status = github.StatusSuccess
	}
	return moreImportantStatus(status, combinedStatus(agent, owner, repo, SHA))
}

// For a commit SHA, if all checks on it and all statuses for it are successful or skipped, then return true
// func allPassed(agent *plugins.Agent, owner, repo, SHA string) bool {
// 	statusChan := make(chan bool, 1)
// 	defer close(statusChan)
// 	go func() {
// 		statusChan <- (combinedStatus(agent, owner, repo, SHA) == github.StatusSuccess)
// 	}()
// 	passed := checksPassed(agent, owner, repo, SHA)
// 	statusPassed := <-statusChan
// 	return passed && statusPassed
// }

func prApproved(agent *plugins.Agent, pullRequest *github.PullRequest, context *UpdateContext) bool {
	client := agent.GitHubClient
	entry := agent.Logger
	owner := pullRequest.Base.Repo.Owner.Login
	repo := pullRequest.Base.Repo.Name

	reviewsChan := make(chan []github.Review, 1)
	defer close(reviewsChan)
	go func() {
		reviews, err := client.ListReviews(owner, repo, pullRequest.Number)
		if err != nil {
			entry.Warn("Cannot list reviews for pull request: ", pullRequest.HTMLURL)
		}
		reviewsChan <- reviews
	}()

	protection, err := client.GetBranchProtection(owner, repo, pullRequest.Base.Ref)
	if err != nil {
		entry.Warn("Cannot get branch protection for branch: ", pullRequest.Base.Ref)
		return false
	}
	reviews := <-reviewsChan

	if protection != nil && protection.RequiredPullRequestReviews.RequireCodeOwnerReviews {
		ownerApproved := false
		for _, review := range reviews {
			if review.User.Login == owner && review.State == github.ReviewStateApproved {
				ownerApproved = true
			}
		}
		if !ownerApproved {
			return false
		}
	}
	if protection != nil && protection.RequiredPullRequestReviews.RequiredApprovingReviewCount > 0 {
		approvedReviews := 0
		for _, review := range reviews {
			isCollaborator, err := client.IsCollaborator(owner, repo, review.User.Login)
			if err != nil {
				entry.Warn("Cannot judge if ", review.User.Login, " is collaborator of ", owner, "/", repo)
				return false
			}
			if isCollaborator && review.State == github.ReviewStateApproved {
				approvedReviews++
			}
		}
		if approvedReviews < protection.RequiredPullRequestReviews.RequiredApprovingReviewCount {
			return false
		} else {
			return true
		}
	}
	return true

}

func submodulesReady(context *UpdateContext) bool {
	for _, submodule := range context.SubmoduleInfo {
		if submodule.Status.State != github.StatusSuccess {
			return false
		}
	}
	return true
}

// Give submodule status a conclusion, success or failure, only conclude when all checks and statuses are finished
func concludeSubmoduleStatus(agent *plugins.Agent, submodule *SubmoduleContext, context *UpdateContext) error {
	// Check if already merged
	if submodule.PRInfo.Merged {
		return nil
	}
	owner := context.OwnerLogin
	repo := submodule.BaseInfo.Name
	SHA := submodule.PRInfo.Head.SHA
	status := commitStatus(agent, owner, repo, SHA)
	if submodule.Status.State == status {
		// Do not need update
		return nil
	}
	submodule.Status.State = status
	switch status {
	case github.StatusPending:
		submodule.Status.Description = "Waiting for checks to complete..."
	case github.StatusError:
		submodule.Status.Description = "Something went wrong!"
	case github.StatusFailure:
		submodule.Status.Description = fmt.Sprintf("Failed in %s", time.Since(submodule.StartedAt).Round(time.Second))
	case github.StatusSuccess:
		submodule.Status.Description = fmt.Sprintf("Successful in %s.", time.Since(submodule.StartedAt).Round(time.Second))
	}
	err := agent.GitHubClient.CreateStatus(context.OwnerLogin, context.MainRepo, context.UpdatePR.Head.SHA, *submodule.Status)
	if err != nil {
		return fmt.Errorf("failed to update status for PR: %s, %w", submodule.PRInfo.HTMLURL, err)
	}
	err = checkAndUpdate(agent, context)
	var notReady *NotReadyError
	if err != nil && !errors.As(err, &notReady) {
		return fmt.Errorf("failed to update, %w", err)
	}
	return nil
}

func handleSubmodulePR(agent *plugins.Agent, context *UpdateContext) error {
	for _, submodule := range context.SubmoduleInfo {
		if submodule.Status.State != "success" {
			return fmt.Errorf("cannot handle submodule pull requests, pull request %s for submodule %s is not ready", submodule.PRInfo.HTMLURL, submodule.BaseInfo.Name)
		}
	}
	for _, submodule := range context.SubmoduleInfo {
		err := agent.GitHubClient.Merge(context.OwnerLogin, submodule.BaseInfo.Name, submodule.PRInfo.Number, github.MergeDetails{
			MergeMethod: "rebase",
			SHA:         submodule.PRInfo.Head.SHA,
		})
		if err != nil {
			return fmt.Errorf("cannot merge pull request %s, error: %w", submodule.PRInfo.HTMLURL, err)
		}
	}
	return nil
}

func updateSubmodule(agent *plugins.Agent, context *UpdateContext) error {
	repo, err := agent.GitClient.ClientFor(context.OwnerLogin, context.MainRepo)
	if err != nil {
		return fmt.Errorf("cannot create git client for %s/%s, error: %w", context.OwnerLogin, context.MainRepo, err)
	}
	defer repo.Clean()

	err = repo.Checkout(context.UpdateBase)
	if err != nil {
		return err
	}

	update := exec.Command("git", "submodule", "update", "--init", "--recursive")
	update.Dir = repo.Directory()
	err = update.Run()
	if err != nil {
		agent.Logger.WithError(err).Warnf("cannot init submodule for %s/%s", context.OwnerLogin, context.MainRepo)
		return err
	}
	for _, submodule := range context.SubmoduleInfo {
		submoduleWD := filepath.Join(repo.Directory(), submodule.BaseInfo.Path)
		fetch := exec.Command("git", "fetch", "--all")
		fetch.Dir = submoduleWD
		err = fetch.Run()
		if err != nil {
			return fmt.Errorf("cannot fetch update from all submodules, error: %w", err)
		}
		checkout := exec.Command("git", "checkout", submodule.MergedSHA)
		checkout.Dir = submoduleWD
		err = checkout.Run()
		if err != nil {
			return fmt.Errorf("cannot checkout to %s for submodule %s/%s, %w",
				submodule.MergedSHA,
				context.OwnerLogin,
				submodule.BaseInfo.Name,
				err,
			)
		}
	}
	if context.BotUser == nil {
		context.BotUser, err = agent.GitHubClient.BotUser()
		if err != nil {
			return fmt.Errorf("cannot get bot user data, %w", err)
		}
	}
	repo.Config("user.name", context.BotUser.Login)
	repo.Config("user.email", context.BotUser.Email)
	repo.Commit("chore: update submodules", fmt.Sprintf("Update submodules to version %s.", context.UpdateToVersion))
	defer agent.GitHubClient.CreateStatus(context.OwnerLogin, context.MainRepo, context.UpdateSHA, github.Status{
		State:       github.StatusSuccess,
		Context:     "auto-update / update-submodules",
		Description: fmt.Sprintf("Successful in %s.", time.Since(context.UpdateSubmodulesStartedAt).Round(time.Second)),
	})
	return repo.PushToCentral(context.UpdateBase, true)
}

func logUpdate(agent *plugins.Agent, context *UpdateContext) (err error) {
	if context.Stage == UPDATING || context.Stage == IDLE {
		return fmt.Errorf("there is already a update process")
	}
	context.mut.Lock()
	context.Stage = UPDATING
	context.mut.Unlock()
	defer func() {
		if err == nil {
			context.mut.Lock()
			context.Stage = IDLE
			context.mut.Unlock()
		}
	}()
	agent.Logger.Infof("Updating %s/%s...", context.OwnerLogin, context.MainRepo)

	err = handleSubmodulePR(agent, context)
	if err != nil {
		agent.Logger.WithError(err).Warnf("Cannot merge all submodules' pull requests")
		return
	}

	context.sem.Wait()

	err = updateSubmodule(agent, context)
	if err != nil {
		agent.Logger.WithError(err).Warnf("Cannot update submodule")
		return
	}

	time.Sleep(10 * time.Second)
	err = agent.GitHubClient.Merge(context.OwnerLogin, context.MainRepo, context.UpdatePRNumber, github.MergeDetails{
		MergeMethod: "rebase",
		SHA:         context.UpdateSHA,
	})

	if err != nil {
		agent.Logger.WithError(err).Warnf("Cannot merge main update pull request")
		return
	}

	return
}

func updateChangelog(agent *plugins.Agent, submoduleContext *SubmoduleContext, context *UpdateContext) error {
	repo, err := agent.GitClient.ClientFor(context.OwnerLogin, submoduleContext.BaseInfo.Name)
	if err != nil {
		return err
	}
	defer repo.Clean()

	err = repo.Checkout(submoduleContext.BaseInfo.Branch)
	if err != nil {
		return err
	}

	err = repo.CheckoutNewBranch(context.UpdateHead)
	if err != nil {
		return err
	}

	gbp := exec.Command("gbp", "deepin-changelog", "-N", context.UpdateToVersion, "--spawn-editor=never", "--distribution=unstable", "--force-distribution", "--git-author", "--ignore-branch")
	gbp.Dir = repo.Directory()
	err = gbp.Run()
	if err != nil {
		agent.Logger.WithError(err).Warn("Execute gbp failed")
		return err
	}

	if context.BotUser == nil {
		context.BotUser, err = agent.GitHubClient.BotUser()
		if err != nil {
			return fmt.Errorf("cannot get bot user data, %w", err)
		}
	}
	repo.Config("user.name", context.BotUser.Login)
	repo.Config("user.email", context.BotUser.Email)
	err = repo.Commit("chore: update changelog", fmt.Sprintf("Release %s.", context.UpdateToVersion))
	if err != nil {
		return err
	}

	return repo.PushToCentral(context.UpdateHead, true)

}

func deliverUpdatePR(agent *plugins.Agent, context *UpdateContext, submoduleContext *SubmoduleContext) error {
	submoduleContext.Status = &github.Status{
		State:       github.StatusPending,
		Context:     fmt.Sprintf("auto-update / check-update (%s)", submoduleContext.BaseInfo.Name),
		Description: "Creating pull request for update...",
		TargetURL:   submoduleContext.BaseInfo.URL,
	}
	submoduleContext.StartedAt = time.Now()
	go agent.GitHubClient.CreateStatus(context.OwnerLogin, context.MainRepo, context.UpdatePR.Head.SHA, *submoduleContext.Status)
	context.UpdateHead = "topic-update"
	err := updateChangelog(agent, submoduleContext, context)
	if err != nil {
		return err
	}
	pullRequests, err := agent.GitHubClient.GetPullRequests(context.OwnerLogin, submoduleContext.BaseInfo.Name)
	if err != nil {
		return err
	}
	for _, pullRequest := range pullRequests {
		if pullRequest.Head.Repo == pullRequest.Base.Repo && pullRequest.Head.Ref == context.UpdateHead && pullRequest.Base.Ref == context.UpdateBase {
			submoduleContext.PRInfo = &pullRequest
			break
		}
	}
	if submoduleContext.PRInfo == nil {
		// Create a new PR
		number, err := agent.GitHubClient.CreatePullRequest(
			context.OwnerLogin,
			submoduleContext.BaseInfo.Name,
			"chore: update changelog",
			fmt.Sprintf("Release %s.", context.UpdateToVersion),
			context.UpdateHead,
			context.UpdateBase,
			true,
		)
		if err != nil {
			return err
		}
		submoduleContext.PRInfo, err = agent.GitHubClient.GetPullRequest(context.OwnerLogin, submoduleContext.BaseInfo.Name, number)
		if err != nil {
			return err
		}
	}
	submoduleContext.Status.TargetURL = submoduleContext.PRInfo.HTMLURL
	submoduleContext.Status.Description = "Waiting for checks to complete..."
	context.sem.Add(1)
	go agent.GitHubClient.CreateStatus(context.OwnerLogin, context.MainRepo, context.UpdatePR.Head.SHA, *submoduleContext.Status)
	go handleEmptyChecks(agent, submoduleContext, context)
	return nil
}

type NotReadyError struct {
	err     error
	message string
}

var _ error = new(NotReadyError)

func (e *NotReadyError) Error() string {
	return fmt.Sprintf(e.message + ", %v", e.err)
}

func (e *NotReadyError) Unwrap() error {
	return e.err
}


func checkAndUpdate(agent *plugins.Agent, context *UpdateContext) error {
	approved := prApproved(agent, context.UpdatePR, context)
	passed := checksPassed(agent, context.OwnerLogin, context.MainRepo, context.UpdateSHA)
	if approved && passed && submodulesReady(context) {
		return logUpdate(agent, context)
	} else {
		return &NotReadyError{message: "update pull request and submodules are not ready"}
	}
}

func handleEmptyChecks(agent *plugins.Agent, submodule *SubmoduleContext, context *UpdateContext) {
	time.Sleep(10 * time.Second)
	concludeSubmoduleStatus(agent, submodule, context)
}

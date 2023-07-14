package updatebot

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	utypes "k8s.io/test-infra/prow/plugins/updatebot/internal"
)

const (
	PluginName string = "updatebot"
)

type SubmoduleInfo struct {
	BaseInfo  Submodule
	PRInfo    *github.PullRequest
	Status    *github.Status
	MergedSHA string
	StartedAt time.Time
}

var UpdateMainRepo = []string{
	"dtk",
}

var Sessions = map[string]*Session{}

type Session struct {
	ID               string
	MainRepo         string
	OwnerLogin       string
	UpdatePRNumber   int
	UpdatePR         *github.PullRequest
	UpdateSHA        string
	Submodules       map[string]*SubmoduleInfo
	UpdateToVersion  string
	UpdateHeadBranch string // Temporary branch used to update
	UpdateBaseBranch string
	BotUser          *github.UserData
	Client           plugins.PluginGitHubClient
	Git              git.ClientFactory
	Logger           *logrus.Entry

	Stage                     utypes.Stage // int of the update process
	mut                       sync.RWMutex
	UpdateSubmodulesStartedAt time.Time
}

func (session *Session) merge() error {
	if !session.Stage.Start() {
		return nil
	}
	err := session.Client.Merge(session.OwnerLogin, session.MainRepo, session.UpdatePRNumber, github.MergeDetails{
		MergeMethod: "rebase",
		SHA:         session.UpdateSHA,
	})
	if err != nil {
		return fmt.Errorf("cannot merge update pull request. %w", err)
	}
	session.requestStage(utypes.IDLE)
	return nil
}

func (session *Session) requestStage(stage int) {
	session.Stage.Request(stage)
	switch stage {
	case utypes.PROCESSING:
		go session.process()
	case utypes.DELIVERING:
		go session.deliver()
	case utypes.WAITING:
		go session.waitingTrigger()
	case utypes.SUBMERGING:
		go session.handleSubmodulePR()
	case utypes.UPDATING:
		go session.updateSubmodule()
	case utypes.MERGING:
		go session.merge()
	}
}

func (session *Session) createStatus(status github.Status) error {
	return session.Client.CreateStatus(session.OwnerLogin, session.MainRepo, session.UpdateSHA, status)
}

func (session *Session) fail(status github.Status, cause string, err error) error {
	status.State = github.StatusFailure
	status.Description = cause
	session.createStatus(status)
	if err != nil {
		return fmt.Errorf("%s %w", status.Description, err)
	} else {
		return fmt.Errorf(status.Description)
	}
}

func (session *Session) succeed(status github.Status, description string) {
	status.State = github.StatusSuccess
	status.Description = description
	session.createStatus(status)
}

func (session *Session) pending(status github.Status, description string) {
	status.State = github.StatusPending
	status.Description = description
	session.createStatus(status)
}

func (session *Session) checkUpdateConflict() error {
	// For speed and convenience, here we just check if there is a ref matching update to version
	checkTag := fmt.Sprintf("tags/%s", session.UpdateToVersion)
	client := session.Client
	_, err := client.GetRef(session.OwnerLogin, session.MainRepo, checkTag)
	pattern := regexp.MustCompile(`status code 404`)
	if err != nil {
		if pattern.FindString(err.Error()) == "" {
			return err
		}
	} else {
		return &TagExistError{repo: session.MainRepo, tag: session.UpdateToVersion}
	}
	for _, submodule := range session.Submodules {
		_, err = client.GetRef(session.OwnerLogin, submodule.BaseInfo.Name, checkTag)
		if err != nil {
			if pattern.FindString(err.Error()) == "" {
				return err
			}
		} else {
			return &TagExistError{repo: submodule.BaseInfo.Name, tag: session.UpdateToVersion}
		}
	}
	return nil
}

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
		for _, repo := range UpdateMainRepo {
			if repo == pullRequestEvent.Repo.Name {
				return triggerUpdate(&agent, &pullRequestEvent.PullRequest)
			}
		}
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
							if session.submoduleMerged() {
								session.requestStage(utypes.UPDATING)
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
		SHA := reviewEvent.PullRequest.Base.SHA
		session, exist := Sessions[generateSessionID(owner, repo, number, SHA)]
		if exist && reviewEvent.PullRequest.Number == session.UpdatePRNumber {
			return session.checkAndUpdate()
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
				if submodule.BaseInfo.Name == statusEvent.Repo.Name && submodule.PRInfo.Head.SHA == statusEvent.SHA {
					return session.concludeSubmoduleStatus(submodule)
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
			for _, session := range Sessions {
				if session.OwnerLogin == workflowRunEvent.Repo.Owner.Login {
					if session.MainRepo == workflowRunEvent.Repo.Name && pullRequest.Number == session.UpdatePRNumber {
						if workflowRunEvent.WorkflowRun.Conclusion == "success" || workflowRunEvent.WorkflowRun.Conclusion == "skipped" {
							return session.checkAndUpdate()
						}
					} else {
						for _, submodule := range session.Submodules {
							if submodule.BaseInfo.Name == workflowRunEvent.Repo.Name {
								return session.concludeSubmoduleStatus(submodule)
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
			SHA := pullRequest.Head.SHA
			id := generateSessionID(owner, repo, number, SHA)
			session, exist := Sessions[id]
			if exist {
				if session.UpdatePRNumber == genericCommentEvent.Number {
					switch genericCommentEvent.Body {
					case "/retrigger":
						return triggerUpdate(&agent, pullRequest)
					}
				}
			}
		}
	}
	return nil
}

type TagExistError struct {
	repo string
	tag  string
}

func (tee *TagExistError) Error() string {
	return fmt.Sprintf("tag %s exists in repo %s", tee.tag, tee.repo)
}

func generateSessionID(owner, repo string, number int, SHA string) string {
	return fmt.Sprintf("/%s/%s/%d/%s", owner, repo, number, SHA)
}

func (session *Session) process() error {
	if !session.Stage.Start() {
		return nil
	}
	err := func() error {
		processStatus := github.Status{
			State:       github.StatusPending,
			Context:     "auto-update / extract-meta",
			Description: "Extracting meta information from pull request...",
		}
		session.createStatus(processStatus)
		gitmodules, err := session.Client.GetFile(session.OwnerLogin, session.MainRepo, ".gitmodules", session.UpdateSHA)
		if err != nil {
			return session.fail(processStatus, "Cannot find .gitmodules.", err)
		}
		moduleEntries, err := ParseDotGitmodulesContent(gitmodules)
		if err != nil {
			return session.fail(processStatus, "Cannot parse submodules from main repo.", err)
		}
		session.mut.Lock()
		defer session.mut.Unlock()
		for _, entry := range moduleEntries {
			session.Submodules[entry.Name] = &SubmoduleInfo{
				BaseInfo: entry,
			}
		}
		err = session.checkUpdateConflict()
		if err != nil {
			return session.fail(processStatus, "Update conflict.", err)
		}
		session.succeed(processStatus, fmt.Sprintf("Successful in %s", time.Since(session.Stage.StartedAt()).Round(time.Second)))
		return nil
	}()
	if err == nil {
		session.requestStage(utypes.DELIVERING)
	}
	return err
}

func (session *Session) deliver() error {
	if !session.Stage.Start() {
		return nil
	}
	err := func() error {
		deliverStatus := github.Status{
			Context: "auto-update / deliver-pr",
		}
		startedAt := time.Now()
		session.pending(deliverStatus, "Delivering pull requests to submodules...")
		var wg sync.WaitGroup
		wg.Add(len(session.Submodules))
		errChan := make(chan error, len(session.Submodules))
		for _, submodule := range session.Submodules {
			go func(s *Session, module *SubmoduleInfo) {
				errChan <- s.deliverUpdatePR(module)
				wg.Done()
			}(session, submodule)
		}
		wg.Wait()
		close(errChan)
		for err := range errChan {
			if err != nil {
				return session.fail(deliverStatus, "Deliver failed.", nil)
			}
		}
		session.succeed(deliverStatus, fmt.Sprintf("Successful in %s.", time.Since(startedAt).Round(time.Second)))
		return nil
	}()
	if err == nil {
		session.requestStage(utypes.WAITING)
	}
	return err
}

func (session *Session) waitingTrigger() error {
	if !session.Stage.Start() {
		return nil
	}
	time.Sleep(10 * time.Second)
	return session.conclude()
}

func (session *Session) conclude() error {
	var errs []error
	for _, submodule := range session.Submodules {
		err := session.concludeSubmoduleStatus(submodule)
		errs = append(errs, err)
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (session *Session) submoduleMerged() bool {
	for _, submodule := range session.Submodules {
		if !submodule.PRInfo.Merged {
			return false
		}
	}
	return true
}

func (session *Session) concludeSubmoduleStatus(submodule *SubmoduleInfo) error {
	// Check if already merged
	if submodule.PRInfo.Merged || submodule.Status == nil {
		return nil
	}
	owner := session.OwnerLogin
	repo := submodule.BaseInfo.Name
	SHA := submodule.PRInfo.Head.SHA
	status := commitStatus(session.Client, owner, repo, SHA)
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
	err := session.createStatus(*submodule.Status)
	if err != nil {
		return fmt.Errorf("failed to update status for PR: %s, %w", submodule.PRInfo.HTMLURL, err)
	}
	err = session.checkAndUpdate()
	var notReady NotReadyError
	if err != nil && !errors.As(err, &notReady) {
		return fmt.Errorf("failed to update, %w", err)
	}
	return nil
}

func triggerUpdate(agent *plugins.Agent, pullRequest *github.PullRequest) error {
	gc := agent.GitHubClient
	owner := pullRequest.Base.Repo.Owner.Login
	repo := pullRequest.Base.Repo.Name
	number := pullRequest.Number
	SHA := pullRequest.Head.SHA
	id := generateSessionID(owner, repo, number, SHA)
	fail := func(description string, cause error) error {
		gc.CreateStatus(owner, repo, SHA, github.Status{
			State:       github.StatusFailure,
			Context:     "auto-update / deliver-pr",
			Description: description,
		})
		gc.CreateStatus(owner, repo, SHA, github.Status{
			State:       github.StatusFailure,
			Context:     "auto-update / update-submodules",
			Description: description,
		})
		if cause != nil {
			return fmt.Errorf("%s %w", strings.ToLower(description), cause)
		} else {
			return fmt.Errorf(description)
		}
	}
	commits, err := gc.ListPRCommits(owner, repo, number)
	if err != nil {
		return fail("Cannot get pull request commits.", err)
	}
	if len(commits) != 1 {
		return fail("There should be one and just one commit.", nil)
	}
	commit := commits[0]
	update := false
	commitDetail, err := gc.GetSingleCommit(owner, repo, commit.SHA)
	if err != nil {
		return fail("Cannot get commit for pull request.", err)
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
			return fail("Cannot find version in changelog.", nil)
		}
		// Create a new update session
		session, exist := Sessions[id]
		if !exist || session.Stage.Is(utypes.IDLE) {
			session = &Session{}
			Sessions[id] = session
			session.mut.Lock()
			session.MainRepo = repo
			session.OwnerLogin = owner
			session.UpdatePR = pullRequest
			session.UpdatePRNumber = number
			session.UpdateSHA = pullRequest.Head.SHA
			session.Client = agent.GitHubClient
			session.Git = agent.GitClient
			session.BotUser, err = agent.GitHubClient.BotUser()
			if err != nil {
				return fail("Cannot get bot user.", err)
			}
			session.UpdateToVersion = result[1]
			session.UpdateBaseBranch = pullRequest.Base.Ref
			// Just build submodule info at this time to ensure submoduleInfo track is correct
			session.Submodules = map[string]*SubmoduleInfo{}
			session.UpdateHeadBranch = "topic-update"
			session.mut.Unlock()
			session.requestStage(utypes.PROCESSING)
		} else {
			session.requestStage(session.Stage.Value())
		}
	}
	return err
}

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

func combinedStatus(client plugins.PluginGitHubClient, owner, repo, SHA string) string {
	statuses, err := client.GetCombinedStatus(owner, repo, SHA)
	if err != nil {
		logrus.WithError(err).Warnf("Cannot list statuses for %s/%s, SHA: %s", owner, repo, SHA)
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

// If all checks are passed (success or skipped), return true
func checksPassed(client plugins.PluginGitHubClient, owner, repo, SHA string) bool {
	checks, err := client.ListCheckRuns(owner, repo, SHA)
	if err != nil {
		logrus.WithError(err).Warnf("Cannot list check runs for %s/%s, SHA: %s", owner, repo, SHA)
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
		"skipped":         1,
		"neutral":         2,
		"success":         3,
		"stale":           4,
		"pending":         5,
		"action_required": 6,
		"failure":         7,
		"timed_out":       8,
		"cancelled":       9,
	}
	prevP, nextP := priority[prev], priority[next]
	if nextP > prevP {
		return next
	} else {
		return prev
	}
}

func commitStatus(client plugins.PluginGitHubClient, owner, repo, SHA string) string {
	checks, err := client.ListCheckRuns(owner, repo, SHA)
	if err != nil {
		logrus.WithError(err).Warnf("Cannot list check runs for %s/%s, SHA: %s", owner, repo, SHA)
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
	return moreImportantStatus(status, combinedStatus(client, owner, repo, SHA))
}

func prApproved(client plugins.PluginGitHubClient, pullRequest *github.PullRequest) bool {
	entry := logrus.New()
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

func (session *Session) submodulesReady() bool {
	for _, submodule := range session.Submodules {
		if submodule.Status.State != github.StatusSuccess {
			return false
		}
	}
	return true
}

func (session *Session) handleSubmodulePR() error {
	if !session.Stage.Start() {
		return nil
	}
	for _, submodule := range session.Submodules {
		if submodule.Status.State != "success" {
			return fmt.Errorf("cannot handle submodule pull requests, pull request %s for submodule %s is not ready", submodule.PRInfo.HTMLURL, submodule.BaseInfo.Name)
		}
	}
	for _, submodule := range session.Submodules {
		err := session.Client.Merge(session.OwnerLogin, submodule.BaseInfo.Name, submodule.PRInfo.Number, github.MergeDetails{
			MergeMethod: "rebase",
			SHA:         submodule.PRInfo.Head.SHA,
		})
		if err != nil {
			return fmt.Errorf("cannot merge pull request %s, error: %w", submodule.PRInfo.HTMLURL, err)
		}
	}
	return nil
}

func (session *Session) updateSubmodule() error {
	if !session.Stage.Start() {
		return nil
	}
	err := func() error {
		repo, err := session.Git.ClientFor(session.OwnerLogin, session.MainRepo)
		if err != nil {
			return fmt.Errorf("cannot create git client for %s/%s, error: %w", session.OwnerLogin, session.MainRepo, err)
		}
		defer repo.Clean()

		err = repo.Checkout(session.UpdateBaseBranch)
		if err != nil {
			return err
		}

		update := exec.Command("git", "submodule", "update", "--init", "--recursive")
		update.Dir = repo.Directory()
		err = update.Run()
		if err != nil {
			session.Logger.WithError(err).Warnf("cannot init submodule for %s/%s", session.OwnerLogin, session.MainRepo)
			return err
		}
		for _, submodule := range session.Submodules {
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
					session.OwnerLogin,
					submodule.BaseInfo.Name,
					err,
				)
			}
		}
		if session.BotUser == nil {
			session.BotUser, err = session.Client.BotUser()
			if err != nil {
				return fmt.Errorf("cannot get bot user data, %w", err)
			}
		}
		repo.Config("user.name", session.BotUser.Login)
		repo.Config("user.email", session.BotUser.Email)
		repo.Commit("chore: update submodules", fmt.Sprintf("Update submodules to version %s.", session.UpdateToVersion))
		defer session.createStatus(github.Status{
			State:       github.StatusSuccess,
			Context:     "auto-update / update-submodules",
			Description: fmt.Sprintf("Successful in %s.", time.Since(session.UpdateSubmodulesStartedAt).Round(time.Second)),
		})
		return repo.PushToCentral(session.UpdateBaseBranch, true)
	}()
	if err == nil {
		session.requestStage(utypes.MERGING)
	}
	return err
}

func (session *Session) updateChangelog(submodule *SubmoduleInfo) error {
	repo, err := session.Git.ClientFor(session.OwnerLogin, submodule.BaseInfo.Name)
	if err != nil {
		return err
	}
	defer repo.Clean()

	err = repo.Checkout(submodule.BaseInfo.Branch)
	if err != nil {
		return err
	}

	err = repo.CheckoutNewBranch(session.UpdateHeadBranch)
	if err != nil {
		return err
	}

	gbp := exec.Command("gbp", "deepin-changelog", "-N", session.UpdateToVersion, "--spawn-editor=never", "--distribution=unstable", "--force-distribution", "--git-author", "--ignore-branch")
	gbp.Dir = repo.Directory()
	err = gbp.Run()
	if err != nil {
		return fmt.Errorf("execute gbp failed! %w", err)
	}

	if session.BotUser == nil {
		session.BotUser, err = session.Client.BotUser()
		if err != nil {
			return fmt.Errorf("cannot get bot user data, %w", err)
		}
	}
	repo.Config("user.name", session.BotUser.Login)
	repo.Config("user.email", session.BotUser.Email)
	err = repo.Commit("chore: update changelog", fmt.Sprintf("Release %s.", session.UpdateToVersion))
	if err != nil {
		return err
	}

	return repo.PushToCentral(session.UpdateHeadBranch, true)
}

func (session *Session) deliverUpdatePR(submodule *SubmoduleInfo) error {
	gc := session.Client
	if submodule.Status == nil || submodule.Status.Description == "Creating pull request for update..." {
		submodule.Status = &github.Status{
			State:       github.StatusPending,
			Context:     fmt.Sprintf("auto-update / check-update (%s)", submodule.BaseInfo.Name),
			Description: "Creating pull request for update...",
			TargetURL:   submodule.BaseInfo.URL,
		}
		submodule.StartedAt = time.Now()
		session.createStatus(*submodule.Status)
		err := session.updateChangelog(submodule)
		if err != nil {
			return err
		}
		pullRequests, err := gc.GetPullRequests(session.OwnerLogin, submodule.BaseInfo.Name)
		if err != nil {
			return err
		}
		for _, pullRequest := range pullRequests {
			if pullRequest.Head.Repo == pullRequest.Base.Repo && pullRequest.Head.Ref == session.UpdateHeadBranch && pullRequest.Base.Ref == session.UpdateBaseBranch {
				submodule.PRInfo = &pullRequest
				break
			}
		}
		if submodule.PRInfo == nil {
			// Create a new PR
			number, err := gc.CreatePullRequest(
				session.OwnerLogin,
				submodule.BaseInfo.Name,
				"chore: update changelog",
				fmt.Sprintf("Release %s.", session.UpdateToVersion),
				session.UpdateHeadBranch,
				session.UpdateBaseBranch,
				true,
			)
			if err != nil {
				return err
			}
			submodule.PRInfo, err = gc.GetPullRequest(session.OwnerLogin, submodule.BaseInfo.Name, number)
			if err != nil {
				return err
			}
		}
		submodule.Status.TargetURL = submodule.PRInfo.HTMLURL
		submodule.Status.Description = "Waiting for checks to complete..."
		session.createStatus(*submodule.Status)
	}
	return nil
}

type NotReadyError struct {
	err     error
	message string
}

var _ error = new(NotReadyError)

func (e NotReadyError) Error() string {
	return fmt.Sprintf(e.message+", %v", e.err)
}

func (e NotReadyError) Unwrap() error {
	return e.err
}

func (session *Session) checkAndUpdate() error {
	client := session.Client
	approved := prApproved(client, session.UpdatePR)
	passed := checksPassed(client, session.OwnerLogin, session.MainRepo, session.UpdateSHA)
	if approved && passed && session.submodulesReady() {
		session.requestStage(utypes.SUBMERGING)
		return nil
	} else {
		return &NotReadyError{message: "update pull request and submodules are not ready"}
	}
}

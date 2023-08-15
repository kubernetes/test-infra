package updatebot

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	utypes "k8s.io/test-infra/prow/plugins/updatebot/internal"
)

func FindSession(ID string) *Session {
	session, exist := Sessions[ID]
	if exist {
		return session
	} else {
		return nil
	}
}

func GenerateSessionID(owner, repo string, number int, SHA string) string {
	return fmt.Sprintf("/%s/%s/%d/%s", owner, repo, number, SHA)
}

var Sessions = map[string]*Session{}

type SubmoduleInfo struct {
	BaseInfo  Submodule
	PRInfo    *github.PullRequest
	Status    *github.Status
	MergedSHA string
	StartedAt time.Time
}

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
	BotUser          github.UserData
	Client           plugins.PluginGitHubClient
	Git              git.ClientFactory
	Logger           *logrus.Entry
	SubmodulesUpdated bool

	stage                     utypes.Stage // int of the update process
	mut                       sync.RWMutex
}

func (s *Session) Continue() bool {
	return s.RequestStage(s.Stage())
}

func (s *Session) Stage() int {
	return s.stage.Value()
}

func (s *Session) Next() bool {
	return s.RequestStage(s.Stage() + 1)
}

func (s *Session) IsStage(stage int) bool {
	return s.stage.Is(stage)
}

func (session *Session) Merge() error {
	if !session.stage.Start() {
		return nil
	}
	err := session.Client.AddLabel(session.OwnerLogin, session.MainRepo, session.UpdatePRNumber, "approved")
	if err != nil {
		return fmt.Errorf("cannot merge update pull request. %w", err)
	}
	session.stage.Release()
	session.Next()
	return nil
}

func (session *Session) RequestStage(stage int) bool {
	if !session.stage.Request(stage) {
		return false
	}
	switch stage {
	case utypes.PROCESSING:
		go session.Process()
	case utypes.DELIVERING:
		go session.Deliver()
	case utypes.WAITING:
		go session.DelayedConclude()
	case utypes.SUBMERGING:
		go session.HandleSubmodulePR()
	case utypes.UPDATING:
		go session.UpdateSubmodule()
	case utypes.MERGING:
		go session.Merge()
	}
	return true
}

func (session *Session) CreateStatus(status github.Status) error {
	return session.Client.CreateStatus(session.OwnerLogin, session.MainRepo, session.UpdateSHA, status)
}

func (session *Session) Fail(status github.Status, cause string, err error) error {
	status.State = github.StatusFailure
	status.Description = cause
	session.CreateStatus(status)
	if err != nil {
		return fmt.Errorf("%s %w", status.Description, err)
	} else {
		return fmt.Errorf(status.Description)
	}
}

func (session *Session) Succeed(status github.Status, description string) {
	status.State = github.StatusSuccess
	status.Description = description
	session.CreateStatus(status)
}

func (session *Session) Pend(status github.Status, description string) {
	status.State = github.StatusPending
	status.Description = description
	session.CreateStatus(status)
}

func (s *Session) CheckSubmoduleTagConflict(submodule *SubmoduleInfo) error {
	repo, err := s.Git.ClientFor(s.OwnerLogin, submodule.BaseInfo.Name)
	if err != nil {
		return &GitClientError{owner: s.OwnerLogin, repo: submodule.BaseInfo.Name, msg: "cannot create repo client", err: err}
	}
	defer repo.Clean()
	repo.Checkout(s.UpdateBaseBranch)

	// Firstly check if the tag exist
	listTag := exec.Command("git", "tag", "-l", s.UpdateToVersion)
	listTag.Dir = repo.Directory()
	result, err := listTag.Output()
	if err != nil {
		return &GitClientError{
			owner: s.OwnerLogin,
			repo: submodule.BaseInfo.Name,
			msg: "list tag error",
			err: err,
		}
	}
	if strings.TrimSpace(string(result)) != s.UpdateToVersion {
		// Tag does not exist, fine!
		return nil
	}
	// If exists, check if the tag is just created
	describe := exec.Command("git", "describe")
	describe.Dir = repo.Directory()
	result, err = describe.Output()
	if err != nil {
		return &GitClientError{
			owner: s.OwnerLogin,
			repo: submodule.BaseInfo.Name,
			msg: "decribe error",
			err: err,
		}
	}
	if strings.TrimSpace(string(result)) == s.UpdateToVersion {
		// Just create the tag, fine. Retrive current merged SHA
		retrive := exec.Command("git", "log", "HEAD", "--pretty=format:%H", "-1")
		retrive.Dir = repo.Directory()
		result, err = retrive.Output()
		if err != nil {
			return &GitClientError{
				owner: s.OwnerLogin,
				repo: submodule.BaseInfo.Name,
				msg: "cannot retrive current SHA",
				err: err,
			}
		}
		submodule.MergedSHA = string(result)
		return nil
	} else {
		return &TagExistError{repo: submodule.BaseInfo.Name, tag: s.UpdateToVersion}
	}
}

func (session *Session) CheckUpdateConflict() error {
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
	// There is a special case when not all submodules' tags are consistent. This may be caused by a failed
	// update process. When submodule's head equals to the tag ref, we assume the user wants to synchronize
	// the tag, this won't cause a tag conflict.
	var wg sync.WaitGroup
	var errs []error
	wg.Add(len(session.Submodules))
	for _, submodule := range session.Submodules {
		go func(s *Session, module *SubmoduleInfo) {
			err := s.CheckSubmoduleTagConflict(module)
			if err != nil {
				errs = append(errs, err)
			}
			wg.Done()
		}(session, submodule)
	}
	wg.Wait()
	if errs != nil {
		return errs[0]
	}
	return nil
}

func (session *Session) Process() error {
	if !session.stage.Start() {
		return nil
	}
	err := func() error {
		processStatus := github.Status{
			State:       github.StatusPending,
			Context:     "auto-update / extract-meta",
			Description: "Extracting meta information from pull request...",
		}
		session.CreateStatus(processStatus)
		gitmodules, err := session.Client.GetFile(session.OwnerLogin, session.MainRepo, ".gitmodules", session.UpdateSHA)
		if err != nil {
			return session.Fail(processStatus, "Cannot find .gitmodules.", err)
		}
		moduleEntries, err := ParseDotGitmodulesContent(gitmodules)
		if err != nil {
			return session.Fail(processStatus, "Cannot parse submodules from main repo.", err)
		}
		session.mut.Lock()
		defer session.mut.Unlock()
		for _, entry := range moduleEntries {
			session.Submodules[entry.Name] = &SubmoduleInfo{
				BaseInfo: entry,
			}
		}
		err = session.CheckUpdateConflict()
		if err != nil {
			return session.Fail(processStatus, "Update conflict.", err)
		}
		session.Succeed(processStatus, fmt.Sprintf("Successful in %s.", time.Since(session.stage.StartedAt()).Round(time.Second)))
		return nil
	}()
	session.stage.Release()
	if err == nil {
		session.Next()
	}
	return err
}

func (session *Session) Deliver() error {
	if !session.stage.Start() {
		return nil
	}
	err := func() error {
		deliverStatus := github.Status{
			Context: "auto-update / deliver-pr",
		}
		startedAt := time.Now()
		session.Pend(deliverStatus, "Delivering pull requests to submodules...")
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
				return session.Fail(deliverStatus, "Deliver failed.", nil)
			}
		}
		session.Succeed(deliverStatus, fmt.Sprintf("Successful in %s.", time.Since(startedAt).Round(time.Second)))
		return nil
	}()
	session.stage.Release()
	if err == nil {
		session.Next()
	}
	return err
}

func (session *Session) DelayedConclude() error {
	if !session.stage.Start() {
		return nil
	}
	time.Sleep(10 * time.Second)
	session.Conclude()
	session.stage.Release()
	return session.CheckAndUpdate()
}

func (session *Session) Conclude() error {
	var errs []error
	for _, submodule := range session.Submodules {
		err := session.ConcludeSubmoduleStatus(submodule)
		errs = append(errs, err)
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (session *Session) SubmoduleMerged() bool {
	for _, submodule := range session.Submodules {
		if submodule.MergedSHA == "" {
			return false
		}
	}
	return true
}

func (session *Session) ConcludeSubmoduleStatus(submodule *SubmoduleInfo) error {
	if submodule.Status == nil || session.Stage() > utypes.WAITING {
		// Don't update any status after WAITING stage
		return nil
	} else if submodule.MergedSHA == "" {
		owner := session.OwnerLogin
		repo := submodule.BaseInfo.Name
		SHA := submodule.PRInfo.Head.SHA
		status := CommitStatus(session.Client, owner, repo, SHA)
		if submodule.Status.State != status {
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
			err := session.CreateStatus(*submodule.Status)
			if err != nil {
				return fmt.Errorf("failed to update status for PR: %s, %w", submodule.PRInfo.HTMLURL, err)
			}
		}
	}
	return nil
}

func (session *Session) SubmodulesReady() bool {
	for _, submodule := range session.Submodules {
		if submodule.Status.State != github.StatusSuccess {
			return false
		}
	}
	return true
}

func (session *Session) HandleSubmodulePR() error {
	if !session.stage.Start() {
		return nil
	}
	mergedCount := 0
	var errs []error
	for _, submodule := range session.Submodules {
		// In case HandleSubmodulePR is invoked directly, refresh pull request here
		if submodule.PRInfo != nil {
			// Has PRInfo tracked
			RemotePR, err := session.Client.GetPullRequest(submodule.PRInfo.Base.Repo.Owner.Login, submodule.PRInfo.Base.Repo.Name, submodule.PRInfo.Number)
			if err == nil {
				submodule.PRInfo = RemotePR
				if RemotePR.Merged && RemotePR.MergeSHA != nil {
					submodule.MergedSHA = *RemotePR.MergeSHA
				}
			}
		}
		if submodule.MergedSHA != "" {
			mergedCount++
			continue
		}
		err := session.
		Client.AddLabel(session.OwnerLogin, submodule.BaseInfo.Name, submodule.PRInfo.Number, "skip-review")
		if err != nil {
			errs = append(errs, err)
		}
	}
	session.stage.Release()
	if len(errs) == 0 && mergedCount == len(session.Submodules) {
		session.Next()
		return nil
	} else {
		return errs[0]
	}
}

func (session *Session) UpdateSubmodule() error {
	if !session.stage.Start() {
		return nil
	}
	err := func() error {
		updateStatus, err := FilteredStatusFromGitHub(session.Client, session.OwnerLogin, session.MainRepo, session.UpdateSHA, "auto-update / update-submodules")
		var nfe *NotFoundError
		if err != nil && !errors.As(err, &nfe)  {
			return err
		}
		if updateStatus != nil {
			if updateStatus.State == github.StatusSuccess {
				session.SubmodulesUpdated = true
			}
		} else {
			updateStatus = &github.Status{
				State:       github.StatusPending,
				Context:     "auto-update / update-submodules",
				Description: "Updating submodules",
			}
			session.CreateStatus(*updateStatus)
		}
		if session.SubmodulesUpdated {
			return nil
		}
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
		repo.Config("user.name", session.BotUser.Login)
		repo.Config("user.email", session.BotUser.Email)
		repo.Commit("chore: update submodules", fmt.Sprintf("Update submodules to version %s.", session.UpdateToVersion))
		defer session.CreateStatus(github.Status{
			State:       github.StatusSuccess,
			Context:     "auto-update / update-submodules",
			Description: fmt.Sprintf("Successful in %s.", time.Since(session.stage.StartedAt()).Round(time.Second)),
		})
		return repo.PushToCentral(session.UpdateBaseBranch, true)
	}()
	session.stage.Release()
	if err == nil {
		session.Next()
	}
	return err
}

func (session *Session) UpdateChangelog(submodule *SubmoduleInfo) error {
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
	if submodule.Status == nil {
		// Firstly read status from GitHub
		remoteStatus, err := FilteredStatusFromGitHub(
			session.Client,
			session.OwnerLogin,
			session.MainRepo,
			session.UpdateSHA,
			fmt.Sprintf("auto-update / check-update (%s)", submodule.BaseInfo.Name),
		)
		if err == nil {
			submodule.Status = remoteStatus
		} else {
			// Create a new status locally
			submodule.Status = &github.Status{
				State:       github.StatusPending,
				Context:  fmt.Sprintf("auto-update / check-update (%s)", submodule.BaseInfo.Name),
				Description: "Creating pull request for update...",
				TargetURL:   submodule.BaseInfo.URL,
			}
			session.CreateStatus(*submodule.Status)
		}
	}
	// Get pull request as long as possible
	// First get pull request from existing status
	if submodule.Status.Description == "Waiting for checks to complete..." {
		// Extract pull request number from target url
		url := submodule.Status.TargetURL
		index := strings.LastIndex(url, "/")
		number, err := strconv.Atoi(url[index+1 : len(url)-1])
		if err == nil {
			submodule.PRInfo, _ = gc.GetPullRequest(session.OwnerLogin, submodule.BaseInfo.Name, number)
		}
	}
	if submodule.PRInfo == nil {
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
	}
	if submodule.MergedSHA != "" {
		// Already merged and tagged. Do not need to deliver PR
		submodule.Status.State = github.StatusSuccess
		submodule.Status.Description = "Already merged or tagged."
		submodule.Status.TargetURL = submodule.BaseInfo.URL
		session.CreateStatus(*submodule.Status)
	} else if submodule.Status.Description == "Creating pull request for update..." {
		// Just need to process status which is creating pull request
		err := session.UpdateChangelog(submodule)
		if err != nil {
			return err
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
		session.CreateStatus(*submodule.Status)
	}
	if submodule.PRInfo != nil {
		submodule.StartedAt = submodule.PRInfo.UpdatedAt
	}
	return nil
}

func (session *Session) CheckAndUpdate() error {
	if !session.IsStage(utypes.WAITING) {
		return nil
	}
	client := session.Client
	approved := PRApproved(client, session.UpdatePR)
	passed := ChecksPassed(client, session.OwnerLogin, session.MainRepo, session.UpdateSHA)
	if approved && passed && session.SubmodulesReady() {
		session.Next()
		return nil
	} else {
		return &NotReadyError{message: "update pull request and submodules are not ready"}
	}
}

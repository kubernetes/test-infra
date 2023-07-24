package updatebot

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

type Submodule struct {
	Name   string
	Path   string
	URL    string
	Branch string
}

type GitClientError struct {
	owner string
	repo string
	msg string
	err error
}

func (gce GitClientError) Error() string {
	if gce.err != nil {
		return fmt.Sprintf("%s/%s %s, %v", gce.owner, gce.repo, gce.msg, gce.err)
	} else {
		return fmt.Sprintf("%s/%s %s", gce.owner, gce.repo, gce.msg)
	}
}

type TagExistError struct {
	repo string
	tag  string
}

func (tee TagExistError) Error() string {
	return fmt.Sprintf("tag %s exists in repo %s", tee.tag, tee.repo)
}

type NotFoundError struct {
	target string
	err error
}

func (nfe NotFoundError) Error() string {
	if nfe.err != nil {
		return fmt.Sprintf("%s not found, %v", nfe.target, nfe.err)
	} else {
		return fmt.Sprintf("%s not found", nfe.target)
	}
}

type NotReadyError struct {
	err     error
	message string
}

func (e NotReadyError) Error() string {
	return fmt.Sprintf(e.message + ", %v", e.err)
}

func (e NotReadyError) Unwrap() error {
	return e.err
}

type NotAuthorizedError struct {
	user string
}

func (nae NotAuthorizedError) Error() string {
	return fmt.Sprintf("User %s is not authorized", nae.user)
}

func ParseDotGitmodulesContent(content []byte) ([]Submodule, error) {
	file, err := ini.Load(content)
	if err != nil {
		return nil, err
	}
	sections := file.Sections()
	var result []Submodule
	for _, section := range sections {
		rg := regexp.MustCompile(`submodule\s"(.*)"`)
		match := rg.FindStringSubmatch(section.Name())
		if len(match) != 2 {
			continue
		}
		entry := Submodule{
			Name:   match[1],
			Path:   section.Key("path").String(),
			URL:    section.Key("url").String(),
			Branch: section.Key("branch").String(),
		}
		result = append(result, entry)
	}
	return result, nil
}

func UpdateChangelog(entry *logrus.Entry, submodule *config.Submodule, context *Session) error {
	cwd, err := os.MkdirTemp("", "updatebot*")
	defer os.RemoveAll(cwd)
	if err != nil {
		entry.WithError(err).Warn("Cannot create a work directory")
		return err
	}
	repo, err := git.PlainClone(cwd, false, &git.CloneOptions{
		URL:           submodule.URL,
		ReferenceName: plumbing.NewBranchReferenceName(submodule.Branch),
	})
	if err != nil {
		entry.WithError(err).Warn("Cannot clone normally")
		return err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		entry.WithError(err).Warn("Cannot get worktree")
		return err
	}
	base, err := repo.Head()
	updateBranch := plumbing.NewBranchReferenceName(context.UpdateHeadBranch)
	if err != nil {
		entry.WithError(err).Warn("Cannot get update base branch")
		return err
	}
	worktree.Checkout(&git.CheckoutOptions{
		Hash:   base.Hash(),
		Branch: updateBranch,
		Create: true,
		Force:  true,
	})
	cmd := exec.Cmd{
		Path: "gbp",
		Dir: cwd,
		Args: []string{
			"deepin-changelog",
			"-N",
			context.UpdateToVersion,
			"--spawn-editor=never",
			"--distribution=unstable",
			"--force-distribution",
			"--git-author",
			"--ignore-branch",
		},
	}
	err = cmd.Run()
	if err != nil {
		logrus.WithError(err).Warn("Execute gbp failed")
		return err
	}
	commitMessage := fmt.Sprintf("chore: update changelog\n\nRelease %s.", context.UpdateToVersion)
	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		All: true,
	})
	if err != nil {
		logrus.WithError(err).Warn("Cannot commit")
		return err
	}
	refSpec := config.RefSpec(fmt.Sprintf("+%s:%s", updateBranch, updateBranch))
	err = repo.Push(&git.PushOptions{
		Force: true,
		RefSpecs: []config.RefSpec{refSpec},
	})
	if err != nil {
		logrus.WithError(err).Warn("Cannot push to remote")
		return err
	}
	return nil
}

// Make sure param is correct
func MoreImportantStatus(result, single string) string {
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

func CombinedStatus(client plugins.PluginGitHubClient, owner, repo, SHA string) string {
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
		result = MoreImportantStatus(result, status.State)
	}
	return result
}

// If all checks are passed (success or skipped), return true
func ChecksPassed(client plugins.PluginGitHubClient, owner, repo, SHA string) bool {
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

func MoreImportantConclusion(prev, next string) string {
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

func CommitStatus(client plugins.PluginGitHubClient, owner, repo, SHA string) string {
	checks, err := client.ListCheckRuns(owner, repo, SHA)
	if err != nil {
		logrus.WithError(err).Warnf("Cannot list check runs for %s/%s, SHA: %s", owner, repo, SHA)
		return github.StatusPending
	}
	conclusion := "skipped"
	for _, check := range checks.CheckRuns {
		switch check.Status {
		case "completed":
			conclusion = MoreImportantConclusion(conclusion, check.Conclusion)
		default:
			conclusion = MoreImportantConclusion(conclusion, "pending")
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
	return MoreImportantStatus(status, CombinedStatus(client, owner, repo, SHA))
}

func PRApproved(client plugins.PluginGitHubClient, pullRequest *github.PullRequest) bool {
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

func FilteredStatusFromGitHub(client plugins.PluginGitHubClient, owner, repo, SHA, context string) (*github.Status, error) {
	combined, err := client.GetCombinedStatus(owner, repo, SHA)
	if err != nil {
		return nil, err
	}
	for _, status := range combined.Statuses {
		if status.Context == context {
			return &status, nil
		}
	}
	return nil, &NotFoundError{target: context, err: nil}
}

package forcemerge

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"

	okrov1beta2 "github.com/traiana/okro/okro/api/v1beta2"
)

const (
	masterRef   = "heads/master"
	tideContext = "tide"
)

var (
	mergeRe = regexp.MustCompile(`(?mi)^/force merge\s*$`)
)

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
	Merge(org, repo string, pr int, details github.MergeDetails) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetRef(org, repo, ref string) (string, error)
	CreateStatus(org, repo, SHA string, s github.Status) error
}

type gitClient interface {
	Clone(repo string) (*git.Repo, error)
}

type ownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

// force-merge implementors should implement this method to construct and validate the relevant model
// clonedDir: path to the directory where the PR code is cloned
// baseSHA: commit SHA of the base branch
// may return different error types:
// ConstructError: means the construction of the model has failed
// okro.Error: means the validation API returned an error or warning
// nil: means the PR is valid
// any other error: means the event processing should stop
type ValidationFunc func(clonedDir string, baseSHA string) error

// HandleGenericComment handles the common flow of force-merge plugins, delegating to concrete plugins for the
// implementation of the validation part
func HandleGenericComment(ghc githubClient, gc gitClient, oc ownersClient, ce *github.GenericCommentEvent,
	validateJobContext string, validate ValidationFunc) error {
	// Only consider open PRs and new comments.
	if !ce.IsPR || ce.IssueState != "open" || ce.Action != github.GenericCommentActionCreated || !mergeRe.MatchString(ce.Body) {
		return nil
	}

	org := ce.Repo.Owner.Login
	repo := ce.Repo.Name
	pr, err := ghc.GetPullRequest(org, repo, ce.Number)
	if err != nil {
		return fmt.Errorf("failed to get PR %s/%s#%d: %s", org, repo, ce.Number, err)
	}

	// Check user is root approver
	owners, err := oc.LoadRepoOwners(org, repo, pr.Base.Ref)
	if err != nil {
		return fmt.Errorf("failed to load repo owners for PR %s/%s#%d: %s", org, repo, ce.Number, err)
	}
	approvers := owners.Approvers("OWNERS")
	if !approvers.Has(ce.User.Login) {
		msg := fmt.Sprintf("Only root approvers are allowed to force-merge this PR.\nRoot approvers are: %s.",
			strings.Join(approvers.List(), ", "))
		return createResponseComment(ghc, ce, msg)
	}

	// Check PR is synced with master. This is important because unlike in presubmit jobs, we validate the PR
	// without merging it to master first, which means the PR may be valid but will break master once it's merged.
	masterSHA, err := ghc.GetRef(org, repo, masterRef)
	if err != nil {
		return fmt.Errorf("failed to get %s ref for repo %s/%s: %s", masterRef, org, repo, err)
	}
	if masterSHA != pr.Base.SHA {
		msg := "This branch is out-of-date with the base branch.\n" +
			"Merge the latest changes from `master` into this branch and run `/force merge` again."
		return createResponseComment(ghc, ce, msg)
	}

	cloned, err := gc.Clone(ce.Repo.FullName)
	if err != nil {
		return fmt.Errorf("failed to clone %s", ce.Repo.FullName)
	}
	if err := cloned.Checkout(pr.Head.SHA); err != nil {
		return fmt.Errorf("failed to checkout commit %s", pr.Head.SHA)
	}

	err = validate(cloned.Dir, pr.Base.SHA)

	switch typedErr := err.(type) {
	case nil:
		msg := "Merge is not allowed: PR is valid.\nIf the validation job has failed, run `/rerun`. " +
			"Otherwise, make sure all required checks pass and wait for the PR to be merged automatically."
		return createResponseComment(ghc, ce, msg)
	case *ConstructError:
		msg := fmt.Sprintf("Merge is not allowed: %v", err)
		return createResponseComment(ghc, ce, msg)
	case okrov1beta2.Error:
		if typedErr.AppCode == okrov1beta2.MergeWarning {
			msg := fmt.Sprintf("Forcing merge ignoring the following warnings:\n\n%s", asCode(typedErr.DetailedError()))
			if err := createResponseComment(ghc, ce, msg); err != nil {
				return err
			}
		} else {
			msg := fmt.Sprintf("Merge is not allowed: %v", typedErr.Message)
			if typedErr.Details != nil {
				msg += fmt.Sprintf("\n\n%s", asCode(typedErr.DetailedError()))
			}
			return createResponseComment(ghc, ce, msg)
		}
	default:
		return err
	}

	// Set the state of both the validation job and tide contexts to success,
	// as they are both required and will prevent the merge
	if err := ghc.CreateStatus(org, repo, pr.Head.SHA, github.Status{State: "success", Context: validateJobContext}); err != nil {
		return fmt.Errorf("failed to create %s status in %s/%s#%d", validateJobContext, org, repo, ce.Number)
	}
	if err := ghc.CreateStatus(org, repo, pr.Head.SHA, github.Status{State: "success", Context: tideContext}); err != nil {
		return fmt.Errorf("failed to create %s status in %s/%s#%d", tideContext, org, repo, ce.Number)
	}

	if err := ghc.Merge(org, repo, pr.Number, github.MergeDetails{SHA: pr.Head.SHA}); err != nil {
		return createResponseComment(ghc, ce, fmt.Sprintf("Merge failed: %v", err))
	}
	return nil
}

func createResponseComment(ghc githubClient, ce *github.GenericCommentEvent, msg string) error {
	comment := plugins.FormatResponseRaw(ce.Body, ce.HTMLURL, ce.User.Login, msg)
	if err := ghc.CreateComment(ce.Repo.Owner.Login, ce.Repo.Name, ce.Number, comment); err != nil {
		return fmt.Errorf("failed to create comment in %s/%s#%d", ce.Repo.Owner.Login, ce.Repo.Name, ce.Number)
	}
	return nil
}

func asCode(data string) string {
	return fmt.Sprintf("```\n%s\n```", data)
}

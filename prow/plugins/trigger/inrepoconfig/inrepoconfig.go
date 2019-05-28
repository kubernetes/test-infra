package inrepoconfig

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins/trigger/inrepoconfig/api"
)

const commentTag = "<!-- inrepoconfig report -->"

type githubClient interface {
	BotName() (string, error)
	CreateStatus(org, repo, sha string, status github.Status) error
	GetRef(org, repo, ref string) (string, error)
	CreateComment(owner, repo string, number int, comment string) error
	EditComment(org, repo string, exitingCommentID int, comment string) error
	DeleteComment(org, repo string, issueCommentToDelete int) error
	ListIssueComments(owner, repo string, issue int) ([]github.IssueComment, error)
}

func HandlePullRequest(log *logrus.Entry, pc config.ProwConfig, ghc githubClient, pr github.PullRequest) (
	[]config.Presubmit, error) {
	org, repo, author, sha := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.User.Login, pr.Head.SHA

	status := github.Status{
		State:   "pending",
		Context: api.ContextName,
	}
	if err := ghc.CreateStatus(org, repo, sha, status); err != nil {
		return nil, fmt.Errorf("failed to create status: %v", err)
	}

	baseSHA, err := ghc.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest SHA for base ref %q: %v", pr.Base.Ref, err)
	}
	refs := []prowapi.Refs{
		prowapi.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: pr.Base.Ref,
			BaseSHA: baseSHA,
			Pulls: []prowapi.Pull{
				prowapi.Pull{
					Number: pr.Number,
					SHA:    pr.Head.SHA,
					Ref:    pr.Head.Ref,
				},
			},
		},
	}

	jc, err := api.NewJobConfig(log, refs, &pc)
	if err != nil {
		log.WithError(err).Error("failed to read JobConfig from repo")
		status.State = "failure"
		if err := ghc.CreateStatus(org, repo, sha, status); err != nil {
			log.WithError(err).Error("failed to create GitHub context")
		}

		comment := fmt.Sprintf("%s\n@%s: Loading `%s` failed with the following error:\n```\n%v\n```",
			commentTag, author, api.ConfigFileName, err)
		_, exitingCommentID, err := getOutdatedIssueComments(ghc, org, repo, pr.Number)
		if err != nil {
			log.WithError(err).Error("failed to list comments")
		}
		if exitingCommentID == 0 {
			if err := ghc.CreateComment(org, repo, pr.Number, comment); err != nil {
				log.WithError(err).Error("failed to create comment")
			}
		} else {
			if err := ghc.EditComment(org, repo, exitingCommentID, comment); err != nil {
				log.WithError(err).Error("failed to update comment")
			}
		}

		return nil, fmt.Errorf("failed to read %q: %v", api.ConfigFileName, err)
	}

	status.State = "success"
	if err := ghc.CreateStatus(org, repo, sha, status); err != nil {
		return nil, fmt.Errorf("failed to set GitHub context to %q after creating ProwJobs: %v", status.State, err)
	}
	if err := removeOutdatedIssueComments(ghc, org, repo, pr.Number); err != nil {
		return nil, fmt.Errorf("failed to return outdated issue comments: %v", err)
	}

	return jc.Presubmits, nil
}

func removeOutdatedIssueComments(ghc githubClient, org, repo string, pr int) error {
	issueCommentsToDelete, _, err := getOutdatedIssueComments(ghc, org, repo, pr)
	if err != nil {
		return err
	}
	for _, issueCommentToDelete := range issueCommentsToDelete {
		if err := ghc.DeleteComment(org, repo, issueCommentToDelete); err != nil {
			return fmt.Errorf("failed to delete comment: %v", err)
		}
	}
	return nil
}

func getOutdatedIssueComments(ghc githubClient, org, repo string, pr int) (all []int, latest int, err error) {
	ics, err := ghc.ListIssueComments(org, repo, pr)
	if err != nil {
		err = fmt.Errorf("failed to list comments: %v", err)
		return
	}

	botName, err := ghc.BotName()
	if err != nil {
		err = fmt.Errorf("failed to get botName: %v", err)
		return
	}

	for _, ic := range ics {
		if ic.User.Login != botName {
			continue
		}
		if !strings.Contains(ic.Body, commentTag) {
			continue
		}
		all = append(all, ic.ID)
		latest = ic.ID
	}

	return
}

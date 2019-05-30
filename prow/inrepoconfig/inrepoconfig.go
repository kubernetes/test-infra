/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package inrepoconfig

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/inrepoconfig/api"
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

func HandlePullRequest(log *logrus.Entry, c *config.Config, ghc githubClient, gc *git.Client, pr github.PullRequest) (
	string, []config.Presubmit, error) {
	org, repo, author, sha := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.User.Login, pr.Head.SHA

	status := github.Status{
		State:   "pending",
		Context: api.ContextName,
	}
	if err := ghc.CreateStatus(org, repo, sha, status); err != nil {
		return "", nil, fmt.Errorf("failed to create status: %v", err)
	}

	baseSHA, err := ghc.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get latest SHA for base ref %q: %v", pr.Base.Ref, err)
	}

	irc, err := api.New(log, c, gc, org, repo, baseSHA, []string{pr.Head.SHA})
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

		return "", nil, fmt.Errorf("failed to read %q: %v", api.ConfigFileName, err)
	}

	status.State = "success"
	if err := ghc.CreateStatus(org, repo, sha, status); err != nil {
		return "", nil, fmt.Errorf("failed to set GitHub context to %q after creating ProwJobs: %v", status.State, err)
	}
	if err := removeOutdatedIssueComments(ghc, org, repo, pr.Number); err != nil {
		return "", nil, fmt.Errorf("failed to return outdated issue comments: %v", err)
	}

	return baseSHA, irc.Presubmits, nil
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

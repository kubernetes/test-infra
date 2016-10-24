/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"sync"

	"k8s.io/test-infra/prow/github"
)

// GitHubAgent consumes events off of the event channels and decides what
// builds to trigger.
type GitHubAgent struct {
	DryRun       bool
	Org          string
	GitHubClient githubClient

	JenkinsJobs *JobAgent

	PullRequestEvents  <-chan github.PullRequestEvent
	IssueCommentEvents <-chan github.IssueCommentEvent

	BuildRequests  chan<- KubeRequest
	DeleteRequests chan<- KubeRequest

	// Cache of org members, protected by the lock.
	orgMembers map[string]bool
	mut        sync.Mutex
}

type githubClient interface {
	IsMember(org, user string) (bool, error)
	ListIssueComments(owner, repo string, issue int) ([]github.IssueComment, error)
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequest(owner, repo string, number int) (*github.PullRequest, error)
}

// Start starts listening for events. It does not block.
func (ga *GitHubAgent) Start() {
	go func() {
		for pr := range ga.PullRequestEvents {
			if err := ga.handlePullRequestEvent(pr); err != nil {
				logrus.WithError(err).Error("Error handling PR.")
			}
		}
	}()
	go func() {
		for ic := range ga.IssueCommentEvents {
			if err := ga.handleIssueCommentEvent(ic); err != nil {
				logrus.WithError(err).Error("Error handling issue.")
			}
		}
	}()
}

func (ga *GitHubAgent) handlePullRequestEvent(pr github.PullRequestEvent) error {
	owner := pr.PullRequest.Base.Repo.Owner.Login
	name := pr.PullRequest.Base.Repo.Name
	logrus.WithFields(logrus.Fields{
		"org":  owner,
		"repo": name,
		"pr":   pr.Number,
		"url":  pr.PullRequest.HTMLURL,
	}).Infof("Pull request %s.", pr.Action)

	switch pr.Action {
	case "opened":
		// When a PR is opened, if the author is in the org then build it.
		// Otherwise, ask for "ok to test". There's no need to look for previous
		// "ok to test" comments since the PR was just opened!
		member, err := ga.isMember(pr.PullRequest.User.Login)
		if err != nil {
			return fmt.Errorf("could not check membership: %s", err)
		} else if member {
			ga.buildAll(pr.PullRequest)
		} else {
			logrus.WithFields(logrus.Fields{
				"org":  owner,
				"repo": name,
				"pr":   pr.Number,
			}).Infof("Commenting \"is this ok to test\".")
			if err := ga.askToJoin(pr.PullRequest); err != nil {
				return fmt.Errorf("could not ask to join: %s", err)
			}
		}
	case "reopened", "synchronize":
		// When a PR is updated, check that the user is in the org or that an org
		// member has said "ok to test" before building. There's no need to ask
		// for "ok to test" because we do that once when the PR is created.
		trusted, err := ga.trustedPullRequest(pr.PullRequest)
		if err != nil {
			return fmt.Errorf("could not validate PR: %s", err)
		} else if trusted {
			ga.buildAll(pr.PullRequest)
		}
	case "closed":
		ga.deleteAll(pr.PullRequest)
	}
	return nil
}

func (ga *GitHubAgent) handleIssueCommentEvent(ic github.IssueCommentEvent) error {
	owner := ic.Repo.Owner.Login
	name := ic.Repo.Name
	number := ic.Issue.Number
	author := ic.Comment.User.Login
	logrus.WithFields(logrus.Fields{
		"org":    owner,
		"repo":   name,
		"pr":     number,
		"author": author,
		"url":    ic.Comment.HTMLURL,
	}).Infof("Issue comment %s.", ic.Action)

	switch ic.Action {
	case "created", "edited":
		// If it's not an open PR, skip it.
		if ic.Issue.PullRequest == nil {
			return nil
		}
		if ic.Issue.State != "open" {
			return nil
		}
		// Skip bot comments.
		if author == "k8s-bot" || author == "k8s-ci-robot" {
			return nil
		}

		// Which jobs does the comment want us to run?
		requestedJobs := ga.JenkinsJobs.MatchingJobs(ic.Repo.FullName, ic.Comment.Body)
		if len(requestedJobs) == 0 {
			return nil
		}

		// Skip untrusted users.
		orgMember, err := ga.isMember(author)
		if err != nil {
			return err
		} else if !orgMember {
			return nil
		}

		pr, err := ga.GitHubClient.GetPullRequest(owner, name, number)
		if err != nil {
			return err
		}

		for _, job := range requestedJobs {
			kr := makeKubeRequest(job, *pr)
			ga.BuildRequests <- kr
		}
	}
	return nil
}

// trustedPullRequest returns whether or not the given PR should be tested.
// It first checks if the author is in the org, then looks for "ok to test
// comments by org members.
func (ga *GitHubAgent) trustedPullRequest(pr github.PullRequest) (bool, error) {
	author := pr.User.Login
	// First check if the author is a member of the org.
	orgMember, err := ga.isMember(author)
	if err != nil {
		return false, err
	} else if orgMember {
		return true, nil
	}
	// Next look for "ok to test" comments on the PR.
	comments, err := ga.GitHubClient.ListIssueComments(pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Number)
	if err != nil {
		return false, err
	}
	for _, comment := range comments {
		commentAuthor := comment.User.Login
		// Skip comments by the PR author.
		if commentAuthor == author {
			continue
		}
		// Skip bot comments.
		if commentAuthor == "k8s-bot" || commentAuthor == "k8s-ci-robot" {
			continue
		}
		// Look for "ok to test"
		if !okToTest.MatchString(comment.Body) {
			continue
		}
		// Ensure that the commenter is in the org.
		commentAuthorMember, err := ga.isMember(commentAuthor)
		if err != nil {
			return false, err
		} else if commentAuthorMember {
			return true, nil
		}
	}
	return false, nil
}

func (ga *GitHubAgent) askToJoin(pr github.PullRequest) error {
	commentTemplate := `
Can a [%s](https://github.com/orgs/%s/people) member verify that this patch is reasonable to test? If so, please reply with "@k8s-bot ok to test" on its own line.

Regular contributors should join the org to skip this step.`
	comment := fmt.Sprintf(commentTemplate, ga.Org, ga.Org)

	owner := pr.Base.Repo.Owner.Login
	name := pr.Base.Repo.Name
	return ga.GitHubClient.CreateComment(owner, name, pr.Number, comment)
}

func makeKubeRequest(job JenkinsJob, pr github.PullRequest) KubeRequest {
	return KubeRequest{
		JobName: job.Name,
		Context: job.Context,

		RerunCommand: job.RerunCommand,

		RepoOwner: pr.Base.Repo.Owner.Login,
		RepoName:  pr.Base.Repo.Name,
		PR:        pr.Number,
		Author:    pr.User.Login,
		BaseRef:   pr.Base.Ref,
		BaseSHA:   pr.Base.SHA,
		PullSHA:   pr.Head.SHA,
	}
}

func (ga *GitHubAgent) buildAll(pr github.PullRequest) {
	for _, job := range ga.JenkinsJobs.AllJobs(pr.Base.Repo.FullName) {
		if !job.AlwaysRun {
			continue
		}
		kr := makeKubeRequest(job, pr)
		ga.BuildRequests <- kr
	}
}

func (ga *GitHubAgent) deleteAll(pr github.PullRequest) {
	for _, job := range ga.JenkinsJobs.AllJobs(pr.Base.Repo.FullName) {
		kr := makeKubeRequest(job, pr)
		ga.DeleteRequests <- kr
	}
}

// Uses a cache for members, but ignores it for non-members.
func (ga *GitHubAgent) isMember(name string) (bool, error) {
	ga.mut.Lock()
	if ga.orgMembers == nil {
		ga.orgMembers = make(map[string]bool)
	} else if ga.orgMembers[name] {
		ga.mut.Unlock()
		return true, nil
	}
	ga.mut.Unlock()
	// Don't hold the lock for the potentially slow IsMember call.
	member, err := ga.GitHubClient.IsMember(ga.Org, name)
	if err != nil {
		return false, err
	}
	if member {
		ga.mut.Lock()
		ga.orgMembers[name] = true
		ga.mut.Unlock()
	}
	return member, nil
}

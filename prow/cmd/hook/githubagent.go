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
	"github.com/Sirupsen/logrus"
	"sync"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jobs"
	"k8s.io/test-infra/prow/plugins/lgtm"
)

// GitHubAgent consumes events off of the event channels and decides what
// builds to trigger.
type GitHubAgent struct {
	DryRun       bool
	Org          string
	GitHubClient githubClient

	Plugins     *PluginAgent
	JenkinsJobs *jobs.JobAgent

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
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

// Start starts listening for events. It does not block.
func (ga *GitHubAgent) Start() {
	go func() {
		for pr := range ga.PullRequestEvents {
			ga.handlePullRequestEvent(pr)
		}
	}()
	go func() {
		for ic := range ga.IssueCommentEvents {
			ga.handleIssueCommentEvent(ic)
		}
	}()
}

func (ga *GitHubAgent) handlePullRequestEvent(pr github.PullRequestEvent) {
	l := logrus.WithFields(logrus.Fields{
		"org":  pr.PullRequest.Base.Repo.Owner.Login,
		"repo": pr.PullRequest.Base.Repo.Name,
		"pr":   pr.Number,
		"url":  pr.PullRequest.HTMLURL,
	})
	l.Infof("Pull request %s.", pr.Action)
	if ga.Plugins.Enabled(pr.PullRequest.Base.Repo.FullName, triggerPluginName) {
		if err := ga.prTrigger(pr); err != nil {
			l.WithError(err).Error("Error triggering after pull request event.")
		}
	}
}

func (ga *GitHubAgent) handleIssueCommentEvent(ic github.IssueCommentEvent) {
	l := logrus.WithFields(logrus.Fields{
		"org":    ic.Repo.Owner.Login,
		"repo":   ic.Repo.Name,
		"pr":     ic.Issue.Number,
		"author": ic.Comment.User.Login,
		"url":    ic.Comment.HTMLURL,
	})
	l.Infof("Issue comment %s.", ic.Action)
	if ga.Plugins.Enabled(ic.Repo.FullName, triggerPluginName) {
		if err := ga.commentTrigger(ic); err != nil {
			l.WithError(err).Error("Error triggering after issue comment event.")
		}
	}
	if ga.Plugins.Enabled(ic.Repo.FullName, lgtm.PluginName) {
		if err := lgtm.HandleIssueComment(ga.GitHubClient, ic); err != nil {
			l.WithError(err).Error("Error dealing with LGTM command.")
		}
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

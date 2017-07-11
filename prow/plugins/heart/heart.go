/*
Copyright 2017 The Kubernetes Authors.

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

package heart

import (
	"math/rand"
	"path/filepath"
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName     = "heart"
	botName        = "k8s-merge-robot"
	ownersFilename = "OWNERS"
)

var mergeRe = regexp.MustCompile(`Automatic merge from submit-queue`)

var reactions = []string{
	github.ReactionThumbsUp,
	github.ReactionLaugh,
	github.ReactionHeart,
	github.ReactionHooray,
}

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest)
}

type githubClient interface {
	CreateCommentReaction(org, repo string, ID int, reaction string) error
	CreateIssueReaction(org, repo string, ID int, reaction string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handleIC(pc.GitHubClient, pc.Logger, ic)
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	return handlePR(pc.GitHubClient, pc.Logger, pre)
}

func handleIC(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
	// Only consider new comments on PRs.
	if !ic.Issue.IsPullRequest() || ic.Action != "created" {
		return nil
	}
	// Only consider the merge bot.
	if ic.Comment.User.Login != botName {
		return nil
	}

	if !mergeRe.MatchString(ic.Comment.Body) {
		return nil
	}

	log.Info("This is a wonderful thing!")
	return gc.CreateCommentReaction(
		ic.Repo.Owner.Login,
		ic.Repo.Name,
		ic.Comment.ID,
		reactions[rand.Intn(len(reactions))])
}

func handlePR(gc githubClient, log *logrus.Entry, pre github.PullRequestEvent) error {
	// Only consider newly opened PRs
	if pre.Action != "opened" {
		return nil
	}

	org := pre.PullRequest.Base.Repo.Owner.Login
	repo := pre.PullRequest.Base.Repo.Name

	changes, err := gc.GetPullRequestChanges(org, repo, pre.PullRequest.Number)
	if err != nil {
		return err
	}

	// Smile at any change that adds to OWNERS files
	for _, change := range changes {
		_, filename := filepath.Split(change.Filename)
		if filename == ownersFilename && change.Additions > 0 {
			log.Info("Adding new OWNERS makes me happy!")
			return gc.CreateIssueReaction(
				pre.PullRequest.Base.Repo.Owner.Login,
				pre.PullRequest.Base.Repo.Name,
				pre.Number,
				reactions[rand.Intn(len(reactions))])
		}
	}

	return nil
}

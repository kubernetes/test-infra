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

package heart

import (
	"math/rand"
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "heart"
	botName    = "k8s-merge-robot"
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
}

type githubClient interface {
	CreateCommentReaction(org, repo string, ID int, reaction string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, ic)
}

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
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

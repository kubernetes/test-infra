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

package shrug

import (
	"fmt"
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "shrug"

var (
	shrugLabel = "¯\\_(ツ)_/¯"
	shrugRe    = regexp.MustCompile(`(?mi)^/shrug\s*$`)
)

type event struct {
	org           string
	repo          string
	number        int
	prAuthor      string
	commentAuthor string
	body          string
	assignees     []github.User
	hasLabel      func(label string) (bool, error)
	htmlurl       string
}

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
}

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	e := &event{
		org:      ic.Repo.Owner.Login,
		repo:     ic.Repo.Name,
		number:   ic.Issue.Number,
		body:     ic.Comment.Body,
		hasLabel: func(label string) (bool, error) { return ic.Issue.HasLabel(label), nil },
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

func handle(gc githubClient, log *logrus.Entry, e *event) error {
	if !shrugRe.MatchString(e.body) {
		return nil
	}

	// Only add the label if it doesn't have it yet.
	hasShrug, err := e.hasLabel(shrugLabel)
	if err != nil {
		return fmt.Errorf("failed to get the labels on %s/%s#%d: %v", e.org, e.repo, e.number, err)
	}
	if !hasShrug {
		log.Info("Adding shrug label.")
		return gc.AddLabel(e.org, e.repo, e.number, shrugLabel)
	}
	return nil
}

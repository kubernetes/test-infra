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

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "shrug"

var (
	shrugLabel = "¯\\_(ツ)_/¯"
	shrugRe    = regexp.MustCompile(`(?mi)^/shrug\s*$`)
	unshrugRe  = regexp.MustCompile(`(?mi)^/unshrug\s*$`)
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
	CreateComment(owner, repo string, number int, comment string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	e := &event{
		org:           ic.Repo.Owner.Login,
		repo:          ic.Repo.Name,
		number:        ic.Issue.Number,
		body:          ic.Comment.Body,
		hasLabel:      func(label string) (bool, error) { return ic.Issue.HasLabel(label), nil },
		htmlurl:       ic.Comment.HTMLURL,
		commentAuthor: ic.Comment.User.Login,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

func handle(gc githubClient, log *logrus.Entry, e *event) error {
	wantShrug := false
	if shrugRe.MatchString(e.body) {
		wantShrug = true
	} else if unshrugRe.MatchString(e.body) {
		wantShrug = false
	} else {
		return nil
	}

	// Only add the label if it doesn't have it yet.
	hasShrug, err := e.hasLabel(shrugLabel)
	if err != nil {
		return fmt.Errorf("failed to get the labels on %s/%s#%d: %v", e.org, e.repo, e.number, err)
	}
	if hasShrug && !wantShrug {
		log.Info("Removing Shrug label.")
		resp := "¯\\\\\\_(ツ)\\_/¯"
		log.Infof("Commenting with \"%s\".", resp)
		if err := gc.CreateComment(e.org, e.repo, e.number, plugins.FormatResponseRaw(e.body, e.htmlurl, e.commentAuthor, resp)); err != nil {
			return fmt.Errorf("failed to comment on %s/%s#%d: %v", e.org, e.repo, e.number, err)
		}
		return gc.RemoveLabel(e.org, e.repo, e.number, shrugLabel)
	} else if !hasShrug && wantShrug {
		log.Info("Adding Shrug label.")
		return gc.AddLabel(e.org, e.repo, e.number, shrugLabel)
	}
	return nil
}

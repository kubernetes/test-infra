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

// Package hold contains a plugin which will allow users to label their
// own pull requests as not ready or ready for merge. The submit queue
// will honor the label to ensure pull requests do not merge when it is
// applied.
package hold

import (
	"fmt"
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "hold"

var (
	label         = "do-not-merge/hold"
	labelRe       = regexp.MustCompile(`(?mi)^/hold\s*$`)
	labelCancelRe = regexp.MustCompile(`(?mi)^/hold cancel\s*$`)
)

type event struct {
	org      string
	repo     string
	number   int
	body     string
	htmlurl  string
	hasLabel func() (bool, error)
}

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
	plugins.RegisterReviewEventHandler(pluginName, handleReview)
	plugins.RegisterReviewCommentEventHandler(pluginName, handleReviewComment)
}

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	// Only consider open PRs.
	if !ic.Issue.IsPullRequest() || ic.Issue.State != "open" || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	e := &event{
		org:    ic.Repo.Owner.Login,
		repo:   ic.Repo.Name,
		number: ic.Issue.Number,
		body:   ic.Comment.Body,
		hasLabel: func() (bool, error) {
			return ic.Issue.HasLabel(label), nil
		},
		htmlurl: ic.Comment.HTMLURL,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

func handleReview(pc plugins.PluginClient, re github.ReviewEvent) error {
	if re.Action != github.ReviewActionSubmitted {
		return nil
	}

	e := &event{
		org:    re.Repo.Owner.Login,
		repo:   re.Repo.Name,
		number: re.PullRequest.Number,
		body:   re.Review.Body,
		hasLabel: func() (bool, error) {
			return hasLabel(pc.GitHubClient, re.Repo.Owner.Login, re.Repo.Name, re.PullRequest.Number, label)
		},
		htmlurl: re.Review.HTMLURL,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

func handleReviewComment(pc plugins.PluginClient, rce github.ReviewCommentEvent) error {
	if rce.Action != github.ReviewCommentActionCreated {
		return nil
	}

	e := &event{
		org:    rce.Repo.Owner.Login,
		repo:   rce.Repo.Name,
		number: rce.PullRequest.Number,
		body:   rce.Comment.Body,
		hasLabel: func() (bool, error) {
			return hasLabel(pc.GitHubClient, rce.Repo.Owner.Login, rce.Repo.Name, rce.PullRequest.Number, label)
		},
		htmlurl: rce.Comment.HTMLURL,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

// handle drives the pull request to the desired state. If any user adds
// a /hold directive, we want to add a label if one does not already exist.
// If they add /hold cancel, we want to remove the label if it exists.
func handle(gc githubClient, log *logrus.Entry, e *event) error {
	needsLabel := false
	if labelRe.MatchString(e.body) {
		needsLabel = true
	} else if labelCancelRe.MatchString(e.body) {
		needsLabel = false
	} else {
		return nil
	}

	hasLabel, err := e.hasLabel()
	if err != nil {
		return err
	}

	if hasLabel && !needsLabel {
		log.Info("Removing %q label for %s/%s#%d", label, e.org, e.repo, e.number)
		return gc.RemoveLabel(e.org, e.repo, e.number, label)
	} else if !hasLabel && needsLabel {
		log.Info("Adding %q label for %s/%s#%d", label, e.org, e.repo, e.number)
		return gc.AddLabel(e.org, e.repo, e.number, label)
	}
	return nil
}

// hasLabel checks if a label is applied to a pr.
func hasLabel(c githubClient, org, repo string, num int, label string) (bool, error) {
	labels, err := c.GetIssueLabels(org, repo, num)
	if err != nil {
		return false, fmt.Errorf("failed to get the labels on %s/%s#%d: %v", org, repo, num, err)
	}
	for _, candidate := range labels {
		if candidate.Name == label {
			return true, nil
		}
	}
	return false, nil
}

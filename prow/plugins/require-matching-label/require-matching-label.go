/*
Copyright 2018 The Kubernetes Authors.

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

// Package requirematchinglabel implements the `require-matching-label` plugin.
// This is a configurable plugin that applies a label (and possibly comments)
// when an issue or PR does not have any labels matching a regexp. If a label
// is added that matches the regexp, the 'MissingLabel' is removed and the comment
// is deleted.
package requirematchinglabel

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"

	"github.com/sirupsen/logrus"
)

var (
	handlePRActions = map[github.PullRequestEventAction]bool{
		github.PullRequestActionOpened:    true,
		github.PullRequestActionReopened:  true,
		github.PullRequestActionLabeled:   true,
		github.PullRequestActionUnlabeled: true,
	}

	handleIssueActions = map[github.IssueEventAction]bool{
		github.IssueActionOpened:    true,
		github.IssueActionReopened:  true,
		github.IssueActionLabeled:   true,
		github.IssueActionUnlabeled: true,
	}
)

const (
	pluginName = "require-matching-label"
)

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	CreateComment(org, repo string, number int, content string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

type commentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

func init() {
	plugins.RegisterIssueHandler(pluginName, handleIssue, helpProvider)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []string) (*pluginhelp.PluginHelp, error) {
	descs := make([]string, 0, len(config.RequireMatchingLabel))
	for _, cfg := range config.RequireMatchingLabel {
		descs = append(descs, cfg.Describe())
	}
	// Only the 'Description' and 'Config' fields are necessary because this plugin does not react
	// to any commands.
	return &pluginhelp.PluginHelp{
			Description: `The require-matching-label plugin is a configurable plugin that applies a label to issues and/or PRs that do not have any labels matching a regular expression. An example of this is applying a 'needs-sig' label to all issues that do not have a 'sig/*' label. This plugin can have multiple configurations to provide this kind of behavior for multiple different label sets. The configuration allows issue type, PR branch, and an optional explanation comment to be specified.`,
			Config: map[string]string{
				"": fmt.Sprintf("The plugin has the following configurations:\n<ul><li>%s</li></ul>", strings.Join(descs, "</li><li>")),
			},
		},
		nil
}

type event struct {
	org    string
	repo   string
	number int
	author string
	// The PR's base branch. If empty this is an Issue, not a PR.
	branch string
	// The label that was added or removed. If empty this is an open or reopen event.
	label string
	// The labels currently on the issue. For PRs this is not contained in the webhook payload and may be omitted.
	currentLabels []github.Label
}

func handleIssue(pc plugins.Agent, ie github.IssueEvent) error {
	if !handleIssueActions[ie.Action] {
		return nil
	}
	e := &event{
		org:           ie.Repo.Owner.Login,
		repo:          ie.Repo.Name,
		number:        ie.Issue.Number,
		author:        ie.Issue.User.Login,
		label:         ie.Label.Name, // This will be empty for non-label events.
		currentLabels: ie.Issue.Labels,
	}
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.Logger, pc.GitHubClient, cp, pc.PluginConfig.RequireMatchingLabel, e)
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	if !handlePRActions[pre.Action] {
		return nil
	}
	e := &event{
		org:    pre.Repo.Owner.Login,
		repo:   pre.Repo.Name,
		number: pre.PullRequest.Number,
		branch: pre.PullRequest.Base.Ref,
		author: pre.PullRequest.User.Login,
		label:  pre.Label.Name, // This will be empty for non-label events.
	}
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.Logger, pc.GitHubClient, cp, pc.PluginConfig.RequireMatchingLabel, e)
}

// matchingConfigs filters irrelevant RequireMtchingLabel configs from
// the list of all configs.
// `branch` should be empty for Issues and non-empty for PRs.
// `label` should be omitted in the case of 'open' and 'reopen' actions.
func matchingConfigs(org, repo, branch, label string, allConfigs []plugins.RequireMatchingLabel) []plugins.RequireMatchingLabel {
	var filtered []plugins.RequireMatchingLabel
	for _, cfg := range allConfigs {
		// Check if the config applies to this issue type.
		if (branch == "" && !cfg.Issues) || (branch != "" && !cfg.PRs) {
			continue
		}
		// Check if the config applies to this 'org[/repo][/branch]'.
		if org != cfg.Org ||
			(cfg.Repo != "" && cfg.Repo != repo) ||
			(cfg.Branch != "" && branch != "" && cfg.Branch != branch) {
			continue
		}
		// If we are reacting to a label event, see if it is relevant.
		if label != "" && !cfg.Re.MatchString(label) {
			continue
		}
		filtered = append(filtered, cfg)
	}
	return filtered
}

func handle(log *logrus.Entry, ghc githubClient, cp commentPruner, configs []plugins.RequireMatchingLabel, e *event) error {
	// Find any configs that may be relevant to this event.
	matchConfigs := matchingConfigs(e.org, e.repo, e.branch, e.label, configs)
	if len(matchConfigs) == 0 {
		return nil
	}

	if e.label == "" /* not a label event */ {
		// If we are reacting to a PR or Issue being created or reopened, we should wait a
		// few seconds to allow other automation to apply labels in order to minimize thrashing.
		// We use the max grace period from applicable configs.
		gracePeriod := time.Duration(0)
		for _, cfg := range matchConfigs {
			if cfg.GracePeriodDuration > gracePeriod {
				gracePeriod = cfg.GracePeriodDuration
			}
		}
		time.Sleep(gracePeriod)
		// If currentLabels was populated it is now stale.
		e.currentLabels = nil
	}
	if e.currentLabels == nil {
		var err error
		e.currentLabels, err = ghc.GetIssueLabels(e.org, e.repo, e.number)
		if err != nil {
			return fmt.Errorf("error getting the issue or pr's labels: %v", err)
		}
	}

	// Handle the potentially relevant configs.
	for _, cfg := range matchConfigs {
		hasMissingLabel := false
		hasMatchingLabel := false
		for _, label := range e.currentLabels {
			hasMissingLabel = hasMissingLabel || label.Name == cfg.MissingLabel
			hasMatchingLabel = hasMatchingLabel || cfg.Re.MatchString(label.Name)
		}

		if hasMatchingLabel && hasMissingLabel {
			if err := ghc.RemoveLabel(e.org, e.repo, e.number, cfg.MissingLabel); err != nil {
				log.WithError(err).Errorf("Failed to remove %q label.", cfg.MissingLabel)
			}
			if cfg.MissingComment != "" {
				cp.PruneComments(func(comment github.IssueComment) bool {
					return strings.Contains(comment.Body, cfg.MissingComment)
				})
			}
		} else if !hasMatchingLabel && !hasMissingLabel {
			if err := ghc.AddLabel(e.org, e.repo, e.number, cfg.MissingLabel); err != nil {
				log.WithError(err).Errorf("Failed to add %q label.", cfg.MissingLabel)
			}
			if cfg.MissingComment != "" {
				msg := plugins.FormatSimpleResponse(e.author, cfg.MissingComment)
				if err := ghc.CreateComment(e.org, e.repo, e.number, msg); err != nil {
					log.WithError(err).Error("Failed to create comment.")
				}
			}
		}

	}
	return nil
}

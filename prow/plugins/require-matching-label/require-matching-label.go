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
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	rblm "k8s.io/test-infra/prow/plugins/internal/regex-based-label-match"

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

	checkRequireLabelsRe = regexp.MustCompile(`(?mi)^/check-required-labels\s*$`)
)

const (
	pluginName = "require-matching-label"
)

func init() {
	plugins.RegisterIssueHandler(pluginName, handleIssue, helpProvider)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
	plugins.RegisterGenericCommentHandler(pluginName, handleCommentEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	descs := make([]string, 0, len(config.RequireMatchingLabel))
	for _, cfg := range config.RequireMatchingLabel {
		descs = append(descs, cfg.Describe())
	}
	// Only the 'Description' and 'Config' fields are necessary because this plugin does not react
	// to any commands.
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		RequireMatchingLabel: []plugins.RequireMatchingLabel{
			{
				RegexBasedLabelMatch: plugins.RegexBasedLabelMatch{
					Org:         "org",
					Repo:        "repo",
					Branch:      "master",
					PRs:         true,
					Issues:      true,
					Regexp:      "^kind/",
					GracePeriod: "5s",
				},
				MissingLabel:   "needs-kind",
				MissingComment: "Please add a label referencing the kind.",
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The require-matching-label plugin is a configurable plugin that applies a label to issues and/or PRs that do not have any labels matching a regular expression. An example of this is applying a 'needs-sig' label to all issues that do not have a 'sig/*' label. This plugin can have multiple configurations to provide this kind of behavior for multiple different label sets. The configuration allows issue type, PR branch, and an optional explanation comment to be specified.`,
		Config: map[string]string{
			"": fmt.Sprintf("The plugin has the following configurations:\n<ul><li>%s</li></ul>", strings.Join(descs, "</li><li>")),
		},
		Snippet: yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/check-required-labels",
		Description: "Checks for required labels.",
		Featured:    true,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/check-required-labels"},
	})
	return pluginHelp, nil
}

func handleIssue(pc plugins.Agent, ie github.IssueEvent) error {
	if !handleIssueActions[ie.Action] {
		return nil
	}
	e := &rblm.Event{
		Org:           ie.Repo.Owner.Login,
		Repo:          ie.Repo.Name,
		Number:        ie.Issue.Number,
		Author:        ie.Issue.User.Login,
		Label:         ie.Label.Name, // This will be empty for non-label events.
		CurrentLabels: ie.Issue.Labels,
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
	e := &rblm.Event{
		Org:    pre.Repo.Owner.Login,
		Repo:   pre.Repo.Name,
		Number: pre.PullRequest.Number,
		Branch: pre.PullRequest.Base.Ref,
		Author: pre.PullRequest.User.Login,
		Label:  pre.Label.Name, // This will be empty for non-label events.
	}
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.Logger, pc.GitHubClient, cp, pc.PluginConfig.RequireMatchingLabel, e)
}

// matchingConfigs filters irrelevant RequireMtchingLabel configs from
// the list of all configs.
func matchingConfigs(org, repo, branch, label string, allConfigs []plugins.RequireMatchingLabel) []plugins.RequireMatchingLabel {
	var filtered []plugins.RequireMatchingLabel
	for _, cfg := range allConfigs {
		if rblm.ShouldConsiderConfig(org, repo, branch, label, cfg.RegexBasedLabelMatch) {
			filtered = append(filtered, cfg)
		}
	}
	return filtered
}

func handle(log *logrus.Entry, ghc rblm.GithubClient, cp rblm.CommentPruner, configs []plugins.RequireMatchingLabel, e *rblm.Event) error {
	// Find any configs that may be relevant to this event.
	matchConfigs := matchingConfigs(e.Org, e.Repo, e.Branch, e.Label, configs)
	if len(matchConfigs) == 0 {
		return nil
	}

	err := rblm.LabelPreChecks(e, ghc, extractRegexBasedLabelMatchConfigs(configs))
	if err != nil {
		return err
	}

	// Handle the potentially relevant configs.
	for _, cfg := range matchConfigs {
		hasMissingLabel := false
		hasMatchingLabel := false
		for _, label := range e.CurrentLabels {
			hasMissingLabel = hasMissingLabel || label.Name == cfg.MissingLabel
			hasMatchingLabel = hasMatchingLabel || cfg.Re.MatchString(label.Name)
		}

		if hasMatchingLabel && hasMissingLabel {
			if err := ghc.RemoveLabel(e.Org, e.Repo, e.Number, cfg.MissingLabel); err != nil {
				log.WithError(err).Errorf("Failed to remove %q label.", cfg.MissingLabel)
			}
			if cfg.MissingComment != "" {
				cp.PruneComments(func(comment github.IssueComment) bool {
					return strings.Contains(comment.Body, cfg.MissingComment)
				})
			}
		} else if !hasMatchingLabel && !hasMissingLabel {
			if err := ghc.AddLabel(e.Org, e.Repo, e.Number, cfg.MissingLabel); err != nil {
				log.WithError(err).Errorf("Failed to add %q label.", cfg.MissingLabel)
			}
			if cfg.MissingComment != "" {
				msg := plugins.FormatSimpleResponse(e.Author, cfg.MissingComment)
				if err := ghc.CreateComment(e.Org, e.Repo, e.Number, msg); err != nil {
					log.WithError(err).Error("Failed to create comment.")
				}
			}
		}
	}
	return nil
}

func handleCommentEvent(pc plugins.Agent, ce github.GenericCommentEvent) error {
	// Only consider open PRs and new comments.
	if ce.IssueState != "open" || ce.Action != github.GenericCommentActionCreated {
		return nil
	}
	// Only consider "/check-required-labels" comments.
	if !checkRequireLabelsRe.MatchString(ce.Body) {
		return nil
	}

	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}

	return handleComment(pc.Logger, pc.GitHubClient, cp, pc.PluginConfig.RequireMatchingLabel, &ce)
}

func handleComment(log *logrus.Entry, ghc rblm.GithubClient, cp rblm.CommentPruner, configs []plugins.RequireMatchingLabel, e *github.GenericCommentEvent) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	event := &rblm.Event{
		Org:    org,
		Repo:   repo,
		Number: number,
		Author: e.User.Login,
	}
	if e.IsPR {
		pr, err := ghc.GetPullRequest(org, repo, number)
		if err != nil {
			return err
		}
		event.Branch = pr.Base.Ref
	}
	return handle(log, ghc, cp, configs, event)
}

func extractRegexBasedLabelMatchConfigs(configs []plugins.RequireMatchingLabel) []plugins.RegexBasedLabelMatch {
	res := make([]plugins.RegexBasedLabelMatch, 0, len(configs))
	for _, cfg := range configs {
		res = append(res, cfg.RegexBasedLabelMatch)
	}

	return res
}

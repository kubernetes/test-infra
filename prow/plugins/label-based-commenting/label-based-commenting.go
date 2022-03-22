/*
Copyright 2022 The Kubernetes Authors.

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

package labelbasedcommenting

import (
	"fmt"
	"regexp"
	"sort"
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

	sigRegex = regexp.MustCompile(`(?m)^sig/(.*?)\s*$`)
	wgRegex  = regexp.MustCompile(`(?m)^wg/(.*?)\s*$`)
)

const (
	pluginName = "label-based-commenting"
)

func init() {
	plugins.RegisterIssueHandler(pluginName, handleIssue, helpProvider)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
	plugins.RegisterGenericCommentHandler(pluginName, handleCommentEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	descs := make([]string, 0, len(config.LabelBasedCommenting))
	for _, cfg := range config.LabelBasedCommenting {
		descs = append(descs, cfg.Describe())
	}
	// Only the 'Description' and 'Config' fields are necessary because this plugin does not react
	// to any commands.
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		LabelBasedCommenting: []plugins.LabelBasedCommenting{
			{
				RegexBasedLabelMatch: plugins.RegexBasedLabelMatch{
					Org:         "org",
					Repo:        "repo",
					Branch:      "master",
					PRs:         true,
					Issues:      true,
					Regexp:      "^kind/feature$",
					GracePeriod: "5s",
				},
				Comment: "Please create a KEP instead.",
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The label-based-commenting plugin is a configurable plugin that adds a comment to issues and/or PRs based on the presence of a label detected through regular expression. This plugin can have multiple configurations to provide this kind of behavior for multiple different label sets. The configuration allows issue type, PR branch, and an optional explanation comment to be specified.`,
		Config: map[string]string{
			"": fmt.Sprintf("The plugin has the following configurations:\n<ul><li>%s</li></ul>", strings.Join(descs, "</li><li>")),
		},
		Snippet: yamlSnippet,
	}

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
		IsPR:          false,
	}
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.Logger, pc.GitHubClient, cp, pc.PluginConfig.LabelBasedCommenting, e)
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
		IsPR:   true,
	}
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.Logger, pc.GitHubClient, cp, pc.PluginConfig.LabelBasedCommenting, e)
}

// matchingConfigs filters irrelevant RequireMtchingLabel configs from
// the list of all configs.
func matchingConfigs(org, repo, branch, label string, allConfigs []plugins.LabelBasedCommenting) []plugins.LabelBasedCommenting {
	var filtered []plugins.LabelBasedCommenting
	for _, cfg := range allConfigs {
		if rblm.ShouldConsiderConfig(org, repo, branch, label, cfg.RegexBasedLabelMatch) {
			filtered = append(filtered, cfg)
		}
	}
	return filtered
}

func handleCommentEvent(pc plugins.Agent, ce github.GenericCommentEvent) error {
	// Only consider open PRs and new comments.
	if ce.IssueState != "open" || ce.Action != github.GenericCommentActionCreated {
		return nil
	}

	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}

	return handleComment(pc.Logger, pc.GitHubClient, cp, pc.PluginConfig.LabelBasedCommenting, &ce)
}

func handleComment(log *logrus.Entry, ghc rblm.GithubClient, cp rblm.CommentPruner, configs []plugins.LabelBasedCommenting, e *github.GenericCommentEvent) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	event := &rblm.Event{
		Org:    org,
		Repo:   repo,
		Number: number,
		Author: e.User.Login,
		IsPR:   e.IsPR,
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

func handle(log *logrus.Entry, ghc rblm.GithubClient, cp rblm.CommentPruner, configs []plugins.LabelBasedCommenting, e *rblm.Event) error {
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
		hasMatchingLabel := false
		for _, label := range e.CurrentLabels {
			hasMatchingLabel = hasMatchingLabel || cfg.Re.MatchString(label.Name)
		}

		commentResponse := plugins.FormatSimpleResponse(e.Author, constructCommentResponse(e, cfg.Comment))
		if hasMatchingLabel {
			// TODO(MadhavJivrajani): we need to check if the comment has already
			// been applied.
			if err := ghc.CreateComment(e.Org, e.Repo, e.Number, commentResponse); err != nil {
				log.WithError(err).Error("Failed to create matching comment.")
			}
		} else {
			cp.PruneComments(func(comment github.IssueComment) bool {
				return strings.Contains(comment.Body, commentResponse)
			})
		}

	}
	return nil
}

func extractRegexBasedLabelMatchConfigs(configs []plugins.LabelBasedCommenting) []plugins.RegexBasedLabelMatch {
	res := make([]plugins.RegexBasedLabelMatch, 0, len(configs))
	for _, cfg := range configs {
		res = append(res, cfg.RegexBasedLabelMatch)
	}

	return res
}

// constructCommentResponse constructs the comment response based on
// the user provided comment. If the repository in question (obtained
// through e) is kubernetes/kubernetes and the comment is being posted
// in response to an event on an *issue*, and it is a label event with
// the label being kind/support, the comment response will have a list
// of relevant communication media present as well (based on the list
// of the current labels). In all other cases, the comment is returned
// as is.
func constructCommentResponse(e *rblm.Event, comment string) string {
	if !isRepoKubernetesKubernetes(e.Org, e.Repo) ||
		e.IsPR ||
		e.Label != "kind/support" {
		return comment
	}

	str := &strings.Builder{}
	fmt.Fprintf(str, "%s\n\n", comment)
	sigLabels, wgLabels := []string{}, []string{}
	for _, l := range e.CurrentLabels {
		if sigRegex.MatchString(l.Name) {
			sigLabels = append(sigLabels, l.Name)
			continue
		}
		if wgRegex.MatchString(l.Name) {
			wgLabels = append(wgLabels, l.Name)
			continue
		}
	}
	sort.Strings(sigLabels)
	sort.Strings(wgLabels)

	fmt.Fprint(str, "Based on the labels present on this issue, here's where you can reach out next:\n")
	const contactURLFormat = "https://git.k8s.io/community/%s/README.md#contact"
	for _, l := range sigLabels {
		// Change label from some/label to some-label
		reFormattedLabel := strings.ReplaceAll(l, "/", "-")
		fmt.Fprintf(str, "- `[%s](", reFormattedLabel)
		fmt.Fprintf(str, contactURLFormat, reFormattedLabel)
		fmt.Fprint(str, ")`\n")
	}
	for _, l := range wgLabels {
		// Change label from some/label to some-label
		reFormattedLabel := strings.ReplaceAll(l, "/", "-")
		fmt.Fprintf(str, "- `[%s](", reFormattedLabel)
		fmt.Fprintf(str, contactURLFormat, reFormattedLabel)
		fmt.Fprint(str, ")`\n")
	}
	return str.String()
}

func isRepoKubernetesKubernetes(org, repo string) bool {
	const k8s = "kubernetes"
	return org == k8s && repo == k8s
}

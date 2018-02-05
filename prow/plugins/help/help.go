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

package help

import (
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "help"

var (
	helpLabel         = "help wanted"
	helpRe            = regexp.MustCompile(`(?mi)^/help\s*$`)
	helpRemoveRe      = regexp.MustCompile(`(?mi)^/remove-help\s*$`)
	helpGuidelinesURL = "https://git.k8s.io/community/contributors/devel/help-wanted.md"
	helpMsgPruneMatch = "This request has been marked as needing help from a contributor."
	helpMsg           = `
This request has been marked as needing help from a contributor.

Please ensure the request meets the requirements listed [here](` + helpGuidelinesURL + `).

If this request no longer meets these requirements, the label can be removed
by commenting with the ` + "`/remove-help`" + ` command.
`
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The help plugin provides commands that add or remove the '" + helpLabel + "' label from issues.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/[remove-]help",
		Description: "Applies or removes the '" + helpLabel + "' to an issue.",
		Featured:    false,
		WhoCanUse:   "Anyone can trigger this command on a PR.",
		Examples:    []string{"/help", "/remove-help"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	BotName() (string, error)
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

type commentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, pc.CommentPruner, &e)
}

func handle(gc githubClient, log *logrus.Entry, cp commentPruner, e *github.GenericCommentEvent) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	commentAuthor := e.User.Login

	// Determine if the issue has the help label
	labels, err := gc.GetIssueLabels(org, repo, e.Number)
	if err != nil {
		log.WithError(err).Errorf("Failed to get issue labels.")
	}
	hasHelp := github.HasLabel(helpLabel, labels)

	// If PR has help label and we're asking it to be removed, remove label
	if hasHelp && helpRemoveRe.MatchString(e.Body) {
		if err := gc.RemoveLabel(org, repo, e.Number, helpLabel); err != nil {
			log.WithError(err).Errorf("Github failed to remove the following label: %s", helpLabel)
		}
		botName, err := gc.BotName()
		if err != nil {
			log.WithError(err).Errorf("Failed to get bot name.")
		}
		cp.PruneComments(shouldPrune(log, botName))
		return nil
	}

	// If PR does not have the label and we're asking it to be added,
	// add the label
	if !hasHelp && helpRe.MatchString(e.Body) {
		if err := gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.IssueHTMLURL, commentAuthor, helpMsg)); err != nil {
			log.WithError(err).Errorf("Failed to create comment \"%s\".", helpMsg)
		}
		if err := gc.AddLabel(org, repo, e.Number, helpLabel); err != nil {
			log.WithError(err).Errorf("Github failed to add the following label: %s", helpLabel)
		}

		return nil
	}

	return nil
}

// shouldPrune finds comments left by this plugin.
func shouldPrune(log *logrus.Entry, botName string) func(github.IssueComment) bool {
	return func(comment github.IssueComment) bool {
		if comment.User.Login != botName {
			return false
		}
		return strings.Contains(comment.Body, helpMsgPruneMatch)
	}
}

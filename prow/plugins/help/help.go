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
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "help"

var (
	helpRe                      = regexp.MustCompile(`(?mi)^/help\s*$`)
	helpRemoveRe                = regexp.MustCompile(`(?mi)^/remove-help\s*$`)
	helpGoodFirstIssueRe        = regexp.MustCompile(`(?mi)^/good-first-issue\s*$`)
	helpGoodFirstIssueRemoveRe  = regexp.MustCompile(`(?mi)^/remove-good-first-issue\s*$`)
	helpMsgPruneMatch           = "This request has been marked as needing help from a contributor."
	goodFirstIssueMsgPruneMatch = "This request has been marked as suitable for new contributors."
)

type issueGuidelines struct {
	issueGuidelinesURL     string
	issueGuidelinesSummary string
}

func (ig issueGuidelines) helpMsg() string {
	if len(ig.issueGuidelinesSummary) != 0 {
		return ig.helpMsgWithGuidelineSummary()
	}
	return `
	This request has been marked as needing help from a contributor.

Please ensure the request meets the requirements listed [here](` + ig.issueGuidelinesURL + `).

If this request no longer meets these requirements, the label can be removed
by commenting with the ` + "`/remove-help`" + ` command.
`
}

func (ig issueGuidelines) helpMsgWithGuidelineSummary() string {
	return fmt.Sprintf(`
	This request has been marked as needing help from a contributor.

### Guidelines
%s

For more details on the requirements of such an issue, please see [here](%s) and ensure that they are met.

If this request no longer meets these requirements, the label can be removed
by commenting with the `+"`/remove-help`"+` command.
`, ig.issueGuidelinesSummary, ig.issueGuidelinesURL)
}

func (ig issueGuidelines) goodFirstIssueMsg() string {
	if len(ig.issueGuidelinesSummary) != 0 {
		return ig.goodFirstIssueMsgWithGuidelinesSummary()
	}
	return `
	This request has been marked as suitable for new contributors.

Please ensure the request meets the requirements listed [here](` + ig.issueGuidelinesURL + "#good-first-issue" + `).

If this request no longer meets these requirements, the label can be removed
by commenting with the ` + "`/remove-good-first-issue`" + ` command.
`
}

func (ig issueGuidelines) goodFirstIssueMsgWithGuidelinesSummary() string {
	return fmt.Sprintf(`
	This request has been marked as suitable for new contributors.

### Guidelines
%s

For more details on the requirements of such an issue, please see [here](%s#good-first-issue) and ensure that they are met.

If this request no longer meets these requirements, the label can be removed
by commenting with the `+"`/remove-good-first-issue`"+` command.
`, ig.issueGuidelinesSummary, ig.issueGuidelinesURL)
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The help plugin provides commands that add or remove the '" + labels.Help + "' and the '" + labels.GoodFirstIssue + "' labels from issues.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/[remove-](help|good-first-issue)",
		Description: "Applies or removes the '" + labels.Help + "' and '" + labels.GoodFirstIssue + "' labels to an issue.",
		Featured:    false,
		WhoCanUse:   "Anyone can trigger this command on a PR.",
		Examples:    []string{"/help", "/remove-help", "/good-first-issue", "/remove-good-first-issue"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	BotUserChecker() (func(candidate string) bool, error)
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

type commentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	cfg := pc.PluginConfig
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	ig := issueGuidelines{
		issueGuidelinesURL:     cfg.Help.HelpGuidelinesURL,
		issueGuidelinesSummary: cfg.Help.HelpGuidelinesSummary,
	}
	return handle(pc.GitHubClient, pc.Logger, cp, &e, ig)
}

func handle(gc githubClient, log *logrus.Entry, cp commentPruner, e *github.GenericCommentEvent, ig issueGuidelines) error {
	// Only consider open issues and new comments.
	if e.IsPR || e.IssueState != "open" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	commentAuthor := e.User.Login

	// Determine if the issue has the help and the good-first-issue label
	issueLabels, err := gc.GetIssueLabels(org, repo, e.Number)
	if err != nil {
		log.WithError(err).Errorf("Failed to get issue labels.")
	}
	hasHelp := github.HasLabel(labels.Help, issueLabels)
	hasGoodFirstIssue := github.HasLabel(labels.GoodFirstIssue, issueLabels)

	// If PR has help label and we're asking for it to be removed, remove label
	if hasHelp && helpRemoveRe.MatchString(e.Body) {
		if err := gc.RemoveLabel(org, repo, e.Number, labels.Help); err != nil {
			log.WithError(err).Errorf("GitHub failed to remove the following label: %s", labels.Help)
		}

		botUserChecker, err := gc.BotUserChecker()
		if err != nil {
			log.WithError(err).Errorf("Failed to get bot name.")
		}
		cp.PruneComments(shouldPrune(log, botUserChecker, helpMsgPruneMatch))

		// if it has the good-first-issue label, remove it too
		if hasGoodFirstIssue {
			if err := gc.RemoveLabel(org, repo, e.Number, labels.GoodFirstIssue); err != nil {
				log.WithError(err).Errorf("GitHub failed to remove the following label: %s", labels.GoodFirstIssue)
			}
			cp.PruneComments(shouldPrune(log, botUserChecker, goodFirstIssueMsgPruneMatch))
		}

		return nil
	}

	// If PR does not have the good-first-issue label and we are asking for it to be added,
	// add both the good-first-issue and help labels
	if !hasGoodFirstIssue && helpGoodFirstIssueRe.MatchString(e.Body) {
		if err := gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.IssueHTMLURL, commentAuthor, ig.goodFirstIssueMsg())); err != nil {
			log.WithError(err).Errorf("Failed to create comment \"%s\".", ig.goodFirstIssueMsg())
		}

		if err := gc.AddLabel(org, repo, e.Number, labels.GoodFirstIssue); err != nil {
			log.WithError(err).Errorf("GitHub failed to add the following label: %s", labels.GoodFirstIssue)
		}

		if !hasHelp {
			if err := gc.AddLabel(org, repo, e.Number, labels.Help); err != nil {
				log.WithError(err).Errorf("GitHub failed to add the following label: %s", labels.Help)
			}
		}

		return nil
	}

	// If PR does not have the help label and we're asking it to be added,
	// add the label
	if !hasHelp && helpRe.MatchString(e.Body) {
		if err := gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.IssueHTMLURL, commentAuthor, ig.helpMsg())); err != nil {
			log.WithError(err).Errorf("Failed to create comment \"%s\".", ig.helpMsg())
		}
		if err := gc.AddLabel(org, repo, e.Number, labels.Help); err != nil {
			log.WithError(err).Errorf("GitHub failed to add the following label: %s", labels.Help)
		}

		return nil
	}

	// If PR has good-first-issue label and we are asking for it to be removed,
	// remove just the good-first-issue label
	if hasGoodFirstIssue && helpGoodFirstIssueRemoveRe.MatchString(e.Body) {
		if err := gc.RemoveLabel(org, repo, e.Number, labels.GoodFirstIssue); err != nil {
			log.WithError(err).Errorf("GitHub failed to remove the following label: %s", labels.GoodFirstIssue)
		}

		botUserChecker, err := gc.BotUserChecker()
		if err != nil {
			log.WithError(err).Errorf("Failed to get bot name.")
		}
		cp.PruneComments(shouldPrune(log, botUserChecker, goodFirstIssueMsgPruneMatch))

		return nil
	}

	return nil
}

// shouldPrune finds comments left by this plugin.
func shouldPrune(log *logrus.Entry, isBot func(string) bool, msgPruneMatch string) func(github.IssueComment) bool {
	return func(comment github.IssueComment) bool {
		if !isBot(comment.User.Login) {
			return false
		}
		return strings.Contains(comment.Body, msgPruneMatch)
	}
}

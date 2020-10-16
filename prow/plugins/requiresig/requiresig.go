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

package requiresig

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"

	"github.com/sirupsen/logrus"
)

var (
	labelPrefixes = []string{"sig/", "committee/", "wg/"}

	sigCommandRe = regexp.MustCompile(`(?m)^/sig\s*(.*)$`)
)

const (
	pluginName = "require-sig"

	needsSIGMessage = "There are no sig labels on this issue. Please add a sig label."
	needsSIGDetails = `A sig label can be added by either:

1. mentioning a sig: ` + "`@kubernetes/sig-<group-name>-<group-suffix>`" + `
    e.g., ` + "`@kubernetes/sig-contributor-experience-<group-suffix>`" + ` to notify the contributor experience sig, OR

2. specifying the label manually: ` + "`/sig <group-name>`" + `
    e.g., ` + "`/sig scalability`" + ` to apply the ` + "`sig/scalability`" + ` label

Note: Method 1 will trigger an email to the group. See the [group list](https://git.k8s.io/community/sig-list.md).
The ` + "`<group-suffix>`" + ` in method 1 has to be replaced with one of these: _**bugs, feature-requests, pr-reviews, test-failures, proposals**_`
)

type githubClient interface {
	BotName() (string, error)
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	CreateComment(org, repo string, number int, content string) error
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	DeleteComment(org, repo string, id int) error
}

type commentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

func init() {
	plugins.RegisterIssueHandler(pluginName, handleIssue, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	url := config.RequireSIG.GroupListURL
	if url == "" {
		url = "<no url provided>"
	}
	// Only the 'Description' and 'Config' fields are necessary because this plugin does not react
	// to any commands.
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		RequireSIG: plugins.RequireSIG{
			GroupListURL: "https://github.com/kubernetes/community/blob/master/sig-list.md",
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	return &pluginhelp.PluginHelp{
			Description: fmt.Sprintf(
				`When a new issue is opened the require-sig plugin adds the %q label and leaves a comment requesting that a SIG (Special Interest Group) label be added to the issue. SIG labels are labels that have one of the following prefixes: %q.
<br>Once a SIG label has been added to an issue, this plugin removes the %q label and deletes the comment it made previously.`,
				labels.NeedsSig,
				labelPrefixes,
				labels.NeedsSig,
			),
			Config: map[string]string{
				"": fmt.Sprintf("The comment the plugin creates includes this link to a list of the existing groups: %s", url),
			},
			Snippet: yamlSnippet,
		},
		nil
}

func handleIssue(pc plugins.Agent, ie github.IssueEvent) error {
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}
	return handle(pc.Logger, pc.GitHubClient, cp, &ie, pc.PluginConfig.SigMention.Re)
}

func isSigLabel(label string) bool {
	for i := range labelPrefixes {
		if strings.HasPrefix(label, labelPrefixes[i]) {
			return true
		}
	}
	return false
}

func hasSigLabel(labels []github.Label) bool {
	for i := range labels {
		if isSigLabel(labels[i].Name) {
			return true
		}
	}
	return false
}

func shouldReact(mentionRe *regexp.Regexp, ie *github.IssueEvent) bool {
	// Ignore PRs and closed issues.
	if ie.Issue.IsPullRequest() || ie.Issue.State == "closed" {
		return false
	}

	switch ie.Action {
	case github.IssueActionOpened:
		// Don't react if the new issue has a /sig command or sig team mention.
		return !mentionRe.MatchString(ie.Issue.Body) && !sigCommandRe.MatchString(ie.Issue.Body)
	case github.IssueActionLabeled, github.IssueActionUnlabeled:
		// Only react to (un)label events for sig labels.
		return isSigLabel(ie.Label.Name)
	default:
		return false
	}
}

// handle is the workhorse notifying issue owner to add a sig label if there is none
// The algorithm:
// (1) return if this is not an opened, labelled, or unlabelled event or if the issue is closed.
// (2) find if the issue has a sig label
// (3) find if the issue has a needs-sig label
// (4) if the issue has both the sig and needs-sig labels, remove the needs-sig label and delete the comment.
// (5) if the issue has none of the labels, add the needs-sig label and comment
// (6) if the issue has only the sig label, do nothing
// (7) if the issue has only the needs-sig label, do nothing
func handle(log *logrus.Entry, ghc githubClient, cp commentPruner, ie *github.IssueEvent, mentionRe *regexp.Regexp) error {
	// Ignore PRs, closed issues, and events that aren't new issues or sig label
	// changes.
	if !shouldReact(mentionRe, ie) {
		return nil
	}

	org := ie.Repo.Owner.Login
	repo := ie.Repo.Name
	number := ie.Issue.Number

	hasSigLabel := hasSigLabel(ie.Issue.Labels)
	hasNeedsSigLabel := github.HasLabel(labels.NeedsSig, ie.Issue.Labels)

	if hasSigLabel && hasNeedsSigLabel {
		if err := ghc.RemoveLabel(org, repo, number, labels.NeedsSig); err != nil {
			log.WithError(err).Errorf("Failed to remove %s label.", labels.NeedsSig)
		}
		botName, err := ghc.BotName()
		if err != nil {
			return fmt.Errorf("error getting bot name: %v", err)
		}
		cp.PruneComments(shouldPrune(log, botName))
	} else if !hasSigLabel && !hasNeedsSigLabel {
		if err := ghc.AddLabel(org, repo, number, labels.NeedsSig); err != nil {
			log.WithError(err).Errorf("Failed to add %s label.", labels.NeedsSig)
		}
		msg := plugins.FormatResponse(ie.Issue.User.Login, needsSIGMessage, needsSIGDetails)
		if err := ghc.CreateComment(org, repo, number, msg); err != nil {
			log.WithError(err).Error("Failed to create comment.")
		}
	}
	return nil
}

// shouldPrune finds comments left by this plugin.
func shouldPrune(log *logrus.Entry, botName string) func(github.IssueComment) bool {
	return func(comment github.IssueComment) bool {
		if comment.User.Login != botName {
			return false
		}
		return strings.Contains(comment.Body, needsSIGMessage)
	}
}

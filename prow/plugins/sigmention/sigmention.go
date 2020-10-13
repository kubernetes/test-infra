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

// Package sigmention recognize SIG '@' mentions and adds 'sig/*' and 'kind/*' labels as appropriate.
// SIG mentions are also reitierated by the bot if the user who made the mention is not a member in
// order for the mention to trigger a notification for the github team.
package sigmention

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

const pluginName = "sigmention"

var (
	chatBack = "Reiterating the mentions to trigger a notification: \n%v\n"

	kindMap = map[string]string{
		"bugs":             labels.Bug,
		"feature-requests": "kind/feature",
		"api-reviews":      "kind/api-change",
		"proposals":        "kind/design",
	}
)

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	IsMember(org, user string) (bool, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetRepoLabels(owner, repo string) ([]github.Label, error)
	BotName() (string, error)
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		SigMention: plugins.SigMention{
			Regexp: "(?m)@kubernetes/sig-([\\w-]*)-(misc|test-failures|bugs|feature-requests|proposals|pr-reviews|api-reviews)",
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	return &pluginhelp.PluginHelp{
			Description: `The sigmention plugin responds to SIG (Special Interest Group) GitHub team mentions like '@kubernetes/sig-testing-bugs'. The plugin responds in two ways:
<ol><li> The appropriate 'sig/*' and 'kind/*' labels are applied to the issue or pull request. In this case 'sig/testing' and 'kind/bug'.</li>
<li> If the user who mentioned the GitHub team is not a member of the organization that owns the repository the bot will create a comment that repeats the mention. This is necessary because non-member mentions do not trigger GitHub notifications.</li></ol>`,
			Config: map[string]string{
				"": fmt.Sprintf("Labels added by the plugin are triggered by mentions of GitHub teams matching the following regexp:\n%s", config.SigMention.Regexp),
			},
			Snippet: yamlSnippet,
		},
		nil
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e, pc.PluginConfig.SigMention.Re)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, re *regexp.Regexp) error {
	// Ignore bot comments and comments that aren't new.
	botName, err := gc.BotName()
	if err != nil {
		return err
	}
	if e.User.Login == botName {
		return nil
	}
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}

	sigMatches := re.FindAllStringSubmatch(e.Body, -1)
	if len(sigMatches) == 0 {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name

	labels, err := gc.GetIssueLabels(org, repo, e.Number)
	if err != nil {
		return err
	}
	repoLabels, err := gc.GetRepoLabels(org, repo)
	if err != nil {
		return err
	}
	RepoLabelsExisting := map[string]string{}
	for _, l := range repoLabels {
		RepoLabelsExisting[strings.ToLower(l.Name)] = l.Name
	}

	var nonexistent, toRepeat []string
	for _, sigMatch := range sigMatches {
		sigLabel := strings.ToLower("sig" + "/" + sigMatch[1])
		sigLabel, ok := RepoLabelsExisting[sigLabel]
		if !ok {
			nonexistent = append(nonexistent, "sig/"+sigMatch[1])
			continue
		}
		if !github.HasLabel(sigLabel, labels) {
			if err := gc.AddLabel(org, repo, e.Number, sigLabel); err != nil {
				log.WithError(err).Errorf("GitHub failed to add the following label: %s", sigLabel)
			}
		}

		if len(sigMatch) > 2 {
			if kindLabel, ok := kindMap[sigMatch[2]]; ok && !github.HasLabel(kindLabel, labels) {
				if err := gc.AddLabel(org, repo, e.Number, kindLabel); err != nil {
					log.WithError(err).Errorf("GitHub failed to add the following label: %s", kindLabel)
				}
			}
		}

		toRepeat = append(toRepeat, sigMatch[0])
	}
	//TODO(grodrigues3): Once labels are standardized, make this reply with a comment.
	if len(nonexistent) > 0 {
		log.Infof("Nonexistent labels: %v", nonexistent)
	}

	isMember, err := gc.IsMember(org, e.User.Login)
	if err != nil {
		log.WithError(err).Errorf("Error from IsMember(%q of org %q).", e.User.Login, org)
	}
	if isMember || len(toRepeat) == 0 {
		return nil
	}

	msg := fmt.Sprintf(chatBack, strings.Join(toRepeat, ", "))
	return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
}

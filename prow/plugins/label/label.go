/*
Copyright 2016 The Kubernetes Authors.

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

package label

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "label"

var (
	labelRegex              = regexp.MustCompile(`(?m)^/(area|priority|kind)\s*(.*)$`)
	removeLabelRegex        = regexp.MustCompile(`(?m)^/remove-(area|priority|kind)\s*(.*)|(do-not-merge)\s*$`)
	sigMatcher              = regexp.MustCompile(`(?m)@sig-([\w-]*)-(?:misc|test-failures|bugs|feature-requests|proposals|pr-reviews|api-reviews)`)
	nonExistentLabel        = "These labels do not exist in this repository: `%v`"
	nonExistentLabelOnIssue = "Those labels are not set on the issue: `%v`"
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	IsMember(org, user string) (bool, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetLabels(owner, repo string) ([]github.Label, error)
	BotName() string
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, ic)
}

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
	commenter := ic.Comment.User.Login
	owner := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	// only parse newly created comments and if non bot author
	if commenter == gc.BotName() || ic.Action != "created" {
		return nil
	}

	labelMatches := labelRegex.FindAllStringSubmatch(ic.Comment.Body, -1)
	removeLabelMatches := removeLabelRegex.FindAllStringSubmatch(ic.Comment.Body, -1)
	sigMatches := sigMatcher.FindAllStringSubmatch(ic.Comment.Body, -1)
	if len(labelMatches) == 0 && len(sigMatches) == 0 && len(removeLabelMatches) == 0 {
		return nil
	}

	labels, err := gc.GetLabels(owner, repo)
	if err != nil {
		return err
	}

	existingLabels := map[string]string{}
	for _, l := range labels {
		existingLabels[strings.ToLower(l.Name)] = l.Name
	}
	var (
		nonexistent         []string
		noSuchLabelsOnIssue []string
	)

	for _, match := range labelMatches {
		for _, newLabel := range strings.Split(match[0], " ")[1:] {
			newLabel = strings.ToLower(match[1] + "/" + strings.TrimSpace(newLabel))
			if ic.Issue.HasLabel(newLabel) {
				continue
			}
			if _, ok := existingLabels[newLabel]; !ok {
				nonexistent = append(nonexistent, newLabel)
				continue
			}
			if err := gc.AddLabel(owner, repo, number, existingLabels[newLabel]); err != nil {
				log.WithError(err).Errorf("Github failed to add the following label: %s", newLabel)
			}
		}
	}

	for _, sigMatch := range sigMatches {
		sigLabel := strings.ToLower("sig" + "/" + strings.TrimSpace(sigMatch[1]))
		if ic.Issue.HasLabel(sigLabel) {
			continue
		}
		if _, ok := existingLabels[sigLabel]; !ok {
			nonexistent = append(nonexistent, sigLabel)
			continue
		}
		if err := gc.AddLabel(owner, repo, number, sigLabel); err != nil {
			log.WithError(err).Errorf("Github failed to add the following label: %s", sigLabel)
		}
	}

	labelsToRemove := []string{}
	for _, match := range removeLabelMatches {
		if len(match) > 0 && match[0] == "do-not-merge" {
			labelsToRemove = append(labelsToRemove, "do-not-merge")
			continue
		}
		for _, removeLabel := range strings.Split(match[0], " ")[1:] {
			removeLabel = strings.ToLower(match[1] + "/" + strings.TrimSpace(removeLabel))
			labelsToRemove = append(labelsToRemove, removeLabel)
		}
	}

	for _, labelToRemove := range labelsToRemove {
		if !ic.Issue.HasLabel(labelToRemove) {
			noSuchLabelsOnIssue = append(noSuchLabelsOnIssue, labelToRemove)
			continue
		}

		if _, ok := existingLabels[labelToRemove]; !ok {
			nonexistent = append(nonexistent, labelToRemove)
			continue
		}

		if err := gc.RemoveLabel(owner, repo, number, labelToRemove); err != nil {
			log.WithError(err).Errorf("Github failed to remove the following label: %s", labelToRemove)
		}
	}

	if len(nonexistent) > 0 {
		msg := fmt.Sprintf(nonExistentLabel, strings.Join(nonexistent, ", "))
		if err := gc.CreateComment(owner, repo, number, plugins.FormatICResponse(ic.Comment, msg)); err != nil {
			log.WithError(err).Errorf("Could not create comment \"%s\".", msg)
		}
	}

	if len(noSuchLabelsOnIssue) > 0 {
		msg := fmt.Sprintf(nonExistentLabelOnIssue, strings.Join(noSuchLabelsOnIssue, ", "))
		if err := gc.CreateComment(owner, repo, number, plugins.FormatICResponse(ic.Comment, msg)); err != nil {
			log.WithError(err).Errorf("Could not create comment \"%s\".", msg)
		}
	}

	return nil
}

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

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "label"

var (
	defaultLabels           = []string{"kind", "priority", "area"}
	labelRegex              = regexp.MustCompile(`(?m)^/(area|committee|kind|language|priority|sig|triage|wg)\s*(.*)$`)
	removeLabelRegex        = regexp.MustCompile(`(?m)^/remove-(area|committee|kind|language|priority|sig|triage|wg)\s*(.*)$`)
	customLabelRegex        = regexp.MustCompile(`(?m)^/label\s*(.*)$`)
	customRemoveLabelRegex  = regexp.MustCompile(`(?m)^/remove-label\s*(.*)$`)
	nonExistentLabelOnIssue = "Those labels are not set on the issue: `%v`"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func configString(labels []string) string {
	var formattedLabels []string
	for _, label := range labels {
		formattedLabels = append(formattedLabels, fmt.Sprintf(`"%s/*"`, label))
	}
	return fmt.Sprintf("The label plugin will work on %s and %s labels.", strings.Join(formattedLabels[:len(formattedLabels)-1], ", "), formattedLabels[len(formattedLabels)-1])
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	labels := []string{}
	labels = append(labels, defaultLabels...)
	labels = append(labels, config.Label.AdditionalLabels...)
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The label plugin provides commands that add or remove certain types of labels. Labels of the following types can be manipulated: 'area/*', 'committee/*', 'kind/*', 'language/*', 'priority/*', 'sig/*', 'triage/*', and 'wg/*'. More labels can be configured to be used via the /label command.",
		Config: map[string]string{
			"": configString(labels),
		},
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/[remove-](area|committee|kind|language|priority|sig|triage|wg|label) <target>",
		Description: "Applies or removes a label from one of the recognized types of labels.",
		Featured:    false,
		WhoCanUse:   "Anyone can trigger this command on a PR.",
		Examples:    []string{"/kind bug", "/remove-area prow", "/sig testing", "/language zh"},
	})
	return pluginHelp, nil
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, pc.PluginConfig.Label.AdditionalLabels, &e)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetRepoLabels(owner, repo string) ([]github.Label, error)
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

// Get Labels from Regexp matches
func getLabelsFromREMatches(matches [][]string) (labels []string) {
	for _, match := range matches {
		for _, label := range strings.Split(match[0], " ")[1:] {
			label = strings.ToLower(match[1] + "/" + strings.TrimSpace(label))
			labels = append(labels, label)
		}
	}
	return
}

// getLabelsFromGenericMatches returns label matches with extra labels if those
// have been configured in the plugin config.
func getLabelsFromGenericMatches(matches [][]string, additionalLabels []string) []string {
	if len(additionalLabels) == 0 {
		return nil
	}
	var labels []string
	for _, match := range matches {
		parts := strings.Split(match[0], " ")
		if ((parts[0] != "/label") && (parts[0] != "/remove-label")) || len(parts) != 2 {
			continue
		}
		for _, l := range additionalLabels {
			if l == parts[1] {
				labels = append(labels, parts[1])
			}
		}
	}
	return labels
}

func handle(gc githubClient, log *logrus.Entry, additionalLabels []string, e *github.GenericCommentEvent) error {
	labelMatches := labelRegex.FindAllStringSubmatch(e.Body, -1)
	removeLabelMatches := removeLabelRegex.FindAllStringSubmatch(e.Body, -1)
	customLabelMatches := customLabelRegex.FindAllStringSubmatch(e.Body, -1)
	customRemoveLabelMatches := customRemoveLabelRegex.FindAllStringSubmatch(e.Body, -1)
	if len(labelMatches) == 0 && len(removeLabelMatches) == 0 && len(customLabelMatches) == 0 && len(customRemoveLabelMatches) == 0 {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name

	repoLabels, err := gc.GetRepoLabels(org, repo)
	if err != nil {
		return err
	}
	labels, err := gc.GetIssueLabels(org, repo, e.Number)
	if err != nil {
		return err
	}

	RepoLabelsExisting := map[string]string{}
	for _, l := range repoLabels {
		RepoLabelsExisting[strings.ToLower(l.Name)] = l.Name
	}
	var (
		nonexistent         []string
		noSuchLabelsOnIssue []string
		labelsToAdd         []string
		labelsToRemove      []string
	)

	// Get labels to add and labels to remove from regexp matches
	labelsToAdd = append(getLabelsFromREMatches(labelMatches), getLabelsFromGenericMatches(customLabelMatches, additionalLabels)...)
	labelsToRemove = append(getLabelsFromREMatches(removeLabelMatches), getLabelsFromGenericMatches(customRemoveLabelMatches, additionalLabels)...)

	// Add labels
	for _, labelToAdd := range labelsToAdd {
		if github.HasLabel(labelToAdd, labels) {
			continue
		}

		if _, ok := RepoLabelsExisting[labelToAdd]; !ok {
			nonexistent = append(nonexistent, labelToAdd)
			continue
		}

		if err := gc.AddLabel(org, repo, e.Number, RepoLabelsExisting[labelToAdd]); err != nil {
			log.WithError(err).Errorf("GitHub failed to add the following label: %s", labelToAdd)
		}
	}

	// Remove labels
	for _, labelToRemove := range labelsToRemove {
		if !github.HasLabel(labelToRemove, labels) {
			noSuchLabelsOnIssue = append(noSuchLabelsOnIssue, labelToRemove)
			continue
		}

		if _, ok := RepoLabelsExisting[labelToRemove]; !ok {
			nonexistent = append(nonexistent, labelToRemove)
			continue
		}

		if err := gc.RemoveLabel(org, repo, e.Number, labelToRemove); err != nil {
			log.WithError(err).Errorf("GitHub failed to remove the following label: %s", labelToRemove)
		}
	}

	//TODO(grodrigues3): Once labels are standardized, make this reply with a comment.
	if len(nonexistent) > 0 {
		log.Infof("Nonexistent labels: %v", nonexistent)
	}

	// Tried to remove Labels that were not present on the Issue
	if len(noSuchLabelsOnIssue) > 0 {
		msg := fmt.Sprintf(nonExistentLabelOnIssue, strings.Join(noSuchLabelsOnIssue, ", "))
		return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
	}

	return nil
}

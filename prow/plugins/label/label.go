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
	labelRegexp             = `(?m)^/(%s)\s*(.*)$`
	removeLabelRegexp       = `(?m)^/remove-(%s)\s*(.*)$`
	customLabelRegex        = regexp.MustCompile(`(?m)^/label\s*(.*)$`)
	customRemoveLabelRegex  = regexp.MustCompile(`(?m)^/remove-label\s*(.*)$`)
	nonExistentLabelOnIssue = "Those labels are not set on the issue: `%v`"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	labelConfig := map[string]string{}
	for _, repo := range enabledRepos {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repo in enabledRepos: %q", repo)
		}
		opts := optionsForRepo(config, parts[0], parts[1])

		var prefixConfigMsg, additionalLabelsConfigMsg string
		if opts.Prefixes != nil {
			prefixConfigMsg = fmt.Sprintf("The label plugin also includes commands based on %v prefixes.", opts.Prefixes)
		}
		if opts.AdditionalLabels != nil {
			additionalLabelsConfigMsg = fmt.Sprintf("%v labels can be used with the `/[remove-]label` command.", opts.AdditionalLabels)
		}
		labelConfig[repo] = prefixConfigMsg + additionalLabelsConfigMsg
	}

	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The label plugin provides commands that add or remove certain types of labels. Labels of the following types can be manipulated: 'area/*', 'committee/*', 'kind/*', 'priority/*', 'sig/*', 'triage/*', and 'wg/*'. More labels can be configured to be used via the /label command.",
		Config:      labelConfig,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/[remove-](area|committee|kind|priority|sig|triage|wg|label) <target>",
		Description: "Applies or removes a label from one of the recognized types of labels.",
		Featured:    false,
		WhoCanUse:   "Anyone can trigger this command on a PR.",
		Examples:    []string{"/kind bug", "/remove-area prow", "/sig testing"},
	})
	return pluginHelp, nil
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	opts := optionsForRepo(pc.PluginConfig, e.Repo.Owner.Login, e.Repo.Name)

	var additionalLabels, prefixes = []string{}, []string{}
	if opts.AdditionalLabels != nil {
		additionalLabels = opts.AdditionalLabels
	}
	if opts.Prefixes != nil {
		prefixes = opts.Prefixes
	}
	return handle(pc.GitHubClient, pc.Logger, additionalLabels, prefixes, &e)
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

// optionsForRepo gets the plugins.Label struct that is applicable to the indicated repo.
func optionsForRepo(config *plugins.Configuration, org, repo string) *plugins.Label {
	fullName := fmt.Sprintf("%s/%s", org, repo)
	for i := range config.Label {
		if !strInSlice(org, config.Label[i].Repos) && !strInSlice(fullName, config.Label[i].Repos) {
			continue
		}
		return &config.Label[i]
	}
	return &plugins.Label{}
}
func strInSlice(str string, slice []string) bool {
	for _, elem := range slice {
		if elem == str {
			return true
		}
	}
	return false
}

func handle(gc githubClient, log *logrus.Entry, additionalLabels, prefixes []string, e *github.GenericCommentEvent) error {
	// arrange prefixes in the format "area|kind|priority|..."
	// so that they can be used to create labelRegex and removeLabelRegex
	var labelPrefixes string
	for k, prefix := range prefixes {
		if k == 0 {
			labelPrefixes = prefix
			continue
		}
		labelPrefixes = labelPrefixes + "|" + prefix
	}

	labelRegex, err := regexp.Compile(fmt.Sprintf(labelRegexp, labelPrefixes))
	if err != nil {
		return err
	}
	removeLabelRegex, err := regexp.Compile(fmt.Sprintf(removeLabelRegexp, labelPrefixes))
	if err != nil {
		return err
	}

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

	existingLabels := map[string]string{}
	for _, l := range repoLabels {
		existingLabels[strings.ToLower(l.Name)] = l.Name
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

		if _, ok := existingLabels[labelToAdd]; !ok {
			nonexistent = append(nonexistent, labelToAdd)
			continue
		}

		if err := gc.AddLabel(org, repo, e.Number, existingLabels[labelToAdd]); err != nil {
			log.WithError(err).Errorf("Github failed to add the following label: %s", labelToAdd)
		}
	}

	// Remove labels
	for _, labelToRemove := range labelsToRemove {
		if !github.HasLabel(labelToRemove, labels) {
			noSuchLabelsOnIssue = append(noSuchLabelsOnIssue, labelToRemove)
			continue
		}

		if _, ok := existingLabels[labelToRemove]; !ok {
			nonexistent = append(nonexistent, labelToRemove)
			continue
		}

		if err := gc.RemoveLabel(org, repo, e.Number, labelToRemove); err != nil {
			log.WithError(err).Errorf("Github failed to remove the following label: %s", labelToRemove)
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

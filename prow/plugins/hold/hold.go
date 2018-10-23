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

// Package hold contains a plugin which will allow users to label their
// own pull requests as not ready or ready for merge. The submit queue
// will honor the label to ensure pull requests do not merge when it is
// applied.
package hold

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const PluginName = "hold"

var (
	labelRe       = regexp.MustCompile(`(?mi)^/hold\s*$`)
	labelCancelRe = regexp.MustCompile(`(?mi)^/hold cancel\s*$`)
)

type hasLabelFunc func(label string, issueLabels []github.Label) bool

func init() {
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The hold plugin allows anyone to add or remove the '" + labels.Hold + "' Label from a pull request in order to temporarily prevent the PR from merging without withholding approval.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/hold [cancel]",
		Description: "Adds or removes the `" + labels.Hold + "` Label which is used to indicate that the PR should not be automatically merged.",
		Featured:    false,
		WhoCanUse:   "Anyone can use the /hold command to add or remove the '" + labels.Hold + "' Label.",
		Examples:    []string{"/hold", "/hold cancel"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	hasLabel := func(label string, labels []github.Label) bool {
		return github.HasLabel(label, labels)
	}
	return handle(pc.GitHubClient, pc.Logger, &e, hasLabel)
}

// handle drives the pull request to the desired state. If any user adds
// a /hold directive, we want to add a label if one does not already exist.
// If they add /hold cancel, we want to remove the label if it exists.
func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, f hasLabelFunc) error {
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	needsLabel := false
	if labelRe.MatchString(e.Body) {
		needsLabel = true
	} else if labelCancelRe.MatchString(e.Body) {
		needsLabel = false
	} else {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	issueLabels, err := gc.GetIssueLabels(org, repo, e.Number)
	if err != nil {
		return fmt.Errorf("failed to get the labels on %s/%s#%d: %v", org, repo, e.Number, err)
	}

	hasLabel := f(labels.Hold, issueLabels)
	if hasLabel && !needsLabel {
		log.Infof("Removing %q Label for %s/%s#%d", labels.Hold, org, repo, e.Number)
		return gc.RemoveLabel(org, repo, e.Number, labels.Hold)
	} else if !hasLabel && needsLabel {
		log.Infof("Adding %q Label for %s/%s#%d", labels.Hold, org, repo, e.Number)
		return gc.AddLabel(org, repo, e.Number, labels.Hold)
	}
	return nil
}

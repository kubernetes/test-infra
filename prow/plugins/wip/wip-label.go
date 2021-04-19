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

// Package wip will label a PR a work-in-progress if the author provides
// a prefix to their pull request title to the same effect. The submit-
// queue will not merge pull requests with the work-in-progress label.
// The label will be removed when the title changes to no longer begin
// with the prefix.
package wip

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName = "wip"
)

var (
	titleRegex = regexp.MustCompile(`(?i)^\W?WIP\W`)
)

type event struct {
	org      string
	repo     string
	number   int
	title    string
	draft    bool
	label    string
	hasLabel bool
}

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	// Only the Description field is specified because this plugin is not triggered with commands and is not configurable.
	return &pluginhelp.PluginHelp{
			Description: "The wip (Work In Progress) plugin applies a work in progress label (default '" + labels.WorkInProgress + "') to pull requests whose title starts with 'WIP' or are in the 'draft' stage, and removes it from pull requests when they remove the title prefix or become ready for review. The label is typically used to block a pull request from merging while it is still in progress.",
		},
		nil
}

// Strict subset of github.Client methods.
type githubClient interface {
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

func handlePullRequest(pc plugins.Agent, pe github.PullRequestEvent) error {
	// These are the only actions indicating the PR title may have changed.
	if pe.Action != github.PullRequestActionOpened &&
		pe.Action != github.PullRequestActionReopened &&
		pe.Action != github.PullRequestActionEdited &&
		pe.Action != github.PullRequestActionReadyForReview &&
		pe.Action != github.PullRequestActionConvertedToDraft {
		return nil
	}

	var (
		org    = pe.PullRequest.Base.Repo.Owner.Login
		repo   = pe.PullRequest.Base.Repo.Name
		number = pe.PullRequest.Number
		title  = pe.PullRequest.Title
		draft  = pe.PullRequest.Draft
		label  = optionsForRepo(pc.PluginConfig, org, repo).Label
	)

	currentLabels, err := pc.GitHubClient.GetIssueLabels(org, repo, number)
	if err != nil {
		return fmt.Errorf("could not get labels for PR %s/%s:%d in WIP plugin: %v", org, repo, number, err)
	}
	hasLabel := false
	for _, l := range currentLabels {
		if l.Name == label {
			hasLabel = true
		}
	}
	e := &event{
		org:      org,
		repo:     repo,
		number:   number,
		title:    title,
		draft:    draft,
		label:    label,
		hasLabel: hasLabel,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

// handle interacts with GitHub to drive the pull request to the
// proper state by adding and removing comments and labels. If a
// PR has a WIP prefix, it needs an explanatory comment and label.
// Otherwise, neither should be present.
func handle(gc githubClient, le *logrus.Entry, e *event) error {
	needsLabel := e.draft || titleRegex.MatchString(e.title)

	if needsLabel && !e.hasLabel {
		if err := gc.AddLabel(e.org, e.repo, e.number, e.label); err != nil {
			le.Warnf("error while adding Label %q: %v", e.label, err)
			return err
		}
	} else if !needsLabel && e.hasLabel {
		if err := gc.RemoveLabel(e.org, e.repo, e.number, e.label); err != nil {
			le.Warnf("error while removing Label %q: %v", e.label, err)
			return err
		}
	}
	return nil
}

// optionsForRepo gets the plugins.Wip struct that is applicable to the indicated repo.
func optionsForRepo(config *plugins.Configuration, org, repo string) *plugins.Wip {
	fullName := fmt.Sprintf("%s/%s", org, repo)

	// First search for repo config
	for _, c := range config.Wip {
		if !sets.NewString(c.Repos...).Has(fullName) {
			continue
		}
		return &c
	}

	// If you don't find anything, loop again looking for an org config
	for _, c := range config.Wip {
		if !sets.NewString(c.Repos...).Has(org) {
			continue
		}
		return &c
	}

	// Return an empty config, and default to defaultWelcomeMessage
	return &plugins.Wip{
		Label: labels.WorkInProgress,
	}
}

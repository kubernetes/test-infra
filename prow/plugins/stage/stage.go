/*
Copyright 2018 The Kubernetes Authors.

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

// Package stage defines a Prow plugin that defines the stage of
// the issue in the features process. Eg: alpha, beta, stable.
package stage

import (
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	stageAlpha  = "stage/alpha"
	stageBeta   = "stage/beta"
	stageStable = "stage/stable"
	stageLabels = []string{stageAlpha, stageBeta, stageStable}
	stageRe     = regexp.MustCompile(`(?mi)^/(remove-)?stage (alpha|beta|stable)\s*$`)
)

func init() {
	plugins.RegisterGenericCommentHandler("stage", stageHandleGenericComment, help)
}

func help(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "Label the stage of an issue as alpha/beta/stable",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/[remove-]stage <alpha|beta|stable>",
		Description: "Labels the stage of an issue as alpha/beta/stable",
		Featured:    false,
		WhoCanUse:   "Anyone can trigger this command.",
		Examples:    []string{"/stage alpha", "/remove-stage alpha"},
	})
	return pluginHelp, nil
}

type stageClient interface {
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func stageHandleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e)
}

func handle(gc stageClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}

	for _, mat := range stageRe.FindAllStringSubmatch(e.Body, -1) {
		if err := handleOne(gc, log, e, mat); err != nil {
			return err
		}
	}
	return nil
}

func handleOne(gc stageClient, log *logrus.Entry, e *github.GenericCommentEvent, mat []string) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	remove := mat[1] != ""
	cmd := mat[2]
	lbl := "stage/" + cmd

	// Let's start simple and allow anyone to add/remove alpha, beta and stable labels.
	// Adjust if we find evidence of the community abusing these labels.
	labels, err := gc.GetIssueLabels(org, repo, number)
	if err != nil {
		log.WithError(err).Errorf("Failed to get labels.")
	}

	// If the label exists and we asked for it to be removed, remove it.
	if github.HasLabel(lbl, labels) && remove {
		return gc.RemoveLabel(org, repo, number, lbl)
	}

	// If the label does not exist and we asked for it to be added,
	// remove other existing stage labels and add it.
	if !github.HasLabel(lbl, labels) && !remove {
		for _, label := range stageLabels {
			if label != lbl && github.HasLabel(label, labels) {
				if err := gc.RemoveLabel(org, repo, number, label); err != nil {
					log.WithError(err).Errorf("GitHub failed to remove the following label: %s", label)
				}
			}
		}

		if err := gc.AddLabel(org, repo, number, lbl); err != nil {
			log.WithError(err).Errorf("GitHub failed to add the following label: %s", lbl)
		}
	}

	return nil
}

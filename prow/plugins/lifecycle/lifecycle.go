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

package lifecycle

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	lifecycleLabels = []string{labels.LifecycleActive, labels.LifecycleFrozen, labels.LifecycleStale, labels.LifecycleRotten}
	lifecycleRe     = regexp.MustCompile(`(?mi)^/(remove-)?lifecycle (active|frozen|stale|rotten)\s*$`)
)

func init() {
	plugins.RegisterGenericCommentHandler("lifecycle", lifecycleHandleGenericComment, help)
}

func help(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "Close, reopen, flag and/or unflag an issue or PR as frozen/stale/rotten",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/close",
		Description: "Closes an issue or PR.",
		Featured:    false,
		WhoCanUse:   "Authors and collaborators on the repository can trigger this command.",
		Examples:    []string{"/close"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/reopen",
		Description: "Reopens an issue or PR",
		Featured:    false,
		WhoCanUse:   "Authors and collaborators on the repository can trigger this command.",
		Examples:    []string{"/reopen"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/[remove-]lifecycle <frozen|stale|rotten>",
		Description: "Flags an issue or PR as frozen/stale/rotten",
		Featured:    false,
		WhoCanUse:   "Anyone can trigger this command.",
		Examples:    []string{"/lifecycle frozen", "/remove-lifecycle stale"},
	})
	return pluginHelp, nil
}

type lifecycleClient interface {
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	CreateComment(owner, repo string, number int, comment string) error
}

func lifecycleHandleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	gc := pc.GitHubClient
	log := pc.Logger
	if err := handleReopen(gc, log, &e); err != nil {
		return err
	}
	if err := handleClose(gc, log, &e); err != nil {
		return err
	}
	return handle(gc, log, &e)
}

func handle(gc lifecycleClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}

	for _, mat := range lifecycleRe.FindAllStringSubmatch(e.Body, -1) {
		if err := handleOne(gc, log, e, mat); err != nil {
			return err
		}
	}
	return nil
}

func handleOne(gc lifecycleClient, log *logrus.Entry, e *github.GenericCommentEvent, mat []string) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	user := e.User.Login

	remove := mat[1] != ""
	cmd := mat[2]
	lbl := "lifecycle/" + cmd

	// Don't allow adding lifecycle/frozen label to PRs
	if e.IsPR && lbl == labels.LifecycleFrozen && !remove {
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, fmt.Sprintf("The `%s` label cannot be applied to Pull Requests.", labels.LifecycleFrozen)))
	}

	// Let's start simple and allow anyone to add/remove frozen, stale, rotten labels.
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
	// remove other existing lifecycle labels and add it.
	if !github.HasLabel(lbl, labels) && !remove {
		for _, label := range lifecycleLabels {
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
